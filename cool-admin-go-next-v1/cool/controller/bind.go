package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/gvalid"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
)

type bindOptions struct {
	method             string
	source             BindSource
	allowUnknownFields bool
}

type domainValidator interface {
	Validate() error
}

func validateBindOptions(method string, source BindSource) error {
	if source == "" {
		source = BindAuto
	}
	switch source {
	case BindAuto, BindQuery, BindForm, BindJSON:
	default:
		return exception.Core(fmt.Sprintf("不支持的请求绑定来源: %s", source))
	}
	if source == BindJSON && (method == http.MethodGet || method == http.MethodHead) {
		return exception.Core("GET/HEAD action 不能使用 BindJSON")
	}
	return nil
}

func bindRequest(r *ghttp.Request, target interface{}, options bindOptions) error {
	source := options.source
	if source == "" {
		source = BindAuto
	}
	var err error
	switch source {
	case BindQuery:
		err = r.ParseQuery(target)
	case BindForm:
		err = r.ParseForm(target)
	case BindJSON:
		err = bindJSON(r, target, options.allowUnknownFields)
	case BindAuto:
		err = bindAuto(r, target, options.allowUnknownFields)
	default:
		return exception.Internal(nil, "请求绑定配置无效")
	}
	if err != nil {
		if _, typed := exception.KindOf(err); typed {
			return err
		}
		return exception.Validate(err.Error())
	}
	if validator, ok := target.(domainValidator); ok {
		if err = validator.Validate(); err != nil {
			if _, typed := exception.KindOf(err); typed {
				return err
			}
			return exception.Validate(err.Error())
		}
	}
	return nil
}

func bindAuto(r *ghttp.Request, target interface{}, allowUnknownFields bool) error {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return r.ParseQuery(target)
	}
	mediaType, err := requestMediaType(r)
	if err != nil {
		return err
	}
	switch {
	case isJSONMediaType(mediaType):
		return bindJSON(r, target, allowUnknownFields)
	case mediaType == "multipart/form-data":
		// GoFrame ParseForm 仅包含普通字段；Parse 还会合并 MultipartForm.File。
		return r.Parse(target)
	case mediaType == "application/x-www-form-urlencoded":
		return r.ParseForm(target)
	case mediaType == "" && len(bytes.TrimSpace(r.GetBody())) == 0:
		return bindJSON(r, target, allowUnknownFields)
	default:
		if mediaType == "" {
			return exception.UnsupportedMediaType("缺少 Content-Type")
		}
		return exception.UnsupportedMediaType(fmt.Sprintf("不支持的 Content-Type: %s", mediaType))
	}
}

func requestMediaType(r *ghttp.Request) (string, error) {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return "", nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", exception.UnsupportedMediaType("Content-Type 格式错误")
	}
	return strings.ToLower(mediaType), nil
}

func isJSONMediaType(mediaType string) bool {
	return mediaType == "application/json" ||
		(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

func bindJSON(r *ghttp.Request, target interface{}, allowUnknownFields bool) error {
	body := bytes.TrimSpace(r.GetBody())
	if len(body) == 0 {
		body = []byte("{}")
	}
	if err := rejectDuplicateJSONFields(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if !allowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求 JSON 格式错误: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if err := gvalid.New().Bail().Data(target).Run(r.Context()); err != nil {
		return err
	}
	return nil
}

func rejectDuplicateJSONFields(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return fmt.Errorf("请求 JSON 格式错误: %w", err)
	}
	return ensureJSONEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("对象字段名无效")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("字段 %q 重复", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("对象未正确结束")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("数组未正确结束")
		}
	default:
		return fmt.Errorf("无效 JSON 分隔符")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("请求 JSON 不能包含多个值")
		}
		return fmt.Errorf("请求 JSON 格式错误: %w", err)
	}
	return nil
}
