package dict_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestDictIntegrationTypesDataAndCRUD(t *testing.T) {
	if os.Getenv("COOL_DICT_INTEGRATION") != "1" {
		t.Skip("set COOL_DICT_INTEGRATION=1 to run real MySQL dict integration test")
	}

	ctx := context.Background()
	definitions := collectModels(t, modules.Specs())
	if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupDictSeedData(t, ctx)

	repoRoot := repositoryRoot(t)
	importer := seed.NewImporter(g.DB(), definitions)
	if _, err := importer.ImportDB(ctx, "base", filepath.Join(repoRoot, "modules/base/db.json")); err != nil {
		t.Fatalf("import base db seed failed: %v", err)
	}
	if _, err := importer.ImportMenu(ctx, "base", filepath.Join(repoRoot, "modules/base/menu.json")); err != nil {
		t.Fatalf("import base menu seed failed: %v", err)
	}
	if _, err := importer.ImportDB(ctx, "dict", filepath.Join(repoRoot, "modules/dict/db.json")); err != nil {
		t.Fatalf("import dict db seed failed: %v", err)
	}

	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		Specs:        modules.Specs(),
		AuthManagerFactory: dictTestAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start dict test server failed: %v", err)
	}
	defer server.Shutdown()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort())

	types := getJSON(t, baseURL+"/admin/dict/info/types")
	typesData, ok := types["data"].([]interface{})
	if !ok || len(typesData) != 2 {
		t.Fatalf("expected 2 dict types, got %#v", types["data"])
	}
	keys := map[string]bool{}
	for _, item := range typesData {
		if row, ok := item.(map[string]interface{}); ok {
			keys[row["key"].(string)] = true
		}
	}
	for _, key := range []string{"brand", "occupation"} {
		if !keys[key] {
			t.Fatalf("expected dict type key %s", key)
		}
	}

	epsBody := getJSON(t, baseURL+"/admin/base/open/eps")
	epsData, ok := epsBody["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected eps data object, got %#v", epsBody)
	}
	dictControllers, ok := epsData["dict"].([]interface{})
	if !ok || len(dictControllers) != 2 {
		t.Fatalf("expected 2 dict eps controllers, got %#v", epsData["dict"])
	}

	token := loginDict(t, baseURL, "admin", "123456")

	data := postJSON(t, baseURL+"/admin/dict/info/data", token, map[string]interface{}{"types": []string{"occupation"}})
	dataBody, ok := data["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dict data object, got %#v", data["data"])
	}
	occupation, ok := dataBody["occupation"].([]interface{})
	if !ok || len(occupation) == 0 {
		t.Fatalf("expected occupation dict items, got %#v", dataBody["occupation"])
	}
	first := occupation[0].(map[string]interface{})
	if value, ok := first["value"].(float64); !ok || value != 4 {
		t.Fatalf("expected numeric value 4 for 法师, got %#v", first["value"])
	}

	typePage := postJSON(t, baseURL+"/admin/dict/type/page", token, map[string]interface{}{"page": 1, "size": 15})
	if code, ok := typePage["code"].(float64); !ok || code != 1000 {
		t.Fatalf("expected dict type page success, got %#v", typePage)
	}

	deleteResp := postJSON(t, baseURL+"/admin/dict/info/delete", token, map[string]interface{}{"ids": []interface{}{26}})
	if code, ok := deleteResp["code"].(float64); !ok || code != 1000 {
		t.Fatalf("expected dict info delete success, got %#v", deleteResp)
	}
	remaining, err := g.DB().Model("dict_info").Ctx(ctx).WhereIn("id", g.Slice{26, 30}).Count()
	if err != nil {
		t.Fatalf("count remaining dict info failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected cascade delete of dict info 26 and child 30, got %d remaining", remaining)
	}

	// 用户列表排除超级管理员
	userPage := postJSON(t, baseURL+"/admin/base/sys/user/page", token, map[string]interface{}{"page": 1, "size": 15})
	userData := userPage["data"].(map[string]interface{})
	userList := userData["list"].([]interface{})
	for _, item := range userList {
		if row, ok := item.(map[string]interface{}); ok && row["username"] == "admin" {
			t.Fatalf("super admin should not appear in user list")
		}
	}

	// 用户详情含角色与部门关联
	infoResp := doRequest(t, http.MethodGet, baseURL+"/admin/base/sys/user/info?id=1", token, nil)
	infoBody := decodeBody(t, infoResp.Body)
	infoData := infoBody["data"].(map[string]interface{})
	roleIDList, ok := infoData["roleIdList"].([]interface{})
	if !ok {
		t.Fatalf("expected roleIdList in user info, got %#v", infoData)
	}
	if len(roleIDList) == 0 {
		t.Fatalf("expected non-empty roleIdList for admin, got %#v", infoData)
	}
	if _, ok := infoData["departmentName"]; !ok {
		t.Fatalf("expected departmentName in user info, got %#v", infoData)
	}

	// 角色列表排除超管角色
	rolePage := postJSON(t, baseURL+"/admin/base/sys/role/page", token, map[string]interface{}{"page": 1, "size": 15})
	roleData := rolePage["data"].(map[string]interface{})
	roleList := roleData["list"].([]interface{})
	for _, item := range roleList {
		if row, ok := item.(map[string]interface{}); ok && row["label"] == "admin" {
			t.Fatalf("super admin role should not appear in role list")
		}
	}
}

