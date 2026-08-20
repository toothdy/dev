package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// AutoScheduler 是仅在启动期允许降级的调度器。
type AutoScheduler struct {
	mu       sync.RWMutex
	primary  Scheduler
	fallback Scheduler
	selected Scheduler
}

// NewAutoScheduler 创建启动期自动降级调度器。
func NewAutoScheduler(primary Scheduler, fallback Scheduler) (*AutoScheduler, error) {
	if primary == nil || fallback == nil {
		return nil, fmt.Errorf("auto 调度器缺少主后端或降级后端")
	}
	return &AutoScheduler{primary: primary, fallback: fallback}, nil
}

// Start 优先启动主后端，失败时只在此处降级。
func (s *AutoScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected != nil {
		return nil
	}
	if err := s.primary.Start(ctx); err == nil {
		s.selected = s.primary
		return nil
	} else {
		g.Log().Warningf(ctx, "Task 主调度后端启动不可用，auto 模式降级: %v", err)
	}
	_ = s.primary.Stop(context.Background())
	if err := s.fallback.Start(ctx); err != nil {
		return err
	}
	s.selected = s.fallback
	return nil
}

// Healthy 检查已选择的调度后端。
func (s *AutoScheduler) Healthy(ctx context.Context) error {
	selected, err := s.getSelected()
	if err != nil {
		return err
	}
	return selected.Healthy(ctx)
}

// Upsert 在已选择的调度后端注册或替换计划。
func (s *AutoScheduler) Upsert(ctx context.Context, plan Schedule) (time.Time, error) {
	selected, err := s.getSelected()
	if err != nil {
		return time.Time{}, err
	}
	return selected.Upsert(ctx, plan)
}

// Remove 从已选择的调度后端删除计划。
func (s *AutoScheduler) Remove(ctx context.Context, scheduleID string) error {
	selected, err := s.getSelected()
	if err != nil {
		return err
	}
	return selected.Remove(ctx, scheduleID)
}

// Enqueue 向已选择的调度后端提交一次消息。
func (s *AutoScheduler) Enqueue(ctx context.Context, message Message) error {
	selected, err := s.getSelected()
	if err != nil {
		return err
	}
	return selected.Enqueue(ctx, message)
}

// NextRunTime 返回已选择后端中的下次执行时间。
func (s *AutoScheduler) NextRunTime(scheduleID string) (time.Time, bool) {
	selected, err := s.getSelected()
	if err != nil {
		return time.Time{}, false
	}
	return selected.NextRunTime(scheduleID)
}

// Stop 停止已选择的调度后端。
func (s *AutoScheduler) Stop(ctx context.Context) error {
	s.mu.RLock()
	selected := s.selected
	s.mu.RUnlock()
	if selected == nil {
		return nil
	}
	return selected.Stop(ctx)
}

func (s *AutoScheduler) getSelected() (Scheduler, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.selected == nil {
		return nil, fmt.Errorf("auto 调度器尚未启动")
	}
	return s.selected, nil
}
