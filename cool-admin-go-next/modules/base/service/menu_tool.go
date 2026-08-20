package service

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

var codeNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

const (
	gMetaImportPath      = "github.com/gogf/gf/v2/frame/g"
	coreEntityImportPath = "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	controllerImportPath = "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
)

// MenuColumn 是菜单代码生成页面使用的实体字段元数据。
type MenuColumn struct {
	PropertyName string `json:"propertyName"`
	Type         string `json:"type"`
	Length       string `json:"length"`
	Comment      string `json:"comment"`
	Nullable     bool   `json:"nullable"`
}

// MenuParseResult 是静态解析实体与 Controller 的结果。
type MenuParseResult struct {
	Columns   []MenuColumn `json:"columns"`
	ClassName string       `json:"className,omitempty"`
	TableName string       `json:"tableName,omitempty"`
	FileName  string       `json:"fileName,omitempty"`
	Path      string       `json:"path"`
}

// MenuCreateInput 是菜单代码创建请求。
type MenuCreateInput struct {
	Module     string `json:"module"`
	Entity     string `json:"entity"`
	Controller string `json:"controller"`
	Service    string `json:"service"`
	FileName   string `json:"fileName"`
}

// MenuTree 是菜单导入导出的稳定字段白名单。
type MenuTree struct {
	Name       string     `json:"name"`
	Router     *string    `json:"router"`
	Perms      *string    `json:"perms"`
	Type       int32      `json:"type"`
	Icon       *string    `json:"icon"`
	OrderNum   int32      `json:"orderNum"`
	ViewPath   *string    `json:"viewPath"`
	KeepAlive  bool       `json:"keepAlive"`
	IsShow     bool       `json:"isShow"`
	ChildMenus []MenuTree `json:"childMenus"`
}

type menuExportRow struct {
	ID        uint64  `orm:"id"`
	ParentID  *uint64 `orm:"parentId"`
	Name      string  `orm:"name"`
	Router    *string `orm:"router"`
	Perms     *string `orm:"perms"`
	Type      int32   `orm:"type"`
	Icon      *string `orm:"icon"`
	OrderNum  int32   `orm:"orderNum"`
	ViewPath  *string `orm:"viewPath"`
	KeepAlive bool    `orm:"keepAlive"`
	IsShow    bool    `orm:"isShow"`
}

type parsedEntity struct {
	className string
	tableName string
	columns   []MenuColumn
}

// MenuToolService 提供菜单代码工具及菜单树导入导出。
type MenuToolService struct {
	menu   *coreservice.Base[entity.Menu, uint64]
	coding *CodingService
}

// NewMenuTool 创建菜单工具服务。
func NewMenuTool(
	menuBase *coreservice.Base[entity.Menu, uint64],
	config base.Config,
) (*MenuToolService, error) {
	if menuBase == nil || menuBase.Descriptor() == nil {
		return nil, exception.Core("菜单基础 Service 无效")
	}
	coding, err := NewCoding(config)
	if err != nil {
		return nil, err
	}

	return &MenuToolService{menu: menuBase, coding: coding}, nil
}

// Parse 静态提取实体列与 Controller 路径，不编译或执行输入源码。
func (service *MenuToolService) Parse(entitySource, controllerSource, moduleName string) (MenuParseResult, error) {
	if !validCodeName(moduleName) {
		return MenuParseResult{}, exception.Validate("模块名称无效")
	}
	parsed, err := parseMenuEntity(entitySource)
	if err != nil {
		return MenuParseResult{}, err
	}
	fileName := tableFileName(parsed.tableName)
	result := MenuParseResult{Columns: parsed.columns}
	if strings.TrimSpace(controllerSource) == "" {
		result.ClassName = parsed.className
		result.TableName = parsed.tableName
		result.FileName = fileName
		result.Path = "/admin/" + moduleName + "/" + fileName

		return result, nil
	}
	controllerPath, err := parseAdminControllerPath(controllerSource)
	if err != nil {
		return MenuParseResult{}, err
	}
	if controllerPath == "" {
		controllerPath = fileName
	}
	result.Path = "/admin/" + moduleName + "/" + strings.Trim(controllerPath, "/")

	return result, nil
}

