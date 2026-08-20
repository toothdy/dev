package schedule

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/modules/recycle"
)

// Service 定义回收站 Runtime 使用的服务能力。
type Service interface {
	ClearExpired(ctx context.Context) (int, error)
	Restore(ctx context.Context, ids []int64) error
}

// Data 管理回收站定时清理生命周期。
type Data struct {
	service  Service
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
}

// NewData 创建回收站定时清理 Runtime。
func NewData(service Service, config recycle.Config) *Data {
	return &Data{service: service, interval: config.CleanupInterval}
}

// Service 返回 Controller 共享的数据服务。
func (s *Data) Service() Service {
	if s == nil {
		return nil
	}
	return s.service
}

// Start 启动回收站定时清理。
func (s *Data) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.run(runCtx, s.done)
	return nil
}

// Stop 停止回收站定时清理并等待当前任务结束。
func (s *Data) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return gerror.Wrap(ctx.Err(), "停止回收站定时清理失败")
	}
}

func (s *Data) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()
			count, err := s.service.ClearExpired(ctx)
			if err != nil {
				g.Log().Errorf(ctx, "回收站定时清理失败: %v", err)
				continue
			}
			g.Log().Infof(ctx, "回收站定时清理完成: count=%d duration=%s", count, time.Since(start))
		}
	}
}
