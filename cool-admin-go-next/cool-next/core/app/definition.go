package app

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/configuration"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
	coreroute "github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

// AssembleInput 生成装配函数的只读配置视图
type AssembleInput struct {
	root    configuration.Source
	modules map[string]configuration.Source
}

type assembleInputKey struct{}

func withAssembleInput(ctx context.Context, input AssembleInput) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, assembleInputKey{}, input)
}

func newAssembleInput(root configuration.Source, modules map[string]configuration.Source) AssembleInput {
	clonedModules := make(map[string]configuration.Source, len(modules))
	for key, source := range modules {
		clonedModules[key] = cloneSource(source)
	}

	return AssembleInput{root: cloneSource(root), modules: clonedModules}
}

// RootSource 根基础设施配置来源副本
func (input AssembleInput) RootSource() configuration.Source {
	return cloneSource(input.root)
}

// ModuleSource 指定模块配置来源副本
func (input AssembleInput) ModuleSource(identity module.Identity) configuration.Source {
	return cloneSource(input.modules[identity.Key()])
}

// AssembleFunc 负责按静态拓扑构造一次应用实例
type AssembleFunc func(context.Context, AssembleInput) (*Assembly, error)

// 保存静态 Graph 和唯一生成装配函数
type Definition struct {
	graph    module.Graph
	assemble AssembleFunc
}

// Definition 保存的静态路由表
func (definition Definition) Routes() coreroute.Table { return definition.graph.Routes() }

// Graph Definition 保存的静态 Graph
func (definition Definition) Graph() module.Graph { return definition.graph }

// Define 创建不可变应用定义
func Define(graph module.Graph, assemble AssembleFunc) Definition {
	if !graph.IsValidated() {
		panic("app Definition requires a validated module Graph")
	}
	if assemble == nil {
		panic("app Definition requires an assemble function")
	}

	return Definition{graph: graph, assemble: assemble}
}

func cloneSource(source configuration.Source) configuration.Source {
	return configuration.Source{Main: append([]byte(nil), source.Main...), LookupEnv: source.LookupEnv}
}
