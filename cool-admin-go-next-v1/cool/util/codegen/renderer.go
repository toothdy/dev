package codegen

import (
	"fmt"
	"go/format"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	contextPackagePath         = "context"
	gdbPackagePath             = "github.com/gogf/gf/v2/database/gdb"
	authPackagePath            = "github.com/toothdy/cool-admin-go-next/cool/security"
	controllerPackagePathFixed = "github.com/toothdy/cool-admin-go-next/cool/controller"
	middlewarePackagePathFixed = "github.com/toothdy/cool-admin-go-next/cool/middleware"
	recyclePackagePath         = "github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	syncPackagePath            = "sync"
)

// RenderProject 只渲染项目级集中装配文件。
func RenderProject(project *Project, analyses []Analysis, graph *ProjectGraph) ([]GeneratedFile, error) {
	if project == nil || graph == nil || len(analyses) == 0 {
		return nil, fmt.Errorf("项目渲染输入不完整")
	}
	content, err := renderGlobalAssembly(project, analyses, graph)
	if err != nil {
		return nil, err
	}
	return []GeneratedFile{{
		Path:    filepath.Join(project.Root, "modules", globalGeneratedFile),
		Content: content,
	}}, nil
}

func renderGlobalAssembly(project *Project, analyses []Analysis, graph *ProjectGraph) ([]byte, error) {
	analysisByKey := make(map[string]Analysis, len(analyses))
	for _, analysis := range analyses {
		analysisByKey[analysis.Module.Key] = analysis
	}
	aliases := projectImportAliases(analyses, graph)
	variables := componentVariables(projectComponents(graph))
	var body strings.Builder
	body.WriteString("//go:build !cool_generate\n\n")
	body.WriteString(generatedHeader + "\n\n")
	body.WriteString("package modules\n\n")
	body.WriteString("import (\n")
	paths := make([]string, 0, len(aliases))
	for importPath := range aliases {
		paths = append(paths, importPath)
	}
	sort.Strings(paths)
	for _, importPath := range paths {
		body.WriteString(fmt.Sprintf("\t%s %q\n", aliases[importPath], importPath))
	}
	body.WriteString(")\n\n")
	renderProjectDependencyType(&body)
	for _, key := range graph.ModuleOrder {
		renderModuleAssemblyType(&body, analysisByKey[key], graph, aliases, variables)
	}
	body.WriteString("type projectAssembly struct {\n")
	for _, key := range graph.ModuleOrder {
		body.WriteString(fmt.Sprintf("\t%s %sAssembly\n", moduleStateField(key), packageIdentifier(key)))
	}
	body.WriteString("}\n\n")
	for _, key := range graph.ModuleOrder {
		renderConfigLoader(&body, analysisByKey[key], aliases)
	}
	for _, node := range graph.Nodes {
		if isCachedComponent(node.Component) {
			renderProjectProviderGetter(&body, node, aliases, variables)
		}
	}
	for _, key := range graph.ModuleOrder {
		renderTaskHandlerBuilder(&body, analysisByKey[key], graph, aliases, variables)
	}
	renderSpecs(&body, project, analysisByKey, graph, aliases, variables)
	formatted, err := format.Source([]byte(body.String()))
	if err != nil {
		return nil, fmt.Errorf("格式化集中模块生成代码失败: %w\n%s", err, body.String())
	}
	return formatted, nil
}