func TestDictTenantIsolationIntegration(t *testing.T) {
	if os.Getenv("COOL_DICT_INTEGRATION") != "1" {
		t.Skip("set COOL_DICT_INTEGRATION=1 to run real MySQL dict tenant integration test")
	}

	const (
		tenantAID      = int64(8101)
		tenantBID      = int64(8102)
		globalTypeID   = int64(98101)
		tenantATypeID  = int64(98102)
		tenantBTypeID  = int64(98103)
		globalInfoID   = int64(98201)
		tenantARootID  = int64(98202)
		tenantAChildID = int64(98203)
		tenantBRootID  = int64(98204)
		tenantBToAID   = int64(98205)
		tenantAToBID   = int64(98206)
	)

	ctx := context.Background()
	definitions := collectModels(t, modules.Specs())
	if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
		t.Fatalf("sync dict schema failed: %v", err)
	}
	for _, statement := range []string{"DELETE FROM recycle_item", "DELETE FROM recycle_data", "DELETE FROM dict_info", "DELETE FROM dict_type"} {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			t.Fatalf("cleanup dict tenant fixture failed: %v", err)
		}
	}
	if _, err := g.DB().Exec(ctx, `INSERT INTO dict_type
		(id, createTime, updateTime, tenantId, name, `+"`key`"+`) VALUES
		(?, '2026-01-01', '2026-01-01', NULL, 'Global', 'global'),
		(?, '2026-01-01', '2026-01-01', ?, 'Tenant A', 'tenant-a'),
		(?, '2026-01-01', '2026-01-01', ?, 'Tenant B', 'tenant-b')`,
		globalTypeID, tenantATypeID, tenantAID, tenantBTypeID, tenantBID,
	); err != nil {
		t.Fatalf("seed dict type tenant fixture failed: %v", err)
	}
	if _, err := g.DB().Exec(ctx, `INSERT INTO dict_info
		(id, createTime, updateTime, tenantId, typeId, name, value, orderNum, parentId) VALUES
		(?, '2026-01-01', '2026-01-01', NULL, ?, 'Global', 'global', 1, NULL),
		(?, '2026-01-01', '2026-01-01', ?, ?, 'A Root', 'a-root', 1, NULL),
		(?, '2026-01-01', '2026-01-01', ?, ?, 'A Child', 'a-child', 2, ?),
		(?, '2026-01-01', '2026-01-01', ?, ?, 'B Root', 'b-root', 1, NULL),
		(?, '2026-01-01', '2026-01-01', ?, ?, 'B To A', 'b-to-a', 2, ?),
		(?, '2026-01-01', '2026-01-01', ?, ?, 'A To B', 'a-to-b', 3, ?)`,
		globalInfoID, globalTypeID,
		tenantARootID, tenantAID, tenantATypeID,
		tenantAChildID, tenantAID, tenantATypeID, tenantARootID,
		tenantBRootID, tenantBID, tenantBTypeID,
		tenantBToAID, tenantBID, tenantATypeID, tenantARootID,
		tenantAToBID, tenantAID, tenantBTypeID, tenantBRootID,
	); err != nil {
		t.Fatalf("seed dict info tenant fixture failed: %v", err)
	}

	tenantAIdentity, err := security.NewTenantIdentity(tenantAID)
	if err != nil {
		t.Fatal(err)
	}
	tenantBIdentity, err := security.NewTenantIdentity(tenantBID)
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]security.TenantIdentity{
		"platform": security.PlatformTenant(),
		"tenant-a": tenantAIdentity,
		"tenant-b": tenantBIdentity,
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()
	_, err = app.Build(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		Specs:        modules.Specs(),
		AuthManagerFactory: dictTestAuthManagerFactory,
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: app.MiddlewareReplaceModules,
			Definitions: []middleware.Definition{{
				Name:  "test.dict-tenant",
				Order: 200,
				Handler: func(r *ghttp.Request) {
					if identity, ok := identities[r.Header.Get("X-Test-Tenant")]; ok {
						r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{UserId: 99001, TenantId: identity}))
					}
					r.Middleware.Next()
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("build dict tenant test app failed: %v", err)
	}
	if err = server.Start(); err != nil {
		t.Fatalf("start dict tenant test server failed: %v", err)
	}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort())

	publicTypes := getScopedJSON(t, baseURL+"/admin/dict/info/types", "")
	assertDictKeys(t, publicTypes["data"], []string{"global"})

	missingData := doScopedRequest(t, http.MethodPost, baseURL+"/admin/dict/info/data", "", map[string]interface{}{"types": []string{"global"}})
	if missingData.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected missing dict data scope rejected, got %d: %s", missingData.StatusCode, missingData.Body)
	}

	tenantAData := postScopedJSON(t, baseURL+"/admin/dict/info/data", "tenant-a", map[string]interface{}{
		"types": []string{"tenant-a", "tenant-b", "global"},
	})
	dataMap := tenantAData["data"].(map[string]interface{})
	if _, ok := dataMap["tenant-a"]; !ok || dataMap["tenant-b"] != nil || dataMap["global"] != nil {
		t.Fatalf("tenant A observed foreign dict data: %#v", dataMap)
	}

	tenantAPage := postScopedJSON(t, baseURL+"/admin/dict/type/page", "tenant-a", map[string]interface{}{"page": 1, "size": 20})
	assertDictKeys(t, tenantAPage["data"].(map[string]interface{})["list"], []string{"tenant-a"})
	tenantAInfoPage := postScopedJSON(t, baseURL+"/admin/dict/info/page", "tenant-a", map[string]interface{}{"page": 1, "size": 20})
	assertDictNames(t, tenantAInfoPage["data"].(map[string]interface{})["list"], []string{"A Root", "A Child", "A To B"})
	platformPage := postScopedJSON(t, baseURL+"/admin/dict/type/page", "platform", map[string]interface{}{"page": 1, "size": 20})
	assertDictKeys(t, platformPage["data"].(map[string]interface{})["list"], []string{"global", "tenant-a", "tenant-b"})
	platformData := postScopedJSON(t, baseURL+"/admin/dict/info/data", "platform", map[string]interface{}{
		"types": []string{"global", "tenant-a", "tenant-b"},
	})
	platformDataMap := platformData["data"].(map[string]interface{})
	for _, key := range []string{"global", "tenant-a", "tenant-b"} {
		if _, ok := platformDataMap[key]; !ok {
			t.Fatalf("platform dict data missing %s: %#v", key, platformDataMap)
		}
	}

	addBody := postScopedJSON(t, baseURL+"/admin/dict/info/add", "tenant-a", map[string]interface{}{
		"typeId": tenantATypeID, "name": "Forged", "value": "forged", "orderNum": 9, "tenantId": tenantBID,
	})
	addedID := int64(addBody["data"].(map[string]interface{})["id"].(float64))
	added, err := g.DB().GetOne(ctx, "SELECT tenantId FROM dict_info WHERE id = ?", addedID)
	if err != nil || added["tenantId"].Int64() != tenantAID {
		t.Fatalf("forged tenant field changed ownership: row=%#v err=%v", added, err)
	}

	updateForeign := postScopedJSON(t, baseURL+"/admin/dict/info/update", "tenant-a", map[string]interface{}{
		"id": tenantBRootID, "name": "Changed",
	})
	if updateForeign["code"] != float64(1001) {
		t.Fatalf("expected cross-tenant update rejected, got %#v", updateForeign)
	}
	foreignRow, err := g.DB().GetOne(ctx, "SELECT name FROM dict_info WHERE id = ?", tenantBRootID)
	if err != nil || foreignRow["name"].String() != "B Root" {
		t.Fatalf("cross-tenant update changed foreign row: row=%#v err=%v", foreignRow, err)
	}

	mixedDelete := postScopedJSON(t, baseURL+"/admin/dict/type/delete", "tenant-a", map[string]interface{}{
		"ids": []interface{}{tenantATypeID, tenantBTypeID},
	})
	if mixedDelete["code"] != float64(1001) {
		t.Fatalf("expected mixed type delete rejected, got %#v", mixedDelete)
	}
	assertRowCount(t, ctx, "dict_type", []int64{tenantATypeID, tenantBTypeID}, 2)

	deleteAInfo := postScopedJSON(t, baseURL+"/admin/dict/info/delete", "tenant-a", map[string]interface{}{"ids": []interface{}{tenantARootID}})
	if deleteAInfo["code"] != float64(1000) {
		t.Fatalf("delete tenant A info failed: %#v", deleteAInfo)
	}
	assertRowCount(t, ctx, "dict_info", []int64{tenantARootID, tenantAChildID}, 0)
	assertRowCount(t, ctx, "dict_info", []int64{tenantBToAID}, 1)
	recycleRow, err := g.DB().GetOne(ctx, "SELECT id, count FROM recycle_data WHERE tenantId = ? ORDER BY id DESC LIMIT 1", tenantAID)
	if err != nil || recycleRow.IsEmpty() || recycleRow["count"].Int() != 2 {
		t.Fatalf("tenant A 字典树未完整进入回收站: row=%#v err=%v", recycleRow, err)
	}
	restoreAInfo := postScopedJSON(t, baseURL+"/admin/recycle/data/restore", "tenant-a", map[string]interface{}{
		"ids": []interface{}{recycleRow["id"].Int64()},
	})
	if restoreAInfo["code"] != float64(1000) {
		t.Fatalf("restore tenant A info failed: %#v", restoreAInfo)
	}
	assertRowCount(t, ctx, "dict_info", []int64{tenantARootID, tenantAChildID}, 2)

	deleteAInfoAgain := postScopedJSON(t, baseURL+"/admin/dict/info/delete", "tenant-a", map[string]interface{}{"ids": []interface{}{tenantARootID}})
	if deleteAInfoAgain["code"] != float64(1000) {
		t.Fatalf("delete tenant A info for conflict retry failed: %#v", deleteAInfoAgain)
	}
	conflictArchive, err := g.DB().GetOne(ctx, "SELECT id FROM recycle_data WHERE tenantId = ? ORDER BY id DESC LIMIT 1", tenantAID)
	if err != nil || conflictArchive.IsEmpty() {
		t.Fatalf("read conflict archive failed: row=%#v err=%v", conflictArchive, err)
	}
	if _, err = g.DB().Exec(ctx, `INSERT INTO dict_info
		(id, createTime, updateTime, tenantId, typeId, name, value, orderNum, parentId)
		VALUES (?, '2026-01-02', '2026-01-02', ?, ?, 'Conflict Root', 'conflict', 1, NULL)`,
		tenantARootID, tenantAID, tenantATypeID,
	); err != nil {
		t.Fatalf("seed restore conflict failed: %v", err)
	}
	conflictRestore := postScopedJSON(t, baseURL+"/admin/recycle/data/restore", "tenant-a", map[string]interface{}{
		"ids": []interface{}{conflictArchive["id"].Int64()},
	})
	if conflictRestore["code"] != float64(1000) {
		t.Fatalf("ordinary restore conflict must stay silent: %#v", conflictRestore)
	}
	assertRowCount(t, ctx, "dict_info", []int64{tenantAChildID}, 0)
	remainingArchive, err := g.DB().GetOne(ctx, "SELECT remainingCount FROM recycle_data WHERE id = ?", conflictArchive["id"].Int64())
	if err != nil || remainingArchive.IsEmpty() || remainingArchive["remainingCount"].Int() != 2 {
		t.Fatalf("conflict archive was not retained: row=%#v err=%v", remainingArchive, err)
	}
	if _, err = g.DB().Exec(ctx, "DELETE FROM dict_info WHERE id = ? AND tenantId = ?", tenantARootID, tenantAID); err != nil {
		t.Fatalf("clear restore conflict failed: %v", err)
	}
	retryRestore := postScopedJSON(t, baseURL+"/admin/recycle/data/restore", "tenant-a", map[string]interface{}{
		"ids": []interface{}{conflictArchive["id"].Int64()},
	})
	if retryRestore["code"] != float64(1000) {
		t.Fatalf("retry restore failed: %#v", retryRestore)
	}
	assertRowCount(t, ctx, "dict_info", []int64{tenantARootID, tenantAChildID}, 2)
	assertRowCount(t, ctx, "recycle_data", []int64{conflictArchive["id"].Int64()}, 0)

	deleteBInfo := postScopedJSON(t, baseURL+"/admin/dict/info/delete", "tenant-b", map[string]interface{}{"ids": []interface{}{tenantBRootID}})
	if deleteBInfo["code"] != float64(1000) {
		t.Fatalf("delete tenant B info failed: %#v", deleteBInfo)
	}
	assertRowCount(t, ctx, "dict_info", []int64{tenantAToBID}, 1)

	deleteAType := postScopedJSON(t, baseURL+"/admin/dict/type/delete", "tenant-a", map[string]interface{}{"ids": []interface{}{tenantATypeID}})
	if deleteAType["code"] != float64(1000) {
		t.Fatalf("delete tenant A type failed: %#v", deleteAType)
	}
	assertRowCount(t, ctx, "dict_type", []int64{tenantATypeID}, 0)
	assertRowCount(t, ctx, "dict_info", []int64{tenantBToAID}, 1)
}

