package service

import (
	"testing"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
)

func newTaskMutable(t *testing.T, fields ...coreservice.FieldValue) *coreservice.Mutable[entity.Info] {
	t.Helper()

	descriptor, err := coreentity.Compile[entity.Info, uint64](entity.InfoSchema())
	if err != nil {
		t.Fatal(err)
	}
	value, err := coreservice.NewMutable[entity.Info, uint64](descriptor, fields)
	if err != nil {
		t.Fatal(err)
	}

	return value
}

// cron 与间隔互斥，未生效的一侧必须显式写 null，否则切换类型后旧配置会继续参与调度
func TestPrepareScheduleClearsUnusedFields(t *testing.T) {
	t.Run("cron 任务清空间隔与次数", func(t *testing.T) {
		value := newTaskMutable(t,
			coreservice.Value("taskType", entity.TaskTypeCron),
			coreservice.Value("cron", "0/5 * * * * *"),
			coreservice.Value("every", int64(1000)),
			coreservice.Value("limit", int64(3)),
		)
		if err := prepareSchedule(value); err != nil {
			t.Fatalf("归一化定时字段失败: %v", err)
		}
		if !value.IsNull("every") || !value.IsNull("limit") {
			t.Error("cron 任务必须清空 every 与 limit")
		}
		if got, _ := value.Get("cron"); got != "0/5 * * * * *" {
			t.Errorf("cron = %v，期望保留", got)
		}
	})

	t.Run("间隔任务清空 cron", func(t *testing.T) {
		value := newTaskMutable(t,
			coreservice.Value("taskType", entity.TaskTypeInterval),
			coreservice.Value("cron", "0/5 * * * * *"),
			coreservice.Value("every", int64(1000)),
		)
		if err := prepareSchedule(value); err != nil {
			t.Fatalf("归一化定时字段失败: %v", err)
		}
		if !value.IsNull("cron") {
			t.Error("间隔任务必须清空 cron")
		}
		if got, _ := value.Get("every"); got != int64(1000) {
			t.Errorf("every = %v，期望保留", got)
		}
	})

	t.Run("未提交类型时不改写", func(t *testing.T) {
		value := newTaskMutable(t, coreservice.Value("name", "只改名字"))
		if err := prepareSchedule(value); err != nil {
			t.Fatalf("归一化定时字段失败: %v", err)
		}
		if value.Has("cron") || value.Has("every") || value.Has("limit") {
			t.Error("未提交 taskType 时不应触碰定时字段")
		}
	})
}