func projectImportAliases(analyses []Analysis, graph *ProjectGraph) map[string]string {
	aliases := map[string]string{
		contextPackagePath:         "context",
		gdbPackagePath:             "gdb",
		authPackagePath:            "auth",
		controllerPackagePathFixed: "controller",
		modelPackagePath:           "entity",
		recyclePackagePath:         "recycle",
		registryPackagePath:        "module",
		syncPackagePath:            "sync",
		taskPackagePath:            "task",
	}
	hasMiddleware := false
	paths := make(map[string]struct{})
	for _, analysis := range analyses {
		paths[configImportPath(analysis)] = struct{}{}
	}
	for _, node := range graph.Nodes {
		component := node.Component
		if component.Kind == ComponentMiddleware || component.Kind == ComponentGlobalMiddleware {
			hasMiddleware = true
		}
		if component.Kind != ComponentConfig && component.ImportPath != "" {
			paths[component.ImportPath] = struct{}{}
		}
		if component.OutputType != nil {
			collectTypePackages(component.OutputType, paths)
		}
		for _, parameter := range component.Parameters {
			if parameter.Raw != nil {
				collectTypePackages(parameter.Raw, paths)
			}
		}
	}
	if hasMiddleware {
		aliases[middlewarePackagePathFixed] = "middleware"
	}
	for fixed := range aliases {
		delete(paths, fixed)
	}
	delete(paths, "")
	ordered := make([]string, 0, len(paths))
	for importPath := range paths {
		ordered = append(ordered, importPath)
	}
	sort.Strings(ordered)
	used := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		used[alias] = struct{}{}
	}
	for _, importPath := range ordered {
		base := packageIdentifier(filepath.Base(importPath))
		alias := base
		for index := 2; ; index++ {
			if _, exists := used[alias]; !exists {
				break
			}
			alias = fmt.Sprintf("%s%d", base, index)
		}
		used[alias] = struct{}{}
		aliases[importPath] = alias
	}
	return aliases
}

func collectTypePackages(typ types.Type, paths map[string]struct{}) {
	_ = types.TypeString(typ, func(pkg *types.Package) string {
		if pkg != nil {
			paths[pkg.Path()] = struct{}{}
		}
		return ""
	})
}

func renderProjectDependencyType(body *strings.Builder) {
	body.WriteString("type moduleDependencies struct {\n")
	body.WriteString("\tcontext context.Context\n")
	body.WriteString("\tdb gdb.DB\n")
	body.WriteString("\tauthManager *auth.Manager\n")
	body.WriteString("\tsessionStore auth.SessionStore\n")
	body.WriteString("\tcontrollerProvider module.ControllerProvider\n")
	body.WriteString("\tuploadDirectory module.UploadDirectory\n")
	body.WriteString("\trecycleManager *recycle.Manager\n")
	body.WriteString("\tmodels []entity.Definition\n")
	body.WriteString("\ttaskHandlers []task.HandlerDefinition\n")
	body.WriteString("\tmiddlewareDeps module.MiddlewareDeps\n")
	body.WriteString("\tauthOptions module.AuthOptions\n")
	body.WriteString("\ti18nOptions module.I18nOptions\n")
	body.WriteString("\tcrudOptions module.CRUDOptions\n")
	body.WriteString("\tredisDefault module.RedisDefaultConfig\n")
	body.WriteString("}\n\n")
	body.WriteString("func runtimeDependencies(source module.RuntimeDeps) moduleDependencies {\n")
	body.WriteString("\treturn moduleDependencies{context: source.Context, db: source.DB, recycleManager: source.Recycle, models: source.Models, taskHandlers: source.TaskHandlers, authOptions: source.AuthOptions, i18nOptions: source.I18nOptions, crudOptions: source.CRUDOptions, redisDefault: source.RedisDefault}\n")
	body.WriteString("}\n\n")
	body.WriteString("func controllerDependencies(source module.Deps) moduleDependencies {\n")
	body.WriteString("\treturn moduleDependencies{context: source.Context, db: source.DB, authManager: source.AuthManager, sessionStore: source.SessionStore, controllerProvider: source.EPSProvider, uploadDirectory: source.UploadDirectory, recycleManager: source.Recycle, models: source.Models, authOptions: source.AuthOptions, i18nOptions: source.I18nOptions, crudOptions: source.CRUDOptions, redisDefault: source.RedisDefault}\n")
	body.WriteString("}\n\n")
	body.WriteString("func middlewareDependencies(source module.MiddlewareDeps) moduleDependencies {\n")
	body.WriteString("\treturn moduleDependencies{context: source.Context, db: source.DB, authManager: source.AuthManager, sessionStore: source.SessionStore, recycleManager: source.Recycle, models: source.Models, middlewareDeps: source, authOptions: source.AuthOptions, i18nOptions: source.I18nOptions, crudOptions: source.CRUDOptions, redisDefault: source.RedisDefault}\n")
	body.WriteString("}\n\n")
}

