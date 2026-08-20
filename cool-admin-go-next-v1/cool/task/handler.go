package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Invocation 是传给显式 Task 处理器的执行上下文
type Invocation struct {
	TaskID      int64
	TenantID    *int64
	ScheduledAt time.Time
	Attempt     int
	Data        string
	Arguments   []json.RawMessage
}

// Handler 执行一个已注册的 Task 调用
type Handler func(ctx context.Context, invocation Invocation) (interface{}, error)

// HandlerDefinition 描述一个启动期注册的 Task 处理器
type HandlerDefinition struct {
	Name        string
	Handler     Handler
	Timeout     time.Duration
	MaxRetry    int
	HasMaxRetry bool
}

// RegistryBuilder 在应用构建期收集处理器，并在启动前冻结
type RegistryBuilder struct {
	mu          sync.Mutex
	definitions map[string]HandlerDefinition
	isFrozen    bool
}

// Registry 是不可变的处理器索引
type Registry struct {
	definitions map[string]HandlerDefinition
}

// NewRegistryBuilder 创建处理器注册表构建器
func NewRegistryBuilder() *RegistryBuilder {
	return &RegistryBuilder{definitions: map[string]HandlerDefinition{}}
}

// Register 注册一个显式 Task 处理器
func (b *RegistryBuilder) Register(definition HandlerDefinition) error {
	if b == nil {
		return fmt.Errorf("任务处理器注册表不能为空")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isFrozen {
		return fmt.Errorf("任务处理器注册表已冻结")
	}
	if !isHandlerName(definition.Name) {
		return fmt.Errorf("任务处理器名称 %q 格式错误", definition.Name)
	}
	if definition.Handler == nil {
		return fmt.Errorf("任务处理器 %s 不能为空", definition.Name)
	}
	if definition.Timeout < 0 {
		return fmt.Errorf("任务处理器 %s 超时不能为负数", definition.Name)
	}
	if definition.HasMaxRetry && definition.MaxRetry < 0 {
		return fmt.Errorf("任务处理器 %s 最大重试次数不能为负数", definition.Name)
	}
	if _, exists := b.definitions[definition.Name]; exists {
		return fmt.Errorf("任务处理器 %s 重复注册", definition.Name)
	}
	b.definitions[definition.Name] = definition
	return nil
}

// Freeze 冻结构建器并返回不可变注册表
func (b *RegistryBuilder) Freeze() (*Registry, error) {
	if b == nil {
		return nil, fmt.Errorf("任务处理器注册表不能为空")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isFrozen = true
	definitions := make(map[string]HandlerDefinition, len(b.definitions))
	for name, definition := range b.definitions {
		definitions[name] = definition
	}
	return &Registry{definitions: definitions}, nil
}

// Find 查找已注册处理器
func (r *Registry) Find(name string) (HandlerDefinition, bool) {
	if r == nil {
		return HandlerDefinition{}, false
	}
	definition, ok := r.definitions[name]
	return definition, ok
}

type permanentError struct {
	cause error
}

func (e *permanentError) Error() string { return e.cause.Error() }
func (e *permanentError) Unwrap() error { return e.cause }

// Permanent 将业务错误标记为不可重试
func Permanent(err error) error {
	if err == nil || IsPermanent(err) {
		return err
	}
	return &permanentError{cause: err}
}

// IsPermanent 判断错误是否不可重试
func IsPermanent(err error) bool {
	var target *permanentError
	return errors.As(err, &target)
}
