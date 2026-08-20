package base_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	eps "github.com/toothdy/cool-admin-go-next/cool/util/eps"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
	baseDTO "github.com/toothdy/cool-admin-go-next/modules/base/dto"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

const (
	customAPIPersonUserID     int64 = 970001
	customAPIMoveDepartmentID int64 = 970002
	customAPIMoveUserOneID    int64 = 970003
	customAPIMoveUserTwoID    int64 = 970004
	customAPIOrderParentID    int64 = 970005
	customAPIOrderFirstID     int64 = 970006
	customAPIOrderSecondID    int64 = 970007
	customAPILogOneID         int64 = 970008
	customAPILogTwoID         int64 = 970009
	customAPIParamOneID       int64 = 970010
	customAPIParamTwoID       int64 = 970011
	customAPITenantLogOneID   int64 = 970012
	customAPITenantLogTwoID   int64 = 970013
	customAPITenantOneID      int64 = 970021
	customAPITenantTwoID      int64 = 970022
	customAPIHTTPAdminID      int64 = 970101
	customAPIHTTPDepartmentID int64 = 970102
	customAPIHTTPUserOneID    int64 = 970103
	customAPIHTTPUserTwoID    int64 = 970104
	customAPIHTTPParentID     int64 = 970105
	customAPIHTTPFirstID      int64 = 970106
	customAPIHTTPSecondID     int64 = 970107
	customAPIHTTPSourceID     int64 = 970109
	customAPIPartialUpdateID  int64 = 970119
)

