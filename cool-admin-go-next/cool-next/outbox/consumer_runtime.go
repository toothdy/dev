package outbox

import (
	"context"
	"errors"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
)

// 启动探测边界
type ConsumerRuntimeStore interface {
	Probe(context.Context) error
}

// 可靠消费生命周期组件
type ConsumerRuntime struct {
	adapter   ConsumerAdapter
	deliverer *Deliverer
	store     ConsumerRuntimeStore

	mutex      sync.Mutex
	isPrepared bool
	isRunning  bool
	terminated <-chan error
}

// 创建受应用 Host 管理的可靠消费组件
func NewConsumerRuntime(
	adapter ConsumerAdapter,
	store ConsumerRuntimeStore,
	deliverer *Deliverer,
) (*ConsumerRuntime, error) {
	if adapter == nil || store == nil || deliverer == nil {
		return nil, gerror.New("outbox consumer runtime: Adapter、Store 和 Deliverer 不能为空")
	}
	return &ConsumerRuntime{adapter: adapter, store: store, deliverer: deliverer}, nil
}

// 探测 Inbox Schema、Broker 能力和订阅拓扑
func (runtime *ConsumerRuntime) OnInit(ctx context.Context) error {
	if runtime == nil {
		return gerror.New("outbox consumer runtime: Runtime 不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	if runtime.isPrepared {
		return gerror.New("outbox consumer runtime: Runtime 已初始化")
	}
	if err := runtime.store.Probe(ctx); err != nil {
		return gerror.Wrap(err, "outbox consumer runtime: Inbox Schema 探测失败")
	}
	if err := runtime.adapter.Prepare(ctx, runtime.deliverer.Subscriptions(), runtime.deliverer.Deliver); err != nil {
		return errors.Join(
			gerror.Wrap(err, "outbox consumer runtime: 准备 Consumer Adapter"),
			runtime.adapter.Stop(ctx),
		)
	}
	runtime.isPrepared = true
	return nil
}

// 启动 Broker 消费循环
func (runtime *ConsumerRuntime) OnStart(ctx context.Context) error {
	if runtime == nil {
		return gerror.New("outbox consumer runtime: Runtime 不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	if !runtime.isPrepared {
		return gerror.New("outbox consumer runtime: Runtime 尚未初始化")
	}
	if runtime.isRunning {
		return gerror.New("outbox consumer runtime: Runtime 已启动")
	}
	terminated, err := runtime.adapter.Start(ctx)
	if err != nil {
		return errors.Join(
			gerror.Wrap(err, "outbox consumer runtime: 启动 Consumer Adapter"),
			runtime.adapter.Stop(ctx),
		)
	}
	if terminated == nil {
		return errors.Join(
			gerror.New("outbox consumer runtime: Consumer Adapter 终止通道不能为空"),
			runtime.adapter.Stop(ctx),
		)
	}
	runtime.isRunning = true
	runtime.terminated = terminated
	return nil
}

// 停止拉取并排空在途消费事务
func (runtime *ConsumerRuntime) OnStop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.mutex.Lock()
	if !runtime.isPrepared {
		runtime.mutex.Unlock()
		return nil
	}
	runtime.isPrepared = false
	runtime.isRunning = false
	runtime.mutex.Unlock()
	return runtime.adapter.Stop(ctx)
}

// 返回消费循环终止信号
func (runtime *ConsumerRuntime) Terminated() <-chan error {
	if runtime == nil {
		return nil
	}
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()
	return runtime.terminated
}
