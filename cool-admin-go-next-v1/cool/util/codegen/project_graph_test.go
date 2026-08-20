package codegen

import (
	"go/types"
	"reflect"
	"strings"
	"testing"
)

func TestResolveProjectGraphBindsCrossModuleProviderAndConfig(t *testing.T) {
	baseConfig := namedTestType("example.com/modules/base", "Config", types.NewStruct(nil, nil))
	baseService := namedTestType("example.com/modules/base/service", "Base", types.NewStruct(nil, nil))
	dictService := namedTestType("example.com/modules/dict/service", "DictService", types.NewStruct(nil, nil))
	analyses := []Analysis{
		{
			Module:      DiscoveredModule{Key: "base"},
			Declaration: ModuleDeclaration{Key: "base", Order: 10, ConfigType: baseConfig},
			Components: []Component{
				testProjectComponent("base", "example.com/modules/base/service", "NewBase", baseService, Parameter{Name: "config", Type: typeID(baseConfig), Raw: baseConfig}),
			},
		},
		{
			Module:      DiscoveredModule{Key: "dict"},
			Declaration: ModuleDeclaration{Key: "dict", ConfigType: namedTestType("example.com/modules/dict", "Config", types.NewStruct(nil, nil))},
			Components: []Component{
				testProjectComponent("dict", "example.com/modules/dict/service", "NewDictService", dictService, Parameter{Name: "baseService", Type: typeID(baseService), Raw: baseService}),
			},
		},
	}
	graph, err := ResolveProjectGraph(analyses)
	if err != nil {
		t.Fatalf("解析全局图失败: %v", err)
	}
	if !reflect.DeepEqual(graph.ModuleOrder, []string{"base", "dict"}) {
		t.Fatalf("跨模块依赖顺序不符: %#v", graph.ModuleOrder)
	}
	dependencies := projectNodeByFunction(t, graph, "NewBase").Dependencies
	if len(dependencies) != 1 || dependencies[0].Source.Kind != ComponentConfig {
		t.Fatalf("Config 未绑定到虚拟配置节点: %#v", dependencies)
	}
}

func TestResolveProjectGraphUsesOrderThenKeyForReadyModules(t *testing.T) {
	analyses := []Analysis{
		testProjectAnalysis("task", 0),
		testProjectAnalysis("recycle", 0),
		testProjectAnalysis("dict", 0),
		testProjectAnalysis("base", 10),
	}
	graph, err := ResolveProjectGraph(analyses)
	if err != nil {
		t.Fatalf("解析模块顺序失败: %v", err)
	}
	want := []string{"base", "dict", "recycle", "task"}
	if !reflect.DeepEqual(graph.ModuleOrder, want) {
		t.Fatalf("模块稳定顺序不符: got=%#v want=%#v", graph.ModuleOrder, want)
	}
}

func TestResolveProjectGraphDependencyOverridesOrder(t *testing.T) {
	dependencyType := namedTestType("example.com/modules/low/service", "Dependency", types.NewStruct(nil, nil))
	consumerType := namedTestType("example.com/modules/high/service", "Consumer", types.NewStruct(nil, nil))
	low := testProjectAnalysis("low", 0)
	low.Components = []Component{testProjectComponent("low", "example.com/modules/low/service", "NewDependency", dependencyType)}
	high := testProjectAnalysis("high", 100)
	high.Components = []Component{testProjectComponent("high", "example.com/modules/high/service", "NewConsumer", consumerType, Parameter{Name: "dependency", Type: typeID(dependencyType), Raw: dependencyType})}
	graph, err := ResolveProjectGraph([]Analysis{high, low})
	if err != nil {
		t.Fatalf("解析逆 Order 依赖失败: %v", err)
	}
	if !reflect.DeepEqual(graph.ModuleOrder, []string{"low", "high"}) {
		t.Fatalf("Order 覆盖了真实依赖: %#v", graph.ModuleOrder)
	}
}

func TestResolveProjectGraphRejectsModuleCycle(t *testing.T) {
	firstType := namedTestType("example.com/modules/first/service", "First", types.NewStruct(nil, nil))
	secondType := namedTestType("example.com/modules/second/service", "Second", types.NewStruct(nil, nil))
	first := testProjectAnalysis("first", 0)
	first.Components = []Component{testProjectComponent("first", "example.com/modules/first/service", "NewFirst", firstType, Parameter{Name: "second", Type: typeID(secondType), Raw: secondType})}
	second := testProjectAnalysis("second", 0)
	second.Components = []Component{testProjectComponent("second", "example.com/modules/second/service", "NewSecond", secondType, Parameter{Name: "first", Type: typeID(firstType), Raw: firstType})}
	_, err := ResolveProjectGraph([]Analysis{first, second})
	if err == nil || !strings.Contains(err.Error(), "循环") || !strings.Contains(err.Error(), "first") || !strings.Contains(err.Error(), "second") {
		t.Fatalf("模块循环诊断不完整: %v", err)
	}
}

func TestResolveProjectGraphRejectsMultipleRecycleProviders(t *testing.T) {
	managerType := namedTestType(recyclePackagePath, "Manager", types.NewStruct(nil, nil))
	first := testProjectAnalysis("first", 0)
	first.Components = []Component{testProjectComponent("first", "example.com/modules/first/service", "NewManager", types.NewPointer(managerType))}
	second := testProjectAnalysis("second", 0)
	second.Components = []Component{testProjectComponent("second", "example.com/modules/second/service", "NewManager", types.NewPointer(managerType))}
	_, err := ResolveProjectGraph([]Analysis{first, second})
	if err == nil || !strings.Contains(err.Error(), "只允许一个") {
		t.Fatalf("多个回收站 Provider 未被拒绝: %v", err)
	}
}

func testProjectAnalysis(key string, order int) Analysis {
	configType := namedTestType("example.com/modules/"+key, "Config", types.NewStruct(nil, nil))
	return Analysis{
		Module:      DiscoveredModule{Key: key},
		Declaration: ModuleDeclaration{Key: key, Order: order, ConfigType: configType},
	}
}

func testProjectComponent(moduleKey string, importPath string, function string, output types.Type, parameters ...Parameter) Component {
	component := testGraphComponent(function, output, parameters...)
	component.ModuleKey = moduleKey
	component.ImportPath = importPath
	return component
}

func projectNodeByFunction(t *testing.T, graph *ProjectGraph, function string) ResolvedNode {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Component.Function == function {
			return node
		}
	}
	t.Fatalf("全局图缺少节点 %s", function)
	return ResolvedNode{}
}