// Create 校验并创建实体、Controller、Service 和缺失的模块配置。
func (service *MenuToolService) Create(input MenuCreateInput) error {
	if service == nil || service.coding == nil {
		return exception.Core("菜单工具服务未初始化")
	}
	if !validCodeName(input.Module) || !validCodeName(input.FileName) {
		return exception.Validate("模块名称或文件名称无效")
	}
	if err := validateSourcePackage(input.Entity, "entity", "实体"); err != nil {
		return err
	}
	if err := validateSourcePackage(input.Controller, "admin", "Controller"); err != nil {
		return err
	}
	if err := validateSourcePackage(input.Service, "service", "Service"); err != nil {
		return err
	}

	service.coding.mu.Lock()
	defer service.coding.mu.Unlock()
	codes := []CodeFile{
		{Path: path.Join("modules", input.Module, "entity", input.FileName+".go"), Content: input.Entity},
		{Path: path.Join("modules", input.Module, "controller", "admin", input.FileName+".go"), Content: input.Controller},
		{Path: path.Join("modules", input.Module, "service", input.FileName+".go"), Content: input.Service},
	}
	configPath := path.Join("modules", input.Module, "config.go")
	exists, err := service.validExistingConfig(configPath, input.Module)
	if err != nil {
		return err
	}
	if !exists {
		codes = append(codes, CodeFile{Path: configPath, Content: moduleConfigSource(input.Module)})
	}

	return service.coding.createGoFiles(codes)
}

// Export 将选中菜单稳定导出为不含维护字段的树。
func (service *MenuToolService) Export(ctx context.Context, ids []uint64) ([]MenuTree, error) {
	if service == nil || service.menu == nil {
		return nil, exception.Core("菜单工具服务未初始化")
	}
	if len(ids) == 0 {
		return []MenuTree{}, nil
	}
	model, err := service.menu.Model(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]menuExportRow, 0, len(ids))
	err = model.
		Fields("id", "parentId", "name", "router", "perms", "type", "icon", "orderNum", "viewPath", "keepAlive", "isShow").
		WhereIn("id", ids).
		Scan(&rows)
	if err != nil {
		return nil, exception.WrapCore(err, "查询导出菜单失败")
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].OrderNum != rows[right].OrderNum {
			return rows[left].OrderNum < rows[right].OrderNum
		}

		return rows[left].ID < rows[right].ID
	})
	children := make(map[uint64][]menuExportRow)
	roots := make([]menuExportRow, 0, len(rows))
	for _, row := range rows {
		if row.ParentID == nil {
			roots = append(roots, row)
			continue
		}
		children[*row.ParentID] = append(children[*row.ParentID], row)
	}
	result := make([]MenuTree, 0, len(roots))
	for _, root := range roots {
		result = append(result, buildMenuTree(root, children, make(map[uint64]bool)))
	}

	return result, nil
}

// Import 在调用方事务中插入菜单树，并用实际新 ID 重建父子关系。
func (service *MenuToolService) Import(ctx context.Context, menus []MenuTree) error {
	if service == nil || service.menu == nil {
		return exception.Core("菜单工具服务未初始化")
	}
	if _, err := service.menu.Tx(ctx); err != nil {
		return err
	}
	model, err := service.menu.Model(ctx)
	if err != nil {
		return err
	}
	for index := range menus {
		if err = importMenuTree(model, service.menu.Descriptor(), &menus[index], nil); err != nil {
			return err
		}
	}

	return nil
}

