package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
)

const defaultShutdownTimeout = 30 * time.Second

// 已启动的应用组件与 Transport 集合
type Application struct {
	ready      atomic.Bool
	runErr     chan error
	stopDone   chan struct{}
	stopOnce   sync.Once
	stopping   atomic.Bool
	stopErr    error
	stoppers   []stopEntry
	transports []Transport
}

type stopEntry struct {
	component module.Component
	stopper   module.Stopper
}

type termination struct {
	channel   <-chan error
	component *module.Component
	name      string
}

// 按生成装配函数启动应用
func StartDefinition(ctx context.Context, definition Definition) (*Application, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !definition.graph.IsValidated() || definition.assemble == nil {
		return nil, exception.Core("应用 Definition 未完成静态校验")
	}
	application := &Application{runErr: make(chan error, 1), stopDone: make(chan struct{})}
	ctx = context.WithValue(ctx, readinessKey{}, ReadyState(application))
	input, _ := ctx.Value(assembleInputKey{}).(AssembleInput)
	assembly, err := definition.assemble(ctx, input)
	if err != nil {
		if prefixErr := validateAssemblyPrefix(definition.graph, assembly); prefixErr != nil {
			err = errors.Join(err, prefixErr)
			application.addAssemblyRollback(validAssemblyPrefix(definition.graph, assembly))
		} else {
			application.addAssemblyRollback(assembly)
		}
		return nil, application.rollback(ctx, err)
	}
	if assembly == nil {
		return nil, application.rollback(ctx, exception.Core("生成装配返回空 Assembly"))
	}
	components, transports, err := validateAssembly(definition.graph, assembly)
	if err != nil {
		application.addAssemblyRollback(validAssemblyPrefix(definition.graph, assembly))
		return nil, application.rollback(ctx, err)
	}
	transports = enabledTransports(transports)
	if err = application.startAssembly(ctx, components, transports); err != nil {
		return nil, application.rollback(ctx, err)
	}
	return application, nil
}

func (application *Application) addAssemblyRollback(assembly *Assembly) {
	for _, current := range assembly.components {
		if current.transport != nil {
			application.transports = append(application.transports, current.transport)
		}
		if current.hooks.Stopper != nil && current.hooks.Initializer == nil {
			application.addStopper(current.definitionAsComponent(), current.hooks.Stopper)
		}
	}
}

func (application *Application) startAssembly(ctx context.Context, components []assembledComponent, transports []Transport) error {
	var err error
	for _, current := range components {
		if current.hooks.Initializer == nil {
			if current.hooks.Stopper != nil {
				application.addStopper(current.definitionAsComponent(), current.hooks.Stopper)
			}
			continue
		}
		if err = current.hooks.Initializer.OnInit(ctx); err != nil {
			return componentError("初始化", current.definitionAsComponent(), err)
		}
		if current.hooks.Stopper != nil {
			application.addStopper(current.definitionAsComponent(), current.hooks.Stopper)
		}
	}
	application.transports = transports
	if err = application.prepareTransports(ctx); err != nil {
		return err
	}
	for _, current := range components {
		if current.hooks.Starter == nil {
			continue
		}
		if err = current.hooks.Starter.OnStart(ctx); err != nil {
			return componentError("启动", current.definitionAsComponent(), err)
		}
	}
	terminations, err := componentTerminations(components)
	if err != nil {
		return err
	}
	transportTerminations, err := application.startTransports(ctx)
	if err != nil {
		return err
	}
	terminations = append(terminations, transportTerminations...)
	application.ready.Store(true)
	for _, current := range terminations {
		go application.supervise(current)
	}
	return nil
}

func (component assembledComponent) definitionAsComponent() module.Component {
	return module.ComponentFromDefinition(component.definition)
}

func validateAssembly(graph module.Graph, assembly *Assembly) ([]assembledComponent, []Transport, error) {
	if assembly == nil {
		return nil, nil, exception.Core("Assembly 不能为空")
	}
	definitions := graph.Components()
	if len(definitions) != len(assembly.components) {
		return nil, nil, exception.Core("Assembly 与 Graph 组件数量不一致")
	}
	lifecycles := graph.Lifecycles()
	if len(lifecycles) != len(definitions) {
		return nil, nil, exception.Core("Graph 组件与生命周期数量不一致")
	}
	transportDefinitions := graph.Transports()
	transportsByID := make(map[string]Transport)
	components := make([]assembledComponent, len(assembly.components))
	transports := make([]Transport, 0, len(transportDefinitions))
	transportIndex := 0
	for index, current := range assembly.components {
		if module.ComponentFromDefinition(current.definition) != definitions[index] {
			return nil, nil, exception.Core("Assembly 与 Graph 组件顺序不一致")
		}
		if err := validateHooks(lifecycles[index], current.hooks); err != nil {
			return nil, nil, err
		}
		components[index] = current
		hasTransport := current.transport != nil
		wantsTransport := transportIndex < len(transportDefinitions) && definitions[index] == transportDefinitions[transportIndex]
		if hasTransport != wantsTransport {
			return nil, nil, exception.Core("Assembly 与 Graph Transport 标记不一致")
		}
		if !hasTransport {
			continue
		}
		name := strings.TrimSpace(current.transport.Name())
		if name == "" || name != current.transport.Name() || transportsByID[name] != nil {
			return nil, nil, exception.Core("Assembly Transport 名称无效或重复")
		}
		transportsByID[name] = current.transport
		transports = append(transports, current.transport)
		transportIndex++
	}
	if transportIndex != len(transportDefinitions) {
		return nil, nil, exception.Core("Assembly 缺少 Transport")
	}
	return components, transports, nil
}