func dictTestAuthManagerFactory(context.Context) *security.Manager {
	return security.NewManager("0123456789abcdef0123456789abcdef", 7200, 604800)
}

/**
 * 请求带测试租户身份的 GET 接口
 * @param t 测试上下文
 * @param url 请求地址
 * @param scope 测试租户身份
 * @returns JSON 响应
 */
func getScopedJSON(t *testing.T, url string, scope string) map[string]interface{} {
	t.Helper()
	response := doScopedRequest(t, http.MethodGet, url, scope, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s failed with status %d: %s", url, response.StatusCode, response.Body)
	}
	return decodeBody(t, response.Body)
}

/**
 * 请求带测试租户身份的 POST 接口
 * @param t 测试上下文
 * @param url 请求地址
 * @param scope 测试租户身份
 * @param body 请求数据
 * @returns JSON 响应
 */
func postScopedJSON(t *testing.T, url string, scope string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	response := doScopedRequest(t, http.MethodPost, url, scope, body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s failed with status %d: %s", url, response.StatusCode, response.Body)
	}
	return decodeBody(t, response.Body)
}

/**
 * 发送带测试租户身份的请求
 * @param t 测试上下文
 * @param method 请求方法
 * @param url 请求地址
 * @param scope 测试租户身份
 * @param body 请求数据
 * @returns HTTP 响应
 */
