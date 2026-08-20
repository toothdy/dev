package base_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gsession"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	eps "github.com/toothdy/cool-admin-go-next/cool/util/eps"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

/**
 * 测试匿名 EPS 路由返回完整 base bootstrap metadata
 * @param t 测试对象
 * @returns null
 */
func TestEPSRouteReturnsAnonymousFullBootstrap(t *testing.T) {
	server := ghttp.GetServer("base-eps-test")
	server.SetPort(0)
	server.SetDumpRouterMap(false)
	server.SetSessionStorage(gsession.NewStorageMemory())
	defer server.Shutdown()

	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		Specs:        applicationSpecs(),
		AuthManagerFactory: baseTestAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/admin/base/open/eps", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := struct {
		Code int                         `json:"code"`
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode EPS response failed: %v", err)
	}
	if body.Code != exception.CodeSuccess {
		t.Fatalf("expected success response, got %#v", body)
	}

	baseControllers := body.Data["base"]
	if len(baseControllers) != 8 {
		t.Fatalf("expected 8 base controllers, got %#v", baseControllers)
	}
	user := findEPSController(t, baseControllers, "BaseSysUserEntity")
	assertEPSAPI(t, user.API, http.MethodPost, "/page", false)
	assertEPSAPI(t, user.API, http.MethodPost, "/add", false)
	assertEPSAPI(t, user.API, http.MethodGet, "/info", false)
	open := findEPSController(t, baseControllers, "BaseOpenController")
	assertEPSAPI(t, open.API, http.MethodGet, "/eps", true)
	assertEPSAPI(t, findEPSController(t, baseControllers, "AdminCommController").API, http.MethodGet, "/program", true)
	assertEPSAPI(t, findEPSController(t, baseControllers, "AdminCommController").API, http.MethodPost, "/personUpdate", false)
	assertEPSAPI(t, findEPSController(t, baseControllers, "AdminCommController").API, http.MethodGet, "/uploadMode", false)
	assertEPSAPI(t, findEPSController(t, baseControllers, "AdminCommController").API, http.MethodPost, "/upload", false)
	assertEPSAPI(t, findEPSController(t, baseControllers, "BaseSysUserEntity").API, http.MethodPost, "/move", false)
	assertEPSAPI(t, findEPSController(t, baseControllers, "BaseSysDepartmentEntity").API, http.MethodPost, "/order", false)
	menu := findEPSController(t, baseControllers, "BaseSysMenuEntity")
	assertEPSAPI(t, menu.API, http.MethodPost, "/parse", false)
	assertEPSAPI(t, menu.API, http.MethodPost, "/export", false)
	assertEPSAPI(t, menu.API, http.MethodPost, "/import", false)
	assertEPSAPI(t, findEPSController(t, baseControllers, "BaseSysParamEntity").API, http.MethodGet, "/html", false)
	log := findEPSController(t, baseControllers, "BaseSysLogEntity")
	assertEPSAPI(t, log.API, http.MethodPost, "/clear", false)
	assertEPSAPI(t, log.API, http.MethodPost, "/setKeep", false)
	assertEPSAPI(t, log.API, http.MethodGet, "/getKeep", false)

	columns := map[string]bool{}
	for _, column := range user.Columns {
		columns[column.PropertyName] = true
	}
	for _, propertyName := range []string{"id", "username", "status", "createTime", "updateTime"} {
		if !columns[propertyName] {
			t.Fatalf("expected user column %s, got %#v", propertyName, user.Columns)
		}
	}
	if !reflect.DeepEqual(user.PageQueryOp, eps.PageQueryOp{KeyWordLikeFields: []string{}, FieldEq: []string{}, FieldLike: []string{}}) {
		t.Fatalf("unexpected user page query fields: %#v", user.PageQueryOp)
	}
	if len(user.Columns) < 2 || user.Columns[len(user.Columns)-2].PropertyName != "createTime" || user.Columns[len(user.Columns)-1].PropertyName != "updateTime" {
		t.Fatalf("expected user time columns at the end, got %#v", user.Columns)
	}
	for _, controller := range baseControllers {
		if hasEPSColumn(controller.Columns, "tenantId") {
			t.Fatalf("expected tenantId excluded from %s EPS columns", controller.Name)
		}
	}

	menuKeepAlive := findEPSColumn(t, menu.Columns, "keepAlive")
	if menuKeepAlive.Type != "boolean" || menuKeepAlive.DefaultValue != true || menuKeepAlive.Nullable {
		t.Fatalf("unexpected menu keepAlive metadata: %#v", menuKeepAlive)
	}
	if !reflect.DeepEqual(menu.PageQueryOp, eps.PageQueryOp{KeyWordLikeFields: []string{}, FieldEq: []string{}, FieldLike: []string{}}) {
		t.Fatalf("unexpected menu page query fields: %#v", menu.PageQueryOp)
	}

	role := findEPSController(t, baseControllers, "BaseSysRoleEntity")
	roleRelevance := findEPSColumn(t, role.Columns, "relevance")
	if roleRelevance.Type != "boolean" || roleRelevance.DefaultValue != false || roleRelevance.Nullable {
		t.Fatalf("unexpected role relevance metadata: %#v", roleRelevance)
	}
	roleLabel := findEPSColumn(t, role.Columns, "label")
	if !roleLabel.Nullable || roleLabel.Length != "50" {
		t.Fatalf("unexpected role label metadata: %#v", roleLabel)
	}
	if roleUserID := findEPSColumn(t, role.Columns, "userId"); roleUserID.Type != "varchar" {
		t.Fatalf("unexpected role userId metadata: %#v", roleUserID)
	}

	if !reflect.DeepEqual(log.PageQueryOp.KeyWordLikeFields, []string{"b.name", "a.action", "a.ip"}) {
		t.Fatalf("unexpected log keyword fields: %#v", log.PageQueryOp)
	}
	expectedLogPageColumns := []string{"id", "userId", "action", "ip", "params", "name", "createTime", "updateTime"}
	if len(log.PageColumns) != len(expectedLogPageColumns) {
		t.Fatalf("unexpected log page columns: %#v", log.PageColumns)
	}
	for index, propertyName := range expectedLogPageColumns {
		if log.PageColumns[index].PropertyName != propertyName {
			t.Fatalf("unexpected log page column order: %#v", log.PageColumns)
		}
	}
	if findEPSColumn(t, log.PageColumns, "name").Source != "b.name" {
		t.Fatalf("unexpected joined log name column: %#v", log.PageColumns)
	}
}

