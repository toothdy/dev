package integration_test

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	recycleEntity "github.com/toothdy/cool-admin-go-next/modules/recycle/entity"
	recycleEvent "github.com/toothdy/cool-admin-go-next/modules/recycle/event"
	taskEntity "github.com/toothdy/cool-admin-go-next/modules/task/entity"
	taskService "github.com/toothdy/cool-admin-go-next/modules/task/service"
)

type taskArchiveStore struct {
	recycle.Store
	beforeSave   func()
	afterSave    func(context.Context, gdb.TX, *recycle.Archive) error
	afterSaveErr error
}

func (s *taskArchiveStore) SaveArchive(ctx context.Context, tx gdb.TX, archive *recycle.Archive) error {
	if s.beforeSave != nil {
		s.beforeSave()
	}
	if err := s.Store.SaveArchive(ctx, tx, archive); err != nil {
		return err
	}
	if s.afterSave != nil {
		if err := s.afterSave(ctx, tx, archive); err != nil {
			return err
		}
	}
	return s.afterSaveErr
}

type taskRecycleInfoEngine struct {
	removeCalls atomic.Int32
}

func (e *taskRecycleInfoEngine) Healthy(context.Context) error {
	return nil
}

func (e *taskRecycleInfoEngine) SyncTask(context.Context, int64) error {
	return nil
}

func (e *taskRecycleInfoEngine) RemoveTask(context.Context, int64) error {
	e.removeCalls.Add(1)
	return nil
}

func (e *taskRecycleInfoEngine) Once(context.Context, taskService.TaskInfo) error {
	return nil
}