func doScopedRequest(t *testing.T, method string, url string, scope string, body interface{}) dictHTTPResponse {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode scoped JSON request failed: %v", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		t.Fatalf("create scoped request failed: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if scope != "" {
		request.Header.Set("X-Test-Tenant", scope)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send scoped request failed: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read scoped response failed: %v", err)
	}
	return dictHTTPResponse{StatusCode: response.StatusCode, Body: string(responseBody)}
}

/**
 * 校验字典键集合
 * @param t 测试上下文
 * @param value 字典列表
 * @param expected 预期字典键
 * @returns null
 */
func assertDictKeys(t *testing.T, value interface{}, expected []string) {
	t.Helper()
	rows, ok := value.([]interface{})
	if !ok {
		t.Fatalf("expected dict list, got %#v", value)
	}
	actual := make(map[string]bool, len(rows))
	for _, item := range rows {
		row, rowOK := item.(map[string]interface{})
		if !rowOK {
			t.Fatalf("expected dict row, got %#v", item)
		}
		key, keyOK := row["key"].(string)
		if !keyOK {
			t.Fatalf("expected dict key, got %#v", row)
		}
		actual[key] = true
	}
	if len(actual) != len(expected) {
		t.Fatalf("unexpected dict keys: %#v", actual)
	}
	for _, key := range expected {
		if !actual[key] {
			t.Fatalf("missing dict key %s in %#v", key, actual)
		}
	}
}

