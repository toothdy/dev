package controller

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/response"
)

// 非 JSON 成功响应的一次性 writer
type Result interface {
	Write(r *ghttp.Request) error
}

type resultWriter func(r *ghttp.Request) error

func (writer resultWriter) Write(r *ghttp.Request) error { return writer(r) }

// 创建 HTML 响应
func HTML(content string) Result {
	return resultWriter(func(r *ghttp.Request) error {
		r.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			r.Response.Write(content)
		}
		return nil
	})
}

// 创建内联文件响应，路径在提交 header 前完成检查
func File(path string) Result {
	return resultWriter(func(r *ghttp.Request) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			_ = file.Close()
			return fmt.Errorf("不是可读普通文件: %s", path)
		}
		contentType := mime.TypeByExtension(filepath.Ext(info.Name()))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return writeStream(r, contentType, file)
	})
}

// 创建流式响应
func Stream(contentType string, reader io.Reader) Result {
	return resultWriter(func(r *ghttp.Request) error {
		if reader == nil {
			return fmt.Errorf("响应流不能为空")
		}
		return writeStream(r, contentType, reader)
	})
}

func writeStream(r *ghttp.Request, contentType string, reader io.Reader) error {
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	if strings.TrimSpace(contentType) != "" {
		r.Response.Header().Set("Content-Type", contentType)
	}
	if r.Method == http.MethodHead {
		return nil
	}
	_, err := io.Copy(r.Response.BufferWriter, reader)
	return err
}

// 创建安全重定向响应
func Redirect(location string, status ...int) Result {
	return resultWriter(func(r *ghttp.Request) error {
		if strings.ContainsAny(location, "\r\n") {
			return fmt.Errorf("重定向地址不能包含换行符")
		}
		code := http.StatusFound
		if len(status) > 0 {
			code = status[0]
		}
		switch code {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		default:
			return fmt.Errorf("不支持的重定向状态码: %d", code)
		}
		r.Response.Header().Set("Location", location)
		r.Response.WriteHeader(code)
		return nil
	})
}

// 创建无响应体的 204 响应
func NoContent() Result {
	return resultWriter(func(r *ghttp.Request) error {
		r.Response.RawWriter().WriteHeader(http.StatusNoContent)
		// GoFrame 会为 buffer 中的非 200 空响应追加 status text。
		r.Response.Status = http.StatusOK
		return nil
	})
}

func writeJSONSuccess(r *ghttp.Request, data interface{}) {
	r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.Response.WriteJson(response.OK(data))
}
