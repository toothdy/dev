package service

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
	"github.com/toothdy/cool-admin-go-next/modules/base"
)

const (
	defaultUploadMaxBytes = int64(100 << 20)
	uploadRandomBytes     = 16
	uploadTemporaryPrefix = ".upload-"
	uploadDateLayout      = "20060102"
)

var trustedUploadMedia = map[string]map[string]bool{
	"image/jpeg": {".jpg": true, ".jpeg": true},
	"image/png":  {".png": true},
	"image/gif":  {".gif": true},
	"image/webp": {".webp": true},
	"audio/mpeg": {".mp3": true},
	"audio/wave": {".wav": true},
	"video/mp4":  {".mp4": true},
	"video/webm": {".webm": true},
}

// Base 模块的本地上传和受控读取
type UploadService struct {
	root          string
	publicBaseURL string
	publicURL     *url.URL
	maxBytes      int64
	now           func() time.Time
	random        io.Reader
}

// 本地受管上传文件位置
type ManagedUploadLocation struct {
	Root         string
	RelativePath string
	Key          string
}

// 按 Base 配置创建本地上传服务
func NewUpload(config base.Config) (*UploadService, error) {
	upload := config.Upload
	if upload.Root == "" {
		return nil, exception.Core("上传根目录配置无效")
	}
	root, err := filepath.Abs(upload.Root)
	if err != nil {
		return nil, exception.Core("上传根目录配置无效")
	}
	publicBaseURL := strings.TrimRight(strings.TrimSpace(upload.PublicBaseURL), "/")
	parsedURL, err := url.Parse(publicBaseURL)
	if err != nil || parsedURL.Host == "" ||
		(parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
		parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return nil, exception.Core("上传公开地址配置无效")
	}
	maxBytes := upload.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultUploadMaxBytes
	}
	if maxBytes < 0 {
		return nil, exception.Core("上传大小配置无效")
	}

	return &UploadService{
		root:          root,
		publicBaseURL: publicBaseURL,
		publicURL:     parsedURL,
		maxBytes:      maxBytes,
		now:           time.Now,
		random:        cryptorand.Reader,
	}, nil
}

// 解析属于当前本地上传配置的公开 URL
func (service *UploadService) ResolveManagedURL(rawURL string) (ManagedUploadLocation, bool) {
	if service == nil || service.publicURL == nil || strings.Contains(rawURL, "#") {
		return ManagedUploadLocation{}, false
	}
	candidate, err := url.Parse(rawURL)
	if err != nil || candidate.Opaque != "" || candidate.User != nil || candidate.RawQuery != "" || candidate.ForceQuery ||
		!strings.EqualFold(candidate.Scheme, service.publicURL.Scheme) ||
		!strings.EqualFold(candidate.Host, service.publicURL.Host) {
		return ManagedUploadLocation{}, false
	}
	prefix := strings.TrimRight(service.publicURL.EscapedPath(), "/") + "/upload/"
	remainder, isManaged := strings.CutPrefix(candidate.EscapedPath(), prefix)
	if !isManaged {
		return ManagedUploadLocation{}, false
	}
	escapedDate, escapedName, exists := strings.Cut(remainder, "/")
	if !exists || strings.Contains(escapedName, "/") {
		return ManagedUploadLocation{}, false
	}
	date, dateErr := url.PathUnescape(escapedDate)
	name, nameErr := url.PathUnescape(escapedName)
	if dateErr != nil || nameErr != nil || !validUploadDate(date) || !validUploadBasename(name) {
		return ManagedUploadLocation{}, false
	}

	return ManagedUploadLocation{
		Root:         service.root,
		RelativePath: filepath.Join(date, name),
		Key:          "/upload/" + date + "/" + url.PathEscape(name),
	}, true
}

