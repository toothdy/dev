package event

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/robfig/cron/v3"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

const (
	droppedLogReportInterval    = time.Minute
	operationLogCleanupSchedule = "0 0 0 * * *"
)

// 操作日志存储端口
type LogStore interface {
	Record(ctx context.Context, request baseSysService.LogRecordRequest) error
	ClearExpired(ctx context.Context) (int64, error)
}

// 异步操作日志配置
type LogOptions struct {
	Enabled         bool
	QueueSize       int
	ShutdownTimeout time.Duration
	WriteTimeout    time.Duration
	CleanupTimeout  time.Duration
}

type queuedLog struct {
	request   baseSysService.LogRecordRequest
	tenantID  int64
	hasTenant bool
}

// 异步操作日志运行时
type Log struct {
	store           LogStore
	enabled         bool
	queue           chan queuedLog
	shutdownTimeout time.Duration
	writeTimeout    time.Duration
	cleanupTimeout  time.Duration

	mutex          sync.RWMutex
	started        bool
	accepting      bool
	done           chan struct{}
	workerCancel   context.CancelFunc
	cleanupCancel  context.CancelFunc
	scheduler      *cron.Cron
	dropped        atomic.Uint64
	cleanupRunning atomic.Bool
}

// NewLog 使用 Base 模块配置创建操作日志运行时。
func NewLog(logService *baseSysService.LogService, config baseModule.Config) (*Log, error) {
	return BuildLog(logService, LogOptions{
		Enabled:         config.Middleware.Log.Enable,
		QueueSize:       config.Middleware.Log.QueueSize,
		ShutdownTimeout: config.Middleware.Log.ShutdownTimeout,
		WriteTimeout:    config.Middleware.Log.WriteTimeout,
		CleanupTimeout:  config.Middleware.Log.CleanupTimeout,
	})
}

/**
 * 创建异步操作日志运行时
 * @param store 操作日志存储端口
 * @param options 运行参数
 * @returns 操作日志运行时和校验错误
 */
func BuildLog(store LogStore, options LogOptions) (*Log, error) {
	if options.QueueSize <= 0 {
		return nil, gerror.New("操作日志队列容量必须大于 0")
	}
	if options.ShutdownTimeout <= 0 {
		return nil, gerror.New("操作日志停机超时必须大于 0")
	}
	if options.WriteTimeout <= 0 {
		return nil, gerror.New("操作日志写入超时必须大于 0")
	}
	if options.CleanupTimeout <= 0 {
		return nil, gerror.New("操作日志清理超时必须大于 0")
	}
	if options.Enabled && store == nil {
		return nil, gerror.New("操作日志存储端口不能为空")
	}
	return &Log{
		store: store, enabled: options.Enabled, queue: make(chan queuedLog, options.QueueSize),
		shutdownTimeout: options.ShutdownTimeout, writeTimeout: options.WriteTimeout,
		cleanupTimeout: options.CleanupTimeout,
	}, nil
}

/**
 * 启动操作日志 worker
 * @param ctx 应用上下文
 * @returns error
 */
