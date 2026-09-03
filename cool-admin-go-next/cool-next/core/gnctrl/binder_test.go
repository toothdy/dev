package gnctrl

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
)

type transientBinderEntity struct {
	g.Meta `orm:"table:transient_binder" description:"临时字段绑定"`
	gnentity.Base
	Name       string    `json:"name" orm:"name" description:"名称"`
	Enabled    bool      `json:"enabled" orm:"enabled" description:"是否启用"`
	RoleIDList *[]uint64 `json:"roleIdList" description:"角色 ID 列表" cool:"transient"`
}

type multipartBinderDTO struct {
	File *ghttp.UploadFile `file:"file" v:"required"`
	Key  string            `form:"key"`
}

func TestDecodeMutableAcceptsBooleanNumbers(t *testing.T) {
	descriptor, err := gnentity.Compile[transientBinderEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{name: "false", raw: "false"},
		{name: "true", raw: "true", want: true},
		{name: "zero", raw: "0"},
		{name: "one", raw: "1", want: true},
		{name: "invalid number", raw: "2", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutable, decodeErr := decodeMutable[transientBinderEntity, uint64](map[string]json.RawMessage{
				"enabled": json.RawMessage(test.raw),
			}, descriptor)
			if test.wantErr {
				if decodeErr == nil {
					t.Fatal("expected decode error")
				}
				return
			}
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			value, exists := mutable.Get("enabled")
			if !exists || value != test.want {
				t.Fatalf("Get(enabled) = %#v/%v", value, exists)
			}
		})
	}
}

func TestDecodeMutablePreservesTransientFieldStates(t *testing.T) {
	descriptor, err := gnentity.Compile[transientBinderEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		raw       map[string]json.RawMessage
		has       bool
		isNull    bool
		wantValue []uint64
	}{
		{name: "missing", raw: map[string]json.RawMessage{}},
		{name: "null", raw: map[string]json.RawMessage{"roleIdList": json.RawMessage("null")}, has: true, isNull: true},
		{name: "empty", raw: map[string]json.RawMessage{"roleIdList": json.RawMessage("[]")}, has: true, wantValue: []uint64{}},
		{name: "values", raw: map[string]json.RawMessage{"roleIdList": json.RawMessage("[1,2]")}, has: true, wantValue: []uint64{1, 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutable, decodeErr := decodeMutable[transientBinderEntity, uint64](test.raw, descriptor)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if mutable.Has("roleIdList") != test.has || mutable.IsNull("roleIdList") != test.isNull {
				t.Fatalf("state = has:%v null:%v", mutable.Has("roleIdList"), mutable.IsNull("roleIdList"))
			}
			value, exists := mutable.Get("roleIdList")
			if exists != test.has || test.has && !test.isNull && !reflect.DeepEqual(value, test.wantValue) {
				t.Fatalf("Get(roleIdList) = %#v/%v", value, exists)
			}
			if values, ok := value.([]uint64); ok && len(values) > 0 {
				values[0] = 99
				if current, _ := mutable.Get("roleIdList"); reflect.DeepEqual(current, values) {
					t.Fatal("Get(roleIdList) exposed internal slice")
				}
			}
		})
	}
}

func TestBindFilesUsesIndependentMultipartBodyLimit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("v"), 512)
	if _, err = part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err = writer.WriteField("key", "video.mp4"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	config := crud.DefaultConfig()
	config.BodyLimit = 128
	config.MultipartBodyLimit = int64(body.Len() + 1)
	binder, err := NewBinder(config)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	var target multipartBinderDTO
	if err = binder.BindDTO(&ghttp.Request{Request: request}, BindFile, &target); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if request.MultipartForm != nil {
			_ = request.MultipartForm.RemoveAll()
		}
	})
	if target.File == nil || target.Key != "video.mp4" {
		t.Fatalf("bound target = %#v", target)
	}
	file, err := target.File.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("file content length = %d", len(got))
	}
}

func TestBindFilesRejectsMultipartBodyOverLimit(t *testing.T) {
	config := crud.DefaultConfig()
	config.MultipartBodyLimit = 64
	binder, err := NewBinder(config)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("x", 65)))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	if err = binder.BindDTO(&ghttp.Request{Request: request}, BindFile, &multipartBinderDTO{}); err == nil ||
		!strings.Contains(err.Error(), "文件上传请求超过 64 字节上限") {
		t.Fatalf("BindDTO() error = %v", err)
	}
}
