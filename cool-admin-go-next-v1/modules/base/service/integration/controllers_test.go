package base_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcfg"
	coolRuntimeController "github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	adminController "github.com/toothdy/cool-admin-go-next/modules/base/controller/admin"
	adminSysController "github.com/toothdy/cool-admin-go-next/modules/base/controller/admin/sys"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

/**
 * 测试 base controllers 覆盖核心资源
 * @param t 测试对象
 * @returns null
 */
func TestControllersDeclareBaseResources(t *testing.T) {
	controllers := testControllers(module.Deps{})
	prefixes := map[string]bool{}
	for _, definition := range controllers {
		prefixes[definition.Prefix] = true
	}

	expected := []string{
		"/admin/base/sys/user",
		"/admin/base/sys/role",
		"/admin/base/sys/menu",
		"/admin/base/sys/department",
		"/admin/base/sys/param",
		"/admin/base/sys/log",
		"/admin/base/open",
		"/admin/base/comm",
		"/app/base/comm",
	}
	for _, prefix := range expected {
		if !prefixes[prefix] {
			t.Fatalf("expected prefix %s in controllers", prefix)
		}
	}
}

/**
 * 测试 EPS 路由声明为匿名 GET
 * @param t 测试对象
 * @returns null
 */
func TestControllersDeclareAnonymousEPSRoute(t *testing.T) {
	definitions := testControllers(module.Deps{EPSProvider: func() []coolRuntimeController.Definition {
		return nil
	}})
	var open coolRuntimeController.Definition
	for _, definition := range definitions {
		if definition.Prefix == "/admin/base/open" {
			open = definition
			break
		}
	}
	for _, route := range open.Routes {
		if route.Path == "/eps" {
			if route.Method != http.MethodGet || !route.IgnoreAuth || route.Action == nil {
				t.Fatalf("expected anonymous GET eps route, got %#v", route)
			}
			return
		}
	}
	t.Fatal("expected EPS route")
}

/**
 * 测试用户 controller CRUD 配置
 * @param t 测试对象
 * @returns null
 */
func TestUserControllerCRUDMetadata(t *testing.T) {
	models := modelMap(findBaseSpec(t).Models)
	definition := adminSysController.BaseAdminUserController(nil, models["base_sys_user"])

	if definition.CRUD == nil {
		t.Fatal("expected user CRUD metadata")
	}
	if definition.CRUD.API[0] != crud.Add {
		t.Fatalf("expected first API add, got %s", definition.CRUD.API[0])
	}
	if definition.CRUD.HiddenFields[0] != "password" {
		t.Fatalf("expected password hidden field, got %#v", definition.CRUD.HiddenFields)
	}
}

/**
 * 测试 base CRUD 元数据与现有路由配置一致
 * @param t 测试对象
 * @returns null
 */