/**
 * 校验字典名称集合
 * @param t 测试上下文
 * @param value 字典列表
 * @param expected 预期字典名称
 * @returns null
 */
func assertDictNames(t *testing.T, value interface{}, expected []string) {
	t.Helper()
	rows, ok := value.([]interface{})
	if !ok {
		t.Fatalf("expected dict info list, got %#v", value)
	}
	actual := make(map[string]bool, len(rows))
	for _, item := range rows {
		row, rowOK := item.(map[string]interface{})
		if !rowOK {
			t.Fatalf("expected dict info row, got %#v", item)
		}
		name, nameOK := row["name"].(string)
		if !nameOK {
			t.Fatalf("expected dict info name, got %#v", row)
		}
		actual[name] = true
	}
	if len(actual) != len(expected) {
		t.Fatalf("unexpected dict info names: %#v", actual)
	}
	for _, name := range expected {
		if !actual[name] {
			t.Fatalf("missing dict info name %s in %#v", name, actual)
		}
	}
}

/**
 * 校验指定数据行数量
 * @param t 测试上下文
 * @param ctx 上下文
 * @param tableName 数据表名
 * @param ids 数据 ID
 * @param expected 预期数量
 * @returns null
 */
func assertRowCount(t *testing.T, ctx context.Context, tableName string, ids []int64, expected int) {
	t.Helper()
	count, err := g.DB().Model(tableName).Ctx(ctx).WhereIn("id", ids).Count()
	if err != nil {
		t.Fatalf("count %s rows failed: %v", tableName, err)
	}
	if count != expected {
		t.Fatalf("expected %d %s rows for %v, got %d", expected, tableName, ids, count)
	}
}