func parseMenuEntity(source string) (parsedEntity, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "entity.go", source, parser.AllErrors)
	if err != nil {
		return parsedEntity{}, exception.WrapValidate(err, "实体 Go 源码语法无效")
	}
	imports := sourceImports(file)
	type candidate struct {
		name    string
		value   *ast.StructType
		meta    reflect.StructTag
		hasBase bool
	}
	candidates := make([]candidate, 0, 1)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			meta, hasMeta := embeddedTag(structure, imports, gMetaImportPath, "Meta")
			if !hasMeta {
				continue
			}
			_, hasBase := embeddedTag(structure, imports, coreEntityImportPath, "Base")
			candidates = append(candidates, candidate{name: typeSpec.Name.Name, value: structure, meta: meta, hasBase: hasBase})
		}
	}
	if len(candidates) != 1 {
		return parsedEntity{}, exception.Validate("实体源码必须包含且只包含一个带 g.Meta 的 struct")
	}
	current := candidates[0]
	if !current.hasBase {
		return parsedEntity{}, exception.Validate("实体必须嵌入 coreentity.Base")
	}
	tableName := structTagOption(current.meta.Get("orm"), "table")
	if !validCodeName(tableName) {
		return parsedEntity{}, exception.Validate("实体 g.Meta 缺少 orm table")
	}
	columns := []MenuColumn{{PropertyName: "id", Type: "number", Comment: "ID"}}
	for _, field := range current.value.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		tag, err := parseASTStructTag(field.Tag)
		if err != nil {
			return parsedEntity{}, err
		}
		jsonName := strings.Split(tag.Get("json"), ",")[0]
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = lowerFirst(field.Names[0].Name)
		}
		typeName, nullable := menuColumnType(field.Type, tag.Get("cool"))
		if typeName == "" {
			return parsedEntity{}, exception.Validate("不支持的实体字段类型: " + field.Names[0].Name)
		}
		columns = append(columns, MenuColumn{
			PropertyName: jsonName,
			Type:         typeName,
			Length:       structTagOption(tag.Get("cool"), "size"),
			Comment:      tag.Get("description"),
			Nullable:     nullable,
		})
	}
	columns = append(columns,
		MenuColumn{PropertyName: "createTime", Type: "date", Comment: "创建时间", Nullable: true},
		MenuColumn{PropertyName: "updateTime", Type: "date", Comment: "更新时间", Nullable: true},
	)

	return parsedEntity{className: current.name, tableName: tableName, columns: columns}, nil
}

func embeddedTag(
	structure *ast.StructType,
	imports map[string]string,
	importPath string,
	name string,
) (reflect.StructTag, bool) {
	for _, field := range structure.Fields.List {
		if len(field.Names) != 0 || !isImportedSelector(field.Type, imports, importPath, name) {
			continue
		}
		tag, err := parseASTStructTag(field.Tag)
		if err == nil {
			return tag, true
		}
	}

	return "", false
}

func parseASTStructTag(tag *ast.BasicLit) (reflect.StructTag, error) {
	if tag == nil {
		return "", nil
	}
	value, err := strconv.Unquote(tag.Value)
	if err != nil {
		return "", exception.WrapValidate(err, "实体字段标签无效")
	}

	return reflect.StructTag(value), nil
}

func isImportedSelector(expression ast.Expr, imports map[string]string, importPath, name string) bool {
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)

	return ok && imports[qualifier.Name] == importPath
}

func menuColumnType(expression ast.Expr, coolTag string) (string, bool) {
	nullable := false
	if pointer, ok := expression.(*ast.StarExpr); ok {
		nullable = true
		expression = pointer.X
	}
	if structTagOption(coolTag, "json") == "true" {
		return "json", nullable
	}
	switch value := expression.(type) {
	case *ast.Ident:
		switch value.Name {
		case "string":
			return "string", nullable
		case "bool":
			return "boolean", nullable
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64":
			return "number", nullable
		}
	case *ast.SelectorExpr:
		if value.Sel.Name == "Time" {
			return "date", nullable
		}
	case *ast.ArrayType, *ast.MapType, *ast.InterfaceType, *ast.StructType:
		return "json", nullable
	}

	return "", nullable
}

func parseAdminControllerPath(source string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "controller.go", source, parser.AllErrors)
	if err != nil {
		return "", exception.WrapValidate(err, "Controller Go 源码语法无效")
	}
	imports := sourceImports(file)
	paths := make([]string, 0, 1)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Admin" {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok || imports[qualifier.Name] != controllerImportPath {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, unquoteErr := strconv.Unquote(literal.Value)
		if unquoteErr == nil {
			paths = append(paths, value)
		}
		return true
	})
	if len(paths) != 1 {
		return "", exception.Validate("Controller 必须包含且只包含一个 controller.Admin 字符串声明")
	}
	value := strings.Trim(paths[0], "/")
	if value != "" {
		for _, segment := range strings.Split(value, "/") {
			if !validCodeName(segment) {
				return "", exception.Validate("Controller 路径无效")
			}
		}
	}

	return value, nil
}

func validateSourcePackage(source, expected, label string) error {
	file, err := parser.ParseFile(token.NewFileSet(), label+".go", source, parser.AllErrors)
	if err != nil {
		return exception.WrapValidate(err, label+" Go 源码语法无效")
	}
	if file.Name == nil || file.Name.Name != expected {
		return exception.Validate(label + " package 必须是 " + expected)
	}

	return nil
}

