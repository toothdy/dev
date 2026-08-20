package base_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

const (
	authSecurityRoleID    = int64(990201)
	authSecurityUserOne   = int64(990211)
	authSecurityUserTwo   = int64(990212)
	authSecurityUserThree = int64(990213)
	authSecurityUserFour  = int64(990214)
	authSecurityUserFive  = int64(990215)
)

func TestAuthSecurityIntegrationBoundariesAndRevocation(t *testing.T) {
	ctx, baseURL := setupAuthSecurityIntegration(t)

	adminPair := authSecurityLogin(t, baseURL, "admin")
	adminUserID := authSecurityScalar(t, ctx, `SELECT u.id
		FROM base_sys_user u
		INNER JOIN base_sys_user_role ur ON ur.userId = u.id
		INNER JOIN base_sys_role r ON r.id = ur.roleId
		WHERE u.status = 1
		AND (u.tenantId IS NULL OR u.tenantId = 0)
		AND r.label = 'admin'
		AND (r.tenantId IS NULL OR r.tenantId = 0)
		LIMIT 1`)
	adminRoleID := authSecurityScalar(t, ctx, `SELECT id FROM base_sys_role
		WHERE label = 'admin' AND (tenantId IS NULL OR tenantId = 0) LIMIT 1`)

	claimMutations := []struct {
		key   string
		value any
	}{
		{key: "username", value: "forged-admin"},
		{key: "roleIds", value: []int64{}},
		{key: "tenantId", value: int64(42)},
	}
	for _, mutation := range claimMutations {
		t.Run("tampered_"+mutation.key, func(t *testing.T) {
			tampered := tamperJWTClaim(t, adminPair.accessToken, mutation.key, mutation.value)
			authSecurityAssertStatus(t, doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", tampered), http.StatusUnauthorized)
		})
	}

	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/update", authSecurityJSON(t, map[string]any{
		"id": adminUserID, "nickName": "blocked",
	}), adminPair.accessToken), http.StatusForbidden)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/delete", authSecurityJSON(t, map[string]any{
		"ids": []int64{adminUserID},
	}), adminPair.accessToken), http.StatusForbidden)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/role/update", authSecurityJSON(t, map[string]any{
		"id": adminRoleID, "name": "blocked",
	}), adminPair.accessToken), http.StatusForbidden)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/role/delete", authSecurityJSON(t, map[string]any{
		"ids": []int64{adminRoleID},
	}), adminPair.accessToken), http.StatusForbidden)

	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/update", authSecurityJSON(t, map[string]any{
		"id": authSecurityUserOne, "roleIdList": []int64{adminRoleID},
	}), adminPair.accessToken), http.StatusForbidden)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/add", authSecurityJSON(t, map[string]any{
		"username": "security-blocked-admin", "password": "123456", "status": 1, "roleIdList": []int64{adminRoleID},
	}), adminPair.accessToken), http.StatusForbidden)
	if count := authSecurityCount(t, ctx, "SELECT COUNT(*) FROM base_sys_user_role WHERE userId = ? AND roleId = ?", authSecurityUserOne, authSecurityRoleID); count != 1 {
		t.Fatalf("forbidden admin assignment changed ordinary role relation: %d", count)
	}
	if count := authSecurityCount(t, ctx, "SELECT COUNT(*) FROM base_sys_user WHERE username = ?", "security-blocked-admin"); count != 0 {
		t.Fatalf("forbidden admin assignment created a user: %d", count)
	}

	thirdPair := authSecurityLogin(t, baseURL, "security-user-three")
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/role/update", authSecurityJSON(t, map[string]any{
		"id": authSecurityRoleID, "menuIdList": []int64{},
	}), adminPair.accessToken), http.StatusOK)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", thirdPair.accessToken), http.StatusUnauthorized)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/refreshToken", authSecurityJSON(t, map[string]any{
		"refreshToken": thirdPair.refreshToken,
	}), ""), http.StatusUnauthorized)

	firstPair := authSecurityLogin(t, baseURL, "security-user-one")
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/update", authSecurityJSON(t, map[string]any{
		"id": authSecurityUserOne, "roleIdList": []int64{},
	}), adminPair.accessToken), http.StatusOK)
	if count := authSecurityCount(t, ctx, "SELECT COUNT(*) FROM base_sys_user_role WHERE userId = ?", authSecurityUserOne); count != 0 {
		t.Fatalf("empty roleIdList did not clear roles: %d", count)
	}
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", firstPair.accessToken), http.StatusUnauthorized)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/refreshToken", authSecurityJSON(t, map[string]any{
		"refreshToken": firstPair.refreshToken,
	}), ""), http.StatusUnauthorized)

	secondPair := authSecurityLogin(t, baseURL, "security-user-two")
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/delete", authSecurityJSON(t, map[string]any{
		"ids": []int64{authSecurityUserTwo},
	}), adminPair.accessToken), http.StatusOK)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", secondPair.accessToken), http.StatusUnauthorized)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/refreshToken", authSecurityJSON(t, map[string]any{
		"refreshToken": secondPair.refreshToken,
	}), ""), http.StatusUnauthorized)

	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/comm/personUpdate", authSecurityJSON(t, map[string]any{
		"nickName": "security-admin",
	}), adminPair.accessToken), http.StatusOK)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodPost, baseURL+"/admin/base/comm/personUpdate", authSecurityJSON(t, map[string]any{
		"oldPassword": "123456", "password": "security-new-password",
	}), adminPair.accessToken), http.StatusOK)
	authSecurityAssertStatus(t, doJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", "", adminPair.accessToken), http.StatusUnauthorized)
}