func TestBaseCRUDMetadataMatchesRoutes(t *testing.T) {
	expected := map[string]struct {
		api         []string
		keywordLike  []string
		fieldEq      []string
		sortFields   []string
		defaultSort  string
		defaultOrder string
	}{
		"base/sys/user": {
			api:         []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			keywordLike:  []string{},
			fieldEq:      []string{},
			sortFields:   []string{"id", "createTime", "updateTime", "username"},
			defaultSort:  "id",
			defaultOrder: "DESC",
		},
		"base/sys/role": {
			api:         []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			keywordLike:  []string{"name", "label"},
			fieldEq:      []string{},
			sortFields:   []string{"id", "createTime", "updateTime", "name"},
			defaultSort:  "id",
			defaultOrder: "DESC",
		},
		"base/sys/menu": {
			api:         []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			keywordLike:  []string{},
			fieldEq:      []string{},
			sortFields:   []string{"id", "orderNum", "createTime", "updateTime"},
			defaultSort:  "id",
			defaultOrder: "DESC",
		},
		"base/sys/department": {
			api:         []string{crud.Add, crud.Delete, crud.Update, crud.List},
			keywordLike:  []string{},
			fieldEq:      []string{},
			sortFields:   []string{"id", "orderNum", "createTime", "updateTime"},
			defaultSort:  "orderNum",
			defaultOrder: "ASC",
		},
		"base/sys/param": {
			api:         []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.Page},
			keywordLike:  []string{"keyName", "name"},
			fieldEq:      []string{"dataType"},
			sortFields:   []string{"id", "createTime", "updateTime", "keyName"},
			defaultSort:  "id",
			defaultOrder: "DESC",
		},
		"base/sys/log": {
			api:         []string{crud.Page},
			keywordLike:  []string{"b.name", "a.action", "a.ip"},
			fieldEq:      []string{},
			sortFields:   []string{"id", "createTime", "updateTime", "userId"},
			defaultSort:  "id",
			defaultOrder: "DESC",
		},
	}

	controllers := testControllers(module.Deps{})
	for _, definition := range controllers {
		if definition.CRUD == nil {
			continue
		}
		expectedDefinition, ok := expected[resourceName(definition.Prefix)]
		if !ok {
			t.Fatalf("unexpected CRUD controller %s", definition.Prefix)
		}
		if !reflect.DeepEqual(definition.CRUD.API, expectedDefinition.api) {
			t.Fatalf("unexpected API for %s: %#v", definition.Prefix, definition.CRUD.API)
		}
		if !reflect.DeepEqual(definition.CRUD.PageQuery.KeyWordLikeFields, expectedDefinition.keywordLike) {
			t.Fatalf("unexpected keyword fields for %s: %#v", definition.Prefix, definition.CRUD.PageQuery.KeyWordLikeFields)
		}
		if !reflect.DeepEqual(definition.CRUD.PageQuery.FieldEq, expectedDefinition.fieldEq) {
			t.Fatalf("unexpected equal fields for %s: %#v", definition.Prefix, definition.CRUD.PageQuery.FieldEq)
		}
		if !reflect.DeepEqual(definition.CRUD.SortFields, expectedDefinition.sortFields) {
			t.Fatalf("unexpected sort fields for %s: %#v", definition.Prefix, definition.CRUD.SortFields)
		}
		if definition.CRUD.DefaultSort != expectedDefinition.defaultSort || definition.CRUD.DefaultOrder != expectedDefinition.defaultOrder {
			t.Fatalf("unexpected default sort for %s: %s %s", definition.Prefix, definition.CRUD.DefaultSort, definition.CRUD.DefaultOrder)
		}
	}
}

/**
 * 测试 open 和 comm 自定义路由元数据
 * @param t 测试对象
 * @returns null
 */
func TestBaseCustomRouteMetadata(t *testing.T) {
	controllers := testControllers(module.Deps{})
	routes := map[string]struct {
		method     string
		path       string
		ignoreAuth bool
	}{
		"login":        {method: http.MethodPost, path: "/admin/base/open/login", ignoreAuth: true},
		"refreshToken": {method: http.MethodPost, path: "/admin/base/open/refreshToken", ignoreAuth: true},
		"captcha":      {method: http.MethodGet, path: "/admin/base/open/captcha", ignoreAuth: true},
		"person":       {method: http.MethodGet, path: "/admin/base/comm/person"},
		"permmenu":     {method: http.MethodGet, path: "/admin/base/comm/permmenu"},
		"logout":       {method: http.MethodPost, path: "/admin/base/comm/logout"},
		"program":      {method: http.MethodGet, path: "/admin/base/comm/program", ignoreAuth: true},
	}

	seen := map[string]bool{}
	for _, definition := range controllers {
		for _, route := range definition.Routes {
			expectedRoute, ok := routes[route.Name]
			if !ok {
				continue
			}
			seen[route.Name] = true
			if route.Method != expectedRoute.method || route.FullPath != expectedRoute.path || route.IgnoreAuth != expectedRoute.ignoreAuth {
				t.Fatalf("unexpected route metadata for %s: %#v", route.Name, route)
			}
			if route.Action == nil {
				t.Fatalf("expected handler for %s", route.Name)
			}
		}
	}
	for name := range routes {
		if !seen[name] {
			t.Fatalf("expected route %s", name)
		}
	}
}