func TestTaskRecycleMySQLDeleteAndRestore(t *testing.T) {
	if os.Getenv("COOL_TASK_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_INTEGRATION=1 to run Task MySQL integration tests")
	}
	ctx := context.Background()
	db := g.DB()
	taskModels := []entity.Definition{taskEntity.TaskInfo(), taskEntity.TaskLog()}
	recycleModels := []entity.Definition{recycleEntity.Data(), recycleEntity.Item()}
	if _, err := schema.NewSyncer(db).Sync(ctx, append(taskModels, recycleModels...)); err != nil {
		t.Fatalf("sync Task Recycle schema failed: %v", err)
	}
	fixtureUUID := uuid.New()
	fixtureSuffix := hex.EncodeToString(fixtureUUID[:])
	guardTableName := "task_recycle_delete_guard_" + fixtureSuffix
	guardConstraintName := "fk_task_recycle_delete_guard_" + fixtureSuffix
	tenantID := int64(binary.BigEndian.Uint64(fixtureUUID[:8]) & ((uint64(1) << 63) - 1))
	if tenantID <= 0 {
		t.Fatal("Task Recycle fixture tenant ID must be positive")
	}
	cleanup := func(cleanupCtx context.Context) {
		dropGuardSQL := fmt.Sprintf("DROP TABLE IF EXISTS `%s`", guardTableName)
		if _, err := db.Exec(cleanupCtx, dropGuardSQL); err != nil {
			t.Fatalf("cleanup Task Recycle delete guard failed: %v", err)
		}
		statements := []string{
			"DELETE FROM recycle_item WHERE tenant_id = ?",
			"DELETE FROM recycle_data WHERE tenant_id = ?",
			"DELETE FROM task_log WHERE tenant_id = ?",
			"DELETE FROM task_info WHERE tenant_id = ?",
		}
		for _, statement := range statements {
			if _, err := db.Exec(cleanupCtx, statement, tenantID); err != nil {
				t.Fatalf("cleanup Task Recycle fixture failed: %v", err)
			}
		}
	}
	cleanup(ctx)
	t.Cleanup(func() { cleanup(context.Background()) })

	catalog, err := recycle.NewCatalog(taskModels)
	if err != nil {
		t.Fatal(err)
	}
	archiveStore, err := recycleEvent.NewStore(db, recycleModels[0], recycleModels[1])
	if err != nil {
		t.Fatal(err)
	}
	engine := &taskRecycleInfoEngine{}
	observedArchiveStore := &taskArchiveStore{
		Store: archiveStore,
		beforeSave: func() {
			if engine.removeCalls.Load() != 0 {
				t.Error("scheduler was removed before Task archive committed")
			}
		},
	}
	manager, err := recycle.NewManager(db, observedArchiveStore, catalog, recycle.Options{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	store, err := taskService.BuildStore(db, taskModels[0], taskModels[1])
	if err != nil {
		t.Fatal(err)
	}
	builder := task.NewRegistryBuilder()
	if err = builder.Register(task.HandlerDefinition{
		Name:    "taskRecycleIntegration.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	infoService, err := taskService.BuildInfoService(store, registry, engine, time.UTC, manager)
	if err != nil {
		t.Fatal(err)
	}
	tenantCtx, err := tenant.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	tenantCtx = security.ContextWithUser(tenantCtx, security.UserContext{UserId: 991, TenantId: identity})
	id, err := store.Insert(tenantCtx, taskService.TaskInfoDO{
		JobID: uuid.NewString(), Name: "task-recycle-integration", Cron: "0 * * * * *",
		Status: 1, Service: "taskRecycleIntegration.run()", Type: 1, TaskType: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, exists, err := store.Find(tenantCtx, id)
	if err != nil || !exists {
		t.Fatalf("read Task Recycle fixture failed: exists=%v err=%v", exists, err)
	}
	if err = store.WriteLog(tenantCtx, info, 1, "recycle integration"); err != nil {
		t.Fatal(err)
	}

	if _, err = infoService.Delete(tenantCtx, crud.DeleteRequest{IDs: []interface{}{id}}); err != nil {
		t.Fatalf("managed Task delete failed: %v", err)
	}
	if engine.removeCalls.Load() != 1 {
		t.Fatalf("scheduler was not removed exactly once after commit: %d", engine.removeCalls.Load())
	}
	archiveValue, err := db.Model("recycle_data").Ctx(ctx).Fields("id").Where("tenantId", tenantID).OrderDesc("id").Value()
	archiveID := archiveValue.Int64()
	if err != nil || archiveID <= 0 {
		t.Fatalf("Task archive was not created: id=%d err=%v", archiveID, err)
	}
	items, err := db.Model("recycle_item").Ctx(ctx).
		Fields("tableName", "parentItemId").Where("recycleId", archiveID).
		OrderAsc("restoreOrder").OrderAsc("id").All()
	if err != nil || len(items) != 2 || items[0]["tableName"].String() != "task_info" ||
		items[1]["tableName"].String() != "task_log" || items[1]["parentItemId"].Int64() <= 0 {
		t.Fatalf("Task root and log were not archived in one parented batch: items=%#v err=%v", items, err)
	}
	infoCount, err := db.Model("task_info").Ctx(ctx).Where("id", id).Count()
	if err != nil || infoCount != 0 {
		t.Fatalf("managed Task delete did not remove task_info: count=%d err=%v", infoCount, err)
	}
	logCount, err := db.Model("task_log").Ctx(ctx).Where("taskId", id).Count()
	if err != nil || logCount != 0 {
		t.Fatalf("managed Task delete did not remove task_log: count=%d err=%v", logCount, err)
	}
	if err = manager.Restore(tenantCtx, archiveID); err != nil {
		t.Fatalf("restore Task archive failed: %v", err)
	}
	if _, exists, err = store.Find(tenantCtx, id); err != nil || !exists {
		t.Fatalf("restored Task is missing: exists=%v err=%v", exists, err)
	}
	logs, err := store.LogPage(tenantCtx, id, nil, 1, 20)
	if err != nil || logs["pagination"].(map[string]interface{})["total"].(int) != 1 {
		t.Fatalf("restored Task log is missing: page=%#v err=%v", logs, err)
	}

	archiveFailure := errors.New("archive unavailable")
	archiveDataCountBefore, err := db.Model("recycle_data").Ctx(ctx).Where("tenantId", tenantID).Count()
	if err != nil {
		t.Fatalf("count Task archives before failure failed: %v", err)
	}
	archiveItemCountBefore, err := db.Model("recycle_item").Ctx(ctx).Where("tenantId", tenantID).Count()
	if err != nil {
		t.Fatalf("count Task archive items before failure failed: %v", err)
	}
	archiveWritesObserved := atomic.Bool{}
	archiveFailureEngine := &taskRecycleInfoEngine{}
	archiveFailureManager, err := recycle.NewManager(db, &taskArchiveStore{
		Store: archiveStore,
		afterSave: func(saveCtx context.Context, tx gdb.TX, archive *recycle.Archive) error {
			dataCount, countErr := tx.Model("recycle_data").Ctx(saveCtx).Where("id", archive.ID).Count()
			if countErr != nil {
				return countErr
			}
			itemCount, countErr := tx.Model("recycle_item").Ctx(saveCtx).Where("recycleId", archive.ID).Count()
			if countErr != nil {
				return countErr
			}
			if dataCount != 1 || itemCount != 2 {
				return fmt.Errorf("unexpected Task archive writes before injected failure: data=%d items=%d", dataCount, itemCount)
			}
			archiveWritesObserved.Store(true)
			return nil
		},
		afterSaveErr: archiveFailure,
	}, catalog, recycle.Options{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	archiveFailureService, err := taskService.BuildInfoService(store, registry, archiveFailureEngine, time.UTC, archiveFailureManager)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = archiveFailureService.Delete(tenantCtx, crud.DeleteRequest{IDs: []interface{}{id}}); !errors.Is(err, archiveFailure) {
		t.Fatalf("managed Task delete did not surface archive failure: %v", err)
	}
	if !archiveWritesObserved.Load() {
		t.Fatal("archive failure was injected before recycle_data and recycle_item were written")
	}
	if _, exists, err = store.Find(tenantCtx, id); err != nil || !exists {
		t.Fatalf("archive failure did not roll back task_info: exists=%v err=%v", exists, err)
	}
	logs, err = store.LogPage(tenantCtx, id, nil, 1, 20)
	if err != nil || logs["pagination"].(map[string]interface{})["total"].(int) != 1 {
		t.Fatalf("archive failure did not roll back task_log: page=%#v err=%v", logs, err)
	}
	if archiveFailureEngine.removeCalls.Load() != 0 {
		t.Fatalf("archive failure removed scheduler before rollback: %d", archiveFailureEngine.removeCalls.Load())
	}
	archiveDataCountAfter, err := db.Model("recycle_data").Ctx(ctx).Where("tenantId", tenantID).Count()
	if err != nil || archiveDataCountAfter != archiveDataCountBefore {
		t.Fatalf("archive failure did not roll back recycle_data: before=%d after=%d err=%v", archiveDataCountBefore, archiveDataCountAfter, err)
	}
	archiveItemCountAfter, err := db.Model("recycle_item").Ctx(ctx).Where("tenantId", tenantID).Count()
	if err != nil || archiveItemCountAfter != archiveItemCountBefore {
		t.Fatalf("archive failure did not roll back recycle_item: before=%d after=%d err=%v", archiveItemCountBefore, archiveItemCountAfter, err)
	}

	createGuardSQL := fmt.Sprintf(
		"CREATE TABLE `%s` (task_id BIGINT UNSIGNED NOT NULL PRIMARY KEY, CONSTRAINT `%s` FOREIGN KEY (task_id) REFERENCES task_info(id)) ENGINE=InnoDB",
		guardTableName,
		guardConstraintName,
	)
	if _, err = db.Exec(ctx, createGuardSQL); err != nil {
		t.Fatalf("create Task delete failure guard failed: %v", err)
	}
	insertGuardSQL := fmt.Sprintf("INSERT INTO `%s` (task_id) VALUES (?)", guardTableName)
	if _, err = db.Exec(ctx, insertGuardSQL, id); err != nil {
		t.Fatalf("insert Task delete failure guard failed: %v", err)
	}
	deleteFailureEngine := &taskRecycleInfoEngine{}
	deleteFailureService, err := taskService.BuildInfoService(store, registry, deleteFailureEngine, time.UTC, manager)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = deleteFailureService.Delete(tenantCtx, crud.DeleteRequest{IDs: []interface{}{id}}); err == nil {
		t.Fatal("managed Task delete unexpectedly ignored physical delete failure")
	}
	if _, exists, err = store.Find(tenantCtx, id); err != nil || !exists {
		t.Fatalf("physical delete failure did not retain task_info: exists=%v err=%v", exists, err)
	}
	logs, err = store.LogPage(tenantCtx, id, nil, 1, 20)
	if err != nil || logs["pagination"].(map[string]interface{})["total"].(int) != 1 {
		t.Fatalf("physical delete failure did not roll back task_log: page=%#v err=%v", logs, err)
	}
	if deleteFailureEngine.removeCalls.Load() != 0 {
		t.Fatalf("physical delete failure removed scheduler before rollback: %d", deleteFailureEngine.removeCalls.Load())
	}
}