// 保存 multipart 文件并返回公开 URL
func (service *UploadService) Save(file *ghttp.UploadFile, key string) (string, error) {
	if service == nil || file == nil || file.FileHeader == nil {
		return "", exception.Validate("上传文件无效")
	}
	name := key
	if name == "" {
		var err error
		name, err = service.randomName(file.Filename)
		if err != nil {
			return "", exception.Core("生成上传文件名失败")
		}
	}
	if !validUploadBasename(name) {
		return "", exception.Validate("上传文件名无效")
	}
	if file.Size > service.maxBytes {
		return "", exception.Validate("上传文件超过大小限制")
	}

	date := service.now().Format(uploadDateLayout)
	root, err := service.openRoot(true)
	if err != nil {
		return "", exception.Core("保存上传文件失败")
	}
	defer root.Close()
	if err = root.MkdirAll(date, 0o750); err != nil {
		return "", exception.Core("保存上传文件失败")
	}
	directory, err := root.Lstat(date)
	if err != nil || !directory.IsDir() {
		return "", exception.Core("保存上传文件失败")
	}

	source, err := file.Open()
	if err != nil {
		return "", exception.Validate("上传文件无效")
	}
	defer source.Close()

	temporaryName, err := service.randomHex(uploadRandomBytes)
	if err != nil {
		return "", exception.Core("生成上传临时文件名失败")
	}
	temporary := filepath.Join(date, uploadTemporaryPrefix+temporaryName)
	target := filepath.Join(date, name)
	destination, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", exception.Core("保存上传文件失败")
	}
	temporaryExists := true
	defer func() {
		_ = destination.Close()
		if temporaryExists {
			_ = root.Remove(temporary)
		}
	}()

	limit := service.maxBytes + 1
	if service.maxBytes == math.MaxInt64 {
		limit = service.maxBytes
	}
	written, copyErr := io.Copy(destination, io.LimitReader(source, limit))
	if copyErr != nil {
		return "", exception.Core("保存上传文件失败")
	}
	if written > service.maxBytes {
		return "", exception.Validate("上传文件超过大小限制")
	}
	if err = destination.Sync(); err != nil {
		return "", exception.Core("保存上传文件失败")
	}
	if err = destination.Close(); err != nil {
		return "", exception.Core("保存上传文件失败")
	}
	if err = root.Link(temporary, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", exception.Validate("上传文件已存在")
		}
		return "", exception.Core("保存上传文件失败")
	}
	if err = root.Remove(temporary); err != nil {
		_ = root.Remove(target)
		return "", exception.Core("保存上传文件失败")
	}
	temporaryExists = false

	return service.publicBaseURL + "/upload/" + date + "/" + url.PathEscape(name), nil
}

// 校验公开文件路径并构造受控文件响应
func (service *UploadService) Read(date, name string) (gnctrl.FileResponse, error) {
	if service == nil || !validUploadDate(date) || !validUploadBasename(name) {
		return gnctrl.FileResponse{}, exception.Validate("上传文件不存在")
	}
	root, err := service.openRoot(false)
	if err != nil {
		return gnctrl.FileResponse{}, exception.Validate("上传文件不存在")
	}
	defer root.Close()

	relative := filepath.Join(date, name)
	directory, err := root.Lstat(date)
	if err != nil || !directory.IsDir() {
		return gnctrl.FileResponse{}, exception.Validate("上传文件不存在")
	}
	entry, err := root.Lstat(relative)
	if err != nil || !entry.Mode().IsRegular() {
		return gnctrl.FileResponse{}, exception.Validate("上传文件不存在")
	}
	file, err := root.Open(relative)
	if err != nil {
		return gnctrl.FileResponse{}, exception.Validate("上传文件不存在")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return gnctrl.FileResponse{}, exception.Validate("上传文件不存在")
	}

	sniff := make([]byte, 512)
	count, err := file.Read(sniff)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = file.Close()
		return gnctrl.FileResponse{}, exception.Core("读取上传文件失败")
	}
	contentType := http.DetectContentType(sniff[:count])
	disposition := gnctrl.FileDispositionAttachment
	extension := strings.ToLower(filepath.Ext(name))
	if extensions := trustedUploadMedia[contentType]; extensions[extension] {
		disposition = gnctrl.FileDispositionInline
	} else {
		contentType = "application/octet-stream"
	}

	return gnctrl.FileResponse{
		Content:     file,
		Name:        name,
		ContentType: contentType,
		Disposition: disposition,
		Headers: http.Header{
			"X-Content-Type-Options": []string{"nosniff"},
		},
	}, nil
}

func (service *UploadService) openRoot(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(service.root, 0o750); err != nil {
			return nil, err
		}
	}

	return os.OpenRoot(service.root)
}

func (service *UploadService) randomName(filename string) (string, error) {
	name, err := service.randomHex(uploadRandomBytes)
	if err != nil {
		return "", err
	}

	return name + safeUploadExtension(filename), nil
}

func (service *UploadService) randomHex(size int) (string, error) {
	random := make([]byte, size)
	if _, err := io.ReadFull(service.random, random); err != nil {
		return "", err
	}

	return hex.EncodeToString(random), nil
}

func safeUploadExtension(filename string) string {
	filename = strings.ReplaceAll(filename, `\`, "/")
	extension := path.Ext(path.Base(filename))
	if len(extension) < 2 || len(extension) > 11 {
		return ""
	}
	for _, character := range extension[1:] {
		isLetter := character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
		isNumber := character >= '0' && character <= '9'
		if !isLetter && !isNumber {
			return ""
		}
	}

	return strings.ToLower(extension)
}

func validUploadBasename(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsRune(name, 0) && !strings.ContainsAny(name, `/\`) &&
		!filepath.IsAbs(name) && filepath.VolumeName(name) == "" && filepath.Base(name) == name
}

func validUploadDate(value string) bool {
	if len(value) != len(uploadDateLayout) {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	parsed, err := time.Parse(uploadDateLayout, value)

	return err == nil && parsed.Format(uploadDateLayout) == value
}