func TestAuthIntegrationConcurrentLoginAndRefreshReadCommittedAuthorization(t *testing.T) {
	ctx, baseURL := setupAuthSecurityIntegration(t)

	loginRequest := authSecurityLoginRequest(t, baseURL, "security-user-four", "123456")
	loginTX := beginAuthSecurityRoleRemoval(t, ctx, authSecurityUserFour)
	defer loginTX.Rollback()
	loginResult := sendAuthSecurityRequest(loginRequest)
	authSecurityAssertRequestBlocked(t, loginResult)
	if err := loginTX.Commit(); err != nil {
		t.Fatalf("commit concurrent login authorization change failed: %v", err)
	}
	loginResponse := awaitAuthSecurityResponse(t, loginResult)
	authSecurityAssertStatus(t, loginResponse, http.StatusOK)
	loginBody := decodeResponseBody(t, loginResponse.Body)
	if loginBody["message"] != "该用户未设置任何角色，无法登录~" {
		t.Fatalf("concurrent login used stale roles: %s", loginResponse.Body)
	}

	fifthPair := authSecurityLogin(t, baseURL, "security-user-five")
	refreshRequest := authSecurityJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/refreshToken", authSecurityJSON(t, map[string]any{
		"refreshToken": fifthPair.refreshToken,
	}), "")
	refreshTX := beginAuthSecurityRoleRemoval(t, ctx, authSecurityUserFive)
	defer refreshTX.Rollback()
	refreshResult := sendAuthSecurityRequest(refreshRequest)
	authSecurityAssertRequestBlocked(t, refreshResult)
	if err := refreshTX.Commit(); err != nil {
		t.Fatalf("commit concurrent refresh authorization change failed: %v", err)
	}
	refreshResponse := awaitAuthSecurityResponse(t, refreshResult)
	authSecurityAssertStatus(t, refreshResponse, http.StatusUnauthorized)
}

func setupAuthSecurityIntegration(t *testing.T) (context.Context, string) {
	t.Helper()
	if os.Getenv("COOL_AUTH_INTEGRATION") != "1" {
		t.Skip("set COOL_AUTH_INTEGRATION=1 to run real MySQL auth security integration test")
	}

	ctx := context.Background()
	definitions := baseAndRecycleDefinitions()
	if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupAuthSeedData(t, ctx)
	t.Cleanup(func() { cleanupAuthSeedData(t, context.Background()) })
	repoRoot := repositoryRoot(t)
	importer := seed.NewImporter(g.DB(), definitions)
	if _, err := importer.ImportDB(ctx, "base", filepath.Join(repoRoot, "modules/base/db.json")); err != nil {
		t.Fatalf("import db seed failed: %v", err)
	}
	if _, err := importer.ImportMenu(ctx, "base", filepath.Join(repoRoot, "modules/base/menu.json")); err != nil {
		t.Fatalf("import menu seed failed: %v", err)
	}
	insertAuthSecurityFixture(t, ctx)

	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		Specs:        applicationSpecs(),
		AuthManagerFactory: baseTestAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start auth security server failed: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	return ctx, "http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort())
}

type authSecurityTokenPair struct {
	accessToken  string
	refreshToken string
}

func authSecurityLogin(t *testing.T, baseURL string, username string) authSecurityTokenPair {
	t.Helper()
	response := authLoginRequest(t, baseURL, username, "123456")
	authSecurityAssertStatus(t, response, http.StatusOK)
	data := responseData(t, decodeResponseBody(t, response.Body))
	return authSecurityTokenPair{
		accessToken:  stringValue(data["token"]),
		refreshToken: stringValue(data["refreshToken"]),
	}
}