func (l *Log) Start(ctx context.Context) error {
	if !l.enabled && l.store == nil {
		return nil
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.started {
		return nil
	}
	runtimeContext := context.WithoutCancel(ctx)
	cleanupContext, cleanupCancel := context.WithCancel(runtimeContext)
	scheduler := cron.New(cron.WithSeconds(), cron.WithLocation(time.Local))
	if _, err := scheduler.AddFunc(operationLogCleanupSchedule, func() {
		l.clearExpired(cleanupContext)
	}); err != nil {
		cleanupCancel()
		return gerror.Wrap(err, "注册操作日志清理任务失败")
	}
	l.started = true
	l.cleanupCancel = cleanupCancel
	l.scheduler = scheduler
	if l.enabled {
		workerContext, workerCancel := context.WithCancel(runtimeContext)
		l.accepting = true
		l.done = make(chan struct{})
		l.workerCancel = workerCancel
		go l.run(workerContext, l.done)
	}
	scheduler.Start()
	return nil
}

/**
 * 停止接收日志并等待队列刷盘
 * @param ctx 停机上下文
 * @returns error
 */
func (l *Log) Stop(ctx context.Context) error {
	l.mutex.Lock()
	if !l.started {
		l.mutex.Unlock()
		return nil
	}
	if l.accepting {
		l.accepting = false
		close(l.queue)
	}
	done := l.done
	cancelWorker := l.workerCancel
	cancelCleanup := l.cleanupCancel
	scheduler := l.scheduler
	l.mutex.Unlock()

	if cancelCleanup != nil {
		cancelCleanup()
	}
	var cleanupDone <-chan struct{}
	if scheduler != nil {
		cleanupDone = scheduler.Stop().Done()
	}
	stopContext, cancel := context.WithTimeout(ctx, l.shutdownTimeout)
	defer cancel()
	if err := waitLogRuntime(stopContext, done); err != nil {
		if cancelWorker != nil {
			cancelWorker()
		}
		return gerror.Wrap(err, "操作日志队列刷盘超时")
	}
	if cancelWorker != nil {
		cancelWorker()
	}
	if err := waitLogRuntime(stopContext, cleanupDone); err != nil {
		return gerror.Wrap(err, "停止操作日志清理任务超时")
	}
	return nil
}

/**
 * 非阻塞提交操作日志
 * @param ctx 请求上下文
 * @param request 操作日志
 * @returns 是否成功入队
 */
func (l *Log) Submit(ctx context.Context, request baseSysService.LogRecordRequest) bool {
	if !l.enabled {
		return false
	}
	entry := queuedLog{request: cloneLogRequest(request)}
	if tenantID, ok := tenant.Resolve(ctx).TenantID(); ok {
		entry.tenantID = tenantID
		entry.hasTenant = true
	}

	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.accepting {
		return false
	}
	select {
	case l.queue <- entry:
		return true
	default:
		l.dropped.Add(1)
		return false
	}
}

/**
 * 获取累计丢弃日志数
 * @returns uint64
 */
func (l *Log) Dropped() uint64 {
	return l.dropped.Load()
}

func (l *Log) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(droppedLogReportInterval)
	defer ticker.Stop()
	var reportedDropped uint64
	for {
		select {
		case entry, ok := <-l.queue:
			if !ok {
				l.reportDropped(ctx, &reportedDropped)
				return
			}
			l.write(ctx, entry)
		case <-ticker.C:
			l.reportDropped(ctx, &reportedDropped)
		case <-ctx.Done():
			l.reportDropped(context.Background(), &reportedDropped)
			return
		}
	}
}

func (l *Log) write(ctx context.Context, entry queuedLog) {
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Errorf(ctx, "异步写入操作日志发生 panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	recordContext := tenant.WithoutTenant(ctx)
	if entry.hasTenant {
		var err error
		recordContext, err = tenant.ForTenant(ctx, entry.tenantID)
		if err != nil {
			g.Log().Error(ctx, gerror.Wrap(err, "重建操作日志租户作用域失败"))
			return
		}
	}
	recordContext, cancel := context.WithTimeout(recordContext, l.writeTimeout)
	defer cancel()
	if err := l.store.Record(recordContext, entry.request); err != nil {
		g.Log().Error(ctx, gerror.Wrap(err, "异步写入操作日志失败"))
	}
}

func (l *Log) clearExpired(ctx context.Context) {
	if l.store == nil {
		return
	}
	if !l.cleanupRunning.CompareAndSwap(false, true) {
		g.Log().Warning(ctx, "上一轮操作日志定时清理仍在执行，跳过本轮")
		return
	}
	defer l.cleanupRunning.Store(false)
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Errorf(ctx, "操作日志定时清理发生 panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	cleanupContext, cancel := context.WithTimeout(ctx, l.cleanupTimeout)
	defer cancel()
	started := time.Now()
	g.Log().Info(cleanupContext, "操作日志定时清理开始执行")
	count, err := l.store.ClearExpired(cleanupContext)
	if err != nil {
		if ctx.Err() == nil {
			message := "操作日志定时清理失败"
			if cleanupContext.Err() != nil {
				message = "操作日志定时清理超时"
			}
			g.Log().Error(ctx, gerror.Wrap(err, message))
		}
		return
	}
	if err = cleanupContext.Err(); err != nil {
		if ctx.Err() == nil {
			g.Log().Error(ctx, gerror.Wrap(err, "操作日志定时清理超时"))
		}
		return
	}
	g.Log().Infof(cleanupContext, "操作日志定时清理完成: count=%d duration=%s", count, time.Since(started))
}

func waitLogRuntime(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Log) reportDropped(ctx context.Context, reported *uint64) {
	total := l.dropped.Load()
	if total <= *reported {
		return
	}
	g.Log().Warningf(ctx, "操作日志队列已丢弃 %d 条日志", total-*reported)
	*reported = total
}

func cloneLogRequest(request baseSysService.LogRecordRequest) baseSysService.LogRecordRequest {
	cloned := request
	if request.UserID != nil {
		userID := *request.UserID
		cloned.UserID = &userID
	}
	return cloned
}