func renderModuleAssemblyType(body *strings.Builder, analysis Analysis, graph *ProjectGraph, aliases map[string]string, variables map[string]string) {
	body.WriteString(fmt.Sprintf("type %sAssembly struct {\n", packageIdentifier(analysis.Module.Key)))
	body.WriteString("\tconfigOnce sync.Once\n")
	body.WriteString(fmt.Sprintf("\tconfig %s\n", renderType(analysis.Declaration.ConfigType, aliases)))
	body.WriteString("\tconfigErr error\n")
	handlers := componentsForModule(analysis.Module.Key, graph.Nodes, ComponentHandler)
	if len(handlers) > 0 {
		body.WriteString("\ttaskHandlers []task.HandlerDefinition\n")
	}
	for _, node := range graph.Nodes {
		component := node.Component
		if component.ModuleKey != analysis.Module.Key || component.Kind == ComponentConfig || component.Kind == ComponentHandler {
			continue
		}
		variable := variables[componentID(component)]
		if component.Kind == ComponentModel {
			body.WriteString(fmt.Sprintf("\t%s %s\n", variable, renderType(component.OutputType, aliases)))
			continue
		}
		if isCachedComponent(component) {
			body.WriteString(fmt.Sprintf("\t%sOnce sync.Once\n", variable))
			body.WriteString(fmt.Sprintf("\t%s %s\n", variable, renderType(component.OutputType, aliases)))
			body.WriteString(fmt.Sprintf("\t%sErr error\n", variable))
		}
	}
	body.WriteString("}\n\n")
}

func renderConfigLoader(body *strings.Builder, analysis Analysis, aliases map[string]string) {
	stateType := packageIdentifier(analysis.Module.Key) + "Assembly"
	rootAlias := aliases[configImportPath(analysis)]
	body.WriteString(fmt.Sprintf("func (a *%s) configure(ctx context.Context) error {\n", stateType))
	body.WriteString("\ta.configOnce.Do(func() {\n")
	body.WriteString(fmt.Sprintf("\t\tdeclaration := %s.ModuleConfig()\n", rootAlias))
	body.WriteString(fmt.Sprintf("\t\ta.config, a.configErr = module.LoadConfig(ctx, %q, declaration.Defaults)\n", analysis.Module.Key))
	body.WriteString("\t})\n")
	body.WriteString("\treturn a.configErr\n")
	body.WriteString("}\n\n")
}

func renderProjectProviderGetter(body *strings.Builder, node ResolvedNode, aliases map[string]string, variables map[string]string) {
	component := node.Component
	state := "a." + moduleStateField(component.ModuleKey)
	variable := variables[componentID(component)]
	body.WriteString(fmt.Sprintf("func (a *projectAssembly) get%s(deps moduleDependencies) (%s, error) {\n", exportedIdentifier(variable), renderType(component.OutputType, aliases)))
	body.WriteString(fmt.Sprintf("\t%s.%sOnce.Do(func() {\n", state, variable))
	arguments := renderProjectDependencies(body, node.Dependencies, variables, variable, "\t\t", true, "")
	call := componentCall(component, aliases, arguments)
	if component.HasError {
		body.WriteString(fmt.Sprintf("\t\t%s.%s, %s.%sErr = %s\n", state, variable, state, variable, call))
	} else {
		body.WriteString(fmt.Sprintf("\t\t%s.%s = %s\n", state, variable, call))
	}
	body.WriteString("\t})\n")
	body.WriteString(fmt.Sprintf("\treturn %s.%s, %s.%sErr\n", state, variable, state, variable))
	body.WriteString("}\n\n")
}

