package event

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/redis/go-redis/v9"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	taskModule "github.com/toothdy/cool-admin-go-next/modules/task"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
	taskService "github.com/toothdy/cool-admin-go-next/modules/task/service"
)

// Comm 负责 Task 模块运行依赖的唯一组装和生命周期。
type Comm struct {
	engine          *taskService.TaskEngine
	info            *taskService.InfoService
	redisClient     redis.UniversalClient
	shutdownTimeout time.Duration
}

// NewComm 构建 Task 模块运行时，启动动作延后到 Schema 和 Seed 完成后。
func NewComm(
	db gdb.DB,
	taskInfoModel entity.Definition,
	taskLogModel entity.Definition,
	handlers []task.HandlerDefinition,
	config taskModule.Config,
	location *time.Location,
	redisClient taskQueue.RedisClient,
	recycleManager *recycle.Manager,
) (*Comm, error) {
	isBuilt := false
	defer func() {
		if !isBuilt && redisClient.Client != nil {
			_ = redisClient.Client.Close()
		}
	}()
	queueMaxRetry := config.Queue.MaxRetry
	for _, definition := range handlers {
		if definition.Timeout > 0 && config.Execution.LockTTL-definition.Timeout < time.Second {
			return nil, gerror.Newf("任务处理器 %s 的执行租约必须至少比超时长 1 秒", definition.Name)
		}
		if definition.HasMaxRetry && definition.MaxRetry > queueMaxRetry {
			queueMaxRetry = definition.MaxRetry
		}
	}
	store, err := taskService.BuildStore(db, taskInfoModel, taskLogModel)
	if err != nil {
		return nil, err
	}
	builder := task.NewRegistryBuilder()
	for _, definition := range handlers {
		if err = builder.Register(definition); err != nil {
			return nil, err
		}
	}
	registry, err := builder.Freeze()
	if err != nil {
		return nil, err
	}
	executor, err := taskService.BuildExecutor(store, registry, taskService.ExecutorConfig{
		Timeout: config.Execution.Timeout, LockTTL: config.Execution.LockTTL,
		MaxRetry: config.Queue.MaxRetry, RetryDelay: config.Queue.RetryDelay,
	})
	if err != nil {
		return nil, err
	}
	var engine *taskService.TaskEngine
	consumer, err := taskQueue.BuildConsumer(func(ctx context.Context, payload taskQueue.Payload) error {
		if engine == nil {
			return gerror.New("Task Engine 尚未完成初始化")
		}
		return engine.Dispatch(ctx, payload)
	})
	if err != nil {
		return nil, err
	}
	scheduler, backend, err := buildScheduler(config, location, redisClient, consumer, queueMaxRetry)
	if err != nil {
		return nil, err
	}
	engine, err = taskService.BuildEngine(store, registry, scheduler, executor, location, backend, config.Log.KeepDays)
	if err != nil {
		return nil, err
	}
	info, err := taskService.BuildInfoService(store, registry, engine, location, recycleManager)
	if err != nil {
		return nil, err
	}
	if recycleManager != nil {
		if err = recycleManager.RegisterRestoreHook(taskInfoModel.ResourceKey(), taskRestoreHook(engine)); err != nil {
			return nil, err
		}
	}
	isBuilt = true
	return &Comm{
		engine: engine, info: info, redisClient: redisClient.Client,
		shutdownTimeout: config.Queue.ShutdownTimeout,
	}, nil
}

// taskRestoreHook 创建任务恢复后的调度对账动作。
func taskRestoreHook(engine interface {
	Reconcile(context.Context) error
}) recycle.RestoreHook {
	return func(ctx context.Context, _ string) error {
		return engine.Reconcile(ctx)
	}
}

// Info 返回 Controller 共享的 TaskInfo Service。
func (c *Comm) Info() *taskService.InfoService {
	return c.info
}

// Healthy 检查任务调度后端能否接收写操作。
func (c *Comm) Healthy(ctx context.Context) error {
	return c.engine.Healthy(ctx)
}

// Start 启动 Scheduler 并执行初始对账。
func (c *Comm) Start(ctx context.Context) error {
	return c.engine.Start(ctx)
}

// Stop 停止 Engine、Scheduler 和模块自管 Redis 客户端。
func (c *Comm) Stop(ctx context.Context) error {
	stopContext, cancel := context.WithTimeout(ctx, c.shutdownTimeout)
	defer cancel()
	engineErr := c.engine.Stop(stopContext)
	var redisErr error
	if c.redisClient != nil {
		redisErr = c.redisClient.Close()
	}
	if engineErr != nil {
		return engineErr
	}
	return redisErr
}

func buildScheduler(config taskModule.Config, location *time.Location, redisClient taskQueue.RedisClient, consumer task.Consumer, queueMaxRetry int) (task.Scheduler, string, error) {
	localScheduler, err := taskService.BuildLocalScheduler(taskService.LocalOptions{
		Concurrency: config.Queue.Concurrency, Location: location, Consumer: consumer,
	})
	if err != nil {
		return nil, "", err
	}
	if config.Mode == taskModule.ModeLocal {
		return localScheduler, "local", nil
	}
	if !redisClient.Configured || redisClient.Client == nil {
		if config.Mode == taskModule.ModeRedis {
			return nil, "", gerror.New("module.task.mode=redis 需要 redis.default 配置")
		}
		return localScheduler, "local", nil
	}
	redisScheduler, err := taskService.BuildRedisScheduler(taskService.RedisOptions{
		Client: redisClient.Client, Concurrency: config.Queue.Concurrency, MaxRetry: queueMaxRetry,
		RetryDelay: config.Queue.RetryDelay, ShutdownTimeout: config.Queue.ShutdownTimeout,
		Location: location, Consumer: consumer,
	})
	if err != nil {
		return nil, "", err
	}
	if config.Mode == taskModule.ModeRedis {
		return redisScheduler, "redis", nil
	}
	autoScheduler, err := task.NewAutoScheduler(redisScheduler, localScheduler)
	if err != nil {
		return nil, "", err
	}
	return autoScheduler, "auto", nil
}
