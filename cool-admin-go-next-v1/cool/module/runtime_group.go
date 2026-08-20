package module

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"
)

// RuntimeDefinition 描述模块内一个具名 Runtime。
type RuntimeDefinition struct {
	Name    string
	Runtime Runtime
}

// RuntimeGroup 按依赖顺序管理模块内的 Runtime。
type RuntimeGroup struct {
	module      string
	definitions []RuntimeDefinition
	started     int
}

// NewRuntimeGroup 创建模块 Runtime 组合。
func NewRuntimeGroup(module string, definitions ...RuntimeDefinition) *RuntimeGroup {
	return &RuntimeGroup{
		module:      module,
		definitions: append([]RuntimeDefinition{}, definitions...),
	}
}

// Start 按声明的依赖顺序启动 Runtime，失败时回滚已启动项。
func (g *RuntimeGroup) Start(ctx context.Context) error {
	if g.started > 0 {
		return nil
	}
	for index, definition := range g.definitions {
		if definition.Runtime == nil {
			startErr := gerror.Newf("模块 %s Runtime %s 不能为空", g.module, runtimeName(definition, index))
			return errors.Join(startErr, g.stopStarted(context.Background()))
		}
		if err := definition.Runtime.Start(ctx); err != nil {
			startErr := gerror.Wrapf(err, "启动模块 %s Runtime %s 失败", g.module, runtimeName(definition, index))
			return errors.Join(startErr, g.stopStarted(context.Background()))
		}
		g.started++
	}
	return nil
}

// Stop 按启动逆序停止 Runtime，并返回全部停止错误。
func (g *RuntimeGroup) Stop(ctx context.Context) error {
	return g.stopStarted(ctx)
}

func (g *RuntimeGroup) stopStarted(ctx context.Context) error {
	var stopErrors []error
	for g.started > 0 {
		g.started--
		definition := g.definitions[g.started]
		if err := definition.Runtime.Stop(ctx); err != nil {
			stopErrors = append(stopErrors, gerror.Wrapf(
				err,
				"停止模块 %s Runtime %s 失败",
				g.module,
				runtimeName(definition, g.started),
			))
		}
	}
	return errors.Join(stopErrors...)
}

func runtimeName(definition RuntimeDefinition, index int) string {
	if definition.Name != "" {
		return definition.Name
	}
	return fmt.Sprintf("#%d", index+1)
}
