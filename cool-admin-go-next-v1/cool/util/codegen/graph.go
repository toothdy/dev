package codegen

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
)

// DependencyKind 表示依赖的注入来源。
type DependencyKind string

const (
	DependencyComponent DependencyKind = "component"
	DependencyFramework DependencyKind = "framework"
	DependencyLazy      DependencyKind = "lazy"
)

// Dependency 保存已解析的参数绑定。
type Dependency struct {
	Parameter Parameter
	Kind      DependencyKind
	Source    Component
}

// ResolvedNode 表示依赖已确定的组件节点。
type ResolvedNode struct {
	Component    Component
	Dependencies []Dependency
}

// ProjectGraph 保存全项目组件图与最终模块顺序。
type ProjectGraph struct {
	Nodes       []ResolvedNode
	ModuleOrder []string
}

var frameworkDependencyTypes = map[string]struct{}{
	"context.Context":                                                        {},
	"github.com/gogf/gf/v2/database/gdb.DB":                                  {},
	"*github.com/toothdy/cool-admin-go-next/cool/security.Manager":               {},
	"github.com/toothdy/cool-admin-go-next/cool/security.SessionStore":           {},
	"github.com/toothdy/cool-admin-go-next/cool/module.UploadDirectory":    {},
	"github.com/toothdy/cool-admin-go-next/cool/module.MiddlewareDeps":     {},
	"*github.com/toothdy/cool-admin-go-next/cool/db/recycle.Manager":            {},
	"[]github.com/toothdy/cool-admin-go-next/cool/entity.Definition":          {},
	"[]github.com/toothdy/cool-admin-go-next/cool/task.HandlerDefinition":    {},
	"github.com/toothdy/cool-admin-go-next/cool/module.AuthOptions":        {},
	"github.com/toothdy/cool-admin-go-next/cool/module.I18nOptions":        {},
	"github.com/toothdy/cool-admin-go-next/cool/module.CRUDOptions":        {},
	"github.com/toothdy/cool-admin-go-next/cool/module.RedisDefaultConfig": {},
}

// ResolveProjectGraph 解析全项目组件依赖与稳定模块拓扑。
func ResolveProjectGraph(analyses []Analysis) (*ProjectGraph, error) {
	components := make([]Component, 0)
	orders := make(map[string]int, len(analyses))
	for _, analysis := range analyses {
		orders[analysis.Module.Key] = analysis.Declaration.Order
		components = append(components, Component{
			Kind: ComponentConfig, ModuleKey: analysis.Module.Key,
			Function: "ModuleConfig", ImportPath: configImportPath(analysis),
			Output: typeID(analysis.Declaration.ConfigType), OutputType: analysis.Declaration.ConfigType,
			Position: analysis.Declaration.Position,
		})
		components = append(components, analysis.Components...)
	}

	recycleProviders := make([]string, 0)
	for _, component := range components {
		if isNamedPointer(component.OutputType, recyclePackagePath, "Manager") && (component.Kind == ComponentProvider || component.Kind == ComponentRuntime) {
			recycleProviders = append(recycleProviders, componentID(component))
		}
	}
	if len(recycleProviders) > 1 {
		sort.Strings(recycleProviders)
		return nil, fmt.Errorf("全局只允许一个 *recycle.Manager Provider: %s", strings.Join(recycleProviders, ", "))
	}
	outputs := make(map[string]Component)
	for _, component := range components {
		if component.Kind != ComponentProvider && component.Kind != ComponentRuntime && component.Kind != ComponentConfig {
			continue
		}
		if previous, exists := outputs[component.Output]; exists {
			return nil, fmt.Errorf("全局类型 %s 存在重复 Provider: %s 与 %s", component.Output, componentID(previous), componentID(component))
		}
		outputs[component.Output] = component
	}

	nodes := make([]ResolvedNode, len(components))
	for index, component := range components {
		nodes[index].Component = component
		dependencies, err := resolveDependencies(component, components)
		if err != nil {
			return nil, fmt.Errorf("模块 %s 组件 %s: %w", component.ModuleKey, componentID(component), err)
		}
		nodes[index].Dependencies = dependencies
	}
	moduleEdges := make(map[string]map[string]struct{}, len(orders))
	for _, node := range nodes {
		for _, dependency := range node.Dependencies {
			if dependency.Kind != DependencyComponent || dependency.Source.ModuleKey == "" || dependency.Source.ModuleKey == node.Component.ModuleKey {
				continue
			}
			if moduleEdges[dependency.Source.ModuleKey] == nil {
				moduleEdges[dependency.Source.ModuleKey] = make(map[string]struct{})
			}
			moduleEdges[dependency.Source.ModuleKey][node.Component.ModuleKey] = struct{}{}
		}
	}
	moduleOrder, err := topologicalModules(orders, moduleEdges)
	if err != nil {
		return nil, err
	}
	ordered, err := topologicalNodes(nodes)
	if err != nil {
		return nil, err
	}
	return &ProjectGraph{Nodes: ordered, ModuleOrder: moduleOrder}, nil
}