func renderTaskHandlerBuilder(body *strings.Builder, analysis Analysis, graph *ProjectGraph, aliases map[string]string, variables map[string]string) {
	handlers := componentsForModule(analysis.Module.Key, graph.Nodes, ComponentHandler)
	if len(handlers) == 0 {
		return
	}
	state := "a." + moduleStateField(analysis.Module.Key)
	body.WriteString(fmt.Sprintf("func (a *projectAssembly) configure%sHandlers(ctx context.Context) error {\n", exportedIdentifier(packageIdentifier(analysis.Module.Key))))
	if componentsNeedDependencies(graph, handlers) {
		body.WriteString("\tdeps := moduleDependencies{context: ctx}\n")
	}
	for index, handler := range handlers {
		dependencies := dependenciesForProject(graph, handler)
		arguments := renderProjectDependencies(body, dependencies, variables, variables[componentID(handler)], "\t", false, "return err")
		call := componentCall(handler, aliases, arguments)
		if handler.HasError {
			body.WriteString(fmt.Sprintf("\tdefinition%d, err := %s\n", index, call))
			body.WriteString("\tif err != nil { return err }\n")
		} else {
			body.WriteString(fmt.Sprintf("\tdefinition%d := %s\n", index, call))
		}
		body.WriteString(fmt.Sprintf("\t%s.taskHandlers[%d] = definition%d\n", state, index, index))
	}
	body.WriteString("\treturn nil\n")
	body.WriteString("}\n\n")
}

func renderSpecs(body *strings.Builder, project *Project, analyses map[string]Analysis, graph *ProjectGraph, aliases map[string]string, variables map[string]string) {
	body.WriteString("// Specs 返回稳定排序且装配状态独立的模块声明。\n")
	body.WriteString("func Specs() []module.Spec {\n")
	body.WriteString("\tassembly := &projectAssembly{}\n")
	for _, node := range graph.Nodes {
		if node.Component.Kind == ComponentModel {
			state := "assembly." + moduleStateField(node.Component.ModuleKey)
			body.WriteString(fmt.Sprintf("\t%s.%s = %s\n", state, variables[componentID(node.Component)], componentCall(node.Component, aliases, nil)))
		}
	}
	for _, key := range graph.ModuleOrder {
		handlers := componentsForModule(key, graph.Nodes, ComponentHandler)
		if len(handlers) > 0 {
			body.WriteString(fmt.Sprintf("\tassembly.%s.taskHandlers = make([]task.HandlerDefinition, %d)\n", moduleStateField(key), len(handlers)))
		}
	}
	body.WriteString(fmt.Sprintf("\tspecs := make([]module.Spec, 0, %d)\n", len(graph.ModuleOrder)))
	for _, key := range graph.ModuleOrder {
		renderSpec(body, project, analyses[key], graph, aliases, variables)
	}
	body.WriteString("\treturn specs\n")
	body.WriteString("}\n")
}

