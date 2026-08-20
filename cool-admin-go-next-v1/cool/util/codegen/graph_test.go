package codegen

import (
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"
)

func TestResolveGraphUsesUniqueInterfaceImplementation(t *testing.T) {
	contractNamed := testStoreContract()
	implementation := namedTestType("example.com/service", "MemoryStore", types.NewStruct(nil, nil))
	addTestStoreMethod(implementation)
	consumer := namedTestType("example.com/service", "Consumer", types.NewStruct(nil, nil))
	analysis := Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{
		testGraphComponent("NewMemoryStore", implementation),
		testGraphComponent("NewConsumer", consumer, Parameter{Name: "store", Type: typeID(contractNamed), Raw: contractNamed}),
	}}

	graph, err := resolveTestProjectGraph(analysis)
	if err != nil {
		t.Fatalf("解析图失败: %v", err)
	}
	if got := projectNodeByFunction(t, graph, "NewConsumer").Dependencies[0].Source.Function; got != "NewMemoryStore" {
		t.Fatalf("接口依赖绑定不符: %s", got)
	}
}

func TestResolveGraphRejectsAmbiguousInterface(t *testing.T) {
	contract := testStoreContract()
	first := namedTestType("example.com/service", "First", types.NewStruct(nil, nil))
	second := namedTestType("example.com/service", "Second", types.NewStruct(nil, nil))
	addTestStoreMethod(first)
	addTestStoreMethod(second)
	consumer := namedTestType("example.com/service", "Consumer", types.NewStruct(nil, nil))
	analysis := Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{
		testGraphComponent("NewFirst", first),
		testGraphComponent("NewSecond", second),
		testGraphComponent("NewConsumer", consumer, Parameter{Name: "store", Type: typeID(contract), Raw: contract}),
	}}

	_, err := resolveTestProjectGraph(analysis)
	if err == nil || !strings.Contains(err.Error(), "多个实现") {
		t.Fatalf("应拒绝多个接口实现: %v", err)
	}
}

func TestResolveGraphBindsModelByParameterName(t *testing.T) {
	modelType := namedTestType(modelPackagePath, "Definition", types.NewStruct(nil, nil))
	serviceType := namedTestType("example.com/service", "UserService", types.NewStruct(nil, nil))
	analysis := Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{
		{Kind: ComponentModel, Function: "User", ImportPath: "example.com/entity", Output: typeID(modelType), OutputType: modelType},
		{Kind: ComponentModel, Function: "Role", ImportPath: "example.com/entity", Output: typeID(modelType), OutputType: modelType},
		testGraphComponent("NewUserService", serviceType, Parameter{Name: "userModel", Type: typeID(modelType), Raw: modelType}),
	}}

	graph, err := resolveTestProjectGraph(analysis)
	if err != nil {
		t.Fatalf("解析模型绑定失败: %v", err)
	}
	if got := projectNodeByFunction(t, graph, "NewUserService").Dependencies[0].Source.Function; got != "User" {
		t.Fatalf("模型参数绑定不符: %s", got)
	}
}

func TestResolveGraphRejectsPrimitiveAndCycle(t *testing.T) {
	primitiveType := namedTestType("example.com/service", "Primitive", types.NewStruct(nil, nil))
	primitive := testGraphComponent("NewPrimitive", primitiveType, Parameter{Name: "value", Type: "string", Raw: types.Typ[types.String]})
	_, err := resolveTestProjectGraph(Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{primitive}})
	if err == nil || !strings.Contains(err.Error(), "标量") {
		t.Fatalf("应拒绝标量依赖: %v", err)
	}

	firstType := namedTestType("example.com/service", "First", types.NewStruct(nil, nil))
	secondType := namedTestType("example.com/service", "Second", types.NewStruct(nil, nil))
	first := testGraphComponent("NewFirst", firstType, Parameter{Name: "second", Type: typeID(secondType), Raw: secondType})
	second := testGraphComponent("NewSecond", secondType, Parameter{Name: "first", Type: typeID(firstType), Raw: firstType})
	_, err = resolveTestProjectGraph(Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{first, second}})
	if err == nil || !strings.Contains(err.Error(), "循环") {
		t.Fatalf("应拒绝 Provider 循环: %v", err)
	}
}