func (service *MenuToolService) validExistingConfig(target, packageName string) (bool, error) {
	root, err := service.coding.openRoot()
	if err != nil {
		return false, err
	}
	defer root.Close()
	info, err := root.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, exception.WrapValidate(err, "检查模块配置失败")
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, exception.Validate("模块配置必须是普通文件")
	}
	content, err := root.ReadFile(target)
	if err != nil {
		return false, exception.WrapCore(err, "读取模块配置失败")
	}
	if err = validateSourcePackage(string(content), packageName, "模块配置"); err != nil {
		return false, err
	}

	return true, nil
}

func moduleConfigSource(moduleName string) string {
	return `package ` + moduleName + `

import "github.com/toothdy/cool-admin-go-next/cool-next/core/module"

type Config struct{}

func ModuleConfig() module.Declaration[Config] {
	return module.Declaration[Config]{
		Name:        "` + moduleName + `",
		Description: "` + moduleName + ` module",
	}
}
`
}

func tableFileName(tableName string) string {
	parts := strings.Split(tableName, "_")

	return parts[len(parts)-1]
}

func structTagOption(value, key string) string {
	for _, option := range strings.Split(value, ",") {
		name, result, found := strings.Cut(strings.TrimSpace(option), ":")
		if !found {
			name, result, found = strings.Cut(strings.TrimSpace(option), "=")
		}
		if found && name == key {
			return result
		}
	}

	return ""
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}

	return strings.ToLower(value[:1]) + value[1:]
}

func sourceImports(file *ast.File) map[string]string {
	result := make(map[string]string, len(file.Imports))
	for _, specification := range file.Imports {
		value, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(value)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name != "_" && name != "." {
			result[name] = value
		}
	}

	return result
}

func validCodeName(value string) bool {
	return codeNamePattern.MatchString(value)
}

func buildMenuTree(row menuExportRow, children map[uint64][]menuExportRow, ancestors map[uint64]bool) MenuTree {
	result := MenuTree{
		Name:      row.Name,
		Router:    row.Router,
		Perms:     row.Perms,
		Type:      row.Type,
		Icon:      row.Icon,
		OrderNum:  row.OrderNum,
		ViewPath:  row.ViewPath,
		KeepAlive: row.KeepAlive,
		IsShow:    row.IsShow,
	}
	if ancestors[row.ID] {
		return result
	}
	ancestors[row.ID] = true
	result.ChildMenus = make([]MenuTree, 0, len(children[row.ID]))
	for _, child := range children[row.ID] {
		result.ChildMenus = append(result.ChildMenus, buildMenuTree(child, children, ancestors))
	}
	delete(ancestors, row.ID)

	return result
}

func importMenuTree(
	model *gdb.Model,
	descriptor coreentity.Descriptor[entity.Menu, uint64],
	menu *MenuTree,
	parentID *uint64,
) error {
	if menu == nil {
		return exception.Validate("导入菜单不能为空")
	}
	do := descriptor.NewDO()
	now := gtime.Now()
	values := []struct {
		field string
		value any
	}{
		{"createTime", *now},
		{"updateTime", *now},
		{"name", menu.Name},
		{"router", stringValue(menu.Router)},
		{"perms", stringValue(menu.Perms)},
		{"type", menu.Type},
		{"icon", stringValue(menu.Icon)},
		{"orderNum", menu.OrderNum},
		{"viewPath", stringValue(menu.ViewPath)},
		{"keepAlive", menu.KeepAlive},
		{"isShow", menu.IsShow},
	}
	if parentID == nil {
		values = append(values, struct {
			field string
			value any
		}{"parentId", nil})
	} else {
		values = append(values, struct {
			field string
			value any
		}{"parentId", *parentID})
	}
	for _, value := range values {
		if err := do.SetColumn(value.field, value.value); err != nil {
			return exception.WrapCore(err, "构造导入菜单失败")
		}
	}
	insertedID, err := model.Data(do.DBData()).InsertAndGetId()
	if err != nil {
		return exception.WrapCore(err, "导入菜单失败")
	}
	if insertedID <= 0 {
		return exception.Core("导入菜单未返回有效 ID")
	}
	id := uint64(insertedID)
	for index := range menu.ChildMenus {
		if err = importMenuTree(model, descriptor, &menu.ChildMenus[index], &id); err != nil {
			return err
		}
	}

	return nil
}

func stringValue(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}