func renderSpec(body *strings.Builder, project *Project, analysis Analysis, graph *ProjectGraph, aliases map[string]string, variables map[string]string) {
	key := analysis.Module.Key
	specVariable := "spec" + exportedIdentifier(packageIdentifier(key))
	state := "assembly." + moduleStateField(key)
	models := componentsForModule(key, graph.Nodes, ComponentModel)
	handlers := componentsForModule(key, graph.Nodes, ComponentHandler)
	runtimes := componentsForModule(key, graph.Nodes, ComponentRuntime)
	controllers := componentsForModule(key, graph.Nodes, ComponentController)
	globalMiddlewares := declaredComponents(analysis, ComponentGlobalMiddleware)
	middlewares := declaredComponents(analysis, ComponentMiddleware)
	recycleProvider := findModuleRecycleProvider(key, graph.Nodes)
	body.WriteString(fmt.Sprintf("\t%s := module.Spec{\n", specVariable))
	body.WriteString(fmt.Sprintf("\t\tKey: %q, Name: %q, Description: %q, Order: %d,\n", key, analysis.Declaration.Name, analysis.Declaration.Description, analysis.Declaration.Order))
	body.WriteString("\t\tModels: []entity.Definition{\n")
	for _, component := range models {
		body.WriteString(fmt.Sprintf("\t\t\t%s.%s,\n", state, variables[componentID(component)]))
	}
	body.WriteString("\t\t},\n")
	body.WriteString("\t\tConfigure: func(ctx context.Context) error {\n")
	body.WriteString(fmt.Sprintf("\t\t\tif err := %s.configure(ctx); err != nil { return err }\n", state))
	if len(handlers) > 0 {
		body.WriteString(fmt.Sprintf("\t\t\treturn assembly.configure%sHandlers(ctx)\n", exportedIdentifier(packageIdentifier(key))))
	} else {
		body.WriteString("\t\t\treturn nil\n")
	}
	body.WriteString("\t\t},\n")
	if len(handlers) > 0 {
		body.WriteString(fmt.Sprintf("\t\tTaskHandlers: %s.taskHandlers,\n", state))
	}
	if _, err := os.Stat(filepath.Join(project.Root, "modules", key, "db.json")); err == nil {
		body.WriteString(fmt.Sprintf("\t\tDB: %q,\n", "modules/"+key+"/db.json"))
	}
	if _, err := os.Stat(filepath.Join(project.Root, "modules", key, "menu.json")); err == nil {
		body.WriteString(fmt.Sprintf("\t\tMenu: %q,\n", "modules/"+key+"/menu.json"))
	}
	body.WriteString("\t}\n")
	if recycleProvider != nil {
		renderProjectRecycleFactory(body, specVariable, key, *recycleProvider, runtimes, variables)
	} else if len(runtimes) > 0 {
		renderProjectRuntimeFactory(body, specVariable, key, runtimes, variables)
	}
	renderProjectControllerFactory(body, specVariable, graph, controllers, aliases, variables)
	renderProjectMiddlewareFactory(body, specVariable, graph, "GlobalMiddlewares", globalMiddlewares, aliases, variables)
	renderProjectMiddlewareFactory(body, specVariable, graph, "Middlewares", middlewares, aliases, variables)
	body.WriteString(fmt.Sprintf("\tspecs = append(specs, %s)\n", specVariable))
}

func renderProjectRuntimeFactory(body *strings.Builder, specVariable string, moduleKey string, runtimes []Component, variables map[string]string) {
	body.WriteString(fmt.Sprintf("\t%s.Runtime = func(source module.RuntimeDeps) (module.Runtime, error) {\n", specVariable))
	body.WriteString("\t\tdeps := runtimeDependencies(source)\n")
	renderProjectRuntimeValues(body, moduleKey, runtimes, variables, "\t\t")
	body.WriteString("\t}\n")
}

func renderProjectRecycleFactory(body *strings.Builder, specVariable string, moduleKey string, provider Component, runtimes []Component, variables map[string]string) {
	body.WriteString(fmt.Sprintf("\t%s.RecycleProvider = func(source module.RuntimeDeps) (*recycle.Manager, module.Runtime, error) {\n", specVariable))
	body.WriteString("\t\tdeps := runtimeDependencies(source)\n")
	body.WriteString(fmt.Sprintf("\t\tmanager, err := assembly.get%s(deps)\n", exportedIdentifier(variables[componentID(provider)])))
	body.WriteString("\t\tif err != nil { return nil, nil, err }\n")
	if len(runtimes) == 0 {
		body.WriteString("\t\treturn manager, nil, nil\n")
	} else {
		for index, runtime := range runtimes {
			body.WriteString(fmt.Sprintf("\t\truntime%d, err := assembly.get%s(deps)\n", index, exportedIdentifier(variables[componentID(runtime)])))
			body.WriteString("\t\tif err != nil { return nil, nil, err }\n")
		}
		if len(runtimes) == 1 {
			body.WriteString("\t\treturn manager, runtime0, nil\n")
		} else {
			body.WriteString(fmt.Sprintf("\t\treturn manager, module.NewRuntimeGroup(%q,\n", moduleKey))
			for index, runtime := range runtimes {
				body.WriteString(fmt.Sprintf("\t\t\tregistry.RuntimeDefinition{Name: %q, Runtime: runtime%d},\n", runtime.Function, index))
			}
			body.WriteString("\t\t), nil\n")
		}
	}
	body.WriteString("\t}\n")
}