func cleanupDictSeedData(t *testing.T, ctx context.Context) {
	t.Helper()
	for _, statement := range []string{
		"DELETE FROM recycle_item",
		"DELETE FROM recycle_data",
		"DELETE FROM dict_info",
		"DELETE FROM dict_type",
		"DELETE FROM base_sys_role_menu",
		"DELETE FROM base_sys_role_department",
		"DELETE FROM base_sys_user_role",
		"DELETE FROM base_sys_menu",
		"DELETE FROM base_sys_user",
		"DELETE FROM base_sys_role",
		"DELETE FROM base_sys_department",
		"DELETE FROM base_sys_param",
		"DELETE FROM base_sys_conf WHERE c_key IN ('init_db_base', 'init_menu_base', 'init_db_dict', 'logKeep', 'recycleKeep')",
	} {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			t.Fatalf("cleanup failed for %s: %v", statement, err)
		}
	}
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

func collectModels(t *testing.T, specs []module.Spec) []entity.Definition {
	t.Helper()
	var definitions []entity.Definition
	for _, spec := range specs {
		definitions = append(definitions, spec.Models...)
	}
	return definitions
}

type dictHTTPResponse struct {
	StatusCode int
	Body       string
}

func getJSON(t *testing.T, url string) map[string]interface{} {
	t.Helper()
	response := doRequest(t, http.MethodGet, url, "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s failed with status %d: %s", url, response.StatusCode, response.Body)
	}
	return decodeBody(t, response.Body)
}

