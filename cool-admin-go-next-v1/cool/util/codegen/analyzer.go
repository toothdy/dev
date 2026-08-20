package codegen

import (
	"fmt"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
)

// Analyze 分析项目中所有已扫描模块。
func Analyze(project *Project) ([]Analysis, error) {
	if project == nil {
		return nil, fmt.Errorf("项目扫描结果不能为 nil")
	}
	runtimeInterface := findNamedInterface(project, registryPackagePath, "Runtime")
	analyses := make([]Analysis, 0, len(project.Modules))
	for _, discovered := range project.Modules {
		declaration, err := analyzeModuleDeclaration(project, discovered)
		if err != nil {
			return nil, err
		}
		analysis := Analysis{Module: discovered, Declaration: declaration}
		outputs := make(map[string]Component)
		for _, loadedPackage := range discovered.Packages {
			for _, function := range packageFunctions(discovered.Dir, loadedPackage) {
				component, recognized, err := analyzeFunction(discovered.Key, function, runtimeInterface)
				if err != nil {
					return nil, err
				}
				if !recognized {
					continue
				}
				if component.Kind == ComponentProvider || component.Kind == ComponentRuntime {
					if previous, exists := outputs[component.Output]; exists {
						return nil, fmt.Errorf(
							"模块 %s 类型 %s 存在重复 Provider: %s.%s 与 %s.%s",
							discovered.Key,
							component.Output,
							previous.ImportPath,
							previous.Function,
							component.ImportPath,
							component.Function,
						)
					}
					outputs[component.Output] = component
				}
				analysis.Components = append(analysis.Components, component)
			}
		}
		sort.Slice(analysis.Components, func(i, j int) bool {
			left := analysis.Components[i]
			right := analysis.Components[j]
			if left.ImportPath != right.ImportPath {
				return left.ImportPath < right.ImportPath
			}
			return left.Function < right.Function
		})
		if err = resolveDeclaredMiddlewares(&analysis); err != nil {
			return nil, err
		}
		analyses = append(analyses, analysis)
	}
	return analyses, nil
}

func analyzeFunction(moduleKey string, function SourceFunction, runtimeInterface *types.Interface) (Component, bool, error) {
	signature, err := functionSignature(function)
	if err != nil {
		return Component{}, false, err
	}
	directory := relativeDirectory(function.Path)
	name := function.Decl.Name.Name
	if signature.Results().Len() > 0 && namedType(signature.Results().At(0).Type(), taskPackagePath, "HandlerDefinition") {
		if directory != "service" {
			return Component{}, false, fmt.Errorf("%s: Task 定义 %s 必须位于 service/**", function.Pos, name)
		}
		return analyzeDefinitionComponent(moduleKey, function, signature, ComponentHandler, taskPackagePath, "HandlerDefinition")
	}
	if directory == "entity" {
		return analyzeModel(moduleKey, function, signature)
	}
	if directory == "controller" && strings.HasSuffix(name, "Controller") {
		return analyzeDefinitionComponent(moduleKey, function, signature, ComponentController, controllerPackagePath, "Definition")
	}
	if directory == "middleware" {
		if signature.Results().Len() > 0 && (namedType(signature.Results().At(0).Type(), middlewarePackagePath, "Definition") || sliceOfNamedType(signature.Results().At(0).Type(), middlewarePackagePath, "Definition")) {
			kind := ComponentMiddleware
			if strings.HasPrefix(filepath.ToSlash(function.Path), "middleware/global/") {
				kind = ComponentGlobalMiddleware
			}
			return newComponent(moduleKey, function, signature, kind)
		}
		if strings.HasSuffix(name, "Middleware") || name == "Definition" || name == "Definitions" {
			return Component{}, false, fmt.Errorf("%s: Middleware %s 必须返回 middleware.Definition 或 []middleware.Definition", function.Pos, name)
		}
		return Component{}, false, nil
	}
	if strings.HasPrefix(name, "New") {
		if signature.Results().Len() > 0 {
			expected := "New" + resultTypeName(signature.Results().At(0).Type())
			if expected != "New" && name != expected {
				return Component{}, false, nil
			}
		}
		component, recognized, providerErr := newComponent(moduleKey, function, signature, ComponentProvider)
		if providerErr != nil {
			return Component{}, false, providerErr
		}
		if !recognized {
			return Component{}, false, nil
		}
		expected := "New" + resultTypeName(component.OutputType)
		if expected == "New" || name != expected {
			return Component{}, false, fmt.Errorf("%s: Provider %s 必须命名为 %s", function.Pos, name, expected)
		}
		if isRuntimeType(component.OutputType, runtimeInterface) {
			component.Kind = ComponentRuntime
		}
		return component, true, nil
	}
	return Component{}, false, nil
}