func renderProjectRuntimeValues(body *strings.Builder, moduleKey string, runtimes []Component, variables map[string]string, indent string) {
	for index, runtime := range runtimes {
		body.WriteString(fmt.Sprintf("%sruntime%d, err := assembly.get%s(deps)\n", indent, index, exportedIdentifier(variables[componentID(runtime)])))
		body.WriteString(fmt.Sprintf("%sif err != nil { return nil, err }\n", indent))
	}
	if len(runtimes) == 1 {
		body.WriteString(fmt.Sprintf("%sreturn runtime0, nil\n", indent))
		return
	}
	body.WriteString(fmt.Sprintf("%sreturn module.NewRuntimeGroup(%q,\n", indent, moduleKey))
	for index, runtime := range runtimes {
		body.WriteString(fmt.Sprintf("%s\tregistry.RuntimeDefinition{Name: %q, Runtime: runtime%d},\n", indent, runtime.Function, index))
	}
	body.WriteString(fmt.Sprintf("%s), nil\n", indent))
}

func renderProjectControllerFactory(body *strings.Builder, specVariable string, graph *ProjectGraph, controllers []Component, aliases map[string]string, variables map[string]string) {
	body.WriteString(fmt.Sprintf("\t%s.Controllers = func(source module.Deps) ([]controller.Definition, error) {\n", specVariable))
	if componentsNeedDependencies(graph, controllers) {
		body.WriteString("\t\tdeps := controllerDependencies(source)\n")
	}
	body.WriteString("\t\tdefinitions := make([]controller.Definition, 0)\n")
	for index, component := range controllers {
		arguments := renderProjectDependencies(body, dependenciesForProject(graph, component), variables, variables[componentID(component)], "\t\t", false, "return nil, err")
		call := componentCall(component, aliases, arguments)
		if component.HasError {
			body.WriteString(fmt.Sprintf("\t\tdefinition%d, err := %s\n", index, call))
			body.WriteString("\t\tif err != nil { return nil, err }\n")
		} else {
			body.WriteString(fmt.Sprintf("\t\tdefinition%d := %s\n", index, call))
		}
		body.WriteString(fmt.Sprintf("\t\tdefinitions = append(definitions, definition%d)\n", index))
	}
	body.WriteString("\t\treturn definitions, nil\n")
	body.WriteString("\t}\n")
}

func renderProjectMiddlewareFactory(body *strings.Builder, specVariable string, graph *ProjectGraph, field string, components []Component, aliases map[string]string, variables map[string]string) {
	if len(components) == 0 {
		return
	}
	body.WriteString(fmt.Sprintf("\t%s.%s = func(source module.MiddlewareDeps) ([]middleware.Definition, error) {\n", specVariable, field))
	if componentsNeedDependencies(graph, components) {
		body.WriteString("\t\tdeps := middlewareDependencies(source)\n")
	}
	body.WriteString("\t\tdefinitions := make([]middleware.Definition, 0)\n")
	for index, component := range components {
		arguments := renderProjectDependencies(body, dependenciesForProject(graph, component), variables, variables[componentID(component)], "\t\t", false, "return nil, err")
		call := componentCall(component, aliases, arguments)
		if component.HasError {
			body.WriteString(fmt.Sprintf("\t\tdefinition%d, err := %s\n", index, call))
			body.WriteString("\t\tif err != nil { return nil, err }\n")
		} else {
			body.WriteString(fmt.Sprintf("\t\tdefinition%d := %s\n", index, call))
		}
		if _, ok := component.OutputType.(*types.Slice); ok {
			body.WriteString(fmt.Sprintf("\t\tdefinitions = append(definitions, definition%d...)\n", index))
		} else {
			body.WriteString(fmt.Sprintf("\t\tdefinitions = append(definitions, definition%d)\n", index))
		}
	}
	body.WriteString("\t\treturn definitions, nil\n")
	body.WriteString("\t}\n")
}

