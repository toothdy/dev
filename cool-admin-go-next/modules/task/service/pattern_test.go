package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gcron"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
	"github.com/toothdy/cool-admin-go-next/modules/task/service"
)

func pointer[T any](value T) *T { return &value }

func TestCompileScheduleProducesGcronPattern(t *testing.T) {
	cases := []struct {
		name     string
		taskType int32
		cron     *string
		every    *int64
		pattern  string
	}{
		{name: "六段 cron 原样保留", taskType: entity.TaskTypeCron, cron: pointer("0/5 * * * * *"), pattern: "0/5 * * * * *"},
		{name: "五段 cron 补秒位", taskType: entity.TaskTypeCron, cron: pointer("*/5 * * * *"), pattern: "0 */5 * * * *"},
		{name: "多余空白归一", taskType: entity.TaskTypeCron, cron: pointer("  0/5   * * * * *  "), pattern: "0/5 * * * * *"},
		{name: "间隔任务用 @every", taskType: entity.TaskTypeInterval, every: pointer(int64(1000)), pattern: "@every 1s"},
		{name: "超过一分钟的间隔", taskType: entity.TaskTypeInterval, every: pointer(int64(90000)), pattern: "@every 90s"},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			schedule, err := service.CompileSchedule(current.taskType, current.cron, current.every)
			if err != nil {
				t.Fatalf("编译定时规则失败: %v", err)
			}
			if schedule.Pattern() != current.pattern {
				t.Errorf("模式 = %q，期望 %q", schedule.Pattern(), current.pattern)
			}
		})
	}
}

// 编译产物必须能被 gcron 接受，否则注册期才会暴露非法模式
func TestCompiledPatternIsAcceptedByGcron(t *testing.T) {
	for _, pattern := range []string{"0/5 * * * * *", "0 */5 * * * *", "@every 1s", "@every 90s"} {
		cron := gcron.New()
		if _, err := cron.Add(t.Context(), pattern, func(context.Context) {}, pattern); err != nil {
			t.Errorf("gcron 拒绝模式 %q: %v", pattern, err)
		}
		cron.Close()
	}
}

func TestCompileScheduleRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		taskType int32
		cron     *string
		every    *int64
	}{
		{name: "间隔缺失", taskType: entity.TaskTypeInterval},
		{name: "间隔小于一秒", taskType: entity.TaskTypeInterval, every: pointer(int64(500))},
		{name: "cron 缺失", taskType: entity.TaskTypeCron},
		{name: "cron 空白", taskType: entity.TaskTypeCron, cron: pointer("   ")},
		{name: "cron 段数不对", taskType: entity.TaskTypeCron, cron: pointer("* * *")},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			if _, err := service.CompileSchedule(current.taskType, current.cron, current.every); err == nil {
				t.Error("期望校验失败")
			}
		})
	}
}

func TestScheduleNext(t *testing.T) {
	base := time.Date(2026, 8, 25, 10, 30, 12, 0, time.UTC)
	cases := []struct {
		name     string
		taskType int32
		cron     *string
		every    *int64
		expected time.Time
		exists   bool
	}{
		{name: "间隔任务", taskType: entity.TaskTypeInterval, every: pointer(int64(90000)),
			expected: base.Add(90 * time.Second), exists: true},
		{name: "秒级步进", taskType: entity.TaskTypeCron, cron: pointer("0/5 * * * * *"),
			expected: time.Date(2026, 8, 25, 10, 30, 15, 0, time.UTC), exists: true},
		{name: "分钟步进", taskType: entity.TaskTypeCron, cron: pointer("0 */15 * * * *"),
			expected: time.Date(2026, 8, 25, 10, 45, 0, 0, time.UTC), exists: true},
		{name: "跨天", taskType: entity.TaskTypeCron, cron: pointer("0 0 3 * * *"),
			expected: time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC), exists: true},
		{name: "跨月并受星期约束", taskType: entity.TaskTypeCron, cron: pointer("0 0 0 1 * 2"),
			expected: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), exists: true},
		{name: "英文星期名不计算", taskType: entity.TaskTypeCron, cron: pointer("0 0 0 * * mon")},
		{name: "永不命中", taskType: entity.TaskTypeCron, cron: pointer("0 0 0 30 2 *")},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			schedule, err := service.CompileSchedule(current.taskType, current.cron, current.every)
			if err != nil {
				t.Fatalf("编译定时规则失败: %v", err)
			}
			next, exists := schedule.Next(base)
			if exists != current.exists {
				t.Fatalf("命中 = %v，期望 %v", exists, current.exists)
			}
			if exists && !next.Equal(current.expected) {
				t.Errorf("下次执行 = %s，期望 %s", next, current.expected)
			}
		})
	}
}
