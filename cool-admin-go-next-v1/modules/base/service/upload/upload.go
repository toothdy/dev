package upload

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

const (
	MaxUploadSize int64 = 10 * 1024 * 1024

	uploadRandomAlphabet   = "abcdefghijklmnopqrstuvwxyz0123456789"
	uploadRandomNameLength = 32
)

var allowedUploadTypes = map[string][]string{
	".png":  {"image/png"},
	".jpg":  {"image/jpeg"},
	".jpeg": {"image/jpeg"},
	".gif":  {"image/gif"},
	".webp": {"image/webp"},
	".pdf":  {"application/pdf"},
	".txt":  {"text/plain"},
	".csv":  {"text/plain", "text/csv", "application/csv"},
}

// 本地上传服务
type Service struct {
	RootDir string
}

/**
 * 创建本地上传服务
 * @param rootDir 上传根目录
 * @returns 本地上传服务
 */
func NewService(rootDir module.UploadDirectory) *Service {
	directory := rootDir.String()
	if directory == "" {
		directory = filepath.Join("resource", "public", "uploads")
	}
	return &Service{RootDir: directory}
}

/**
 * 获取静态文件目录
 * @returns 上传根目录
 */
func (s *Service) StaticDir() string {
	return s.RootDir
}

/**
 * 生成安全的随机上传文件名
 * @returns 随机文件 basename 和错误
 */
func randomUploadBasename() (string, error) {
	randomBytes := make([]byte, uploadRandomNameLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", gerror.Wrap(err, "生成上传文件名失败")
	}
	for index, randomByte := range randomBytes {
		randomBytes[index] = uploadRandomAlphabet[int(randomByte)%len(uploadRandomAlphabet)]
	}
	return string(randomBytes), nil
}

/**
 * 保存上传文件并返回访问路径
 * @param ctx 请求上下文
 * @param file 上传文件
 * @returns 上传访问路径和错误
 */
func (s *Service) Upload(ctx context.Context, file *ghttp.UploadFile) (string, error) {
	return s.UploadWithKey(ctx, file, "")
}

// 保存上传文件，并支持 Node 本地上传插件的可选 key 字段
func (s *Service) UploadWithKey(ctx context.Context, file *ghttp.UploadFile, key string) (string, error) {
	if file == nil || file.FileHeader == nil {
		return "", exception.Validate("上传文件为空")
	}
	if file.Size > MaxUploadSize {
		return "", exception.Validate("文件大小不能超过10MB")
	}

	if key != "" {
		if _, err := sanitizeUploadKey(s.RootDir, key); err != nil {
			return "", err
		}
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(file.Filename)))
	if err := validateUploadType(file, extension); err != nil {
		return "", err
	}
	basename, err := randomUploadBasename()
	if err != nil {
		return "", exception.Internal(err, "生成上传文件名失败")
	}
	date := time.Now().Format("20060102")
	scope := uploadScope(ctx)
	filename := basename + extension
	targetDir := filepath.Join(s.RootDir, date, scope)
	if err = os.MkdirAll(targetDir, 0o755); err != nil {
		return "", exception.Internal(err, "创建上传目录失败")
	}
	targetPath := filepath.Join(targetDir, filename)
	if err = saveUploadExclusive(file, targetPath); err != nil {
		return "", err
	}
	urlKey := filepath.ToSlash(filepath.Join(scope, filename))
	path := "/upload/" + date + "/" + strings.TrimPrefix(urlKey, "./")
	return uploadDomain(ctx) + path, nil
}

func validateUploadType(file *ghttp.UploadFile, extension string) error {
	allowedMIMEs, ok := allowedUploadTypes[extension]
	if !ok {
		return exception.Validate("不支持的文件类型")
	}
	source, err := file.FileHeader.Open()
	if err != nil {
		return exception.Internal(err, "读取上传文件失败")
	}
	defer source.Close()
	header := make([]byte, 512)
	read, err := source.Read(header)
	if err != nil && err != io.EOF {
		return exception.Internal(err, "读取上传文件失败")
	}
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(header[:read]), ";")[0]))
	declared := strings.ToLower(strings.TrimSpace(strings.Split(file.Header.Get("Content-Type"), ";")[0]))
	if !containsUploadMIME(allowedMIMEs, detected) || (declared != "" && declared != "application/octet-stream" && !containsUploadMIME(allowedMIMEs, declared)) {
		return exception.Validate("文件内容与扩展名不匹配")
	}
	return nil
}

func containsUploadMIME(allowed []string, value string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func uploadScope(ctx context.Context) string {
	if user, ok := security.UserFromContext(ctx); ok && user.UserId > 0 {
		if tenantID, hasTenant := user.TenantId.TenantID(); hasTenant {
			return filepath.Join(fmt.Sprintf("tenant-%d", tenantID), fmt.Sprintf("user-%d", user.UserId))
		}
		return filepath.Join("platform", fmt.Sprintf("user-%d", user.UserId))
	}
	return "anonymous"
}

func saveUploadExclusive(file *ghttp.UploadFile, targetPath string) error {
	source, err := file.FileHeader.Open()
	if err != nil {
		return exception.Internal(err, "读取上传文件失败")
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return exception.Internal(err, "保存上传文件失败")
	}
	written, copyErr := io.Copy(target, io.LimitReader(source, MaxUploadSize+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil || written > MaxUploadSize {
		_ = os.Remove(targetPath)
		if written > MaxUploadSize {
			return exception.Validate("文件大小不能超过10MB")
		}
		if copyErr != nil {
			return exception.Internal(copyErr, "保存上传文件失败")
		}
		return exception.Internal(closeErr, "保存上传文件失败")
	}
	return nil
}

func uploadDomain(ctx context.Context) string {
	request := ghttp.RequestFromCtx(ctx)
	if request == nil || request.Request == nil || request.Host == "" {
		return "http://127.0.0.1:8001"
	}
	scheme := request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if request.TLS != nil {
			scheme = "https"
		}
	}
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	return scheme + "://" + request.Host
}

var windowsDrivePath = regexp.MustCompile(`^[A-Za-z]:`)

func sanitizeUploadKey(rootDir string, key string) (string, error) {
	if key == "" {
		return "", nil
	}
	if strings.Contains(key, "..") || strings.Contains(key, "./") || strings.Contains(key, `.\`) ||
		strings.Contains(key, `\`) || strings.Contains(key, "//") || strings.ContainsRune(key, '\x00') ||
		windowsDrivePath.MatchString(key) || strings.HasPrefix(key, "/") {
		return "", exception.Comm("非法的文件路径")
	}
	normalized := filepath.Clean(key)
	if normalized == "." || strings.Contains(normalized, "..") || filepath.IsAbs(normalized) {
		return "", exception.Comm("非法的文件路径")
	}
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", exception.Internal(err, "读取上传目录失败")
	}
	target, err := filepath.Abs(filepath.Join(root, time.Now().Format("20060102"), normalized))
	if err != nil {
		return "", exception.Internal(err, "读取上传路径失败")
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", exception.Comm("文件路径超出允许范围")
	}
	if filepath.Base(normalized) == "." || filepath.Base(normalized) == string(filepath.Separator) {
		return "", exception.Comm("非法的文件路径")
	}
	return normalized, nil
}
