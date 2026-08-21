package eps

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
	coreroute "github.com/toothdy/cool-admin-go-next/cool-next/core/route"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
)

const (
	openAPIVersion = "3.0.3"
	bearerAuthName = "bearerAuth"
	jsonMediaType  = "application/json"
)

var (
	fieldNamePattern   = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	fieldSourcePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]*\.[a-z][A-Za-z0-9]*$`)
)

// 文档编译输入
type Input struct {
	Graph       module.Graph
	Controllers []ControllerSpec
	Title       string
	Version     string
}

// CRUD Controller 的静态文档规格
type ControllerSpec struct {
	Key              string
	Descriptor       coreentity.RuntimeDescriptor
	HiddenFields     []string
	ReadonlyFields   []string
	InfoIgnoreFields []string
	SortFields       []string
	Queries          []QuerySchema
}

// List 或 Page 的静态请求与响应形状
type QuerySchema struct {
	Action           crud.Action
	Dynamic          bool
	Fields           []SelectField
	RequestFields    []QueryField
	ExtensionAliases []string
}

// 查询响应字段及其 Descriptor 来源
type SelectField struct {
	Name       string
	Descriptor coreentity.Metadata
	Field      string
	Source     string
}

// 查询请求字段及其 Descriptor 来源
type QueryField struct {
	Name       string
	Descriptor coreentity.Metadata
	Field      string
	Multiple   bool
}

type Views struct {
	Admin Document `json:"admin"`
	App   Document `json:"app"`
}

// EPS 根文档
type Document struct {
	Modules []Module `json:"modules"`
}

// EPS 模块元数据
type Module struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Controllers []Controller `json:"controllers"`
}

// EPS Controller 元数据
type Controller struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Prefix      string  `json:"prefix"`
	Description string  `json:"description"`
	Entity      *Entity `json:"entity,omitempty"`
	API         []API   `json:"api"`
}

// EPS 实体元数据
type Entity struct {
	Name        string  `json:"name"`
	Table       string  `json:"table"`
	Description string  `json:"description"`
	Fields      []Field `json:"fields"`
}

// EPS 字段元数据
type Field struct {
	Name             string  `json:"name"`
	JSONName         string  `json:"jsonName"`
	Column           string  `json:"column"`
	GoType           string  `json:"goType"`
	JSONType         string  `json:"jsonType"`
	DatabaseType     string  `json:"databaseType"`
	Description      string  `json:"description"`
	Source           string  `json:"source"`
	Nullable         bool    `json:"nullable"`
	Primary          bool    `json:"primary"`
	AutoIncrement    bool    `json:"autoIncrement"`
	SystemMaintained bool    `json:"systemMaintained"`
	Hidden           bool    `json:"hidden"`
	Readonly         bool    `json:"readonly"`
	Sortable         bool    `json:"sortable"`
	Default          any     `json:"default,omitempty"`
	HasDefault       bool    `json:"hasDefault"`
	Size             *uint64 `json:"size,omitempty"`
	Precision        *uint64 `json:"precision,omitempty"`
	Scale            *uint64 `json:"scale,omitempty"`
}

// EPS API 元数据
type API struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	Summary       string `json:"summary"`
	Description   string `json:"description"`
	Bind          string `json:"bind"`
	Authenticated bool   `json:"authenticated"`
}

type compiler struct {
	input            Input
	modules          []Module
	moduleIndexes    map[string]int
	controllers      map[string]*Controller
	controllerRoutes map[string][]coreroute.Route
	specs            map[string]compiledSpec
	tables           map[string]string
}

type compiledSpec struct {
	spec       ControllerSpec
	hidden     map[string]bool
	readonly   map[string]bool
	infoIgnore map[string]bool
	sortable   map[string]bool
	queries    map[crud.Action]compiledQuery
}

type compiledQuery struct {
	fields        []Field
	requestFields []compiledQueryField
}

type compiledQueryField struct {
	name     string
	field    coreentity.Field
	multiple bool
}

// CompileViews 编译并按后台、App 及运行环境投影 EPS
func CompileViews(input Input, includeDevelopment bool) (*Views, error) {
	current := &compiler{
		input:            input,
		moduleIndexes:    make(map[string]int),
		controllers:      make(map[string]*Controller),
		controllerRoutes: make(map[string][]coreroute.Route),
		specs:            make(map[string]compiledSpec),
		tables:           make(map[string]string),
	}
	if err := current.compile(); err != nil {
		return nil, err
	}
	document := Document{Modules: current.modules}

	return &Views{
		Admin: projectDocument(document, input.Graph, false, includeDevelopment),
		App:   projectDocument(document, input.Graph, true, includeDevelopment),
	}, nil
}

func projectDocument(document Document, graph module.Graph, appArea, includeDevelopment bool) Document {
	controllers := make(map[string]bool)
	for _, definition := range graph.Routes().Controllers() {
		if !includeDevelopment && definition.DevelopmentOnly() {
			continue
		}
		isApp := strings.HasPrefix(definition.Path(), "/app/")
		if isApp == appArea {
			controllers[definition.Key()] = true
		}
	}
	routes := make(map[string]map[string]bool)
	for _, route := range graph.Routes().Routes() {
		if !controllers[route.Controller()] || !includeDevelopment && route.DevelopmentOnly() {
			continue
		}
		if routes[route.Controller()] == nil {
			routes[route.Controller()] = make(map[string]bool)
		}
		routes[route.Controller()][route.Method()+" "+route.Path()] = true
	}

	result := Document{Modules: make([]Module, 0, len(document.Modules))}
	for _, sourceModule := range document.Modules {
		projectedModule := sourceModule
		projectedModule.Controllers = make([]Controller, 0, len(sourceModule.Controllers))
		for _, sourceController := range sourceModule.Controllers {
			if !controllers[sourceController.Key] {
				continue
			}
			projectedController := sourceController
			projectedController.API = make([]API, 0, len(sourceController.API))
			for _, api := range sourceController.API {
				if routes[sourceController.Key][api.Method+" "+api.Path] {
					projectedController.API = append(projectedController.API, api)
				}
			}
			if len(projectedController.API) > 0 {
				projectedModule.Controllers = append(projectedModule.Controllers, projectedController)
			}
		}
		if len(projectedModule.Controllers) > 0 {
			result.Modules = append(result.Modules, projectedModule)
		}
	}

	return result
}

func (current *compiler) compile() error {
	if !current.input.Graph.IsValidated() {
		return exception.Core("EPS 输入必须使用已校验的模块 Graph")
	}
	if err := current.compileModules(); err != nil {
		return err
	}
	if err := current.compileTables(); err != nil {
		return err
	}
	if err := current.compileControllers(); err != nil {
		return err
	}
	if err := current.compileSpecs(); err != nil {
		return err
	}
	if err := current.compileRoutes(); err != nil {
		return err
	}
	current.attachControllers()

	return nil
}

func (current *compiler) compileModules() error {
	modules := current.input.Graph.Modules()
	current.modules = make([]Module, len(modules))
	for index, item := range modules {
		key := item.Identity().Key()
		if _, exists := current.moduleIndexes[key]; exists {
			return exception.Core(fmt.Sprintf("EPS 模块 %s 重复", key))
		}
		current.moduleIndexes[key] = index
		current.modules[index] = Module{
			Key:         key,
			Name:        item.Name(),
			Description: item.Description(),
			Controllers: make([]Controller, 0),
		}
	}

	return nil
}

func (current *compiler) compileTables() error {
	for _, descriptor := range current.input.Graph.Descriptors() {
		if previous, exists := current.tables[descriptor.Table()]; exists {
			return exception.Core(fmt.Sprintf("EPS Descriptor 表 %s 在模块 %s 与 %s 重复", descriptor.Table(), previous, descriptor.Module()))
		}
		current.tables[descriptor.Table()] = descriptor.Module()
	}

	return nil
}

func (current *compiler) compileControllers() error {
	for _, definition := range current.input.Graph.Routes().Controllers() {
		if _, exists := current.controllers[definition.Key()]; exists {
			return exception.Core(fmt.Sprintf("EPS Controller %s 重复", definition.Key()))
		}
		if _, exists := current.moduleIndexes[definition.Module()]; !exists {
			return exception.Core(fmt.Sprintf("EPS Controller %s 引用未知模块 %s", definition.Key(), definition.Module()))
		}
		controller := &Controller{
			Key:         definition.Key(),
			Name:        definition.Factory().Symbol,
			Prefix:      definition.Path(),
			Description: definition.Description(),
			API:         make([]API, 0),
		}
		current.controllers[definition.Key()] = controller
	}
	for _, route := range current.input.Graph.Routes().Routes() {
		if _, exists := current.controllers[route.Controller()]; !exists {
			return exception.Core(fmt.Sprintf("EPS 路由 %s %s 引用未知 Controller", route.Method(), route.Path()))
		}
		current.controllerRoutes[route.Controller()] = append(current.controllerRoutes[route.Controller()], route)
	}
	for _, definition := range current.input.Graph.Routes().Controllers() {
		controller := current.controllers[definition.Key()]
		prefix, err := routePrefix(controller.Prefix, current.controllerRoutes[definition.Key()])
		if err != nil {
			return exception.WrapCore(err, fmt.Sprintf("EPS Controller %s 的 CRUD 前缀无效", definition.Key()))
		}
		controller.Prefix = prefix
	}

	return nil
}

func (current *compiler) compileSpecs() error {
	for _, input := range current.input.Controllers {
		if _, exists := current.specs[input.Key]; exists {
			return exception.Core(fmt.Sprintf("EPS Controller 规格 %s 重复", input.Key))
		}
		controller, exists := current.controllers[input.Key]
		if !exists {
			return exception.Core(fmt.Sprintf("EPS Controller 规格 %s 不存在", input.Key))
		}
		if isNil(input.Descriptor) {
			return exception.Core(fmt.Sprintf("EPS Controller %s 的 Descriptor 不能为空", input.Key))
		}
		moduleKey := current.controllerModule(input.Key)
		if registeredModule, exists := current.tables[input.Descriptor.Table()]; !exists || registeredModule != moduleKey {
			return exception.Core(fmt.Sprintf("EPS Controller %s 的 Descriptor 表 %s 未在所属模块登记", input.Key, input.Descriptor.Table()))
		}
		compiled, err := current.compileSpec(input)
		if err != nil {
			return err
		}
		current.specs[input.Key] = compiled
		entity := current.compileEntity(compiled)
		controller.Entity = &entity
	}
	for key, routes := range current.controllerRoutes {
		if hasCRUDRoute(routes) {
			if _, exists := current.specs[key]; !exists {
				return exception.Core(fmt.Sprintf("CRUD Controller %s 缺少 EPS 规格", key))
			}
		}
	}

	return nil
}

func (current *compiler) compileSpec(input ControllerSpec) (compiledSpec, error) {
	compiled := compiledSpec{
		spec:       input,
		hidden:     make(map[string]bool),
		readonly:   make(map[string]bool),
		infoIgnore: make(map[string]bool),
		sortable:   make(map[string]bool),
		queries:    make(map[crud.Action]compiledQuery),
	}
	for _, fieldSet := range []struct {
		label  string
		values []string
		target map[string]bool
	}{
		{label: "隐藏", values: input.HiddenFields, target: compiled.hidden},
		{label: "只读", values: input.ReadonlyFields, target: compiled.readonly},
		{label: "详情忽略", values: input.InfoIgnoreFields, target: compiled.infoIgnore},
		{label: "排序", values: input.SortFields, target: compiled.sortable},
	} {
		if err := compileFieldSet(input.Key, input.Descriptor, fieldSet.label, fieldSet.values, fieldSet.target); err != nil {
			return compiledSpec{}, err
		}
	}
	for _, query := range input.Queries {
		if query.Action != crud.ActionList && query.Action != crud.ActionPage {
			return compiledSpec{}, exception.Core(fmt.Sprintf("EPS Controller %s 的查询动作 %s 无效", input.Key, query.Action))
		}
		if _, exists := compiled.queries[query.Action]; exists {
			return compiledSpec{}, exception.Core(fmt.Sprintf("EPS Controller %s 的查询动作 %s 重复", input.Key, query.Action))
		}
		value, err := current.compileQuery(input, compiled, query)
		if err != nil {
			return compiledSpec{}, err
		}
		compiled.queries[query.Action] = value
	}

	return compiled, nil
}

func (current *compiler) compileQuery(input ControllerSpec, spec compiledSpec, query QuerySchema) (compiledQuery, error) {
	if query.Dynamic && (len(query.Fields) > 0 || len(query.ExtensionAliases) > 0) {
		return compiledQuery{}, exception.Core(fmt.Sprintf("EPS Controller %s 的动态查询 %s 不能改变响应字段", input.Key, query.Action))
	}
	result := compiledQuery{}
	outputs := make(map[string]bool)
	for _, selected := range query.Fields {
		if !fieldNamePattern.MatchString(selected.Name) || !fieldSourcePattern.MatchString(selected.Source) {
			return compiledQuery{}, exception.Core(fmt.Sprintf("EPS Controller %s 的查询输出 %s 无效", input.Key, selected.Name))
		}
		if outputs[selected.Name] {
			return compiledQuery{}, exception.Core(fmt.Sprintf("EPS Controller %s 的查询输出 %s 重复", input.Key, selected.Name))
		}
		field, err := current.resolveRegisteredField(selected.Descriptor, selected.Field)
		if err != nil {
			return compiledQuery{}, exception.WrapCore(err, fmt.Sprintf("EPS Controller %s 的查询输出 %s 无效", input.Key, selected.Name))
		}
		outputs[selected.Name] = true
		result.fields = append(result.fields, compileField(
			field,
			selected.Name,
			selected.Source,
			spec.hidden[field.Name()] && selected.Descriptor.Table() == input.Descriptor.Table(),
			spec.readonly[field.Name()] && selected.Descriptor.Table() == input.Descriptor.Table(),
			spec.sortable[field.Name()] && selected.Descriptor.Table() == input.Descriptor.Table(),
		))
	}
	for _, alias := range query.ExtensionAliases {
		if !outputs[alias] {
			return compiledQuery{}, exception.Core(fmt.Sprintf("EPS Controller %s 的查询扩展输出别名 %s 未声明", input.Key, alias))
		}
	}
	requestNames := make(map[string]bool)
	for _, requested := range query.RequestFields {
		if !fieldNamePattern.MatchString(requested.Name) || requestNames[requested.Name] {
			return compiledQuery{}, exception.Core(fmt.Sprintf("EPS Controller %s 的查询请求字段 %s 无效或重复", input.Key, requested.Name))
		}
		field, err := current.resolveRegisteredField(requested.Descriptor, requested.Field)
		if err != nil {
			return compiledQuery{}, exception.WrapCore(err, fmt.Sprintf("EPS Controller %s 的查询请求字段 %s 无效", input.Key, requested.Name))
		}
		requestNames[requested.Name] = true
		result.requestFields = append(result.requestFields, compiledQueryField{
			name:     requested.Name,
			field:    field,
			multiple: requested.Multiple,
		})
	}

	return result, nil
}

func (current *compiler) resolveRegisteredField(metadata coreentity.Metadata, name string) (coreentity.Field, error) {
	if isNil(metadata) {
		return nil, exception.Core("Descriptor 不能为空")
	}
	if _, exists := current.tables[metadata.Table()]; !exists {
		return nil, exception.Core(fmt.Sprintf("Descriptor 表 %s 未登记", metadata.Table()))
	}
	field, exists := metadata.Field(name)
	if !exists || isNil(field) {
		return nil, exception.Core(fmt.Sprintf("Descriptor 表 %s 不存在字段 %s", metadata.Table(), name))
	}

	return field, nil
}

func (current *compiler) compileEntity(spec compiledSpec) Entity {
	descriptor := spec.spec.Descriptor
	fields := make([]Field, 0, len(descriptor.Fields()))
	for _, field := range descriptor.Fields() {
		fields = append(fields, compileField(
			field,
			field.JSONName(),
			"a."+field.JSONName(),
			spec.hidden[field.Name()],
			spec.readonly[field.Name()],
			spec.sortable[field.Name()],
		))
	}

	return Entity{
		Name:        descriptor.EntityType().Name(),
		Table:       descriptor.Table(),
		Description: descriptor.Description(),
		Fields:      fields,
	}
}

func compileField(
	field coreentity.Field,
	name string,
	source string,
	hidden bool,
	readonly bool,
	sortable bool,
) Field {
	constraints := field.Constraints()
	result := Field{
		Name:             name,
		JSONName:         field.JSONName(),
		Column:           field.Column(),
		GoType:           field.GoType().String(),
		JSONType:         jsonType(field.LogicalType()),
		DatabaseType:     string(field.LogicalType()),
		Description:      field.Description(),
		Source:           source,
		Nullable:         field.Nullable(),
		Primary:          field.Primary(),
		AutoIncrement:    field.AutoIncrement(),
		SystemMaintained: field.SystemMaintained(),
		Hidden:           hidden,
		Readonly:         readonly || field.Primary() || field.AutoIncrement() || field.SystemMaintained(),
		Sortable:         sortable,
		HasDefault:       constraints.HasDefault,
	}
	if constraints.HasDefault {
		result.Default = parseDefault(field.LogicalType(), constraints.Default)
	}
	if constraints.HasSize {
		result.Size = pointer(constraints.Size)
	}
	if constraints.HasPrecision {
		result.Precision = pointer(constraints.Precision)
	}
	if constraints.HasScale {
		result.Scale = pointer(constraints.Scale)
	}

	return result
}

func (current *compiler) compileRoutes() error {
	for _, definition := range current.input.Graph.Routes().Controllers() {
		controller := current.controllers[definition.Key()]
		_, hasSpec := current.specs[definition.Key()]
		for _, route := range current.controllerRoutes[definition.Key()] {
			api, err := compileAPI(route, hasSpec)
			if err != nil {
				return err
			}
			controller.API = append(controller.API, api)
		}
	}

	return nil
}

// 将静态路由编译为 EPS API 元数据
func compileAPI(route coreroute.Route, hasSpec bool) (API, error) {
	if route.Kind() == coreroute.KindCRUD {
		if !hasSpec {
			return API{}, exception.Core(fmt.Sprintf("CRUD 路由 %s %s 缺少 EPS 规格", route.Method(), route.Path()))
		}
		if action := crud.Action(routeAction(route.Path())); !isCRUDAction(action) {
			return API{}, exception.Core(fmt.Sprintf("CRUD 路由动作 %s 无效", action))
		}
	}

	return API{
		Method:        route.Method(),
		Path:          route.Path(),
		Summary:       route.Summary(),
		Description:   route.Description(),
		Bind:          string(route.Bind()),
		Authenticated: !contains(route.Tags(), "ignoreToken"),
	}, nil
}

// CRUD 路由允许的动作
func isCRUDAction(action crud.Action) bool {
	switch action {
	case crud.ActionAdd, crud.ActionDelete, crud.ActionUpdate, crud.ActionInfo, crud.ActionList, crud.ActionPage:
		return true
	default:
		return false
	}
}

func (current *compiler) attachControllers() {
	for _, definition := range current.input.Graph.Routes().Controllers() {
		controller := current.controllers[definition.Key()]
		index := current.moduleIndexes[definition.Module()]
		current.modules[index].Controllers = append(current.modules[index].Controllers, *controller)
	}
}

func (current *compiler) controllerModule(key string) string {
	for _, definition := range current.input.Graph.Routes().Controllers() {
		if definition.Key() == key {
			return definition.Module()
		}
	}

	return ""
}

func compileFieldSet(
	controller string,
	descriptor coreentity.Metadata,
	label string,
	values []string,
	target map[string]bool,
) error {
	for _, name := range values {
		field, exists := descriptor.Field(name)
		if !exists || isNil(field) {
			return exception.Core(fmt.Sprintf("EPS Controller %s 的%s字段 %s 不存在", controller, label, name))
		}
		if target[field.Name()] {
			return exception.Core(fmt.Sprintf("EPS Controller %s 的%s字段 %s 重复", controller, label, name))
		}
		target[field.Name()] = true
	}

	return nil
}

func hasCRUDRoute(routes []coreroute.Route) bool {
	for _, route := range routes {
		if route.Kind() == coreroute.KindCRUD {
			return true
		}
	}

	return false
}

func jsonType(logicalType coreentity.LogicalType) string {
	switch logicalType {
	case coreentity.LogicalBool:
		return "boolean"
	case coreentity.LogicalInt, coreentity.LogicalUint, coreentity.LogicalFloat:
		return "number"
	case coreentity.LogicalString, coreentity.LogicalBytes, coreentity.LogicalTime:
		return "string"
	default:
		return "object"
	}
}

func parseDefault(logicalType coreentity.LogicalType, value string) any {
	switch logicalType {
	case coreentity.LogicalBool:
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	case coreentity.LogicalInt:
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	case coreentity.LogicalUint:
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
			return parsed
		}
	case coreentity.LogicalFloat:
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}

	return value
}

func routePrefix(fallback string, routes []coreroute.Route) (string, error) {
	prefix := ""
	for _, route := range routes {
		if route.Kind() != coreroute.KindCRUD {
			continue
		}
		current := path.Dir(route.Path())
		if prefix == "" {
			prefix = current
			continue
		}
		if prefix != current {
			return "", exception.Core(fmt.Sprintf("CRUD 路由前缀 %s 与 %s 不一致", prefix, current))
		}
	}
	if prefix == "" {
		return fallback, nil
	}

	return prefix, nil
}

func routeAction(routePath string) string {
	index := strings.LastIndexByte(routePath, '/')
	if index < 0 || index == len(routePath)-1 {
		return routePath
	}

	return routePath[index+1:]
}

func upperFirst(value string) string {
	if value == "" {
		return ""
	}
	characters := []rune(value)
	characters[0] = unicode.ToUpper(characters[0])

	return string(characters)
}

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func pointer[T any](value T) *T {
	return &value
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
