package codegen

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const modulePackagePath = "github.com/toothdy/cool-admin-go-next/cool/module"

// ModuleDeclaration 保存静态解析后的模块根声明。
type ModuleDeclaration struct {
	Key               string
	Name              string
	Description       string
	Order             int
	Middlewares       []string
	GlobalMiddlewares []string
	ConfigType        types.Type
	Defaults          ast.Expr
	Position          token.Position
}

func analyzeModuleDeclaration(project *Project, discovered DiscoveredModule) (ModuleDeclaration, error) {
	rootPath := project.ModulePath + "/modules/" + discovered.Key
	var rootPackage *types.Package
	var loadedRootIndex int
	for index, loadedPackage := range discovered.Packages {
		if loadedPackage.PkgPath == rootPath {
			rootPackage = loadedPackage.Types
			loadedRootIndex = index
			break
		}
	}
	if rootPackage == nil {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s 根包 %s 未加载", discovered.Key, rootPath)
	}
	loadedRoot := discovered.Packages[loadedRootIndex]
	if err := validateRootConfigFile(discovered, loadedRoot, rootPath); err != nil {
		return ModuleDeclaration{}, err
	}

	var source *SourceFunction
	for _, function := range packageFunctions(discovered.Dir, loadedRoot) {
		if function.Path == "config.go" && function.Decl.Name.Name == "ModuleConfig" {
			if source != nil {
				return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: ModuleConfig 重复声明", discovered.Key, function.Pos)
			}
			copy := function
			source = &copy
		}
	}
	if source == nil {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s config.go 缺少 ModuleConfig() module.Declaration[Config]", discovered.Key)
	}
	return parseModuleDeclaration(discovered.Key, rootPath, *source)
}

func parseModuleDeclaration(moduleKey string, rootPath string, source SourceFunction) (ModuleDeclaration, error) {
	signature, err := functionSignature(source)
	if err != nil {
		return ModuleDeclaration{}, err
	}
	if signature.Params().Len() != 0 || signature.Results().Len() != 1 {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 必须无参且仅返回 module.Declaration[Config]", moduleKey, source.Pos)
	}
	declarationType, ok := types.Unalias(signature.Results().At(0).Type()).(*types.Named)
	if !ok || declarationType.Obj() == nil || declarationType.Obj().Pkg() == nil || declarationType.Obj().Pkg().Path() != modulePackagePath || declarationType.Obj().Name() != "Declaration" || declarationType.TypeArgs() == nil || declarationType.TypeArgs().Len() != 1 {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 返回值必须是 module.Declaration[Config]", moduleKey, source.Pos)
	}
	configType := types.Unalias(declarationType.TypeArgs().At(0))
	configNamed, ok := configType.(*types.Named)
	if !ok || configNamed.Obj() == nil || configNamed.Obj().Pkg() == nil || configNamed.Obj().Pkg().Path() != rootPath || configNamed.Obj().Name() != "Config" {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 泛型实参必须是当前模块根包的具体 Config", moduleKey, source.Pos)
	}
	if source.Decl.Body == nil || len(source.Decl.Body.List) != 1 {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 必须直接返回声明复合字面量", moduleKey, source.Pos)
	}
	returnStatement, ok := source.Decl.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returnStatement.Results) != 1 {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 必须直接返回声明复合字面量", moduleKey, source.Pos)
	}
	literal, ok := returnStatement.Results[0].(*ast.CompositeLit)
	if !ok || !types.Identical(source.Package.TypesInfo.TypeOf(literal), declarationType) {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 必须直接返回 module.Declaration[Config] 复合字面量", moduleKey, source.Pos)
	}
	fields := make(map[string]ast.Expr, len(literal.Elts))
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 声明必须使用字段名", moduleKey, source.Pos)
		}
		identifier, ok := keyValue.Key.(*ast.Ident)
		if !ok {
			return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 包含非法字段", moduleKey, source.Pos)
		}
		if _, exists := fields[identifier.Name]; exists {
			return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 字段 %s 重复", moduleKey, source.Pos, identifier.Name)
		}
		fields[identifier.Name] = keyValue.Value
	}
	name, err := constantString(source, fields["Name"], "Name")
	if err != nil || strings.TrimSpace(name) == "" {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config Name 必须是非空常量字符串", moduleKey, source.Pos)
	}
	description, err := constantString(source, fields["Description"], "Description")
	if err != nil || strings.TrimSpace(description) == "" {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config Description 必须是非空常量字符串", moduleKey, source.Pos)
	}
	order := 0
	if expression := fields["Order"]; expression != nil {
		value := source.Package.TypesInfo.Types[expression].Value
		parsed, exact := constant.Int64Val(value)
		if value == nil || !exact || int64(int(parsed)) != parsed {
			return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config Order 必须是 int 常量", moduleKey, source.Pos)
		}
		order = int(parsed)
	}
	defaults := fields["Defaults"]
	if defaults == nil || !types.Identical(source.Package.TypesInfo.TypeOf(defaults), configType) {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config Defaults 必须是 Config 值", moduleKey, source.Pos)
	}
	if err = validatePureDefaultExpression(source, defaults); err != nil {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Defaults 必须是纯配置值: %w", moduleKey, source.Pos, err)
	}
	if err = validateConfigType(configNamed, rootPath, map[types.Type]bool{}); err != nil {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: Config 类型无效: %w", moduleKey, source.Pos, err)
	}
	middlewares, err := componentReferences(source, fields["Middlewares"], "Middlewares")
	if err != nil {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: %w", moduleKey, source.Pos, err)
	}
	globalMiddlewares, err := componentReferences(source, fields["GlobalMiddlewares"], "GlobalMiddlewares")
	if err != nil {
		return ModuleDeclaration{}, fmt.Errorf("模块 %s %s: %w", moduleKey, source.Pos, err)
	}
	return ModuleDeclaration{
		Key: moduleKey, Name: name, Description: description, Order: order,
		Middlewares: middlewares, GlobalMiddlewares: globalMiddlewares,
		ConfigType: configType, Defaults: defaults, Position: source.Pos,
	}, nil
}