func configImportPath(analysis Analysis) string {
	if named, ok := types.Unalias(analysis.Declaration.ConfigType).(*types.Named); ok && named.Obj() != nil && named.Obj().Pkg() != nil {
		return named.Obj().Pkg().Path()
	}
	return ""
}

func topologicalModules(orders map[string]int, edges map[string]map[string]struct{}) ([]string, error) {
	indegree := make(map[string]int, len(orders))
	for key := range orders {
		indegree[key] = 0
	}
	for _, dependents := range edges {
		for dependent := range dependents {
			indegree[dependent]++
		}
	}
	ready := make([]string, 0)
	for key, degree := range indegree {
		if degree == 0 {
			ready = append(ready, key)
		}
	}
	ordered := make([]string, 0, len(orders))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			if orders[ready[i]] != orders[ready[j]] {
				return orders[ready[i]] > orders[ready[j]]
			}
			return ready[i] < ready[j]
		})
		key := ready[0]
		ready = ready[1:]
		ordered = append(ordered, key)
		dependents := make([]string, 0, len(edges[key]))
		for dependent := range edges[key] {
			dependents = append(dependents, dependent)
		}
		sort.Strings(dependents)
		for _, dependent := range dependents {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if len(ordered) == len(orders) {
		return ordered, nil
	}
	cycle := make([]string, 0)
	for key, degree := range indegree {
		if degree > 0 {
			cycle = append(cycle, key)
		}
	}
	sort.Strings(cycle)
	return nil, fmt.Errorf("模块循环依赖: %s", strings.Join(cycle, " -> "))
}

func resolveDependencies(component Component, components []Component) ([]Dependency, error) {
	dependencies := make([]Dependency, 0, len(component.Parameters))
	for _, parameter := range component.Parameters {
		if isControllerProvider(parameter.Raw) {
			dependencies = append(dependencies, Dependency{Parameter: parameter, Kind: DependencyLazy})
			continue
		}
		configCandidates := exactConfigProviders(parameter.Raw, components)
		if len(configCandidates) == 1 {
			dependencies = append(dependencies, Dependency{Parameter: parameter, Kind: DependencyComponent, Source: configCandidates[0]})
			continue
		}
		if parameter.Type == "*github.com/toothdy/cool-admin-go-next/cool/db/recycle.Manager" {
			candidates := assignableProviders(parameter.Raw, components)
			if len(candidates) == 1 {
				dependencies = append(dependencies, Dependency{Parameter: parameter, Kind: DependencyComponent, Source: candidates[0]})
				continue
			}
			if len(candidates) > 1 {
				return nil, fmt.Errorf("回收站 Manager 依赖 %s 存在多个 Provider", parameter.Name)
			}
		}
		if _, ok := frameworkDependencyTypes[parameter.Type]; ok {
			dependencies = append(dependencies, Dependency{Parameter: parameter, Kind: DependencyFramework})
			continue
		}
		if namedType(parameter.Raw, modelPackagePath, "Definition") {
			model, err := resolveModel(parameter.Name, components)
			if err != nil {
				return nil, err
			}
			dependencies = append(dependencies, Dependency{Parameter: parameter, Kind: DependencyComponent, Source: model})
			continue
		}
		if isPrimitiveDependency(parameter.Raw) {
			return nil, fmt.Errorf("标量依赖 %s %s 不参与自动注入，请使用强类型 Config", parameter.Name, parameter.Type)
		}
		candidates := assignableProviders(parameter.Raw, components)
		switch len(candidates) {
		case 0:
			return nil, fmt.Errorf("依赖 %s %s 没有可用 Provider", parameter.Name, parameter.Type)
		case 1:
			dependencies = append(dependencies, Dependency{Parameter: parameter, Kind: DependencyComponent, Source: candidates[0]})
		default:
			names := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				names = append(names, candidate.ImportPath+"."+candidate.Function)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("接口依赖 %s %s 存在多个实现: %s", parameter.Name, parameter.Type, strings.Join(names, ", "))
		}
	}
	return dependencies, nil
}