func TestPersonUpdatePartialMySQLIntegration(t *testing.T) {
	if os.Getenv("COOL_CUSTOM_API_INTEGRATION") != "1" {
		t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run real MySQL person update integration test")
	}

	ctx := context.Background()
	db := g.DB()
	if _, err := schema.NewSyncer(db).Sync(ctx, baseDefinitions()); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	count, err := db.Model("base_sys_user").Ctx(ctx).Where("id", customAPIPartialUpdateID).Count()
	if err != nil {
		t.Fatalf("check person update fixture failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("person update fixture id %d is occupied; use an isolated test database", customAPIPartialUpdateID)
	}

	_, err = db.Exec(ctx, "INSERT INTO base_sys_user (id, createTime, updateTime, tenantId, departmentId, userId, name, username, password, passwordV, nickName, headImg, phone, email, remark, status, socketId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		customAPIPartialUpdateID, "2026-01-02 03:04:05", "2026-01-02 03:04:05", 7, 23, 42, "original-name", "partial-update-user", "original-password", 9, "original-nickname", "/original.png", "13800000000", "original@example.com", "original-remark", 1, "original-socket")
	if err != nil {
		t.Fatalf("insert person update fixture failed: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := db.Model("base_sys_user").Ctx(ctx).Where("id", customAPIPartialUpdateID).Delete(); cleanupErr != nil {
			t.Errorf("cleanup person update fixture failed: %v", cleanupErr)
		}
	})

	var request baseDTO.PersonUpdateRequest
	if err = json.Unmarshal([]byte(`{"headImg":"http://127.0.0.1:8001/upload/20260723/avatar.jpg","nickName":"管理员","oldPassword":"","password":""}`), &request); err != nil {
		t.Fatalf("decode person update request failed: %v", err)
	}
	service := baseSysService.NewUserService(db, baseModel.BaseSysUser(), baseModel.BaseSysUserRole(), nil, nil)
	if err = service.PersonUpdate(ctx, customAPIPartialUpdateID, request); err != nil {
		t.Fatalf("partial person update failed: %v", err)
	}

	after, err := db.Model("base_sys_user").Ctx(ctx).Where("id", customAPIPartialUpdateID).One()
	if err != nil {
		t.Fatalf("read person update fixture failed: %v", err)
	}
	if after["nickName"].String() != "管理员" || after["headImg"].String() != "http://127.0.0.1:8001/upload/20260723/avatar.jpg" {
		t.Fatalf("submitted profile fields were not updated: %#v", after)
	}
	expected := map[string]string{
		"createTime": "2026-01-02 03:04:05", "tenantId": "7", "departmentId": "23", "userId": "42",
		"name": "original-name", "username": "partial-update-user", "password": "original-password", "passwordV": "9",
		"phone": "13800000000", "email": "original@example.com", "remark": "original-remark", "status": "1", "socketId": "original-socket",
	}
	for field, value := range expected {
		if after[field].String() != value {
			t.Fatalf("partial person update changed %s: expected %q, got %q", field, value, after[field].String())
		}
	}
}

func TestCustomAPIServiceIntegration(t *testing.T) {
	if os.Getenv("COOL_CUSTOM_API_INTEGRATION") != "1" {
		t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run real MySQL custom API integration test")
	}

	ctx := context.Background()
	bypassCtx := tenant.WithoutTenant(ctx)
	db := g.DB()
	definitions := baseDefinitions()
	if _, err := schema.NewSyncer(db).Sync(ctx, definitions); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}

	cleanupCustomAPIData(t, ctx)
	t.Cleanup(func() {
		cleanupCustomAPIData(t, ctx)
	})

	userService := baseSysService.NewUserService(db, baseModel.BaseSysUser(), baseModel.BaseSysUserRole(), nil, nil)
	departmentService := baseSysService.NewDepartmentService(
		db,
		baseModel.BaseSysDepartment(),
		baseModel.BaseSysUser(),
		baseModel.BaseSysUserRole(),
		baseModel.BaseSysRoleDepartment(),
		nil,
		nil,
	)
	logService := baseSysService.NewLogService(db, baseModel.BaseSysLog(), baseModel.BaseSysUser(), baseModel.BaseSysConf())
	confService := baseSysService.NewConfService(db, baseModel.BaseSysConf())

	insertCustomAPIUser(t, ctx, customAPIPersonUserID, 0, "custom-api-person")
	if err := userService.PersonUpdate(ctx, customAPIPersonUserID, baseDTO.PersonUpdateRequest{
		NickName: "集成昵称",
		HeadImg:  "/integration.png",
		Phone:    "13800000001",
		Email:    "integration@example.com",
		Remark:   "集成备注",
	}); err != nil {
		t.Fatalf("person update failed: %v", err)
	}
	person, err := db.Model("base_sys_user").Ctx(ctx).Where("id", customAPIPersonUserID).One()
	if err != nil {
		t.Fatalf("read updated person failed: %v", err)
	}
	if person["nickName"].String() != "集成昵称" || person["headImg"].String() != "/integration.png" || person["phone"].String() != "13800000001" || person["email"].String() != "integration@example.com" || person["remark"].String() != "集成备注" {
		t.Fatalf("person allowed fields were not updated: %#v", person)
	}
	if person["username"].String() != "custom-api-person" || person["status"].Int64() != 1 {
		t.Fatalf("person protected fields changed: %#v", person)
	}

	insertCustomAPIDepartment(t, ctx, customAPIMoveDepartmentID, nil, 0, "custom-api-move")
	insertCustomAPIUser(t, ctx, customAPIMoveUserOneID, 0, "custom-api-move-one")
	insertCustomAPIUser(t, ctx, customAPIMoveUserTwoID, 0, "custom-api-move-two")
	if err := userService.Move(bypassCtx, baseDTO.MoveReq{
		DepartmentID: customAPIMoveDepartmentID,
		UserIDs:      []int64{customAPIMoveUserOneID, customAPIMoveUserTwoID},
	}); err != nil {
		t.Fatalf("move users failed: %v", err)
	}
	movedCount, err := db.Model("base_sys_user").Ctx(ctx).WhereIn("id", []int64{customAPIMoveUserOneID, customAPIMoveUserTwoID}).Where("departmentId", customAPIMoveDepartmentID).Count()
	if err != nil {
		t.Fatalf("count moved users failed: %v", err)
	}
	if movedCount != 2 {
		t.Fatalf("expected two moved users, got %d", movedCount)
	}

	insertCustomAPIDepartment(t, ctx, customAPIOrderParentID, nil, 0, "custom-api-order-parent")
	insertCustomAPIDepartment(t, ctx, customAPIOrderFirstID, nil, 10, "custom-api-order-first")
	insertCustomAPIDepartment(t, ctx, customAPIOrderSecondID, nil, 20, "custom-api-order-second")
	parentID := customAPIOrderParentID
	if err := departmentService.Order(bypassCtx, []baseDTO.DepartmentOrderItem{
		{ID: customAPIOrderFirstID, ParentID: &parentID, OrderNum: 3},
		{ID: customAPIOrderSecondID, ParentID: &parentID, OrderNum: 4},
	}); err != nil {
		t.Fatalf("department order failed: %v", err)
	}
	ordered, err := db.Model("base_sys_department").Ctx(ctx).WhereIn("id", []int64{customAPIOrderFirstID, customAPIOrderSecondID}).Order("id").All()
	if err != nil {
		t.Fatalf("read ordered departments failed: %v", err)
	}
	if len(ordered) != 2 || ordered[0]["parentId"].Int64() != customAPIOrderParentID || ordered[0]["orderNum"].Int64() != 3 || ordered[1]["parentId"].Int64() != customAPIOrderParentID || ordered[1]["orderNum"].Int64() != 4 {
		t.Fatalf("department order values are incorrect: %#v", ordered)
	}

	triggerName := "trg_custom_api_order_" + strings.ReplaceAll(guid.S(), "-", "")
	triggerSQL := fmt.Sprintf("CREATE TRIGGER `%s` BEFORE UPDATE ON `base_sys_department` FOR EACH ROW BEGIN IF NEW.id = %d THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'custom api order failure'; END IF; END", triggerName, customAPIOrderSecondID)
	if _, err := db.Exec(ctx, triggerSQL); err != nil {
		t.Fatalf("create order rollback trigger failed: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(ctx, "DROP TRIGGER IF EXISTS `"+triggerName+"`"); err != nil {
			t.Errorf("drop order rollback trigger failed: %v", err)
		}
	})
	rollbackErr := departmentService.Order(bypassCtx, []baseDTO.DepartmentOrderItem{
		{ID: customAPIOrderFirstID, ParentID: &parentID, OrderNum: 30},
		{ID: customAPIOrderSecondID, ParentID: &parentID, OrderNum: 40},
	})
	if rollbackErr == nil {
		t.Fatal("expected department order transaction to fail")
	}
	rolledBack, err := db.Model("base_sys_department").Ctx(ctx).WhereIn("id", []int64{customAPIOrderFirstID, customAPIOrderSecondID}).Order("id").All()
	if err != nil {
		t.Fatalf("read rolled back departments failed: %v", err)
	}
	if rolledBack[0]["orderNum"].Int64() != 3 || rolledBack[1]["orderNum"].Int64() != 4 {
		t.Fatalf("department transaction did not roll back: %#v", rolledBack)
	}

	var restoreErr error
	var insertFailureErr error
	var restoredSQLMode string
	if err := db.Transaction(ctx, func(_ context.Context, tx gdb.TX) (transactionErr error) {
		sqlModeValue, err := tx.GetValue("SELECT @@SESSION.sql_mode")
		if err != nil {
			return fmt.Errorf("read session sql mode failed: %w", err)
		}
		originalSQLMode := sqlModeValue.String()
		modeChanged := false
		defer func() {
			if !modeChanged {
				return
			}
			if _, err := tx.Exec("SET SESSION sql_mode = ?", originalSQLMode); err != nil {
				restoreErr = err
				if transactionErr == nil {
					transactionErr = fmt.Errorf("restore session sql mode failed: %w", err)
				} else {
					transactionErr = fmt.Errorf("%w; restore session sql mode failed: %v", transactionErr, err)
				}
				return
			}
			restoredValue, err := tx.GetValue("SELECT @@SESSION.sql_mode")
			if err != nil {
				restoreErr = err
				if transactionErr == nil {
					transactionErr = fmt.Errorf("verify restored session sql mode failed: %w", err)
				}
				return
			}
			restoredSQLMode = restoredValue.String()
			if restoredSQLMode != originalSQLMode {
				restoreErr = fmt.Errorf("restored sql mode %q differs from original %q", restoredSQLMode, originalSQLMode)
				if transactionErr == nil {
					transactionErr = restoreErr
				}
			}
		}()
		if _, err := tx.Exec("SET SESSION sql_mode = CONCAT_WS(',', @@SESSION.sql_mode, 'NO_AUTO_VALUE_ON_ZERO')"); err != nil {
			return fmt.Errorf("enable NO_AUTO_VALUE_ON_ZERO failed: %w", err)
		}
		modeChanged = true
		changedModeValue, err := tx.GetValue("SELECT @@SESSION.sql_mode")
		if err != nil {
			return fmt.Errorf("verify enabled session sql mode failed: %w", err)
		}
		if !strings.Contains(changedModeValue.String(), "NO_AUTO_VALUE_ON_ZERO") {
			return fmt.Errorf("verify enabled session sql mode failed: mode=%q", changedModeValue.String())
		}
		if _, err := tx.Exec("INSERT INTO base_sys_log (id, tenantId, action) VALUES (?, ?, ?)", 0, 0, "custom-api-log-zero"); err != nil {
			return fmt.Errorf("insert zero-id log failed: %w", err)
		}
		if _, err := tx.Exec("INSERT INTO base_sys_log (id, tenantId, action) VALUES (?, ?, ?)", customAPILogOneID, customAPITenantOneID, "custom-api-log-one"); err != nil {
			return fmt.Errorf("insert first log failed: %w", err)
		}
		if _, err := tx.Exec("INSERT INTO base_sys_log (id, tenantId, action) VALUES (?, ?, ?)", customAPILogTwoID, customAPITenantTwoID, "custom-api-log-two"); err != nil {
			return fmt.Errorf("insert second log failed: %w", err)
		}
		if _, err := tx.Exec("INSERT INTO base_sys_log (id, tenantId, action) VALUES (?, ?, ?)", 0, 0, "custom-api-log-duplicate"); err == nil {
			return fmt.Errorf("expected duplicate log insert to fail")
		} else {
			insertFailureErr = err
		}
		return nil
	}); err != nil {
		if restoreErr != nil {
			t.Fatalf("restore session sql mode failed: %v", restoreErr)
		}
		t.Fatalf("prepare logs failed: %v", err)
	}
	if restoreErr != nil {
		t.Fatalf("restore session sql mode failed: %v", restoreErr)
	}
	if insertFailureErr == nil {
		t.Fatal("expected duplicate log insert to fail")
	}
	zeroLogCount, err := db.Model("base_sys_log").Ctx(ctx).Where("id", 0).Count()
	if err != nil {
		t.Fatalf("count zero-id log failed: %v", err)
	}
	if zeroLogCount != 1 {
		t.Fatalf("expected explicit id=0 log, got %d", zeroLogCount)
	}
	successfulLogCount, err := db.Model("base_sys_log").Ctx(ctx).WhereIn("id", []int64{customAPILogOneID, customAPILogTwoID}).Count()
	if err != nil {
		t.Fatalf("count successful log inserts failed: %v", err)
	}
	if successfulLogCount != 2 {
		t.Fatalf("expected normal successful log inserts, got %d", successfulLogCount)
	}
	tenantIdentity, err := security.NewTenantIdentity(customAPITenantOneID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	tenantOneCtx := security.ContextWithUser(ctx, security.UserContext{UserId: customAPIMoveUserOneID, TenantId: tenantIdentity})
	if err := logService.Clear(tenantOneCtx); err != nil {
		t.Fatalf("clear tenant logs failed: %v", err)
	}
	deletedLogCount, err := db.Model("base_sys_log").Ctx(ctx).Where("id", customAPILogOneID).Count()
	if err != nil {
		t.Fatalf("count cleared tenant log failed: %v", err)
	}
	retainedLogCount, err := db.Model("base_sys_log").Ctx(ctx).Where("id", customAPILogTwoID).Count()
	if err != nil {
		t.Fatalf("count retained tenant log failed: %v", err)
	}
	if deletedLogCount != 0 || retainedLogCount != 1 {
		t.Fatalf("tenant log clear crossed scope: deleted=%d retained=%d", deletedLogCount, retainedLogCount)
	}

	if _, err := db.Exec(ctx, "INSERT INTO base_sys_conf (cKey, cValue) VALUES ('customApiKeep', '31')"); err != nil {
		t.Fatalf("insert custom config fixture failed: %v", err)
	}
	if err := confService.UpdateValue(ctx, "customApiKeep", 23); err != nil {
		t.Fatalf("update custom config failed: %v", err)
	}
	keep, err := confService.GetValue(ctx, "customApiKeep")
	if err != nil || fmt.Sprint(keep) != "23" {
		t.Fatalf("expected updated custom config 23, got %v, %v", keep, err)
	}
}