func validateAssemblyPrefix(graph module.Graph, assembly *Assembly) error {
	if assembly == nil {
		return exception.Core("Assembly 不能为空")
	}
	graphComponents := graph.Components()
	if len(assembly.components) > len(graphComponents) {
		return exception.Core("Assembly 超出 Graph 组件数量")
	}
	lifecycles := graph.Lifecycles()
	if len(lifecycles) != len(graphComponents) {
		return exception.Core("Graph 组件与生命周期数量不一致")
	}
	transportDefinitions := graph.Transports()
	transportIndex := 0
	for index, current := range assembly.components {
		if module.ComponentFromDefinition(current.definition) != graphComponents[index] {
			return exception.Core("Assembly 不是 Graph 拓扑的合法前缀")
		}
		if err := validateHooks(lifecycles[index], current.hooks); err != nil {
			return err
		}
		hasTransport := current.transport != nil
		wantsTransport := transportIndex < len(transportDefinitions) && graphComponents[index] == transportDefinitions[transportIndex]
		if hasTransport != wantsTransport {
			return exception.Core("Assembly 与 Graph Transport 标记不一致")
		}
		if hasTransport {
			transportIndex++
		}
	}
	return nil
}

func validAssemblyPrefix(graph module.Graph, assembly *Assembly) *Assembly {
	validated := NewAssembly()
	if assembly == nil {
		return validated
	}
	components := graph.Components()
	lifecycles := graph.Lifecycles()
	transports := graph.Transports()
	if len(lifecycles) != len(components) {
		return validated
	}
	transportIndex := 0
	for index, current := range assembly.components {
		if index >= len(components) || module.ComponentFromDefinition(current.definition) != components[index] {
			break
		}
		if validateHooks(lifecycles[index], current.hooks) != nil {
			break
		}
		hasTransport := current.transport != nil
		wantsTransport := transportIndex < len(transports) && components[index] == transports[transportIndex]
		if hasTransport != wantsTransport {
			break
		}
		validated.components = append(validated.components, current)
		if hasTransport {
			transportIndex++
		}
	}
	return validated
}

func validateHooks(lifecycle module.Lifecycle, hooks Hooks) error {
	if (hooks.Initializer != nil) != lifecycle.HasInitializer() {
		return exception.Core("Assembly 与 Graph Initializer 能力不一致")
	}
	if (hooks.Starter != nil) != lifecycle.HasStarter() {
		return exception.Core("Assembly 与 Graph Starter 能力不一致")
	}
	if (hooks.Stopper != nil) != lifecycle.HasStopper() {
		return exception.Core("Assembly 与 Graph Stopper 能力不一致")
	}
	if (hooks.Supervisor != nil) != lifecycle.HasSupervisor() {
		return exception.Core("Assembly 与 Graph Supervisor 能力不一致")
	}
	return nil
}

type switchableTransport interface {
	Enabled() bool
}

func enabledTransports(transports []Transport) []Transport {
	enabled := make([]Transport, 0, len(transports))
	for _, transport := range transports {
		if current, ok := transport.(switchableTransport); ok && !current.Enabled() {
			continue
		}
		enabled = append(enabled, transport)
	}

	return enabled
}

// 运行应用直到上下文取消或 Transport 异常终止
func Run(ctx context.Context, definition Definition) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	input, err := loadAssembleInput(ctx, definition)
	if err != nil {
		return err
	}
	ctx = withAssembleInput(ctx, input)
	application, err := StartDefinition(ctx, definition)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return application.Stop(ctx)
	case err = <-application.runErr:
		return err
	}
}

// 返回应用是否统一就绪
func (application *Application) Ready() bool {
	return application != nil && application.ready.Load()
}