/**
 * 测试 base 自定义 API 路由元数据和权限映射
 * @param t 测试对象
 * @returns null
 */
func TestBaseCustomAPIMetadata(t *testing.T) {
	expected := map[string]struct {
		method      string
		path        string
		description string
		permission  string
	}{
		"personUpdate": {http.MethodPost, "/admin/base/comm/personUpdate", "修改个人信息", ""},
		"uploadMode":   {http.MethodGet, "/admin/base/comm/uploadMode", "文件上传模式", ""},
		"upload":       {http.MethodPost, "/admin/base/comm/upload", "文件上传", ""},
		"move":         {http.MethodPost, "/admin/base/sys/user/move", "移动部门", "base:sys:user:move"},
		"order":        {http.MethodPost, "/admin/base/sys/department/order", "排序", "base:sys:department:order"},
		"clear":        {http.MethodPost, "/admin/base/sys/log/clear", "清理", "base:sys:log:clear"},
		"setKeep":      {http.MethodPost, "/admin/base/sys/log/setKeep", "日志保存时间", "base:sys:log:setKeep"},
		"getKeep":      {http.MethodGet, "/admin/base/sys/log/getKeep", "获得日志保存时间", "base:sys:log:getKeep"},
		"export":       {http.MethodPost, "/admin/base/sys/menu/export", "导出", "base:sys:menu:export"},
		"import":       {http.MethodPost, "/admin/base/sys/menu/import", "导入", "base:sys:menu:import"},
		"parse":        {http.MethodPost, "/admin/base/sys/menu/parse", "解析", "base:sys:menu:parse"},
	}
	seen := map[string]bool{}
	for _, definition := range testControllers(module.Deps{}) {
		if definition.Area == coolRuntimeController.AreaApp {
			continue
		}
		for _, route := range definition.Routes {
			want, ok := expected[route.Name]
			if !ok {
				continue
			}
			seen[route.Name] = true
			if route.Method != want.method || route.FullPath != want.path || route.Description != want.description || route.Permission != want.permission || route.IgnoreAuth || route.Action == nil {
				t.Fatalf("unexpected route %s: %#v", route.Name, route)
			}
		}
	}
	if !reflect.DeepEqual(seen, map[string]bool{"personUpdate": true, "uploadMode": true, "upload": true, "move": true, "order": true, "clear": true, "setKeep": true, "getKeep": true, "export": true, "import": true, "parse": true}) {
		t.Fatalf("missing custom API routes: %#v", seen)
	}

	permissions, err := coolRuntimeController.PermissionMap(testControllers(module.Deps{}))
	if err != nil {
		t.Fatalf("build permission map failed: %v", err)
	}
	for _, item := range []struct {
		key        string
		permission string
	}{
		{"POST:/admin/base/sys/user/move", "base:sys:user:move"},
		{"POST:/admin/base/sys/department/order", "base:sys:department:order"},
		{"POST:/admin/base/sys/log/clear", "base:sys:log:clear"},
		{"POST:/admin/base/sys/log/setKeep", "base:sys:log:setKeep"},
		{"GET:/admin/base/sys/log/getKeep", "base:sys:log:getKeep"},
		{"POST:/admin/base/sys/menu/export", "base:sys:menu:export"},
		{"POST:/admin/base/sys/menu/import", "base:sys:menu:import"},
		{"POST:/admin/base/sys/menu/parse", "base:sys:menu:parse"},
		{"GET:/admin/base/sys/param/html", "base:sys:param:html"},
	} {
		if permissions[item.key] != item.permission {
			t.Fatalf("unexpected permission for %s: %q", item.key, permissions[item.key])
		}
	}
	for _, key := range []string{
		"POST:/admin/base/comm/personUpdate",
		"GET:/admin/base/comm/uploadMode",
		"POST:/admin/base/comm/upload",
	} {
		if _, ok := permissions[key]; ok {
			t.Fatalf("unexpected comm permission for %s", key)
		}
	}
}