func exactConfigProviders(target types.Type, components []Component) []Component {
	result := make([]Component, 0, 1)
	for _, component := range components {
		if component.Kind == ComponentConfig && component.OutputType != nil && types.Identical(component.OutputType, target) {
			result = append(result, component)
		}
	}
	return result
}

func resolveModel(parameterName string, components []Component) (Component, error) {
	candidates := make([]Component, 0)
	expected := strings.TrimSuffix(parameterName, "Model")
	for _, component := range components {
		if component.Kind != ComponentModel {
			continue
		}
		candidate := lowerCamel(component.Function)
		if candidate == expected {
			candidates = append(candidates, component)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	available := make([]string, 0)
	for _, component := range components {
		if component.Kind == ComponentModel {
			available = append(available, lowerCamel(component.Function)+"Model")
		}
	}
	sort.Strings(available)
	return Component{}, fmt.Errorf("模型参数 %s 无法唯一绑定，期望名称 %s，候选: %s", parameterName, expected+"Model", strings.Join(available, ", "))
}

func assignableProviders(target types.Type, components []Component) []Component {
	candidates := make([]Component, 0)
	for _, component := range components {
		if component.Kind != ComponentProvider && component.Kind != ComponentRuntime {
			continue
		}
		if component.OutputType != nil && types.AssignableTo(component.OutputType, target) {
			candidates = append(candidates, component)
		}
	}
	return candidates
}

func topologicalNodes(nodes []ResolvedNode) ([]ResolvedNode, error) {
	byID := make(map[string]int, len(nodes))
	for index, node := range nodes {
		byID[componentID(node.Component)] = index
	}
	indegree := make([]int, len(nodes))
	dependents := make([][]int, len(nodes))
	for index, node := range nodes {
		for _, dependency := range node.Dependencies {
			if dependency.Kind != DependencyComponent {
				continue
			}
			sourceIndex, ok := byID[componentID(dependency.Source)]
			if !ok {
				continue
			}
			indegree[index]++
			dependents[sourceIndex] = append(dependents[sourceIndex], index)
		}
	}
	ready := make([]int, 0)
	for index, degree := range indegree {
		if degree == 0 {
			ready = append(ready, index)
		}
	}
	ordered := make([]ResolvedNode, 0, len(nodes))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			return componentID(nodes[ready[i]].Component) < componentID(nodes[ready[j]].Component)
		})
		index := ready[0]
		ready = ready[1:]
		ordered = append(ordered, nodes[index])
		for _, dependent := range dependents[index] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if len(ordered) == len(nodes) {
		return ordered, nil
	}
	cycle := make([]string, 0)
	for index, degree := range indegree {
		if degree > 0 {
			cycle = append(cycle, componentID(nodes[index].Component))
		}
	}
	sort.Strings(cycle)
	return nil, fmt.Errorf("Provider 循环依赖: %s", strings.Join(cycle, " -> "))
}

func isControllerProvider(typ types.Type) bool {
	return namedType(typ, registryPackagePath, "ControllerProvider")
}

func componentID(component Component) string {
	return component.ImportPath + "." + component.Function
}

func lowerCamel(value string) string {
	if value == "" {
		return ""
	}
	if len(value) == 1 {
		return strings.ToLower(value)
	}
	return strings.ToLower(value[:1]) + value[1:]
}