func TestTenantBoundaryServiceIntegration(t *testing.T) {
	if os.Getenv("COOL_CUSTOM_API_INTEGRATION") != "1" {
		t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run real MySQL tenant boundary integration test")
	}

	ctx := context.Background()
	db := g.DB()
	if _, err := schema.NewSyncer(db).Sync(ctx, baseDefinitions()); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupCustomAPIData(t, ctx)
	t.Cleanup(func() {
		cleanupCustomAPIData(t, ctx)
	})

	if _, err := db.Exec(ctx, "INSERT INTO base_sys_param (id, keyName, name, data, dataType, tenantId) VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)",
		customAPIParamOneID, "tenant-boundary-one", "<b>tenant one</b>", "<script>alert(1)</script>", 1, customAPITenantOneID,
		customAPIParamTwoID, "tenant-boundary-two", "tenant two", "two", 1, customAPITenantTwoID,
	); err != nil {
		t.Fatalf("insert tenant params failed: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO base_sys_log (id, action, tenantId) VALUES (?, ?, ?), (?, ?, ?)",
		customAPITenantLogOneID, "tenant-boundary-one", customAPITenantOneID,
		customAPITenantLogTwoID, "tenant-boundary-two", customAPITenantTwoID,
	); err != nil {
		t.Fatalf("insert tenant logs failed: %v", err)
	}

	tenantIdentity, err := security.NewTenantIdentity(customAPITenantOneID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	tenantOneCtx := security.ContextWithUser(ctx, security.UserContext{UserId: customAPIHTTPAdminID, TenantId: tenantIdentity})
	paramService := baseSysService.NewParamService(db, baseModel.BaseSysParam(), nil)
	logService := baseSysService.NewLogService(db, baseModel.BaseSysLog(), baseModel.BaseSysUser(), baseModel.BaseSysConf())

	value, err := paramService.DataByKey(tenantOneCtx, "tenant-boundary-two")
	if err != nil || value != nil {
		t.Fatalf("cross-tenant key lookup leaked data: %#v %v", value, err)
	}
	pageValue, err := paramService.Page(tenantOneCtx, crud.QueryRequest{Page: 1, Size: 15, Keyword: "tenant-boundary-", Raw: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("tenant param page failed: %v", err)
	}
	page := pageValue.(crud.PageResult)
	if page.Pagination.Total != 1 || len(page.List) != 1 || fmt.Sprint(page.List[0]["id"]) != fmt.Sprint(customAPIParamOneID) {
		t.Fatalf("tenant page leaked records: %#v", page)
	}
	if _, err = paramService.Info(tenantOneCtx, crud.InfoRequest{ID: customAPIParamTwoID}); err == nil {
		t.Fatal("cross-tenant param info must fail")
	}
	if _, err = paramService.Update(tenantOneCtx, crud.UpdateRequest{Data: map[string]interface{}{"id": customAPIParamTwoID, "name": "changed"}}); err == nil {
		t.Fatal("cross-tenant param update must fail")
	}
	if _, err = paramService.Delete(tenantOneCtx, crud.DeleteRequest{IDs: []interface{}{customAPIParamTwoID}}); err == nil {
		t.Fatal("cross-tenant param delete must fail")
	}
	htmlValue, err := paramService.HTMLByKey(tenantOneCtx, "tenant-boundary-one")
	if err != nil {
		t.Fatalf("tenant HTML lookup failed: %v", err)
	}
	if strings.Contains(htmlValue, "<script>") || strings.Contains(htmlValue, "<b>") || !strings.Contains(htmlValue, "&lt;script&gt;") {
		t.Fatalf("parameter HTML was not escaped: %s", htmlValue)
	}
	if err = logService.Clear(tenantOneCtx); err != nil {
		t.Fatalf("tenant log clear failed: %v", err)
	}
	oneCount, err := db.Model("base_sys_log").Ctx(ctx).Where("id", customAPITenantLogOneID).Count()
	if err != nil {
		t.Fatalf("count tenant one log failed: %v", err)
	}
	twoCount, err := db.Model("base_sys_log").Ctx(ctx).Where("id", customAPITenantLogTwoID).Count()
	if err != nil {
		t.Fatalf("count tenant two log failed: %v", err)
	}
	if oneCount != 0 || twoCount != 1 {
		t.Fatalf("tenant log clear crossed scope: tenantOne=%d tenantTwo=%d", oneCount, twoCount)
	}
}

func insertCustomAPIUser(t *testing.T, ctx context.Context, id int64, departmentID int64, username string) {
	t.Helper()
	if _, err := g.DB().Exec(ctx, "INSERT INTO base_sys_user (id, departmentId, username, password, passwordV, status) VALUES (?, ?, ?, ?, ?, ?)", id, departmentID, username, "integration-password", 1, 1); err != nil {
		t.Fatalf("insert user %d failed: %v", id, err)
	}
}

func insertCustomAPIDepartment(t *testing.T, ctx context.Context, id int64, parentID *int64, orderNum int64, name string) {
	t.Helper()
	if _, err := g.DB().Exec(ctx, "INSERT INTO base_sys_department (id, parentId, orderNum, name) VALUES (?, ?, ?, ?)", id, parentID, orderNum, name); err != nil {
		t.Fatalf("insert department %d failed: %v", id, err)
	}
}

func cleanupCustomAPIData(t *testing.T, ctx context.Context) {
	t.Helper()
	statements := []string{
		"DELETE FROM base_sys_user WHERE id IN (970001, 970003, 970004)",
		"DELETE FROM base_sys_department WHERE id IN (970002, 970005, 970006, 970007)",
		"DELETE FROM base_sys_log WHERE id IN (0, 970008, 970009)",
		"DELETE FROM base_sys_log WHERE id IN (970012, 970013)",
		"DELETE FROM base_sys_param WHERE id IN (970010, 970011)",
		"DELETE FROM base_sys_conf WHERE cKey = 'customApiKeep'",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			t.Errorf("cleanup failed for %s: %v", statement, err)
		}
	}
}

func TestCustomAPIIntegration(t *testing.T) {
	if os.Getenv("COOL_CUSTOM_API_INTEGRATION") != "1" {
		t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run custom API integration test")
	}

	ctx := context.Background()
	definitions := baseDefinitions()
	if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupCustomAPIHTTPSeedData(t, ctx)
	repoRoot := repositoryRoot(t)
	importer := seed.NewImporter(g.DB(), definitions)
	if _, err := importer.ImportDB(ctx, "base", repoRoot+"/modules/base/db.json"); err != nil {
		t.Fatalf("import db seed failed: %v", err)
	}
	if _, err := importer.ImportMenu(ctx, "base", repoRoot+"/modules/base/menu.json"); err != nil {
		t.Fatalf("import menu seed failed: %v", err)
	}
	createCustomAPIAdmin(t, ctx)
	createLimitedUser(t, ctx)
	t.Cleanup(func() {
		cleanupCustomAPIHTTPSeedData(t, ctx)
	})

	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	app.New(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		Specs:              applicationSpecs(),
		AuthManagerFactory: baseTestAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start custom API integration server failed: %v", err)
	}
	defer server.Shutdown()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort())
	adminToken := loginCustomAPI(t, baseURL, "custom-api-admin", "123456")

	uploadModeResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/uploadMode", adminToken, nil)
	uploadModeData := responseData(t, assertCustomAPISuccess(t, uploadModeResponse))
	if len(uploadModeData) != 2 || uploadModeData["mode"] != "local" || uploadModeData["type"] != "local" {
		t.Fatalf("expected exact local upload mode, got %#v", uploadModeData)
	}

	content := []byte("custom API integration upload")
	uploadResponse := postMultipart(t, baseURL+"/admin/base/comm/upload", adminToken, "integration.txt", content)
	uploadBody := assertCustomAPISuccess(t, uploadResponse)
	uploadPath, ok := uploadBody["data"].(string)
	if !ok || !regexp.MustCompile(`^`+regexp.QuoteMeta(baseURL)+`/upload/[0-9]{8}/platform/user-970101/[a-z0-9]{32}\.txt$`).MatchString(uploadPath) {
		t.Fatalf("unexpected upload path: %#v", uploadBody)
	}
	uploadedFileResponse := customAPIJSONRequest(t, http.MethodGet, uploadPath, adminToken, nil)
	if uploadedFileResponse.StatusCode != http.StatusOK || uploadedFileResponse.Body != string(content) {
		t.Fatalf("uploaded file mismatch: status=%d body=%q", uploadedFileResponse.StatusCode, uploadedFileResponse.Body)
	}

	beforePersonResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", adminToken, nil)
	if beforePersonResponse.StatusCode != http.StatusOK {
		t.Fatalf("read person before update failed: %d %s", beforePersonResponse.StatusCode, beforePersonResponse.Body)
	}
	beforePerson := responseData(t, decodeResponseBody(t, beforePersonResponse.Body))
	protectedFields := []string{"id", "username", "passwordV", "status", "departmentId", "tenantId"}
	for _, field := range protectedFields {
		if _, ok := beforePerson[field]; !ok {
			t.Fatalf("person response omitted protected field %s: %#v", field, beforePerson)
		}
	}
	beforeUserRecord, err := g.DB().Model("base_sys_user").Ctx(ctx).Where("id", customAPIHTTPAdminID).One()
	if err != nil {
		t.Fatalf("read authentication user before update failed: %v", err)
	}
	personUpdateResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/comm/personUpdate", adminToken, map[string]interface{}{
		"id":           999999,
		"username":     "malicious-user",
		"passwordV":    999,
		"status":       0,
		"departmentId": customAPIHTTPDepartmentID,
		"tenantId":     999999,
		"nickName":     "集成 HTTP 昵称",
		"headImg":      "/integration-http.png",
		"phone":        "13800000002",
		"email":        "integration-http@example.com",
		"remark":       "集成 HTTP 备注",
	})
	assertCustomAPISuccess(t, personUpdateResponse)
	afterPersonResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", adminToken, nil)
	if afterPersonResponse.StatusCode != http.StatusOK {
		t.Fatalf("read person after update failed: %d %s", afterPersonResponse.StatusCode, afterPersonResponse.Body)
	}
	afterPerson := responseData(t, decodeResponseBody(t, afterPersonResponse.Body))
	for _, field := range protectedFields {
		if fmt.Sprint(afterPerson[field]) != fmt.Sprint(beforePerson[field]) {
			t.Fatalf("person update changed response protected field %s: before=%#v after=%#v", field, beforePerson[field], afterPerson[field])
		}
	}
	if _, ok := afterPerson["password"]; ok {
		t.Fatalf("person response exposed password: %#v", afterPerson)
	}
	if afterPerson["nickName"] != "集成 HTTP 昵称" {
		t.Fatalf("person update did not update allowed fields: %#v", afterPerson)
	}
	partialPersonUpdateResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/comm/personUpdate", adminToken, map[string]interface{}{
		"nickName": "部分更新昵称",
	})
	assertCustomAPISuccess(t, partialPersonUpdateResponse)
	partialPersonResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", adminToken, nil)
	if partialPersonResponse.StatusCode != http.StatusOK {
		t.Fatalf("read person after partial update failed: %d %s", partialPersonResponse.StatusCode, partialPersonResponse.Body)
	}
	partialPerson := responseData(t, decodeResponseBody(t, partialPersonResponse.Body))
	if partialPerson["nickName"] != "部分更新昵称" || partialPerson["headImg"] != "/integration-http.png" || partialPerson["phone"] != "13800000002" || partialPerson["email"] != "integration-http@example.com" || partialPerson["remark"] != "集成 HTTP 备注" {
		t.Fatalf("partial person update cleared omitted fields: %#v", partialPerson)
	}
	emptyPersonUpdateResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/comm/personUpdate", adminToken, map[string]interface{}{})
	assertCustomAPISuccess(t, emptyPersonUpdateResponse)
	emptyPersonResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/person", adminToken, nil)
	if emptyPersonResponse.StatusCode != http.StatusOK {
		t.Fatalf("read person after empty update failed: %d %s", emptyPersonResponse.StatusCode, emptyPersonResponse.Body)
	}
	emptyPerson := responseData(t, decodeResponseBody(t, emptyPersonResponse.Body))
	if emptyPerson["nickName"] != "部分更新昵称" || emptyPerson["headImg"] != "/integration-http.png" || emptyPerson["phone"] != "13800000002" || emptyPerson["email"] != "integration-http@example.com" || emptyPerson["remark"] != "集成 HTTP 备注" {
		t.Fatalf("empty person update cleared existing fields: %#v", emptyPerson)
	}
	afterUserRecord, err := g.DB().Model("base_sys_user").Ctx(ctx).Where("id", customAPIHTTPAdminID).One()
	if err != nil {
		t.Fatalf("read authentication user after update failed: %v", err)
	}
	if beforeUserRecord["id"].Int64() != afterUserRecord["id"].Int64() || beforeUserRecord["username"].String() != afterUserRecord["username"].String() || beforeUserRecord["password"].String() != afterUserRecord["password"].String() || beforeUserRecord["passwordV"].Int64() != afterUserRecord["passwordV"].Int64() || beforeUserRecord["status"].Int64() != afterUserRecord["status"].Int64() || beforeUserRecord["departmentId"].Int64() != afterUserRecord["departmentId"].Int64() || beforeUserRecord["tenantId"].String() != afterUserRecord["tenantId"].String() {
		t.Fatalf("person update changed authentication user protected fields: before=%#v after=%#v", beforeUserRecord, afterUserRecord)
	}

	insertCustomAPIDepartment(t, ctx, customAPIHTTPSourceID, nil, 0, "custom-api-http-source")
	insertCustomAPIDepartment(t, ctx, customAPIHTTPDepartmentID, nil, 1, "custom-api-http-target")
	insertCustomAPIUser(t, ctx, customAPIHTTPUserOneID, customAPIHTTPSourceID, "custom-api-http-one")
	insertCustomAPIUser(t, ctx, customAPIHTTPUserTwoID, customAPIHTTPSourceID, "custom-api-http-two")
	moveResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/user/move", adminToken, map[string]interface{}{
		"departmentId": customAPIHTTPDepartmentID,
		"userIds":      []int64{customAPIHTTPUserOneID, customAPIHTTPUserTwoID},
	})
	assertCustomAPISuccess(t, moveResponse)
	movedCount, err := g.DB().Model("base_sys_user").Ctx(ctx).WhereIn("id", []int64{customAPIHTTPUserOneID, customAPIHTTPUserTwoID}).Where("departmentId", customAPIHTTPDepartmentID).Count()
	if err != nil {
		t.Fatalf("count moved users failed: %v", err)
	}
	if movedCount != 2 {
		t.Fatalf("expected two moved users, got %d", movedCount)
	}

	insertCustomAPIDepartment(t, ctx, customAPIHTTPParentID, nil, 0, "custom-api-http-parent")
	insertCustomAPIDepartment(t, ctx, customAPIHTTPFirstID, nil, 10, "custom-api-http-first")
	insertCustomAPIDepartment(t, ctx, customAPIHTTPSecondID, nil, 20, "custom-api-http-second")
	parentID := customAPIHTTPParentID
	invalidOrderResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/department/order", adminToken, []map[string]interface{}{
		{"id": customAPIHTTPFirstID, "parentId": parentID, "orderNum": 3, "foo": 1},
		{"id": customAPIHTTPSecondID, "parentId": parentID, "orderNum": 4},
	})
	invalidOrderBody := decodeResponseBody(t, invalidOrderResponse.Body)
	if invalidOrderResponse.StatusCode != http.StatusOK || invalidOrderBody["code"] != float64(1002) || fmt.Sprint(invalidOrderBody["message"]) == "" {
		t.Fatalf("unknown department order field was not rejected: status=%d body=%s", invalidOrderResponse.StatusCode, invalidOrderResponse.Body)
	}
	unchanged, err := g.DB().Model("base_sys_department").Ctx(ctx).WhereIn("id", []int64{customAPIHTTPFirstID, customAPIHTTPSecondID}).Order("id").All()
	if err != nil {
		t.Fatalf("read departments after rejected order failed: %v", err)
	}
	if len(unchanged) != 2 || unchanged[0]["orderNum"].Int64() != 10 || unchanged[1]["orderNum"].Int64() != 20 {
		t.Fatalf("rejected department order changed data: %#v", unchanged)
	}
	orderResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/sys/department/order", adminToken, []map[string]interface{}{
		{"id": customAPIHTTPFirstID, "parentId": parentID, "orderNum": 3},
		{"id": customAPIHTTPSecondID, "parentId": parentID, "orderNum": 4},
	})
	assertCustomAPISuccess(t, orderResponse)
	ordered, err := g.DB().Model("base_sys_department").Ctx(ctx).WhereIn("id", []int64{customAPIHTTPFirstID, customAPIHTTPSecondID}).Order("id").All()
	if err != nil {
		t.Fatalf("read ordered departments failed: %v", err)
	}
	if len(ordered) != 2 || ordered[0]["parentId"].Int64() != customAPIHTTPParentID || ordered[0]["orderNum"].Int64() != 3 || ordered[1]["parentId"].Int64() != customAPIHTTPParentID || ordered[1]["orderNum"].Int64() != 4 {
		t.Fatalf("department order values are incorrect: %#v", ordered)
	}

	limitedToken := loginCustomAPI(t, baseURL, "limited", "123456")
	limitedRequests := []struct {
		method  string
		path    string
		payload interface{}
	}{
		{method: http.MethodPost, path: "/admin/base/sys/user/move", payload: map[string]interface{}{}},
		{method: http.MethodPost, path: "/admin/base/sys/department/order", payload: []map[string]interface{}{}},
		{method: http.MethodPost, path: "/admin/base/sys/log/clear"},
		{method: http.MethodPost, path: "/admin/base/sys/log/setKeep", payload: map[string]interface{}{"value": 45}},
		{method: http.MethodGet, path: "/admin/base/sys/log/getKeep"},
	}
	for _, item := range limitedRequests {
		response := customAPIJSONRequest(t, item.method, baseURL+item.path, limitedToken, item.payload)
		if response.StatusCode != http.StatusForbidden || response.Body != `{"code":1001,"message":"登录失效或无权限访问~"}` {
			t.Fatalf("limited request %s %s was not exact 403: status=%d body=%q", item.method, item.path, response.StatusCode, response.Body)
		}
	}

	for _, item := range []struct {
		method  string
		path    string
		payload interface{}
	}{
		{method: http.MethodPost, path: "/admin/base/comm/personUpdate", payload: map[string]interface{}{}},
		{method: http.MethodGet, path: "/admin/base/comm/uploadMode"},
		{method: http.MethodPost, path: "/admin/base/comm/upload"},
		{method: http.MethodPost, path: "/admin/base/sys/user/move", payload: map[string]interface{}{}},
		{method: http.MethodPost, path: "/admin/base/sys/department/order", payload: []map[string]interface{}{}},
		{method: http.MethodPost, path: "/admin/base/sys/log/clear"},
		{method: http.MethodPost, path: "/admin/base/sys/log/setKeep", payload: map[string]interface{}{}},
		{method: http.MethodGet, path: "/admin/base/sys/log/getKeep"},
	} {
		response := customAPIJSONRequest(t, item.method, baseURL+item.path, "", item.payload)
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("anonymous request %s %s expected 401, got %d: %s", item.method, item.path, response.StatusCode, response.Body)
		}
	}
	invalidTokenResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/comm/uploadMode", "invalid-token", nil)
	if invalidTokenResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid token request expected 401, got %d: %s", invalidTokenResponse.StatusCode, invalidTokenResponse.Body)
	}

	epsResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/open/eps", "", nil)
	if epsResponse.StatusCode != http.StatusOK {
		t.Fatalf("EPS request failed with status %d: %s", epsResponse.StatusCode, epsResponse.Body)
	}
	epsBody := struct {
		Code int                         `json:"code"`
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err := json.Unmarshal([]byte(epsResponse.Body), &epsBody); err != nil {
		t.Fatalf("decode HTTP EPS response failed: %v", err)
	}
	if epsBody.Code != 1000 {
		t.Fatalf("expected EPS success code 1000, got %d", epsBody.Code)
	}
	if len(epsBody.Data["base"]) != 8 {
		t.Fatalf("expected 8 base EPS controllers, got %#v", epsBody.Data["base"])
	}
	assertEPSAPI(t, findEPSController(t, epsBody.Data["base"], "AdminCommController").API, http.MethodPost, "/personUpdate", false)
	assertEPSAPI(t, findEPSController(t, epsBody.Data["base"], "AdminCommController").API, http.MethodGet, "/uploadMode", false)
	assertEPSAPI(t, findEPSController(t, epsBody.Data["base"], "AdminCommController").API, http.MethodPost, "/upload", false)
	assertEPSAPI(t, findEPSController(t, epsBody.Data["base"], "BaseSysUserEntity").API, http.MethodPost, "/move", false)
	assertEPSAPI(t, findEPSController(t, epsBody.Data["base"], "BaseSysDepartmentEntity").API, http.MethodPost, "/order", false)
	logController := findEPSController(t, epsBody.Data["base"], "BaseSysLogEntity")
	assertEPSAPI(t, logController.API, http.MethodPost, "/clear", false)
	assertEPSAPI(t, logController.API, http.MethodPost, "/setKeep", false)
	assertEPSAPI(t, logController.API, http.MethodGet, "/getKeep", false)
}