func postJSON(t *testing.T, url string, token string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	response := doRequest(t, http.MethodPost, url, token, body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s failed with status %d: %s", url, response.StatusCode, response.Body)
	}
	return decodeBody(t, response.Body)
}

func doRequest(t *testing.T, method string, url string, token string, body interface{}) dictHTTPResponse {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode JSON request failed: %v", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		t.Fatalf("create HTTP request failed: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send HTTP request failed: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read HTTP response failed: %v", err)
	}
	return dictHTTPResponse{StatusCode: response.StatusCode, Body: string(responseBody)}
}

func decodeBody(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	decoded := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode response failed: %v; body=%q", err, body)
	}
	return decoded
}

func loginDict(t *testing.T, baseURL string, username string, password string) string {
	t.Helper()
	captcha := getJSON(t, baseURL+"/admin/base/open/captcha")
	captchaData, ok := captcha["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected captcha data object, got %#v", captcha)
	}
	captchaID, ok := captchaData["captchaId"].(string)
	if !ok || captchaID == "" {
		t.Fatalf("expected captcha id, got %#v", captchaData)
	}
	verifyCode := captchaCodeFromSVG(t, captchaData["data"].(string))

	response := doRequest(t, http.MethodPost, baseURL+"/admin/base/open/login", "", map[string]interface{}{
		"username":   username,
		"password":   password,
		"captchaId":  captchaID,
		"verifyCode": verifyCode,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login failed with status %d: %s", response.StatusCode, response.Body)
	}
	body := decodeBody(t, response.Body)
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected login data object, got %#v", body)
	}
	token, ok := data["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected login token, got %#v", data)
	}
	return token
}

func captchaCodeFromSVG(t *testing.T, dataURI string) string {
	t.Helper()
	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(dataURI, prefix) {
		t.Fatalf("unexpected captcha data URI: %q", dataURI)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURI, prefix))
	if err != nil {
		t.Fatalf("decode captcha SVG failed: %v", err)
	}
	matches := regexp.MustCompile(`<text[^>]*>([A-Za-z0-9])</text>`).FindAllStringSubmatch(string(decoded), -1)
	if len(matches) != 4 {
		t.Fatalf("expected four captcha characters, got %q", decoded)
	}
	var code strings.Builder
	for _, match := range matches {
		code.WriteString(match[1])
	}
	return code.String()
}
