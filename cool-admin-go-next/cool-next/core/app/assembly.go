package app

import "github.com/toothdy/cool-admin-go-next/cool-next/core/module"

// 组件可选的生命周期能力
type Hooks struct {
	Initializer module.Initializer
	Starter     module.Starter
	Stopper     module.Stopper
	Supervisor  module.Supervisor
}

// Assembly 一次生成装配产生的组件和 Transport 集合
type Assembly struct {
	components []assembledComponent
}

type assembledComponent struct {
	definition module.ComponentDefinition
	hooks      Hooks
	transport  Transport
}

// NewAssembly 创建空装配结果
func NewAssembly() *Assembly {
	return &Assembly{}
}

// AddComponent 按 Graph 拓扑登记普通组件
func (assembly *Assembly) AddComponent(definition module.ComponentDefinition, hooks Hooks) {
	if assembly == nil {
		return
	}
	assembly.components = append(assembly.components, assembledComponent{definition: definition, hooks: hooks})
}

// AddTransport 按 Graph 拓扑登记 Transport 组件
func (assembly *Assembly) AddTransport(definition module.ComponentDefinition, transport Transport, hooks Hooks) {
	if assembly == nil {
		return
	}
	assembly.components = append(assembly.components, assembledComponent{
		definition: definition,
		hooks:      hooks,
		transport:  transport,
	})
}