func constantString(source SourceFunction, expression ast.Expr, field string) (string, error) {
	if expression == nil {
		return "", fmt.Errorf("字段 %s 缺失", field)
	}
	value := source.Package.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.String {
		return "", fmt.Errorf("字段 %s 不是常量字符串", field)
	}
	return constant.StringVal(value), nil
}

func componentReferences(source SourceFunction, expression ast.Expr, field string) ([]string, error) {
	if expression == nil {
		return nil, nil
	}
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s 必须是组件引用复合字面量", field)
	}
	result := make([]string, 0, len(literal.Elts))
	for _, element := range literal.Elts {
		value := source.Package.TypesInfo.Types[element].Value
		if value == nil || value.Kind() != constant.String {
			return nil, fmt.Errorf("%s 的组件引用必须是常量字符串", field)
		}
		result = append(result, constant.StringVal(value))
	}
	return result, nil
}

func validateConfigType(typ types.Type, rootPath string, visited map[types.Type]bool) error {
	typ = types.Unalias(typ)
	if visited[typ] {
		return nil
	}
	visited[typ] = true
	switch current := typ.(type) {
	case *types.Basic:
		return nil
	case *types.Pointer:
		return validateConfigType(current.Elem(), rootPath, visited)
	case *types.Slice, *types.Array:
		var element types.Type
		if slice, ok := current.(*types.Slice); ok {
			element = slice.Elem()
		} else {
			element = current.(*types.Array).Elem()
		}
		return validateConfigType(element, rootPath, visited)
	case *types.Map:
		if basic, ok := types.Unalias(current.Key()).(*types.Basic); !ok || basic.Kind() != types.String {
			return fmt.Errorf("Map 只允许 string 键")
		}
		return validateConfigType(current.Elem(), rootPath, visited)
	case *types.Named:
		object := current.Obj()
		if object == nil || object.Pkg() == nil {
			return fmt.Errorf("类型 %s 缺少包信息", current)
		}
		if object.Pkg().Path() == "time" && object.Name() == "Duration" {
			return nil
		}
		if object.Pkg().Path() != rootPath {
			return fmt.Errorf("类型 %s 不是模块纯配置类型", typeID(current))
		}
		return validateConfigType(current.Underlying(), rootPath, visited)
	case *types.Struct:
		seenTags := make(map[string]struct{}, current.NumFields())
		for index := 0; index < current.NumFields(); index++ {
			field := current.Field(index)
			if !field.Exported() {
				continue
			}
			tag := reflect.StructTag(current.Tag(index)).Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "" || name == "-" {
				return fmt.Errorf("字段 %s 必须声明非空 json 标签", field.Name())
			}
			if _, exists := seenTags[name]; exists {
				return fmt.Errorf("json 标签 %q 重复", name)
			}
			seenTags[name] = struct{}{}
			if err := validateConfigType(field.Type(), rootPath, visited); err != nil {
				return fmt.Errorf("字段 %s: %w", field.Name(), err)
			}
		}
		return nil
	default:
		return fmt.Errorf("类型 %s 不是纯配置值", typeID(typ))
	}
}

func validatePureDefaultExpression(source SourceFunction, expression ast.Expr) error {
	var invalid error
	ast.Inspect(expression, func(node ast.Node) bool {
		if invalid != nil || node == nil {
			return false
		}
		switch current := node.(type) {
		case *ast.CallExpr, *ast.FuncLit:
			invalid = fmt.Errorf("不允许函数调用或函数字面量")
			return false
		case *ast.Ident:
			object := source.Package.TypesInfo.ObjectOf(current)
			if object != nil && source.Package.TypesInfo.TypeOf(current) != nil {
				if variable, isVariable := object.(*types.Var); isVariable && variable.Parent() == nil {
					break
				}
				if _, ok := object.(*types.Const); !ok {
					if _, isType := object.(*types.TypeName); !isType {
						invalid = fmt.Errorf("标识符 %s 不是常量或类型", current.Name)
						return false
					}
				}
			}
		case *ast.UnaryExpr:
			if current.Op == token.AND {
				if _, ok := current.X.(*ast.CompositeLit); !ok {
					invalid = fmt.Errorf("取地址只允许用于配置复合字面量")
					return false
				}
			}
		}
		return true
	})
	return invalid
}

func validateRootConfigFile(discovered DiscoveredModule, loadedRoot *packages.Package, rootPath string) error {
	for index, file := range loadedRoot.Syntax {
		path := loadedRoot.CompiledGoFiles[index]
		if !strings.HasSuffix(path, string(filepath.Separator)+"config.go") {
			continue
		}
		for _, imported := range file.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err == nil && strings.HasPrefix(value, rootPath+"/") {
				return fmt.Errorf("模块 %s config.go 禁止导入当前模块子包 %s", discovered.Key, value)
			}
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Name.Name == "ModuleConfig" {
				continue
			}
			if function.Name.IsExported() {
				return fmt.Errorf("模块 %s 根配置 config.go 禁止声明装配函数 %s", discovered.Key, function.Name.Name)
			}
		}
	}
	return nil
}
