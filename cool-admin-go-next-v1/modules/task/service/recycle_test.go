package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

type recycleInfoEngine struct {
	removed  []int64
	removeAt int64
}

func (e *recycleInfoEngine) Healthy(context.Context) error {
	return nil
}

func (e *recycleInfoEngine) SyncTask(context.Context, int64) error {
	return nil
}

func (e *recycleInfoEngine) RemoveTask(_ context.Context, taskID int64) error {
	e.removed = append(e.removed, taskID)
	if taskID == e.removeAt {
		return errors.New("scheduler unavailable")
	}
	return nil
}

func (e *recycleInfoEngine) Once(context.Context, TaskInfo) error {
	return nil
}

func TestTaskDeleteIDsDeduplicatesStableInput(t *testing.T) {
	ids, requestIDs, err := taskDeleteIDs([]interface{}{json.Number("2"), int64(1), "2"})
	if err != nil {
		t.Fatalf("校验任务删除 ID 失败: %v", err)
	}
	if !reflect.DeepEqual(ids, []int64{2, 1}) || !reflect.DeepEqual(requestIDs, []interface{}{int64(2), int64(1)}) {
		t.Fatalf("任务删除 ID 去重结果异常: ids=%#v requestIDs=%#v", ids, requestIDs)
	}
	if _, _, err = taskDeleteIDs(nil); err == nil {
		t.Fatal("空任务删除 ID 应被拒绝")
	}
}

func TestRemoveTasksAfterCommitContinuesAfterSchedulerFailure(t *testing.T) {
	engine := &recycleInfoEngine{removeAt: 2}
	if err := removeTasksAfterCommit(engine, []int64{1, 2, 3})(context.Background()); err != nil {
		t.Fatalf("提交后调度移除不应返回运行态错误: %v", err)
	}
	if !reflect.DeepEqual(engine.removed, []int64{1, 2, 3}) {
		t.Fatalf("调度移除未继续处理独立任务: %#v", engine.removed)
	}
}

func TestCleanupLogsContextUsesTenantAndRecycleBypass(t *testing.T) {
	tenantContext, err := tenant.ForTenant(context.Background(), 91)
	if err != nil {
		t.Fatal(err)
	}
	maintenanceContext := cleanupLogsContext(tenantContext)
	if tenant.Resolve(maintenanceContext).Kind() != tenant.KindBypass {
		t.Fatal("任务日志清理上下文未启用跨租户维护作用域")
	}
	if !recycle.IsBypass(maintenanceContext) {
		t.Fatal("任务日志清理上下文未启用 Recycle bypass")
	}
}
