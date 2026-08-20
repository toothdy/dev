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

func TestAuthIntegrationAdminLoginRefreshAndPerson(t *testing.T) {
	if os.Getenv("COOL_AUTH_INTEGRATION") != "1" {
		t.Skip("set COOL_AUTH_INTEGRATION=1 to run real MySQL auth integration test")
	}

	ctx := context.Background()
	definitions := baseAndRecycleDefinitions()
	if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupAuthSeedData(t, ctx)

	repoRoot := repositoryRoot(t)
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
		t.Fatalf("start auth integration server failed: %v", err)
	}
	defer server.Shutdown()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort())
	loginResponse := authLoginRequest(t, baseURL, "admin", "123456")
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login failed with status %d: %s", loginResponse.StatusCode, loginResponse.Body)
	}
	loginBody := decodeResponseBody(t, loginResponse.Body)
	loginData := responseData(t, loginBody)
	accessToken := stringValue(loginData["token"])
	refreshToken := stringValue(loginData["refreshToken"])
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("expected token pair, got %#v", loginBody)
	}

	wrongPasswordResponse := authLoginRequest(t, baseURL, "admin", "wrong")
	wrongPasswordBody := decodeResponseBody(t, wrongPasswordResponse.Body)
	if wrongPasswordResponse.StatusCode != http.StatusOK || wrongPasswordBody["message"] != "账户或密码不正确~" {
		t.Fatalf("expected business login error, got status %d body %s", wrongPasswordResponse.StatusCode, wrongPasswordResponse.Body)
	}

	refreshResponse := doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/refreshToken", `{"refreshToken":"`+refreshToken+`"}`, "")
	if refreshResponse.StatusCode != http.StatusOK {
		t.Fatalf("refresh token failed with status %d: %s", refreshResponse.StatusCode, refreshResponse.Body)
	}
	refreshBody := decodeResponseBody(t, refreshResponse.Body)
	refreshData := responseData(t, refreshBody)
	refreshedToken := stringValue(refreshData["token"])
	if refreshedToken == "" || stringValue(refreshData["refreshToken"]) == "" {
		t.Fatalf("expected refreshed token pair, got %#v", refreshBody)
	}

	personResponse := doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", refreshedToken)
	if personResponse.StatusCode != http.StatusOK {
		t.Fatalf("person failed with status %d: %s", personResponse.StatusCode, personResponse.Body)
	}
	personBody := decodeResponseBody(t, personResponse.Body)
	personData := responseData(t, personBody)
	if personData["username"] != "admin" {
		t.Fatalf("expected admin person, got %#v", personBody)
	}
	if _, ok := personData["password"]; ok {
		t.Fatal("expected person response without password")
	}

	unauthorizedResponse := doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", "")
	if unauthorizedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated person to return 401, got %d", unauthorizedResponse.StatusCode)
	}
	if unauthorizedResponse.Body != `{"code":1001,"message":"登录失效~"}` {
		t.Fatalf("expected exact unauthorized body, got %q", unauthorizedResponse.Body)
	}
}

func authLoginRequest(t *testing.T, baseURL string, username string, password string) httpResponse {
	t.Helper()
	captchaResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/open/captcha", "", nil)
	if captchaResponse.StatusCode != http.StatusOK {
		t.Fatalf("captcha failed with status %d: %s", captchaResponse.StatusCode, captchaResponse.Body)
	}
	captchaData := responseData(t, decodeResponseBody(t, captchaResponse.Body))
	captchaID, _ := captchaData["captchaId"].(string)
	captchaImage, _ := captchaData["data"].(string)
	payload, err := json.Marshal(map[string]interface{}{
		"username":   username,
		"password":   password,
		"captchaId":  captchaID,
		"verifyCode": captchaCodeFromSVG(t, captchaImage),
	})
	if err != nil {
		t.Fatalf("encode login request failed: %v", err)
	}
	return doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/login", string(payload), "")
}

type httpResponse struct {
	StatusCode int
	Body       string
}

func doJSONRequest(t *testing.T, method string, url string, body string, token string) httpResponse {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("create HTTP request failed: %v", err)
	}
	if body != "" {
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
	data, err := io.ReadAll(clientResponse.Body)
	if err != nil {
		t.Fatalf("read HTTP response failed: %v", err)
	}
	return httpResponse{
		StatusCode: clientResponse.StatusCode,
		Body:       string(data),
	}
}

func decodeResponseBody(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode response body failed: %v; body=%q", err, body)
	}
	return decoded
}

func responseData(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response data object, got %#v", body)
	}
	return data
}

func stringValue(value interface{}) string {
	valueString, _ := value.(string)
	return valueString
}

func repositoryRoot(t *testing.T) string {
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

func cleanupAuthSeedData(t *testing.T, ctx context.Context) {
	t.Helper()
	statements := []string{
		"DELETE FROM recycle_item",
		"DELETE FROM recycle_data",
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
