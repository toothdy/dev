package global

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/response"
	baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
	baseEvent "github.com/toothdy/cool-admin-go-next/modules/base/event"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

type recordingLogSubmitter struct {
	records  []baseSysService.LogRecordRequest
	onSubmit func()
}

func (r *recordingLogSubmitter) Submit(_ context.Context, request baseSysService.LogRecordRequest) bool {
	r.records = append(r.records, request)
	if r.onSubmit != nil {
		r.onSubmit()
	}
	return true
}

func (r *recordingLogSubmitter) Record(_ context.Context, request baseSysService.LogRecordRequest) error {
	r.records = append(r.records, request)
	if r.onSubmit != nil {
		r.onSubmit()
	}
	return nil
}

func (r *recordingLogSubmitter) ClearExpired(context.Context) (int64, error) {
	return 0, nil
}

type prefixTranslator struct{}

func (prefixTranslator) Translate(kind string, language string, text string) string {
	return language + ":" + kind + ":" + text
}

func TestMiddlewareDefinitionsFollowOptions(t *testing.T) {
	translate, err := TranslateDefinition(
		module.MiddlewareDeps{Context: context.Background()},
		module.I18nOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if translate.Name != "base.translate" {
		t.Fatalf("unexpected translate middleware: %#v", translate)
	}
	authority, err := AuthorityDefinitions(
		module.MiddlewareDeps{Context: context.Background()},
		baseModule.Config{},
		module.AuthOptions{},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	log, err := LogDefinition(baseModule.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(authority) != 0 || len(log) != 0 {
		t.Fatalf("disabled Base middleware must be omitted: authority=%#v log=%#v", authority, log)
	}
}

func TestMiddlewareDefinitionsUseExpectedDefaultOrder(t *testing.T) {
	logRuntime, buildErr := baseEvent.BuildLog(&recordingLogSubmitter{}, baseEvent.LogOptions{
		Enabled: true, QueueSize: 1, ShutdownTimeout: time.Second, WriteTimeout: time.Second, CleanupTimeout: time.Second,
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	config := baseModule.Config{Middleware: baseModule.Middleware{
		Authority: baseModule.Switch{Enable: true},
		Log:       baseModule.Log{Enable: true},
	}}
	translate, err := TranslateDefinition(
		module.MiddlewareDeps{Context: context.Background()},
		module.I18nOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := AuthorityDefinitions(
		module.MiddlewareDeps{Context: context.Background()},
		config,
		module.AuthOptions{SSO: true},
		baseSysService.NewPermissionService(nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	log, err := LogDefinition(config, logRuntime)
	if err != nil {
		t.Fatal(err)
	}
	definitions := append([]middleware.Definition{translate}, authority...)
	definitions = append(definitions, log...)
	want := []string{"base.translate", "base.authority", "base.permission", "base.log"}
	if got := middlewareNames(definitions); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected definitions: got %#v want %#v", got, want)
	}
	for index := 1; index < len(definitions); index++ {
		if definitions[index-1].Order >= definitions[index].Order {
			t.Fatalf("definitions are not in ascending order: %#v", definitions)
		}
	}
}

func TestMiddlewareDefinitionsUseNamedApplicationOptions(t *testing.T) {
	translate, err := TranslateDefinition(
		module.MiddlewareDeps{I18nEnabled: false, I18nLanguages: []string{"legacy"}, Translator: prefixTranslator{}},
		module.I18nOptions{Enabled: true, Languages: []string{"en"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := newMiddlewareServer(t, translate.Handler, map[string]ghttp.HandlerFunc{
		"/admin/dict/info/list": func(r *ghttp.Request) {
			r.Response.WriteJson(response.OK([]map[string]interface{}{{"name": "Status"}}))
		},
	})
	translated := serve(server, http.MethodGet, "/admin/dict/info/list", "", "", "unsupported")
	if !strings.Contains(translated.Body.String(), `"name":"en:dict:info:Status"`) {
		t.Fatalf("named i18n options were not used: %s", translated.Body.String())
	}

	config := baseModule.Config{Middleware: baseModule.Middleware{Authority: baseModule.Switch{Enable: true}}}
	authority, err := AuthorityDefinitions(
		module.MiddlewareDeps{Context: context.Background(), SSO: false},
		config,
		module.AuthOptions{SSO: true},
		baseSysService.NewPermissionService(nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(authority) != 2 {
		t.Fatalf("unexpected authority definitions: %#v", authority)
	}
}

func TestAuthorityProtectsAdminAndAppRoutes(t *testing.T) {
	options := security.MiddlewareOptions{ProtectedPrefixes: append([]string{}, authorityPrefixes...)}
	for _, path := range []string{"/admin/base/comm/person", "/app/base/comm/upload"} {
		if !options.IsProtected(path) {
			t.Fatalf("expected %s to require authentication", path)
		}
	}
	if options.IsProtected("/health") {
		t.Fatal("health route must stay outside base authentication")
	}
}

func TestLogRedactsAndCapsParams(t *testing.T) {
	redacted := redactLogValue(map[string]interface{}{
		"password": "secret",
		"nested": map[string]interface{}{
			"RefreshToken": "refresh-secret",
			"safe":         "visible",
		},
	}).(map[string]interface{})
	if redacted["password"] != "[REDACTED]" {
		t.Fatalf("password was not redacted: %#v", redacted)
	}
	nested := redacted["nested"].(map[string]interface{})
	if nested["RefreshToken"] != "[REDACTED]" || nested["safe"] != "visible" {
		t.Fatalf("unexpected nested redaction: %#v", nested)
	}

	request := httptest.NewRequest(http.MethodPost, "/admin/base/sys/param/add", strings.NewReader(`{"payload":"`+strings.Repeat("x", maxLogParamsBytes*2)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	params := logParams(&ghttp.Request{Request: request})
	if len(params) > maxLogParamsBytes {
		t.Fatalf("logged params exceed cap: %d", len(params))
	}
	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(params), &decoded); err != nil || decoded["truncated"] != true {
		t.Fatalf("expected valid truncated JSON, got %q err=%v", params, err)
	}

	for _, path := range []string{"/admin/base/open/login", "/admin/base/open/refreshToken", "/admin/base/comm/personUpdate"} {
		sensitiveRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"password":"secret"}`))
		sensitiveRequest.Header.Set("Content-Type", "application/json")
		if got := logParams(&ghttp.Request{Request: sensitiveRequest}); got != `{}` {
			t.Fatalf("sensitive route %s must not log its body: %s", path, got)
		}
	}
}

func TestLogSubmissionDoesNotBlockHandlerAndAppIsSkipped(t *testing.T) {
	recorder := &recordingLogSubmitter{}
	server := newMiddlewareServer(t, NewLog(recorder), map[string]ghttp.HandlerFunc{
		"/admin/test": func(r *ghttp.Request) { r.Response.Write("admin-ok") },
		"/app/test":   func(r *ghttp.Request) { r.Response.Write("app-ok") },
	})

	admin := serve(server, http.MethodPost, "/admin/test", `{"password":"hidden","value":1}`, "application/json", "")
	if admin.Code != http.StatusOK || admin.Body.String() != "admin-ok" {
		t.Fatalf("log failure changed response: %d %q", admin.Code, admin.Body.String())
	}
	if len(recorder.records) != 1 || strings.Contains(recorder.records[0].Params, "hidden") {
		t.Fatalf("unexpected recorded log: %#v", recorder.records)
	}

	appResponse := serve(server, http.MethodGet, "/app/test", "", "", "")
	if appResponse.Code != http.StatusOK || len(recorder.records) != 1 {
		t.Fatalf("app request should bypass base log: status=%d records=%d", appResponse.Code, len(recorder.records))
	}
}

func TestLogSubmitsAfterHandlerAndKeepsClearAction(t *testing.T) {
	var handlerFinished bool
	recorder := &recordingLogSubmitter{
		records: []baseSysService.LogRecordRequest{{Action: "/old"}},
		onSubmit: func() {
			if !handlerFinished {
				t.Fatal("operation log submitted before handler finished")
			}
		},
	}
	server := newMiddlewareServer(t, NewLog(recorder), map[string]ghttp.HandlerFunc{
		"/admin/base/sys/log/clear": func(r *ghttp.Request) {
			recorder.records = nil
			handlerFinished = true
			r.Response.Write("cleared")
		},
	})

	responseRecorder := serve(server, http.MethodPost, "/admin/base/sys/log/clear", "", "", "")
	if responseRecorder.Code != http.StatusOK || responseRecorder.Body.String() != "cleared" {
		t.Fatalf("unexpected clear response: %d %q", responseRecorder.Code, responseRecorder.Body.String())
	}
	if len(recorder.records) != 1 || recorder.records[0].Action != "/admin/base/sys/log/clear" {
		t.Fatalf("clear action log was not retained: %#v", recorder.records)
	}
}

func TestLogSubmitsWhenHandlerPanics(t *testing.T) {
	recorder := &recordingLogSubmitter{}
	server := newMiddlewareChainServer(t, []ghttp.HandlerFunc{
		func(r *ghttp.Request) {
			defer func() {
				if recover() != nil {
					r.Response.WriteStatus(http.StatusInternalServerError, "recovered")
				}
			}()
			r.Middleware.Next()
		},
		NewLog(recorder),
	}, map[string]ghttp.HandlerFunc{
		"/admin/panic": func(*ghttp.Request) { panic("boom") },
	})

	responseRecorder := serve(server, http.MethodPost, "/admin/panic", "", "", "")
	if responseRecorder.Code != http.StatusInternalServerError || len(recorder.records) != 1 {
		t.Fatalf("panic request operation log missing: status=%d records=%#v", responseRecorder.Code, recorder.records)
	}
}

func TestTranslateHonorsSwitchPathAndLanguageFallback(t *testing.T) {
	handler := NewTranslate(TranslateOptions{
		Enabled: true, Languages: []string{"en"}, Translator: prefixTranslator{},
	})
	server := newMiddlewareServer(t, handler, map[string]ghttp.HandlerFunc{
		"/admin/dict/info/list": func(r *ghttp.Request) {
			r.Response.WriteJson(response.OK([]map[string]interface{}{{"name": "Status", "value": 1}}))
		},
		"/plain": func(r *ghttp.Request) {
			r.Response.Header().Set("Content-Type", "text/plain")
			r.Response.Write(`{"message":"keep"}`)
		},
	})

	translated := serve(server, http.MethodGet, "/admin/dict/info/list", "", "", "unsupported")
	if !strings.Contains(translated.Body.String(), `"name":"en:dict:info:Status"`) {
		t.Fatalf("dict response was not translated with fallback language: %s", translated.Body.String())
	}
	plain := serve(server, http.MethodGet, "/plain", "", "", "en")
	if plain.Body.String() != `{"message":"keep"}` {
		t.Fatalf("plain response should not be translated: %q", plain.Body.String())
	}
}

func TestTranslateDisabledKeepsResponseBytes(t *testing.T) {
	server := newMiddlewareServer(t, NewTranslate(TranslateOptions{
		Enabled: false, Languages: []string{"en"}, Translator: prefixTranslator{},
	}), map[string]ghttp.HandlerFunc{
		"/admin/base/sys/menu/list": func(r *ghttp.Request) {
			r.Response.WriteJson(response.OK([]map[string]interface{}{{"name": "Menu"}}))
		},
	})

	responseRecorder := serve(server, http.MethodGet, "/admin/base/sys/menu/list", "", "", "en")
	if responseRecorder.Body.String() != `{"code":1000,"message":"success","data":[{"name":"Menu"}]}` {
		t.Fatalf("disabled translation changed response: %q", responseRecorder.Body.String())
	}
}

func middlewareNames(definitions []middleware.Definition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return names
}

func newMiddlewareServer(t *testing.T, middleware ghttp.HandlerFunc, handlers map[string]ghttp.HandlerFunc) *ghttp.Server {
	return newMiddlewareChainServer(t, []ghttp.HandlerFunc{middleware}, handlers)
}

func newMiddlewareChainServer(t *testing.T, middlewares []ghttp.HandlerFunc, handlers map[string]ghttp.HandlerFunc) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Use(middlewares...)
	for path, handler := range handlers {
		server.BindHandler(path, handler)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start middleware test server failed: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	return server
}

func serve(server *ghttp.Server, method string, path string, body string, contentType string, language string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if language != "" {
		request.Header.Set("language", language)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}
