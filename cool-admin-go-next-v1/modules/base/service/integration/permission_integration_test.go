package base_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

func TestPermissionIntegrationPermmenuAndCRUDPermission(t *testing.T) {
	if os.Getenv("COOL_PERMISSION_INTEGRATION") != "1" {
		t.Skip("set COOL_PERMISSION_INTEGRATION=1 to run real MySQL permission integration test")
	}

	ctx := context.Background()
	definitions := baseDefinitions()
	if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupPermissionSeedData(t, ctx)

	repoRoot := permissionRepositoryRoot(t)
	importer := seed.NewImporter(g.DB(), definitions)
	if _, err := importer.ImportDB(ctx, "base", filepath.Join(repoRoot, "modules/base/db.json")); err != nil {
		t.Fatalf("import db seed failed: %v", err)
	}
	if _, err := importer.ImportMenu(ctx, "base", filepath.Join(repoRoot, "modules/base/menu.json")); err != nil {
		t.Fatalf("import menu seed failed: %v", err)
	}

	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		Specs:        applicationSpecs(),
		AuthManagerFactory: baseTestAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start permission integration server failed: %v", err)
	}
	defer server.Shutdown()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort())
	adminToken := loginForPermissionTest(t, baseURL, "admin", "123456")

	permmenu := getJSON(t, baseURL+"/admin/base/comm/permmenu", adminToken)
	data, ok := permmenu["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected permmenu data object, got %#v", permmenu)
	}
	menus, ok := data["menus"].([]interface{})
	if !ok || len(menus) == 0 {
		t.Fatalf("expected non-empty menus, got %#v", data["menus"])
	}
	assertPermMenuFlagsAreBoolean(t, menus)
	menuCount, err := g.DB().Model("base_sys_menu").Ctx(ctx).Count()
	if err != nil {
		t.Fatalf("count database menus failed: %v", err)
	}
	if len(menus) != menuCount {
		t.Fatalf("expected all %d database menus in permmenu response, got %d", menuCount, len(menus))
	}
	perms, ok := data["perms"].([]interface{})
	if !ok || len(perms) == 0 {
		t.Fatalf("expected non-empty perms, got %#v", data["perms"])
	}
	assertStringSliceContains(t, perms, "base:sys:user:page")
	assertStringSliceContains(t, perms, "base:sys:user:add")

	adminPage := postJSON(t, baseURL+"/admin/base/sys/user/page", adminToken, map[string]interface{}{
		"page": 1,
		"size": 15,
	})
	if adminPage.StatusCode != http.StatusOK {
		t.Fatalf("expected admin CRUD success, got %d: %s", adminPage.StatusCode, adminPage.Body)
	}

	createLimitedUser(t, ctx)
	limitedToken := loginForPermissionTest(t, baseURL, "limited", "123456")
	limitedPage := postJSON(t, baseURL+"/admin/base/sys/user/page", limitedToken, map[string]interface{}{
		"page": 1,
		"size": 15,
	})
	if limitedPage.StatusCode != http.StatusForbidden {
		t.Fatalf("expected limited user CRUD request to return 403, got %d: %s", limitedPage.StatusCode, limitedPage.Body)
	}
	if limitedPage.Body != `{"code":1001,"message":"登录失效或无权限访问~"}` {
		t.Fatalf("unexpected forbidden body: %s", limitedPage.Body)
	}
}

type testHTTPResponse struct {
	StatusCode int
	Body       string
}

func loginForPermissionTest(t *testing.T, baseURL string, username string, password string) string {
	t.Helper()
	return loginCustomAPI(t, baseURL, username, password)
}

func getJSON(t *testing.T, url string, token string) map[string]interface{} {
	t.Helper()

	response := doPermissionJSONRequest(t, http.MethodGet, url, token, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s failed with status %d: %s", url, response.StatusCode, response.Body)
	}
	body := map[string]interface{}{}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("decode GET response failed: %v; body=%q", err, response.Body)
	}
	return body
}

func postJSON(t *testing.T, url string, token string, body map[string]interface{}) testHTTPResponse {
	t.Helper()
	return doPermissionJSONRequest(t, http.MethodPost, url, token, body)
}

func doPermissionJSONRequest(t *testing.T, method string, url string, token string, body map[string]interface{}) testHTTPResponse {
	t.Helper()

	var (
		requestBody []byte
		err         error
	)
	if body != nil {
		requestBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("encode JSON request failed: %v", err)
		}
	}

	request, err := http.NewRequest(method, url, bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("create HTTP request failed: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", token)
	}

	clientResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send HTTP request failed: %v", err)
	}
	defer clientResponse.Body.Close()

	responseBody, err := io.ReadAll(clientResponse.Body)
	if err != nil {
		t.Fatalf("read HTTP response failed: %v", err)
	}
	return testHTTPResponse{
		StatusCode: clientResponse.StatusCode,
		Body:       string(responseBody),
	}
}

func assertPermMenuFlagsAreBoolean(t *testing.T, menus []interface{}) {
	t.Helper()
	for _, item := range menus {
		menu, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected menu object, got %#v", item)
		}
		if _, ok = menu["keepAlive"].(bool); !ok {
			t.Fatalf("expected keepAlive bool, got %#v", menu["keepAlive"])
		}
		if _, ok = menu["isShow"].(bool); !ok {
			t.Fatalf("expected isShow bool, got %#v", menu["isShow"])
		}
	}
}

func assertStringSliceContains(t *testing.T, items []interface{}, expected string) {
	t.Helper()
	for _, item := range items {
		if itemString, ok := item.(string); ok && itemString == expected {
			return
		}
	}
	t.Fatalf("expected string slice to contain %q, got %#v", expected, items)
}

func createLimitedUser(t *testing.T, ctx context.Context) {
	t.Helper()

	var (
		roleID     int64 = 9001
		userID     int64 = 9002
		statements       = []struct {
			query string
			args  []interface{}
		}{
			{
				query: "INSERT INTO base_sys_role (id, name, label, relevance) VALUES (?, ?, ?, ?)",
				args:  []interface{}{roleID, "受限角色", "limited", 1},
			},
			{
				query: "INSERT INTO base_sys_user (id, username, password, passwordV, status) VALUES (?, ?, ?, ?, ?)",
				args:  []interface{}{userID, "limited", "e10adc3949ba59abbe56e057f20f883e", 7, 1},
			},
			{
				query: "INSERT INTO base_sys_user_role (userId, roleId) VALUES (?, ?)",
				args:  []interface{}{userID, roleID},
			},
		}
	)
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("create limited user failed for %s: %v", statement.query, err)
		}
	}
}

func cleanupPermissionSeedData(t *testing.T, ctx context.Context) {
	t.Helper()

	statements := []string{
		"DELETE FROM base_sys_role_menu",
		"DELETE FROM base_sys_role_department",
		"DELETE FROM base_sys_user_role",
		"DELETE FROM base_sys_menu",
		"DELETE FROM base_sys_user",
		"DELETE FROM base_sys_role",
		"DELETE FROM base_sys_department",
		"DELETE FROM base_sys_param",
		"DELETE FROM base_sys_conf WHERE cKey IN ('init_db_base', 'init_menu_base', 'logKeep', 'recycleKeep')",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			t.Fatalf("cleanup failed for %s: %v", statement, err)
		}
	}
}

func permissionRepositoryRoot(t *testing.T) string {
	t.Helper()

	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory failed: %v", err)
	}
	for {
		if _, err = os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", current)
		}
		current = parent
	}
}