// 撤销就绪并按逆序停止 Transport 与组件
func (application *Application) Stop(ctx context.Context) error {
	if application == nil {
		return nil
	}
	application.ready.Store(false)
	application.stopping.Store(true)
	application.stopOnce.Do(func() {
		shutdownCtx, cancel := shutdownContext(ctx)
		defer cancel()
		errorsByOrder := make([]error, 0, len(application.transports)+len(application.stoppers))
		for index := len(application.transports) - 1; index >= 0; index-- {
			transport := application.transports[index]
			if err := stopWithinDeadline(shutdownCtx, transport.Stop); err != nil {
				errorsByOrder = append(errorsByOrder, transportError("停止", transport.Name(), err))
			}
			if shutdownCtx.Err() != nil {
				break
			}
		}
		if shutdownCtx.Err() == nil {
			for index := len(application.stoppers) - 1; index >= 0; index-- {
				entry := application.stoppers[index]
				if err := stopWithinDeadline(shutdownCtx, entry.stopper.OnStop); err != nil {
					errorsByOrder = append(errorsByOrder, componentError("停止", entry.component, err))
				}
				if shutdownCtx.Err() != nil {
					break
				}
			}
		}
		if err := shutdownCtx.Err(); err != nil {
			errorsByOrder = append(errorsByOrder, errors.Join(exception.WrapCore(err, "应用关闭超过全局期限"), err))
		}
		application.stopErr = errors.Join(errorsByOrder...)
		close(application.stopDone)
	})
	<-application.stopDone
	return application.stopErr
}

func (application *Application) prepareTransports(ctx context.Context) error {
	prepared := application.transports[:0]
	for _, transport := range application.transports {
		name := transport.Name()
		if err := transport.Prepare(ctx); err != nil {
			application.transports = prepared
			return transportError("准备", name, err)
		}
		prepared = append(prepared, transport)
	}
	return nil
}

func (application *Application) startTransports(ctx context.Context) ([]termination, error) {
	terminations := make([]termination, 0, len(application.transports))
	for _, transport := range application.transports {
		terminated, err := transport.Start(ctx)
		if err != nil {
			return nil, transportError("启动", transport.Name(), err)
		}
		if terminated == nil {
			return nil, exception.Core(fmt.Sprintf("Transport 终止 Channel 为空: %s", transport.Name()))
		}
		terminations = append(terminations, termination{channel: terminated, name: transport.Name()})
	}
	return terminations, nil
}

func componentTerminations(components []assembledComponent) ([]termination, error) {
	terminations := make([]termination, 0)
	for _, current := range components {
		if current.hooks.Supervisor == nil {
			continue
		}
		terminated := current.hooks.Supervisor.Terminated()
		component := current.definitionAsComponent()
		if terminated == nil {
			return nil, componentError("监督", component, exception.Core("组件终止 Channel 为空"))
		}
		terminations = append(terminations, termination{
			channel:   terminated,
			component: &component,
			name:      component.PackagePath() + "." + component.Name(),
		})
	}
	return terminations, nil
}

func (application *Application) supervise(current termination) {
	var (
		err  error
		open bool
	)
	select {
	case err, open = <-current.channel:
	case <-application.stopDone:
		return
	}
	if application.stopping.Load() {
		return
	}
	if !open || err == nil {
		if current.component != nil {
			err = exception.Core(fmt.Sprintf("组件意外终止: %s", current.name))
		} else {
			err = exception.Core(fmt.Sprintf("Transport 意外终止: %s", current.name))
		}
	} else if current.component != nil {
		err = componentError("运行", *current.component, err)
	} else {
		err = transportError("运行", current.name, err)
	}
	application.ready.Store(false)
	err = errors.Join(err, application.Stop(context.Background()))
	select {
	case application.runErr <- err:
	default:
	}
}

func (application *Application) addStopper(component module.Component, stopper module.Stopper) {
	application.stoppers = append(application.stoppers, stopEntry{component: component, stopper: stopper})
}

func (application *Application) rollback(ctx context.Context, cause error) error {
	return errors.Join(cause, application.Stop(ctx))
}

func shutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	base := context.WithoutCancel(ctx)
	timeout := defaultShutdownTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(base, timeout)
}

func stopWithinDeadline(ctx context.Context, stop func(context.Context) error) error {
	result := make(chan error, 1)
	go func() {
		result <- stop(ctx)
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func componentError(action string, component module.Component, cause error) error {
	label := fmt.Sprintf("%s.%s", component.PackagePath(), component.Name())
	return exception.WrapCore(cause, fmt.Sprintf("%s组件 %s 失败", action, label))
}

func transportError(action, name string, cause error) error {
	return exception.WrapCore(cause, fmt.Sprintf("%s Transport %s 失败", action, name))
}