func TestAppCommRouteMetadata(t *testing.T) {
	expected := map[string]struct {
		method     string
		ignoreAuth bool
	}{
		"/app/base/comm/param":      {http.MethodGet, true},
		"/app/base/comm/eps":        {http.MethodGet, true},
		"/app/base/comm/upload":     {http.MethodPost, false},
		"/app/base/comm/uploadMode": {http.MethodGet, false},
	}
	for _, definition := range testControllers(module.Deps{}, []string{"site.info"}) {
		if definition.Area != coolRuntimeController.AreaApp {
			continue
		}
		for _, route := range definition.Routes {
			want, ok := expected[route.FullPath]
			if !ok {
				t.Fatalf("unexpected app route: %#v", route)
			}
			if route.Method != want.method || route.IgnoreAuth != want.ignoreAuth || route.Action == nil {
				t.Fatalf("unexpected app route metadata: %#v", route)
			}
			delete(expected, route.FullPath)
		}
	}
	if len(expected) != 0 {
		t.Fatalf("missing app routes: %#v", expected)
	}
}

func TestNodeAlignedHTMLRoutes(t *testing.T) {
	foundAdminRoute := false
	for _, definition := range testControllers(module.Deps{}) {
		for _, route := range definition.Routes {
			key := route.Method + ":" + route.FullPath
			if key == "GET:/admin/base/open/html" {
				t.Fatal("public HTML route must not be registered")
			}
			if key == "GET:/admin/base/sys/param/html" {
				foundAdminRoute = true
			}
		}
	}
	if !foundAdminRoute {
		t.Fatal("expected authenticated parameter HTML route")
	}
}

/**
 * 测试 controller 绑定模型和服务
 * @param t 测试对象
 * @returns null
 */
func TestBaseControllerBindings(t *testing.T) {
	models := modelMap(findBaseSpec(t).Models)
	controllers := testControllers(module.Deps{})
	expectedTables := map[string]string{
		"/admin/base/sys/user":       "base_sys_user",
		"/admin/base/sys/role":       "base_sys_role",
		"/admin/base/sys/menu":       "base_sys_menu",
		"/admin/base/sys/department": "base_sys_department",
		"/admin/base/sys/param":      "base_sys_param",
		"/admin/base/sys/log":        "base_sys_log",
	}
	for _, definition := range controllers {
		tableName, ok := expectedTables[definition.Prefix]
		if !ok {
			continue
		}
		if definition.Model.TableName != models[tableName].TableName {
			t.Fatalf("unexpected model for %s: %s", definition.Prefix, definition.Model.TableName)
		}
		if definition.Service == nil {
			t.Fatalf("expected service for %s", definition.Prefix)
		}
	}
}

/**
 * 测试自定义 handler 保持现有响应
 * @param t 测试对象
 * @returns null
 */
func TestBaseCustomRouteHandlersPreserveLegacyResponses(t *testing.T) {
	server := ghttp.GetServer("base-controller-handler-test")
	server.SetPort(0)
	registerControllerErrorBoundary(server)
	server.Use(func(r *ghttp.Request) {
		r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{UserId: 1, Username: "demo"}))
		r.Middleware.Next()
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	if err := coolRuntimeController.RegisterRoutes(server, nil, testControllers(module.Deps{})); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{
			method: http.MethodGet,
			path:   "/admin/base/comm/program",
			body:   `{"code":1000,"message":"success","data":"Go"}`,
		},
		{
			method: http.MethodPost,
			path:   "/admin/base/comm/logout",
			body:   `{"code":1000,"message":"success"}`,
		},
		{
			method: http.MethodPost,
			path:   "/admin/base/open/login",
			body:   `{"code":1002,"message":"用户名和密码不能为空"}`,
		},
	}
	for _, item := range cases {
		request := httptest.NewRequest(item.method, item.path, nil)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Body.String() != item.body {
			t.Fatalf("unexpected response for %s %s: %s", item.method, item.path, recorder.Body.String())
		}
	}
}