func TestAppUploadModeRejectsAnonymousRequest(t *testing.T) {
	server := ghttp.GetServer("base-app-auth-test")
	server.SetPort(0)
	server.SetDumpRouterMap(false)
	server.SetSessionStorage(gsession.NewStorageMemory())
	defer server.Shutdown()

	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		Specs:        applicationSpecs(),
		AuthManagerFactory: baseTestAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/app/base/comm/uploadMode", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous app upload mode to return 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func baseTestAuthManagerFactory(context.Context) *security.Manager {
	return security.NewManager("0123456789abcdef0123456789abcdef", 7200, 604800)
}

/**
 * 查找 EPS controller
 * @param t 测试对象
 * @param controllers EPS controller 列表
 * @param name controller 名称
 * @returns eps.Controller
 */
func findEPSController(t *testing.T, controllers []eps.Controller, name string) eps.Controller {
	t.Helper()
	for _, controller := range controllers {
		if controller.Name == name {
			return controller
		}
	}
	t.Fatalf("expected EPS controller %s, got %#v", name, controllers)
	return eps.Controller{}
}

/**
 * 断言 EPS API 元数据
 * @param t 测试对象
 * @param API API 元数据列表
 * @param method HTTP 方法
 * @param path API 路径
 * @param ignoreToken 是否忽略 token
 * @returns null
 */
func assertEPSAPI(t *testing.T, API []eps.API, method string, path string, ignoreToken bool) {
	t.Helper()
	for _, API := range API {
		if API.Method == method && API.Path == path {
			if API.IgnoreToken != ignoreToken {
				t.Fatalf("unexpected ignoreToken for %s %s: %#v", method, path, API)
			}
			return
		}
	}
	t.Fatalf("expected EPS API %s %s, got %#v", method, path, API)
}

func loadEPSFixture(t *testing.T) map[string][]eps.Controller {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), "docs", "protocol", "fixtures", "eps-admin-success.json"))
	if err != nil {
		t.Fatalf("read EPS fixture failed: %v", err)
	}
	body := struct {
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err = json.Unmarshal(contents, &body); err != nil {
		t.Fatalf("decode EPS fixture failed: %v", err)
	}
	return body.Data
}

func TestBaseEPSMatchesFixtureContract(t *testing.T) {
	fixture := loadEPSFixture(t)
	actual := eps.Generate(module.CollectControllers(app.New(app.Options{
		StartServer: false,
		UploadDir:   t.TempDir(),
		Specs: applicationSpecs(),
	}).Modules()))

	fixtureUser := findEPSController(t, fixture["base"], "BaseSysUserEntity")
	actualUser := findEPSController(t, actual["base"], "BaseSysUserEntity")
	if actualUser.Prefix != fixtureUser.Prefix {
		t.Fatalf("expected user prefix %s, got %s", fixtureUser.Prefix, actualUser.Prefix)
	}
	assertEPSAPI(t, actualUser.API, http.MethodPost, "/page", false)
	assertEPSColumn(t, actualUser.Columns, "username", "varchar", "a.username")
	if !reflect.DeepEqual(actualUser.PageQueryOp, fixtureUser.PageQueryOp) {
		t.Fatalf("expected page query %#v, got %#v", fixtureUser.PageQueryOp, actualUser.PageQueryOp)
	}
}

func assertEPSColumn(t *testing.T, columns []eps.Column, propertyName string, fieldType string, source string) {
	t.Helper()
	for _, column := range columns {
		if column.PropertyName == propertyName && column.Type == fieldType && column.Source == source {
			return
		}
	}
	t.Fatalf("expected column %s %s %s, got %#v", propertyName, fieldType, source, columns)
}

func findEPSColumn(t *testing.T, columns []eps.Column, propertyName string) eps.Column {
	t.Helper()
	for _, column := range columns {
		if column.PropertyName == propertyName {
			return column
		}
	}
	t.Fatalf("expected EPS column %s, got %#v", propertyName, columns)
	return eps.Column{}
}

func hasEPSColumn(columns []eps.Column, propertyName string) bool {
	for _, column := range columns {
		if column.PropertyName == propertyName {
			return true
		}
	}
	return false
}
