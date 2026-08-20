package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

var renewLeaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
  return 1
end
if redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2]) then
  return 1
end
return 0
`)

const redisBusyRedeliveryRetryDelay = 25 * time.Millisecond

type redisWorker interface {
	Start(handler asynq.Handler) error
	Stop()
	Shutdown()
}

type redisProducer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Upsert(ctx context.Context, plan Schedule) (time.Time, error)
	Remove(ctx context.Context, scheduleID string) error
}

// RedisSchedulerConfig 是 Redis 调度器的完整构造参数。
type RedisSchedulerConfig struct {
	RedisClient              redis.UniversalClient
	Concurrency              int
	MaxRetry                 int
	RetryDelay               time.Duration
	ShutdownTimeout          time.Duration
	DelayedTaskCheckInterval time.Duration
	Location                 *time.Location
	Consumer                 Consumer
	QueueName                string
	TaskType                 string
	LeaseKey                 string
	LeaseTTL                 time.Duration
}

// RedisScheduler 是 Asynq Worker 和 Redis 租约协调调度器。
type RedisScheduler struct {
	mu                 sync.RWMutex
	redisClient        redis.UniversalClient
	client             *asynq.Client
	server             redisWorker
	producer           redisProducer
	consumer           Consumer
	redeliver          func(context.Context, Message, time.Duration) error
	plans              map[string]Schedule
	nextRuns           map[string]time.Time
	location           *time.Location
	maxRetry           int
	queueName          string
	taskType           string
	leaseKey           string
	leaseTTL           time.Duration
	token              string
	coordinatorContext context.Context
	stopCoordinator    context.CancelFunc
	workers            sync.WaitGroup
	stopDone           chan struct{}
	isStarted          bool
	isStopping         bool
	isLeader           bool
}

// NewRedisScheduler 创建 Asynq Worker 和租约协调调度器。
func NewRedisScheduler(config RedisSchedulerConfig) (*RedisScheduler, error) {
	if err := validateRedisSchedulerConfig(config); err != nil {
		return nil, err
	}
	coordinatorContext, stopCoordinator := context.WithCancel(context.Background())
	result := &RedisScheduler{
		redisClient:        config.RedisClient,
		client:             asynq.NewClientFromRedisClient(config.RedisClient),
		consumer:           config.Consumer,
		plans:              map[string]Schedule{},
		nextRuns:           map[string]time.Time{},
		location:           config.Location,
		maxRetry:           config.MaxRetry,
		queueName:          config.QueueName,
		taskType:           config.TaskType,
		leaseKey:           config.LeaseKey,
		leaseTTL:           config.LeaseTTL,
		token:              uuid.NewString(),
		coordinatorContext: coordinatorContext,
		stopCoordinator:    stopCoordinator,
		stopDone:           make(chan struct{}),
	}
	result.redeliver = result.enqueueMessageAfter
	result.server = asynq.NewServerFromRedisClient(config.RedisClient, asynq.Config{
		Concurrency:              config.Concurrency,
		Queues:                   map[string]int{config.QueueName: 1},
		DelayedTaskCheckInterval: config.DelayedTaskCheckInterval,
		RetryDelayFunc: func(attempt int, _ error, _ *asynq.Task) time.Duration {
			return config.RetryDelay * time.Duration(1<<minInt(attempt, 10))
		},
		ShutdownTimeout: config.ShutdownTimeout,
		BaseContext:     context.Background,
	})
	producer, err := NewLocalScheduler(1, config.Location, result.enqueueMessage)
	if err != nil {
		return nil, err
	}
	result.producer = producer
	return result, nil
}

func validateRedisSchedulerConfig(config RedisSchedulerConfig) error {
	if config.RedisClient == nil || config.Concurrency <= 0 || config.MaxRetry < 0 ||
		config.RetryDelay <= 0 || config.ShutdownTimeout <= 0 || config.DelayedTaskCheckInterval < 0 || config.Location == nil ||
		config.Consumer == nil || config.LeaseTTL <= 0 {
		return fmt.Errorf("Redis Task Scheduler 配置无效")
	}
	if strings.TrimSpace(config.QueueName) == "" || strings.TrimSpace(config.TaskType) == "" || strings.TrimSpace(config.LeaseKey) == "" {
		return fmt.Errorf("Redis Task Scheduler 命名空间配置无效")
	}
	return nil
}

// Start 验证 Redis、启动 Worker 并竞争协调者租约。
func (s *RedisScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isStarted {
		return nil
	}
	if s.isStopping {
		return fmt.Errorf("Redis Task Scheduler 正在停止")
	}
	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis Task Scheduler 连接失败: %w", err)
	}
	mux := asynq.NewServeMux()
	mux.HandleFunc(s.taskType, s.processTask)
	if err := s.server.Start(mux); err != nil {
		return fmt.Errorf("启动 Asynq Worker 失败: %w", err)
	}
	if err := s.producer.Start(ctx); err != nil {
		s.server.Shutdown()
		return err
	}
	s.isStarted = true
	s.workers.Add(1)
	go s.coordinate()
	return nil
}

// Healthy 检查 Redis 调度后端能否接收写操作。
func (s *RedisScheduler) Healthy(ctx context.Context) error {
	s.mu.RLock()
	isStopping := s.isStopping
	isStarted := s.isStarted
	s.mu.RUnlock()
	if isStopping {
		return fmt.Errorf("Redis Task Scheduler 正在停止")
	}
	if !isStarted {
		return fmt.Errorf("Redis Task Scheduler 尚未启动")
	}
	return s.redisClient.Ping(ctx).Err()
}

// Upsert 保存期望计划，并由当前租约持有者产生消息。
func (s *RedisScheduler) Upsert(ctx context.Context, plan Schedule) (time.Time, error) {
	if err := s.Healthy(ctx); err != nil {
		return time.Time{}, err
	}
	if plan.ID == "" || plan.Message == nil {
		return time.Time{}, fmt.Errorf("任务计划缺少 ID 或消息工厂")
	}
	next, err := calculateNextRun(plan, s.location)
	if err != nil {
		return time.Time{}, err
	}
	s.mu.Lock()
	s.plans[plan.ID] = plan
	s.nextRuns[plan.ID] = next
	isLeader := s.isLeader
	s.mu.Unlock()
	if isLeader {
		producerNext, producerErr := s.producer.Upsert(ctx, plan)
		if producerErr != nil {
			return time.Time{}, producerErr
		}
		s.mu.Lock()
		s.nextRuns[plan.ID] = producerNext
		s.mu.Unlock()
		return producerNext, nil
	}
	return next, nil
}

// Remove 删除期望计划和协调者本地计划。
func (s *RedisScheduler) Remove(ctx context.Context, scheduleID string) error {
	s.mu.Lock()
	delete(s.plans, scheduleID)
	delete(s.nextRuns, scheduleID)
	isLeader := s.isLeader
	s.mu.Unlock()
	if isLeader {
		return s.producer.Remove(ctx, scheduleID)
	}
	return nil
}

// Enqueue 立即提交一次消息。
func (s *RedisScheduler) Enqueue(ctx context.Context, message Message) error {
	if err := s.Healthy(ctx); err != nil {
		return err
	}
	return s.enqueueMessage(ctx, message)
}

// NextRunTime 返回已知计划的下次执行时间。
func (s *RedisScheduler) NextRunTime(scheduleID string) (time.Time, bool) {
	s.mu.RLock()
	next, ok := s.nextRuns[scheduleID]
	s.mu.RUnlock()
	return next, ok
}

// Stop 停止协调者、Worker 和生产器，不关闭外部 Redis 客户端。
func (s *RedisScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.isStopping {
		stopDone := s.stopDone
		s.mu.Unlock()
		select {
		case <-stopDone:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.isStopping = true
	s.mu.Unlock()
	defer close(s.stopDone)

	s.server.Stop()
	s.stopCoordinator()
	s.workers.Wait()
	producerErr := s.producer.Stop(ctx)
	s.server.Shutdown()
	return producerErr
}

func (s *RedisScheduler) processTask(ctx context.Context, asynqTask *asynq.Task) error {
	messageID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	err := s.consumer(ctx, Message{
		ID: messageID, Payload: append([]byte(nil), asynqTask.Payload()...),
		RetryCount: retryCount, IsRetryManaged: true,
	})
	if message, delay, isBusy := BusyRedelivery(err); isBusy {
		return s.persistBusyRedelivery(ctx, err, message, delay)
	}
	if IsSkipRetry(err) {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}
	return err
}

func (s *RedisScheduler) persistBusyRedelivery(ctx context.Context, busyErr error, message Message, delay time.Duration) error {
	if err := validateRedisMessage(message); err != nil {
		return fmt.Errorf("%w: %v", busyErr, err)
	}
	if s.redeliver == nil {
		return fmt.Errorf("%w: Redis Task Scheduler 缺少重投函数", busyErr)
	}
	for {
		if err := ctx.Err(); err != nil {
			return busyErr
		}
		redeliveryErr := s.redeliver(ctx, message, delay)
		if redeliveryErr == nil {
			return nil
		}
		timer := time.NewTimer(redisBusyRedeliveryRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: Redis busy 重投持久化失败: %v", busyErr, redeliveryErr)
		case <-timer.C:
		}
	}
}

func (s *RedisScheduler) enqueueMessage(ctx context.Context, message Message) error {
	return s.enqueueMessageAfter(ctx, message, 0)
}

func (s *RedisScheduler) enqueueMessageAfter(ctx context.Context, message Message, delay time.Duration) error {
	if err := validateRedisMessage(message); err != nil {
		return err
	}
	maxRetry := s.maxRetry - message.RetryBase
	if maxRetry < 0 {
		maxRetry = 0
	}
	asynqTask := asynq.NewTask(s.taskType, append([]byte(nil), message.Payload...))
	options := []asynq.Option{asynq.Queue(s.queueName), asynq.MaxRetry(maxRetry), asynq.TaskID(message.ID)}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err := s.client.EnqueueContext(ctx, asynqTask, options...)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func validateRedisMessage(message Message) error {
	if strings.TrimSpace(message.ID) == "" {
		return fmt.Errorf("队列消息 ID 不能为空")
	}
	if message.RetryBase < 0 {
		return fmt.Errorf("队列消息重试基数无效")
	}
	return nil
}

func (s *RedisScheduler) coordinate() {
	defer s.workers.Done()
	renewInterval := s.leaseTTL / 3
	if renewInterval <= 0 {
		renewInterval = s.leaseTTL
	}
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	s.renewLeadership()
	for {
		select {
		case <-s.coordinatorContext.Done():
			s.pauseProducer()
			return
		case <-ticker.C:
			s.renewLeadership()
		}
	}
}

func (s *RedisScheduler) renewLeadership() {
	result, err := renewLeaseScript.Run(
		s.coordinatorContext, s.redisClient, []string{s.leaseKey}, s.token, s.leaseTTL.Milliseconds(),
	).Int()
	if err != nil || result != 1 {
		s.pauseProducer()
		return
	}
	s.mu.Lock()
	wasLeader := s.isLeader
	s.isLeader = true
	plans := make([]Schedule, 0, len(s.plans))
	for _, plan := range s.plans {
		plans = append(plans, plan)
	}
	s.mu.Unlock()
	if wasLeader {
		return
	}
	for _, plan := range plans {
		if next, upsertErr := s.producer.Upsert(s.coordinatorContext, plan); upsertErr == nil {
			s.mu.Lock()
			s.nextRuns[plan.ID] = next
			s.mu.Unlock()
		}
	}
}

func (s *RedisScheduler) pauseProducer() {
	s.mu.Lock()
	if !s.isLeader {
		s.mu.Unlock()
		return
	}
	s.isLeader = false
	ids := make([]string, 0, len(s.plans))
	for id := range s.plans {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.producer.Remove(context.Background(), id)
	}
}

func calculateNextRun(plan Schedule, location *time.Location) (time.Time, error) {
	return CalculateNextRun(plan, time.Now().In(location), location)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
