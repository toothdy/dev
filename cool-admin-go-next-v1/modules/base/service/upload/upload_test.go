package upload

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

func newTestService(root string) *Service {
	return NewService(module.UploadDirectory(root))
}

var validPNG = []byte("\x89PNG\r\n\x1a\n")

func TestServiceSavesRandomFileUnderDailyDirectory(t *testing.T) {
	root := t.TempDir()
	service := newTestService(root)
	request, uploadFile := uploadRequest(t, "../avatar.png", validPNG)
	defer request.Body.Close()

	savedURL, err := service.Upload(context.Background(), uploadFile)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	date := time.Now().Format("20060102")
	if !regexp.MustCompile(`^http://127\.0\.0\.1:8001/upload/` + date + `/anonymous/[a-z0-9]{32}\.png$`).MatchString(savedURL) {
		t.Fatalf("unexpected upload URL: %s", savedURL)
	}
	relative := strings.TrimPrefix(savedURL, "http://127.0.0.1:8001/upload/")
	content, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil || !bytes.Equal(content, validPNG) {
		t.Fatalf("expected saved content, got %q, %v", content, err)
	}
}

func TestServiceGeneratesLowercaseRandomBasename(t *testing.T) {
	root := t.TempDir()
	service := newTestService(root)
	date := time.Now().Format("20060102")
	pattern := regexp.MustCompile(`^http://127\.0\.0\.1:8001/upload/` + date + `/anonymous/[a-z0-9]+\.png$`)

	for i := 0; i < 32; i++ {
		request, uploadFile := uploadRequest(t, "../avatar.png", validPNG)
		savedURL, err := service.Upload(context.Background(), uploadFile)
		request.Body.Close()
		if err != nil {
			t.Fatalf("upload failed on attempt %d: %v", i, err)
		}
		if !pattern.MatchString(savedURL) {
			t.Fatalf("unexpected upload URL on attempt %d: %s", i, savedURL)
		}
	}
}

func TestServiceUsesRequestDomain(t *testing.T) {
	root := t.TempDir()
	service := newTestService(root)
	request, uploadFile := uploadRequest(t, "avatar.png", validPNG)
	defer request.Body.Close()

	savedURL, err := service.Upload(request.Context(), uploadFile)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	date := time.Now().Format("20060102")
	if !regexp.MustCompile(`^http://example\.com/upload/` + date + `/anonymous/[a-z0-9]{32}\.png$`).MatchString(savedURL) {
		t.Fatalf("unexpected request-domain upload URL: %s", savedURL)
	}
}

func TestServiceRejectsNilFile(t *testing.T) {
	service := newTestService(t.TempDir())

	_, err := service.Upload(context.Background(), nil)
	if err == nil || err.Error() != "上传文件为空" {
		t.Fatalf("expected empty file error, got %v", err)
	}
}

func TestServiceDoesNotUseClientKeyAsStoragePath(t *testing.T) {
	root := t.TempDir()
	service := newTestService(root)
	request, uploadFile := uploadRequest(t, "source.txt", []byte("proof"))
	defer request.Body.Close()

	savedURL, err := service.UploadWithKey(context.Background(), uploadFile, "avatars/proof.txt")
	if err != nil {
		t.Fatalf("upload with key failed: %v", err)
	}
	date := time.Now().Format("20060102")
	if !regexp.MustCompile(`^http://127\.0\.0\.1:8001/upload/` + date + `/anonymous/[a-z0-9]{32}\.txt$`).MatchString(savedURL) {
		t.Fatalf("unexpected keyed upload URL: %s", savedURL)
	}
	relative := strings.TrimPrefix(savedURL, "http://127.0.0.1:8001/upload/")
	content, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil || string(content) != "proof" {
		t.Fatalf("expected keyed file content, got %q, %v", content, err)
	}
	if _, err = service.UploadWithKey(context.Background(), uploadFile, "../proof.txt"); err == nil || err.Error() != "非法的文件路径" {
		t.Fatalf("expected unsafe key rejection, got %v", err)
	}
}

func TestServiceRejectsOversizedFile(t *testing.T) {
	service := newTestService(t.TempDir())
	uploadFile := &ghttp.UploadFile{
		FileHeader: &multipart.FileHeader{
			Filename: "large.bin",
			Size:     MaxUploadSize + 1,
		},
	}

	_, err := service.Upload(context.Background(), uploadFile)
	if err == nil || err.Error() != "文件大小不能超过10MB" {
		t.Fatalf("expected oversized file error, got %v", err)
	}
}

func TestServiceClassifiesSaveFailureAsInternal(t *testing.T) {
	rootFile, err := os.CreateTemp(t.TempDir(), "upload-root-file")
	if err != nil {
		t.Fatalf("create root file failed: %v", err)
	}
	root := rootFile.Name()
	if err = rootFile.Close(); err != nil {
		t.Fatalf("close root file failed: %v", err)
	}

	service := newTestService(root)
	request, uploadFile := uploadRequest(t, "avatar.png", validPNG)
	defer request.Body.Close()

	_, err = service.Upload(context.Background(), uploadFile)
	if err == nil {
		t.Fatal("expected save failure")
	}
	resolved := exception.Resolve(err)
	if resolved.Kind != exception.KindInternal || resolved.Message != "操作失败" {
		t.Fatalf("expected internal failure with safe client message, got %#v (%v)", resolved, err)
	}
}

func TestServiceRejectsActiveAndMismatchedContent(t *testing.T) {
	service := newTestService(t.TempDir())
	tests := []struct {
		filename string
		content  []byte
	}{
		{filename: "payload.html", content: []byte("<script>alert(1)</script>")},
		{filename: "payload.svg", content: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{filename: "payload.js", content: []byte("alert(1)")},
		{filename: "avatar.png", content: []byte("<script>alert(1)</script>")},
	}
	for _, test := range tests {
		request, uploadFile := uploadRequest(t, test.filename, test.content)
		_, err := service.Upload(context.Background(), uploadFile)
		request.Body.Close()
		if err == nil {
			t.Fatalf("expected %s rejection", test.filename)
		}
	}
}

func TestServiceScopesAuthenticatedFiles(t *testing.T) {
	root := t.TempDir()
	service := newTestService(root)
	request, uploadFile := uploadRequest(t, "avatar.png", validPNG)
	defer request.Body.Close()
	tenantIdentity, err := security.NewTenantIdentity(7)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	ctx := security.ContextWithUser(context.Background(), security.UserContext{UserId: 9, TenantId: tenantIdentity})

	savedURL, err := service.Upload(ctx, uploadFile)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	date := time.Now().Format("20060102")
	pattern := regexp.MustCompile(`^http://127\.0\.0\.1:8001/upload/` + date + `/tenant-7/user-9/[a-z0-9]{32}\.png$`)
	if !pattern.MatchString(savedURL) {
		t.Fatalf("unexpected scoped upload URL: %s", savedURL)
	}
}

func uploadRequest(t *testing.T, filename string, content []byte) (*ghttp.Request, *ghttp.UploadFile) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file failed: %v", err)
	}
	if _, err = fileWriter.Write(content); err != nil {
		t.Fatalf("write multipart file failed: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	gfRequest := &ghttp.Request{Request: request, Server: ghttp.GetServer("custom-api-test")}
	uploadFile := gfRequest.GetUploadFile("file")
	if uploadFile == nil {
		t.Fatal("expected multipart upload file")
	}
	return gfRequest, uploadFile
}