func renderProjectDependencies(body *strings.Builder, dependencies []Dependency, variables map[string]string, owner string, indent string, cached bool, onError string) []string {
	arguments := make([]string, 0, len(dependencies))
	for index, dependency := range dependencies {
		if dependency.Kind == DependencyFramework || dependency.Kind == DependencyLazy {
			arguments = append(arguments, frameworkExpression(dependency.Parameter))
			continue
		}
		source := dependency.Source
		state := "a." + moduleStateField(source.ModuleKey)
		if !cached {
			state = "assembly." + moduleStateField(source.ModuleKey)
		}
		switch source.Kind {
		case ComponentConfig:
			arguments = append(arguments, state+".config")
		case ComponentModel:
			arguments = append(arguments, state+"."+variables[componentID(source)])
		default:
			local := safeIdentifier(dependency.Parameter.Name + exportedIdentifier(owner))
			if local == "" {
				local = fmt.Sprintf("dependency%d", index)
			}
			receiver := "a"
			if !cached {
				receiver = "assembly"
			}
			body.WriteString(fmt.Sprintf("%s%s, err := %s.get%s(deps)\n", indent, local, receiver, exportedIdentifier(variables[componentID(source)])))
			if cached {
				ownerState := "a." + moduleStateField(findOwnerModule(owner, variables))
				body.WriteString(fmt.Sprintf("%sif err != nil { %s.%sErr = err; return }\n", indent, ownerState, owner))
			} else {
				body.WriteString(fmt.Sprintf("%sif err != nil { %s }\n", indent, onError))
			}
			arguments = append(arguments, local)
		}
	}
	return arguments
}

func findOwnerModule(owner string, variables map[string]string) string {
	for id, variable := range variables {
		if variable == owner {
			parts := strings.Split(id, "/modules/")
			if len(parts) == 2 {
				return strings.SplitN(parts[1], "/", 2)[0]
			}
		}
	}
	return ""
}

func componentCall(component Component, aliases map[string]string, arguments []string) string {
	return fmt.Sprintf("%s.%s(%s)", aliases[component.ImportPath], component.Function, strings.Join(arguments, ", "))
}

func componentsForModule(moduleKey string, nodes []ResolvedNode, kind ComponentKind) []Component {
	components := make([]Component, 0)
	for _, node := range nodes {
		if node.Component.ModuleKey == moduleKey && node.Component.Kind == kind {
			components = append(components, node.Component)
		}
	}
	return components
}

func declaredComponents(analysis Analysis, kind ComponentKind) []Component {
	components := make([]Component, 0)
	for _, component := range analysis.Components {
		if component.Kind == kind {
			components = append(components, component)
		}
	}
	return components
}

func findModuleRecycleProvider(moduleKey string, nodes []ResolvedNode) *Component {
	for _, node := range nodes {
		if node.Component.ModuleKey == moduleKey && isNamedPointer(node.Component.OutputType, recyclePackagePath, "Manager") {
			component := node.Component
			return &component
		}
	}
	return nil
}

func isNamedPointer(typ types.Type, packagePath string, name string) bool {
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	pointer, ok := typ.(*types.Pointer)
	return ok && namedType(pointer.Elem(), packagePath, name)
}

func isCachedComponent(component Component) bool {
	return component.Kind == ComponentProvider || component.Kind == ComponentRuntime
}

func renderType(typ types.Type, aliases map[string]string) string {
	return types.TypeString(typ, func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		return aliases[pkg.Path()]
	})
}

func projectComponents(graph *ProjectGraph) []Component {
	components := make([]Component, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		components = append(components, node.Component)
	}
	return components
}