func analyzeModel(moduleKey string, function SourceFunction, signature *types.Signature) (Component, bool, error) {
	if signature.Params().Len() != 0 || signature.Results().Len() != 1 || !namedType(signature.Results().At(0).Type(), modelPackagePath, "Definition") {
		return Component{}, false, fmt.Errorf("%s: entity 导出函数 %s 必须无参且返回 model.Definition", function.Pos, function.Decl.Name.Name)
	}
	return newComponent(moduleKey, function, signature, ComponentModel)
}

func analyzeDefinitionComponent(moduleKey string, function SourceFunction, signature *types.Signature, kind ComponentKind, packagePath string, typeName string) (Component, bool, error) {
	if signature.Variadic() || signature.Results().Len() < 1 || signature.Results().Len() > 2 || !namedType(signature.Results().At(0).Type(), packagePath, typeName) {
		return Component{}, false, fmt.Errorf("%s: %s %s 签名无效", function.Pos, kind, function.Decl.Name.Name)
	}
	if signature.Results().Len() == 2 && !isErrorType(signature.Results().At(1).Type()) {
		return Component{}, false, fmt.Errorf("%s: %s %s 的第二返回值必须为 error", function.Pos, kind, function.Decl.Name.Name)
	}
	return newComponent(moduleKey, function, signature, kind)
}

func newComponent(moduleKey string, function SourceFunction, signature *types.Signature, kind ComponentKind) (Component, bool, error) {
	if signature.Variadic() {
		return Component{}, false, fmt.Errorf("%s: %s %s 不允许可变参数", function.Pos, kind, function.Decl.Name.Name)
	}
	if signature.Results().Len() < 1 || signature.Results().Len() > 2 {
		return Component{}, false, fmt.Errorf("%s: %s %s 必须返回 T 或 (T, error)", function.Pos, kind, function.Decl.Name.Name)
	}
	output := signature.Results().At(0).Type()
	hasError := signature.Results().Len() == 2
	if hasError && !isErrorType(signature.Results().At(1).Type()) {
		return Component{}, false, fmt.Errorf("%s: %s %s 的第二返回值必须为 error", function.Pos, kind, function.Decl.Name.Name)
	}
	parameters := make([]Parameter, 0, signature.Params().Len())
	for index := 0; index < signature.Params().Len(); index++ {
		parameter := signature.Params().At(index)
		parameters = append(parameters, Parameter{Name: parameter.Name(), Type: typeID(parameter.Type()), Raw: parameter.Type()})
	}
	return Component{
		Kind:       kind,
		ModuleKey:  moduleKey,
		Package:    function.Package,
		Function:   function.Decl.Name.Name,
		ImportPath: function.Package.PkgPath,
		Output:     typeID(output),
		OutputType: output,
		Parameters: parameters,
		HasError:   hasError,
		Position:   function.Pos,
	}, true, nil
}

func findNamedInterface(project *Project, packagePath string, name string) *types.Interface {
	visited := make(map[string]struct{})
	var visit func(*types.Package) *types.Interface
	visit = func(current *types.Package) *types.Interface {
		if current == nil {
			return nil
		}
		if _, ok := visited[current.Path()]; ok {
			return nil
		}
		visited[current.Path()] = struct{}{}
		if current.Path() == packagePath {
			if object := current.Scope().Lookup(name); object != nil {
				if named, ok := object.Type().(*types.Named); ok {
					if contract, ok := named.Underlying().(*types.Interface); ok {
						return contract.Complete()
					}
				}
			}
		}
		for _, imported := range current.Imports() {
			if found := visit(imported); found != nil {
				return found
			}
		}
		return nil
	}
	for _, discovered := range project.Modules {
		for _, loadedPackage := range discovered.Packages {
			if found := visit(loadedPackage.Types); found != nil {
				return found
			}
		}
	}
	return nil
}

func implementsInterface(typ types.Type, contract *types.Interface) bool {
	return types.Implements(typ, contract) || types.Implements(types.NewPointer(typ), contract)
}

func isRuntimeType(typ types.Type, contract *types.Interface) bool {
	if contract != nil && implementsInterface(typ, contract) {
		return true
	}
	methodSet := types.NewMethodSet(typ)
	if methodSet.Lookup(nil, "Start") == nil || methodSet.Lookup(nil, "Stop") == nil {
		methodSet = types.NewMethodSet(types.NewPointer(typ))
	}
	for _, name := range []string{"Start", "Stop"} {
		selection := methodSet.Lookup(nil, name)
		if selection == nil {
			return false
		}
		signature, ok := selection.Obj().Type().(*types.Signature)
		if !ok || signature.Params().Len() != 1 || signature.Results().Len() != 1 || !isErrorType(signature.Results().At(0).Type()) {
			return false
		}
		if !namedType(signature.Params().At(0).Type(), "context", "Context") {
			return false
		}
	}
	return true
}