func insertAuthSecurityFixture(t *testing.T, ctx context.Context) {
	t.Helper()
	now := "2026-07-27 00:00:00"
	if _, err := g.DB().Exec(ctx, `INSERT INTO base_sys_role
		(id, userId, name, label, remark, relevance, menuIdList, departmentIdList, createTime, updateTime, tenantId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		authSecurityRoleID, 1, "Security role", "security-role", "", 0, "[]", "[]", now, now,
	); err != nil {
		t.Fatalf("insert security role failed: %v", err)
	}
	users := []struct {
		id       int64
		username string
	}{
		{id: authSecurityUserOne, username: "security-user-one"},
		{id: authSecurityUserTwo, username: "security-user-two"},
		{id: authSecurityUserThree, username: "security-user-three"},
		{id: authSecurityUserFour, username: "security-user-four"},
		{id: authSecurityUserFive, username: "security-user-five"},
	}
	for _, user := range users {
		if _, err := g.DB().Exec(ctx, `INSERT INTO base_sys_user
			(id, username, password, passwordV, status, createTime, updateTime, tenantId)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`,
			user.id, user.username, "e10adc3949ba59abbe56e057f20f883e", 1, 1, now, now,
		); err != nil {
			t.Fatalf("insert security user failed: %v", err)
		}
		if _, err := g.DB().Exec(ctx, "INSERT INTO base_sys_user_role (userId, roleId) VALUES (?, ?)", user.id, authSecurityRoleID); err != nil {
			t.Fatalf("insert security user role failed: %v", err)
		}
	}
}

func beginAuthSecurityRoleRemoval(t *testing.T, ctx context.Context, userID int64) gdb.TX {
	t.Helper()
	tx, err := g.DB().Begin(ctx)
	if err != nil {
		t.Fatalf("begin authorization transaction failed: %v", err)
	}
	if _, err = tx.Ctx(ctx).GetOne("SELECT id FROM base_sys_user WHERE id = ? FOR UPDATE", userID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("lock authorization user failed: %v", err)
	}
	if _, err = tx.Ctx(ctx).Exec("DELETE FROM base_sys_user_role WHERE userId = ?", userID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("remove authorization relation failed: %v", err)
	}
	return tx
}

type authSecurityAsyncResponse struct {
	response httpResponse
	err      error
}

func sendAuthSecurityRequest(request *http.Request) <-chan authSecurityAsyncResponse {
	result := make(chan authSecurityAsyncResponse, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			result <- authSecurityAsyncResponse{err: err}
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			result <- authSecurityAsyncResponse{err: err}
			return
		}
		result <- authSecurityAsyncResponse{response: httpResponse{
			StatusCode: response.StatusCode,
			Body:       string(body),
		}}
	}()
	return result
}

func authSecurityAssertRequestBlocked(t *testing.T, result <-chan authSecurityAsyncResponse) {
	t.Helper()
	select {
	case response := <-result:
		t.Fatalf("request completed before authorization commit: response=%#v err=%v", response.response, response.err)
	case <-time.After(250 * time.Millisecond):
	}
}

func awaitAuthSecurityResponse(t *testing.T, result <-chan authSecurityAsyncResponse) httpResponse {
	t.Helper()
	select {
	case response := <-result:
		if response.err != nil {
			t.Fatalf("send concurrent auth request failed: %v", response.err)
		}
		return response.response
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent auth request did not finish after authorization commit")
		return httpResponse{}
	}
}

func authSecurityLoginRequest(t *testing.T, baseURL string, username string, password string) *http.Request {
	t.Helper()
	captchaResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/open/captcha", "", nil)
	if captchaResponse.StatusCode != http.StatusOK {
		t.Fatalf("captcha failed with status %d: %s", captchaResponse.StatusCode, captchaResponse.Body)
	}
	captchaData := responseData(t, decodeResponseBody(t, captchaResponse.Body))
	payload := authSecurityJSON(t, map[string]any{
		"username":   username,
		"password":   password,
		"captchaId":  stringValue(captchaData["captchaId"]),
		"verifyCode": captchaCodeFromSVG(t, stringValue(captchaData["data"])),
	})
	return authSecurityJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/login", payload, "")
}

func authSecurityJSONRequest(t *testing.T, method string, url string, body string, token string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("create auth security request failed: %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	return request
}

func authSecurityScalar(t *testing.T, ctx context.Context, query string, args ...any) int64 {
	t.Helper()
	record, err := g.DB().GetOne(ctx, query, args...)
	if err != nil || record.IsEmpty() {
		t.Fatalf("query security scalar failed: record=%#v err=%v", record, err)
	}
	for _, value := range record {
		return value.Int64()
	}
	t.Fatal("security scalar query returned no value")
	return 0
}

func authSecurityCount(t *testing.T, ctx context.Context, query string, args ...any) int {
	t.Helper()
	count, err := g.DB().GetCount(ctx, query, args...)
	if err != nil {
		t.Fatalf("query security count failed: %v", err)
	}
	return count
}

func authSecurityJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func authSecurityAssertStatus(t *testing.T, response httpResponse, want int) {
	t.Helper()
	if response.StatusCode != want {
		t.Fatalf("unexpected status %d, want %d: %s", response.StatusCode, want, response.Body)
	}
}

func tamperJWTClaim(t *testing.T, token string, key string, value any) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{}
	if err = json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims[key] = value
	payload, err = json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	return strings.Join(parts, ".")
}