/**
 * 测试自定义 JSON handler 的参数错误响应
 * @param t 测试对象
 * @returns null
 */
func TestBaseCustomJSONHandlersRejectMalformedJSON(t *testing.T) {
	server := ghttp.GetServer("base-custom-json-error-handler-test")
	server.SetPort(0)
	registerControllerErrorBoundary(server)
	server.Use(func(r *ghttp.Request) {
		r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{UserId: 1, Username: "demo"}))
		r.Middleware.Next()
	})
	if err := coolRuntimeController.RegisterRoutes(server, nil, testControllers(module.Deps{})); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	for _, path := range []string{
		"/admin/base/comm/personUpdate",
		"/admin/base/sys/user/move",
		"/admin/base/sys/department/order",
		"/admin/base/sys/log/setKeep",
	} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("not-json"))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		body := struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode malformed JSON response for %s failed: %v", path, err)
		}
		if body.Code != exception.CodeValidateFail || body.Message == "" {
			t.Fatalf("unexpected malformed JSON response for %s: %s", path, recorder.Body.String())
		}
	}
}

/**
 * 测试非空上传服务通过 controller 路由保存文件
 * @param t 测试对象
 * @returns null
 */
func TestDepartmentOrderRejectsUnknownJSONFields(t *testing.T) {
	server := ghttp.GetServer("base-department-order-unknown-field-test")
	server.SetPort(0)
	registerControllerErrorBoundary(server)
	server.Use(func(r *ghttp.Request) {
		r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{UserId: 1, Username: "demo"}))
		r.Middleware.Next()
	})
	if err := coolRuntimeController.RegisterRoutes(server, nil, testControllers(module.Deps{})); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	request := httptest.NewRequest(http.MethodPost, "/admin/base/sys/department/order", strings.NewReader(`[{"id":1,"parentId":null,"orderNum":0,"foo":1}]`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	body := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode unknown field response failed: %v", err)
	}
	if body.Code != exception.CodeValidateFail || body.Message == "" {
		t.Fatalf("unexpected unknown field response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBaseUploadHandlerUsesConfiguredService(t *testing.T) {
	server := ghttp.GetServer("base-upload-handler-test")
	server.SetPort(0)
	registerControllerErrorBoundary(server)
	if err := coolRuntimeController.RegisterRoutes(server, nil, testControllers(module.Deps{UploadDir: t.TempDir()})); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("key", "avatars/proof.txt"); err != nil {
		t.Fatalf("write multipart key failed: %v", err)
	}
	part, err := writer.CreateFormFile("file", "proof.txt")
	if err != nil {
		t.Fatalf("create multipart file failed: %v", err)
	}
	if _, err := part.Write([]byte("proof")); err != nil {
		t.Fatalf("write multipart file failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/base/comm/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "文件上传失败") {
		t.Fatalf("unexpected upload response: %d %s", recorder.Code, recorder.Body.String())
	}
	result := struct {
		Code int    `json:"code"`
		Data string `json:"data"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode upload response failed: %v", err)
	}
	date := time.Now().Format("20060102")
	pattern := regexp.MustCompile(`^http://example\.com/upload/` + date + `/anonymous/[a-z0-9]{32}\.txt$`)
	if result.Code != exception.CodeSuccess || !pattern.MatchString(result.Data) {
		t.Fatalf("unexpected upload result: %#v", result)
	}
}

/**
 * 测试权限菜单服务错误返回 Node 兼容禁止响应
 * @param t 测试对象
 * @returns null
 */
func TestPermmenuHandlerReturnsNodeForbiddenOnPermissionError(t *testing.T) {
	server := ghttp.GetServer("base-permmenu-error-handler-test")
	server.SetPort(0)
	registerControllerErrorBoundary(server)
	server.Use(func(r *ghttp.Request) {
		r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{UserId: 1, Username: "demo", TenantId: security.PlatformTenant()}))
		r.Middleware.Next()
	})
	definition := adminController.BaseAdminCommController(nil, nil, baseSysService.NewPermissionService(permissionErrorDB{}), nil)
	if err := coolRuntimeController.RegisterRoutes(server, nil, []coolRuntimeController.Definition{definition}); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	request := httptest.NewRequest(http.MethodGet, "/admin/base/comm/permmenu", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected protocol-compatible 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != `{"code":1001,"message":"操作失败"}` {
		t.Fatalf("unexpected internal response: %s", recorder.Body.String())
	}
}

type permissionErrorDB struct {
	gdb.DB
}

func (permissionErrorDB) GetCount(context.Context, string, ...interface{}) (int, error) {
	return 0, errors.New("permission query failed")
}

/**
 * 测试验证码路由返回可渲染图片
 * @param t 测试对象
 * @returns null
 */
func TestCaptchaRouteReturnsRenderableImage(t *testing.T) {
	server := ghttp.GetServer("base-captcha-handler-test")
	server.SetPort(0)
	registerControllerErrorBoundary(server)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()
	if err := coolRuntimeController.RegisterRoutes(server, nil, testControllers(module.Deps{})); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/admin/base/open/captcha?height=45&width=150&color=%232c3142", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := struct {
		Code int `json:"code"`
		Data struct {
			CaptchaID string `json:"captchaId"`
			Data      string `json:"data"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode captcha response failed: %v", err)
	}
	if body.Code != exception.CodeSuccess || body.Data.CaptchaID == "" || !strings.HasPrefix(body.Data.Data, "data:image/svg+xml;base64,") {
		t.Fatalf("unexpected captcha response: %#v", body)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(body.Data.Data, "data:image/svg+xml;base64,"))
	if err != nil || !strings.Contains(string(svg), `width="150"`) || !strings.Contains(string(svg), `height="45"`) {
		t.Fatalf("unexpected captcha SVG: %q, %v", svg, err)
	}
}

func registerControllerErrorBoundary(server *ghttp.Server) {
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	server.Use(definitions[0].Handler, definitions[1].Handler)
}

// 构建测试使用的 Base Controller
func testControllers(deps module.Deps, allowKeys ...[]string) []coolRuntimeController.Definition {
	if deps.DB == nil {
		db, err := gdb.New(gdb.ConfigNode{Type: "mysql", DryRun: true})
		if err != nil {
			panic(err)
		}
		deps.DB = db
	}
	if deps.UploadDirectory == "" {
		deps.UploadDirectory = module.UploadDirectory(deps.UploadDir)
	}
	if len(allowKeys) > 0 {
		content, err := json.Marshal(map[string]interface{}{
			"module": map[string]interface{}{
				"base": map[string]interface{}{"allowKeys": allowKeys[0]},
			},
		})
		if err != nil {
			panic(err)
		}
		adapter, err := gcfg.NewAdapterContent(string(content))
		if err != nil {
			panic(err)
		}
		config := g.Cfg()
		previous := config.GetAdapter()
		config.SetAdapter(adapter)
		defer config.SetAdapter(previous)
	}
	spec := moduleSpec("base")
	if err := spec.Configure(context.Background()); err != nil {
		panic(err)
	}
	controllers, err := spec.Controllers(deps)
	if err != nil {
		panic(err)
	}
	return controllers
}

func modelMap(definitions []entity.Definition) map[string]entity.Definition {
	models := make(map[string]entity.Definition, len(definitions))
	for _, definition := range definitions {
		models[definition.TableName] = definition
	}
	return models
}

/**
 * 获取资源名
 * @param prefix controller 前缀
 * @returns string
 */
func resourceName(prefix string) string {
	const adminPrefix = "/admin/"
	if len(prefix) < len(adminPrefix) {
		return prefix
	}
	return prefix[len(adminPrefix):]
}