func loginCustomAPI(t *testing.T, baseURL string, username string, password string) string {
	t.Helper()
	captchaResponse := customAPIJSONRequest(t, http.MethodGet, baseURL+"/admin/base/open/captcha", "", nil)
	if captchaResponse.StatusCode != http.StatusOK {
		t.Fatalf("captcha failed with status %d: %s", captchaResponse.StatusCode, captchaResponse.Body)
	}
	captchaData := responseData(t, decodeResponseBody(t, captchaResponse.Body))
	captchaID, ok := captchaData["captchaId"].(string)
	if !ok || captchaID == "" {
		t.Fatalf("expected captcha id, got %#v", captchaData)
	}
	captchaImage, ok := captchaData["data"].(string)
	if !ok {
		t.Fatalf("expected captcha image, got %#v", captchaData)
	}
	verifyCode := captchaCodeFromSVG(t, captchaImage)
	loginResponse := customAPIJSONRequest(t, http.MethodPost, baseURL+"/admin/base/open/login", "", map[string]interface{}{
		"username":   username,
		"password":   password,
		"captchaId":  captchaID,
		"verifyCode": verifyCode,
	})
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login failed for %s with status %d: %s", username, loginResponse.StatusCode, loginResponse.Body)
	}
	loginData := responseData(t, decodeResponseBody(t, loginResponse.Body))
	token, ok := loginData["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected login token for %s, got %#v", username, loginData)
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

