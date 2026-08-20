package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// ErrScheduleExpired 表示计划窗口内已无下一次执行时间。
var ErrScheduleExpired = errors.New("任务计划已超过结束时间")

// MessageFactory 按计划触发时间生成一条消息。
type MessageFactory func(scheduledAt time.Time) (Message, error)

// Schedule 描述一个业务无关的周期计划。
type Schedule struct {
	ID        string
	Cron      string
	Every     time.Duration
	Anchor    time.Time
	StartDate *time.Time
	EndDate   *time.Time
	Message   MessageFactory
}

// Scheduler 提供计划注册、单次投递和生命周期管理。
type Scheduler interface {
	Queue
	Start(ctx context.Context) error
	Healthy(ctx context.Context) error
	Upsert(ctx context.Context, schedule Schedule) (time.Time, error)
	Remove(ctx context.Context, scheduleID string) error
	NextRunTime(scheduleID string) (time.Time, bool)
	Stop(ctx context.Context) error
}

// CalculateNextRun 计算指定时间之后的下一次合法计划时间。
func CalculateNextRun(plan Schedule, after time.Time, location *time.Location) (time.Time, error) {
	if location == nil {
		return time.Time{}, fmt.Errorf("任务计划时区不能为空")
	}
	var schedule cron.Schedule
	if plan.Cron != "" && plan.Every > 0 {
		return time.Time{}, fmt.Errorf("Cron 和执行间隔不能同时设置")
	}
	if plan.Cron != "" {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		parsed, err := parser.Parse(plan.Cron)
		if err != nil {
			return time.Time{}, err
		}
		schedule = parsed
	} else if plan.Every > 0 {
		anchor := plan.Anchor
		if anchor.IsZero() {
			anchor = after.In(location)
		}
		schedule = alignedInterval{every: plan.Every, anchor: anchor}
	} else {
		return time.Time{}, fmt.Errorf("任务计划缺少 Cron 或执行间隔")
	}
	next := nextWithinWindow(schedule, after.In(location), plan.StartDate, plan.EndDate)
	if next.IsZero() {
		return time.Time{}, ErrScheduleExpired
	}
	return next, nil
}