func TestResolveGraphAllowsProviderForNamedScalar(t *testing.T) {
	directoryType := namedTestType("example.com/service", "Directory", types.Typ[types.String])
	consumerType := namedTestType("example.com/service", "Consumer", types.NewStruct(nil, nil))
	analysis := Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{
		testGraphComponent("NewDirectory", directoryType),
		testGraphComponent("NewConsumer", consumerType, Parameter{Name: "directory", Type: typeID(directoryType), Raw: directoryType}),
	}}

	graph, err := resolveTestProjectGraph(analysis)
	if err != nil {
		t.Fatalf("专用命名标量应允许 Provider 注入: %v", err)
	}
	if got := projectNodeByFunction(t, graph, "NewConsumer").Dependencies[0].Source.Function; got != "NewDirectory" {
		t.Fatalf("命名标量绑定不符: %s", got)
	}
}

func TestResolveGraphUsesStableTopologicalOrder(t *testing.T) {
	firstType := namedTestType("example.com/service", "First", types.NewStruct(nil, nil))
	secondType := namedTestType("example.com/service", "Second", types.NewStruct(nil, nil))
	thirdType := namedTestType("example.com/service", "Third", types.NewStruct(nil, nil))
	analysis := Analysis{Module: DiscoveredModule{Key: "sample"}, Components: []Component{
		testGraphComponent("NewThird", thirdType, Parameter{Name: "first", Type: typeID(firstType), Raw: firstType}),
		testGraphComponent("NewSecond", secondType),
		testGraphComponent("NewFirst", firstType),
	}}

	graph, err := resolveTestProjectGraph(analysis)
	if err != nil {
		t.Fatalf("解析稳定拓扑顺序失败: %v", err)
	}
	functions := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.Component.Kind == ComponentConfig {
			continue
		}
		functions = append(functions, node.Component.Function)
	}
	if !reflect.DeepEqual(functions, []string{"NewFirst", "NewSecond", "NewThird"}) {
		t.Fatalf("拓扑顺序不符: %#v", functions)
	}
}

func resolveTestProjectGraph(analysis Analysis) (*ProjectGraph, error) {
	configType := namedTestType("example.com/modules/"+analysis.Module.Key, "Config", types.NewStruct(nil, nil))
	analysis.Declaration = ModuleDeclaration{Key: analysis.Module.Key, ConfigType: configType}
	for index := range analysis.Components {
		analysis.Components[index].ModuleKey = analysis.Module.Key
	}
	return ResolveProjectGraph([]Analysis{analysis})
}

func namedTestType(packagePath string, name string, underlying types.Type) *types.Named {
	return types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage(packagePath, filepathBase(packagePath)), name, nil), underlying, nil)
}

func filepathBase(path string) string {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return path
	}
	return path[index+1:]
}

func testGraphComponent(function string, output types.Type, parameters ...Parameter) Component {
	return Component{
		Kind:       ComponentProvider,
		Function:   function,
		ImportPath: "example.com/service",
		Output:     typeID(output),
		OutputType: output,
		Parameters: parameters,
	}
}

func testStoreContract() *types.Named {
	method := types.NewFunc(
		token.NoPos,
		types.NewPackage("example.com/contracts", "contracts"),
		"Store",
		types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(), false),
	)
	contract := types.NewInterfaceType([]*types.Func{method}, nil).Complete()
	return namedTestType("example.com/contracts", "Store", contract)
}

func addTestStoreMethod(named *types.Named) {
	named.AddMethod(types.NewFunc(
		token.NoPos,
		named.Obj().Pkg(),
		"Store",
		types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(), false),
	))
}