func assertCustomAPISuccess(t *testing.T, response testHTTPResponse) map[string]interface{} {
	t.Helper()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", response.StatusCode, response.Body)
	}
	body := decodeResponseBody(t, response.Body)
	if code, ok := body["code"].(float64); !ok || code != 1000 {
		t.Fatalf("expected success code 1000, got %#v", body)
	}
	return body
}

func customAPIJSONRequest(t *testing.T, method string, url string, token string, payload interface{}) testHTTPResponse {
	t.Helper()
	var requestBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode JSON request failed: %v", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		t.Fatalf("create HTTP request failed: %v", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	return executeRequest(t, request)
}

func postMultipart(t *testing.T, url string, token string, filename string, content []byte) testHTTPResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", token)
	return executeRequest(t, request)
}

func executeRequest(t *testing.T, request *http.Request) testHTTPResponse {
	t.Helper()
	request.Header.Set("X-Forwarded-For", "198.51.100.77")
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

func cleanupCustomAPIHTTPSeedData(t *testing.T, ctx context.Context) {
	t.Helper()
	statements := []string{
		"DELETE FROM base_sys_user_role WHERE userId IN (970101, 970103, 970104, 9002) OR roleId = 9001",
		"DELETE FROM base_sys_role_menu WHERE roleId = 9001",
		"DELETE FROM base_sys_role_department WHERE roleId = 9001",
		"DELETE FROM base_sys_user WHERE id IN (970101, 970103, 970104, 9002)",
		"DELETE FROM base_sys_role WHERE id = 9001",
		"DELETE FROM base_sys_department WHERE id IN (970102, 970105, 970106, 970107, 970109)",
		"DELETE FROM base_sys_log WHERE id = 970108 OR ip = '198.51.100.77'",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			t.Errorf("HTTP integration cleanup failed for %s: %v", statement, err)
		}
	}
}

func createCustomAPIAdmin(t *testing.T, ctx context.Context) {
	t.Helper()
	role, err := g.DB().GetOne(ctx, "SELECT id FROM base_sys_role WHERE label = ? AND (tenantId IS NULL OR tenantId = 0) LIMIT 1", "admin")
	if err != nil {
		t.Fatalf("query platform admin role failed: %v", err)
	}
	if role.IsEmpty() {
		t.Fatal("platform admin role is missing")
	}
	if _, err = g.DB().Exec(ctx, "INSERT INTO base_sys_user (id, username, password, passwordV, status) VALUES (?, ?, ?, ?, ?)", customAPIHTTPAdminID, "custom-api-admin", "e10adc3949ba59abbe56e057f20f883e", 7, 1); err != nil {
		t.Fatalf("create custom API admin failed: %v", err)
	}
	if _, err = g.DB().Exec(ctx, "INSERT INTO base_sys_user_role (userId, roleId) VALUES (?, ?)", customAPIHTTPAdminID, role["id"].Int64()); err != nil {
		t.Fatalf("bind custom API admin role failed: %v", err)
	}
}