func componentVariables(components []Component) map[string]string {
	bases := make(map[string]int, len(components))
	for _, component := range components {
		base := componentVariableBase(component)
		bases[base]++
	}
	variables := make(map[string]string, len(components))
	for _, component := range components {
		base := componentVariableBase(component)
		if bases[base] > 1 {
			base = safeIdentifier(lowerCamel(component.ModuleKey + exportedIdentifier(componentPackageSuffix(component)) + exportedIdentifier(component.Function)))
			if component.Kind == ComponentModel {
				base += "Model"
			}
		}
		variables[componentID(component)] = base
	}
	return variables
}

func componentVariableBase(component Component) string {
	base := safeIdentifier(lowerCamel(component.ModuleKey + exportedIdentifier(component.Function)))
	if component.Kind == ComponentModel {
		base += "Model"
	}
	return base
}

func componentPackageSuffix(component Component) string {
	marker := "/modules/" + component.ModuleKey + "/"
	relative := component.ImportPath
	if index := strings.Index(relative, marker); index >= 0 {
		relative = relative[index+len(marker):]
	}
	parts := strings.Split(relative, "/")
	var suffix strings.Builder
	for _, part := range parts {
		suffix.WriteString(exportedIdentifier(packageIdentifier(part)))
	}
	return suffix.String()
}

func frameworkExpression(parameter Parameter) string {
	switch parameter.Type {
	case "context.Context":
		return "deps.context"
	case "github.com/gogf/gf/v2/database/gdb.DB":
		return "deps.db"
	case "*github.com/toothdy/cool-admin-go-next/cool/security.Manager":
		return "deps.authManager"
	case "github.com/toothdy/cool-admin-go-next/cool/security.SessionStore":
		return "deps.sessionStore"
	case "github.com/toothdy/cool-admin-go-next/cool/module.ControllerProvider":
		return "deps.controllerProvider"
	case "github.com/toothdy/cool-admin-go-next/cool/module.UploadDirectory":
		return "deps.uploadDirectory"
	case "*github.com/toothdy/cool-admin-go-next/cool/db/recycle.Manager":
		return "deps.recycleManager"
	case "[]github.com/toothdy/cool-admin-go-next/cool/entity.Definition":
		return "deps.models"
	case "[]github.com/toothdy/cool-admin-go-next/cool/task.HandlerDefinition":
		return "deps.taskHandlers"
	case "github.com/toothdy/cool-admin-go-next/cool/module.MiddlewareDeps":
		return "deps.middlewareDeps"
	case "github.com/toothdy/cool-admin-go-next/cool/module.AuthOptions":
		return "deps.authOptions"
	case "github.com/toothdy/cool-admin-go-next/cool/module.I18nOptions":
		return "deps.i18nOptions"
	case "github.com/toothdy/cool-admin-go-next/cool/module.CRUDOptions":
		return "deps.crudOptions"
	case "github.com/toothdy/cool-admin-go-next/cool/module.RedisDefaultConfig":
		return "deps.redisDefault"
	default:
		return "nil"
	}
}

func dependenciesForProject(graph *ProjectGraph, component Component) []Dependency {
	for _, node := range graph.Nodes {
		if componentID(node.Component) == componentID(component) {
			return node.Dependencies
		}
	}
	return nil
}

func componentsNeedDependencies(graph *ProjectGraph, components []Component) bool {
	for _, component := range components {
		if len(dependenciesForProject(graph, component)) > 0 {
			return true
		}
	}
	return false
}

func moduleStateField(key string) string {
	return safeIdentifier(lowerCamel(key))
}

func safeIdentifier(value string) string {
	var builder strings.Builder
	for index, current := range value {
		if unicode.IsLetter(current) || current == '_' || (index > 0 && unicode.IsDigit(current)) {
			builder.WriteRune(current)
		}
	}
	return builder.String()
}

func exportedIdentifier(value string) string {
	if value == "" {
		return "Value"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func packageIdentifier(value string) string {
	identifier := safeIdentifier(value)
	if identifier == "" {
		return "module"
	}
	return identifier
}
