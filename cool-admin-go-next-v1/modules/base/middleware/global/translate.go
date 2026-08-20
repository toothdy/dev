package global

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
)

// 菜单、字典和消息翻译器
type Translator = middleware.Translator

// 翻译中间件配置
type TranslateOptions struct {
	Enabled    bool
	Languages  []string
	Translator Translator
}

// 基于语言 JSON 文件的翻译器
type MapTranslator struct {
	values map[string]string
}

// 加载 Node 兼容的 locales 目录
func NewMapTranslator(root string) (*MapTranslator, error) {
	translator := &MapTranslator{values: map[string]string{}}
	for _, kind := range []string{"menu", "msg", "comm", "dict/info", "dict/type"} {
		directory := filepath.Join(root, filepath.FromSlash(kind))
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, gerror.Wrapf(err, "读取翻译目录失败: %s", directory)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			content, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
			if readErr != nil {
				return nil, gerror.Wrap(readErr, "读取翻译文件失败")
			}
			items := map[string]string{}
			if unmarshalErr := json.Unmarshal(content, &items); unmarshalErr != nil {
				return nil, gerror.Wrap(unmarshalErr, "解析翻译文件失败")
			}
			language := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			keyKind := strings.ReplaceAll(kind, "/", ":")
			for source, target := range items {
				translator.values[translationKey(keyKind, language, source)] = target
			}
		}
	}
	return translator, nil
}

// 返回匹配翻译，不存在时保留原文
func (t *MapTranslator) Translate(kind string, language string, source string) string {
	if t == nil {
		return source
	}
	if target, ok := t.values[translationKey(kind, language, source)]; ok {
		return target
	}
	return source
}

func translationKey(kind string, language string, source string) string {
	return strings.ToLower(language) + ":" + kind + ":" + source
}

// 创建纯响应翻译中间件
func NewTranslate(options TranslateOptions) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		r.Middleware.Next()
		translateResponse(r, options)
	}
}

func translateResponse(r *ghttp.Request, options TranslateOptions) {
	if !options.Enabled || options.Translator == nil || r.Response.BufferLength() == 0 {
		return
	}
	if !strings.Contains(strings.ToLower(r.Response.Header().Get("Content-Type")), "application/json") {
		return
	}
	language := normalizedLanguage(r.Header.Get("language"), options.Languages)
	if language == "" {
		return
	}
	body := map[string]interface{}{}
	decoder := json.NewDecoder(bytes.NewReader(r.Response.Buffer()))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		return
	}
	if message, ok := body["message"].(string); ok && message != "" && message != "success" {
		body["message"] = options.Translator.Translate("msg", language, message)
	}
	translateResponseData(r.URL.Path, body["data"], language, options.Translator)
	encoded, err := json.Marshal(body)
	if err == nil {
		r.Response.Header().Add("Vary", "language")
		r.Response.SetBuffer(encoded)
	}
}

func normalizedLanguage(language string, allowed []string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	for _, item := range allowed {
		if language == strings.ToLower(item) {
			return language
		}
	}
	if len(allowed) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(allowed[0]))
}

func translateResponseData(path string, data interface{}, language string, translator Translator) {
	switch path {
	case "/admin/base/comm/permmenu":
		if object, ok := data.(map[string]interface{}); ok {
			translateNamedList(object["menus"], "menu", language, translator)
		}
	case "/admin/base/sys/menu/list":
		translateNamedList(data, "menu", language, translator)
	case "/admin/dict/info/list":
		translateNamedList(data, "dict:info", language, translator)
	case "/admin/dict/type/page":
		if object, ok := data.(map[string]interface{}); ok {
			translateNamedList(object["list"], "dict:type", language, translator)
		}
	case "/admin/dict/info/data", "/app/dict/info/data":
		if object, ok := data.(map[string]interface{}); ok {
			for _, items := range object {
				translateNamedList(items, "dict:info", language, translator)
			}
		}
	}
}

func translateNamedList(value interface{}, kind string, language string, translator Translator) {
	items, ok := value.([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		object, objectOK := item.(map[string]interface{})
		if !objectOK {
			continue
		}
		name, nameOK := object["name"].(string)
		if nameOK {
			object["name"] = translator.Translate(kind, language, name)
		}
	}
}
