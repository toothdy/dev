package eps

import (
	"fmt"
	"net/http"
	"path"
	"reflect"
	"regexp"
	"sort"
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

// EPS 与 OpenAPI 的同源编译结果
type Bundle struct {
	EPS     Document        `json:"eps"`
	OpenAPI OpenAPIDocument `json:"openapi"`
}

// Views 是前端按身份区域读取的 EPS 文档。
type Views struct {
	Admin Document `json:"admin"`
	App   Document `json:"app"`
}

// EPS 根文档
type Document struct {
	Modules []Module          `json:"modules"`
	Schemas map[string]Schema `json:"schemas"`
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
	Method         string  `json:"method"`
	Path           string  `json:"path"`
	Summary        string  `json:"summary"`
	Description    string  `json:"description"`
	Bind           string  `json:"bind"`
	Authenticated  bool    `json:"authenticated"`
	Permission     string  `json:"permission,omitempty"`
	RequestSchema  *Schema `json:"requestSchema,omitempty"`
	ResponseSchema Schema  `json:"responseSchema"`
	Errors         []Error `json:"errors"`
}

// EPS 错误响应元数据
type Error struct {
	Status      int    `json:"status"`
	Code        int    `json:"code"`
	Description string `json:"description"`
}

// OpenAPI 3 文档
type OpenAPIDocument struct {
	OpenAPI    string              `json:"openapi"`
	Info       OpenAPIInfo         `json:"info"`
	Tags       []OpenAPITag        `json:"tags"`
	Paths      map[string]PathItem `json:"paths"`
	Components OpenAPIComponents   `json:"components"`
}

// OpenAPI 基本信息
type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

// OpenAPI 标签
type OpenAPITag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// OpenAPI 路径项
type PathItem struct {
	Connect *Operation `json:"connect,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Get     *Operation `json:"get,omitempty"`
	Head    *Operation `json:"head,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Trace   *Operation `json:"trace,omitempty"`
}

// OpenAPI 操作
type Operation struct {
	Tags        []string                   `json:"tags,omitempty"`
	Summary     string                     `json:"summary,omitempty"`
	Description string                     `json:"description,omitempty"`
	OperationID string                     `json:"operationId"`
	Parameters  []Parameter                `json:"parameters,omitempty"`
	RequestBody *RequestBody               `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
	Permission  string                     `json:"x-permission,omitempty"`
}

// OpenAPI 参数
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Schema      Schema `json:"schema"`
}

// OpenAPI 请求体
type RequestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]MediaType `json:"content"`
}

// OpenAPI 响应
type OpenAPIResponse struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// OpenAPI 媒体类型
type MediaType struct {
	Schema Schema `json:"schema"`
}

// OpenAPI Components
type OpenAPIComponents struct {
	Schemas         map[string]Schema         `json:"schemas"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes"`
}

// OpenAPI 安全方案
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme"`
	BearerFormat string `json:"bearerFormat"`
}

// OpenAPI Schema
type Schema struct {
	Ref                  string            `json:"$ref,omitempty"`
	OneOf                []Schema          `json:"oneOf,omitempty"`
	Type                 string            `json:"type,omitempty"`
	Format               string            `json:"format,omitempty"`
	Description          string            `json:"description,omitempty"`
	Enum                 []any             `json:"enum,omitempty"`
	Default              *any              `json:"default,omitempty"`
	Nullable             bool              `json:"nullable,omitempty"`
	ReadOnly             bool              `json:"readOnly,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	AdditionalProperties *bool             `json:"additionalProperties,omitempty"`
	MaxLength            *uint64           `json:"maxLength,omitempty"`
	GoType               string            `json:"x-go-type,omitempty"`
	DatabaseType         string            `json:"x-database-type,omitempty"`
	Source               string            `json:"x-source,omitempty"`
	Hidden               bool              `json:"x-hidden,omitempty"`
	Sortable             bool              `json:"x-sortable,omitempty"`
	Precision            *uint64           `json:"x-precision,omitempty"`
	Scale                *uint64           `json:"x-scale,omitempty"`
}

type compiler struct {
	input            Input
	modules          []Module
	moduleIndexes    map[string]int
	controllers      map[string]*Controller
	controllerRoutes map[string][]coreroute.Route
	specs            map[string]compiledSpec
	tables           map[string]string
	schemas          map[string]Schema
	paths            map[string]PathItem
	tags             []OpenAPITag
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

// 从 Descriptor 和静态路由图编译 EPS 与 OpenAPI
func Compile(input Input) (*Bundle, error) {
	current := &compiler{
		input:            input,
		moduleIndexes:    make(map[string]int),
		controllers:      make(map[string]*Controller),
		controllerRoutes: make(map[string][]coreroute.Route),
		specs:            make(map[string]compiledSpec),
		tables:           make(map[string]string),
		schemas:          make(map[string]Schema),
		paths:            make(map[string]PathItem),
	}
	if err := current.compile(); err != nil {
		return nil, err
	}

	document := Document{Modules: current.modules, Schemas: current.schemas}
	return &Bundle{
		EPS: document,
		OpenAPI: OpenAPIDocument{
			OpenAPI: openAPIVersion,
			Info: OpenAPIInfo{
				Title:   defaultText(input.Title, "Cool Admin API"),
				Version: defaultText(input.Version, "1.0.0"),
			},
			Tags:  current.tags,
			Paths: current.paths,
			Components: OpenAPIComponents{
				Schemas: current.schemas,
				SecuritySchemes: map[string]SecurityScheme{
					bearerAuthName: {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
				},
			},
		},
	}, nil
}

// CompileViews 编译并按后台、App 及运行环境投影 EPS。
func CompileViews(input Input, includeDevelopment bool) (*Views, error) {
	bundle, err := Compile(input)
	if err != nil {
		return nil, err
	}

	return &Views{
		Admin: projectDocument(bundle.EPS, input.Graph, false, includeDevelopment),
		App:   projectDocument(bundle.EPS, input.Graph, true, includeDevelopment),
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

	result := Document{
		Modules: make([]Module, 0, len(document.Modules)),
		Schemas: make(map[string]Schema, len(document.Schemas)),
	}
	for name, schema := range document.Schemas {
		result.Schemas[name] = schema
	}
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
	current.tags = make([]OpenAPITag, len(modules))
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
		current.tags[index] = OpenAPITag{Name: key, Description: item.Description()}
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
		tagName := definition.TagName()
		if tagName == "" {
			tagName = definition.Key()
		}
		current.tags = append(current.tags, OpenAPITag{Name: tagName, Description: definition.Description()})
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
		spec, hasSpec := current.specs[definition.Key()]
		for _, route := range current.controllerRoutes[definition.Key()] {
			api, operation, schemas, err := current.compileRoute(definition, route, spec, hasSpec)
			if err != nil {
				return err
			}
			controller.API = append(controller.API, api)
			for name, schema := range schemas {
				if previous, exists := current.schemas[name]; exists && !reflect.DeepEqual(previous, schema) {
					return exception.Core(fmt.Sprintf("OpenAPI Schema %s 冲突", name))
				}
				current.schemas[name] = schema
			}
			pathItem := current.paths[route.Path()]
			if err = setOperation(&pathItem, route.Method(), operation); err != nil {
				return err
			}
			current.paths[route.Path()] = pathItem
		}
	}
	current.schemas["ErrorResponse"] = errorSchema()

	return nil
}

func (current *compiler) compileRoute(
	controller coreroute.Controller,
	route coreroute.Route,
	spec compiledSpec,
	hasSpec bool,
) (API, *Operation, map[string]Schema, error) {
	authenticated := !contains(route.Tags(), "ignoreToken")
	errors := apiErrors(authenticated, route.Permission())
	api := API{
		Method:        route.Method(),
		Path:          route.Path(),
		Summary:       route.Summary(),
		Description:   route.Description(),
		Bind:          string(route.Bind()),
		Authenticated: authenticated,
		Permission:    route.Permission(),
		Errors:        errors,
	}
	prefix := schemaPrefix(controller.Key(), controller.Factory().Symbol)
	schemas := make(map[string]Schema)
	responseData := Schema{Type: "object"}
	if route.Kind() == coreroute.KindCRUD {
		if !hasSpec {
			return API{}, nil, nil, exception.Core(fmt.Sprintf("CRUD 路由 %s %s 缺少 EPS 规格", route.Method(), route.Path()))
		}
		action := crud.Action(routeAction(route.Path()))
		request, response, generated, err := compileCRUDSchemas(prefix, action, spec)
		if err != nil {
			return API{}, nil, nil, err
		}
		api.RequestSchema = request
		api.ResponseSchema = response
		for name, schema := range generated {
			schemas[name] = schema
		}
		responseData = response
	} else {
		api.ResponseSchema = genericResponseEnvelope()
		responseData = api.ResponseSchema
	}
	operationName := routeAction(route.Path())
	if route.Kind() == coreroute.KindCustom && route.Handler().Method != "" {
		operationName = route.Handler().Method
	}
	tagName := controller.TagName()
	if tagName == "" {
		tagName = controller.Key()
	}
	operationID := prefix + upperFirst(operationName)
	operation := &Operation{
		Tags:        []string{tagName},
		Summary:     route.Summary(),
		Description: route.Description(),
		OperationID: operationID,
		Responses:   operationResponses(responseData, authenticated, route.Permission()),
		Permission:  route.Permission(),
	}
	if authenticated {
		operation.Security = []map[string][]string{{bearerAuthName: {}}}
	}
	if api.RequestSchema != nil {
		if route.Bind() == coreroute.BindQuery {
			operation.Parameters = schemaParameters(resolveSchema(*api.RequestSchema, schemas))
		} else {
			operation.RequestBody = &RequestBody{
				Required: true,
				Content:  map[string]MediaType{jsonMediaType: {Schema: *api.RequestSchema}},
			}
		}
	}

	return api, operation, schemas, nil
}

func compileCRUDSchemas(
	prefix string,
	action crud.Action,
	spec compiledSpec,
) (*Schema, Schema, map[string]Schema, error) {
	if action != crud.ActionAdd && action != crud.ActionDelete && action != crud.ActionUpdate &&
		action != crud.ActionInfo && action != crud.ActionList && action != crud.ActionPage {
		return nil, Schema{}, nil, exception.Core(fmt.Sprintf("CRUD 路由动作 %s 无效", action))
	}
	generated := make(map[string]Schema)
	requestName := prefix + upperFirst(string(action)) + "Request"
	responseName := prefix + upperFirst(string(action)) + "Response"
	request := crudRequestSchema(action, spec)
	responseFields := responseFields(action, spec)
	record := recordSchema(responseFields)
	var data Schema
	switch action {
	case crud.ActionAdd:
		id := fieldSchema(spec.spec.Descriptor.Primary(), false, false, false)
		data = Schema{OneOf: []Schema{
			objectSchema(map[string]Schema{"id": id}, "id"),
			objectSchema(map[string]Schema{"id": arraySchema(id)}, "id"),
		}}
	case crud.ActionDelete, crud.ActionUpdate:
	case crud.ActionInfo:
		data = record
	case crud.ActionList:
		data = arraySchema(record)
	case crud.ActionPage:
		pagination := objectSchema(map[string]Schema{
			"page":  {Type: "integer", Format: "int32"},
			"size":  {Type: "integer", Format: "int32"},
			"total": {Type: "integer", Format: "int64"},
		}, "page", "size", "total")
		data = objectSchema(map[string]Schema{
			"list":       arraySchema(record),
			"pagination": pagination,
		}, "list", "pagination")
	}
	response := responseEnvelope(data)
	if action == crud.ActionDelete || action == crud.ActionUpdate {
		response = responseWithoutData()
	}
	generated[requestName] = request
	generated[responseName] = response
	requestRef := refSchema(requestName)
	responseRef := refSchema(responseName)

	return &requestRef, responseRef, generated, nil
}

func crudRequestSchema(action crud.Action, spec compiledSpec) Schema {
	descriptor := spec.spec.Descriptor
	primary := descriptor.Primary()
	writable := make(map[string]Schema)
	addRequired := make([]string, 0)
	for _, field := range descriptor.Fields() {
		if spec.hidden[field.Name()] || spec.readonly[field.Name()] || field.Primary() || field.AutoIncrement() || field.SystemMaintained() {
			continue
		}
		writable[field.JSONName()] = fieldSchema(field, false, false, false)
		if !field.Nullable() && !field.Constraints().HasDefault {
			addRequired = append(addRequired, field.JSONName())
		}
	}
	switch action {
	case crud.ActionAdd:
		object := objectSchema(writable, addRequired...)
		return Schema{OneOf: []Schema{object, arraySchema(object)}}
	case crud.ActionDelete:
		id := fieldSchema(primary, false, false, false)
		ids := Schema{OneOf: []Schema{id, {Type: "string"}, arraySchema(id)}}
		return Schema{OneOf: []Schema{
			id,
			{Type: "string"},
			arraySchema(id),
			objectSchema(map[string]Schema{"ids": ids}, "ids"),
		}}
	case crud.ActionUpdate:
		properties := cloneProperties(writable)
		properties[primary.JSONName()] = fieldSchema(primary, false, false, false)
		object := objectSchema(properties, primary.JSONName())
		return Schema{OneOf: []Schema{object, arraySchema(object)}}
	case crud.ActionInfo:
		return objectSchema(map[string]Schema{
			primary.JSONName(): fieldSchema(primary, false, false, false),
		}, primary.JSONName())
	case crud.ActionList, crud.ActionPage:
		return queryRequestSchema(action, spec)
	default:
		return objectSchema(nil)
	}
}

func queryRequestSchema(action crud.Action, spec compiledSpec) Schema {
	properties := map[string]Schema{
		"keyWord":        {Type: "string", Description: "关键词"},
		"order":          arraySchema(Schema{Type: "string", Enum: stringValues(spec.spec.SortFields)}),
		"sort":           arraySchema(Schema{Type: "string", Enum: []any{"asc", "desc"}}),
		"isExport":       {Type: "boolean", Description: "是否导出"},
		"maxExportLimit": {Type: "integer", Format: "int32", Description: "导出数量上限"},
	}
	if action == crud.ActionPage {
		properties["page"] = Schema{Type: "integer", Format: "int32", Description: "页码"}
		properties["size"] = Schema{Type: "integer", Format: "int32", Description: "每页数量"}
	}
	if query, exists := spec.queries[action]; exists {
		for _, requested := range query.requestFields {
			schema := fieldSchema(requested.field, false, false, false)
			if requested.multiple {
				schema = Schema{OneOf: []Schema{schema, arraySchema(schema)}}
			}
			properties[requested.name] = schema
		}
	}

	return objectSchema(properties)
}

func responseFields(action crud.Action, spec compiledSpec) []Field {
	if query, exists := spec.queries[action]; exists && len(query.fields) > 0 {
		result := make([]Field, 0, len(query.fields))
		for _, field := range query.fields {
			if !field.Hidden {
				result = append(result, field)
			}
		}

		return result
	}
	result := make([]Field, 0, len(spec.spec.Descriptor.Fields()))
	for _, field := range spec.spec.Descriptor.Fields() {
		if spec.hidden[field.Name()] || action == crud.ActionInfo && spec.infoIgnore[field.Name()] {
			continue
		}
		result = append(result, compileField(
			field,
			field.JSONName(),
			"a."+field.JSONName(),
			false,
			spec.readonly[field.Name()],
			spec.sortable[field.Name()],
		))
	}

	return result
}

func recordSchema(fields []Field) Schema {
	properties := make(map[string]Schema, len(fields))
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		properties[field.Name] = epsFieldSchema(field)
		if !field.Nullable {
			required = append(required, field.Name)
		}
	}

	return objectSchema(properties, required...)
}

func epsFieldSchema(field Field) Schema {
	result := Schema{
		Type:         field.JSONType,
		Description:  field.Description,
		Nullable:     field.Nullable,
		ReadOnly:     field.Readonly,
		MaxLength:    field.Size,
		GoType:       field.GoType,
		DatabaseType: field.DatabaseType,
		Source:       field.Source,
		Hidden:       field.Hidden,
		Sortable:     field.Sortable,
		Precision:    field.Precision,
		Scale:        field.Scale,
	}
	setSchemaFormat(&result, coreentity.LogicalType(field.DatabaseType))
	if field.HasDefault {
		result.Default = pointer[any](field.Default)
	}

	return result
}

func fieldSchema(field coreentity.Field, hidden, readonly, sortable bool) Schema {
	return epsFieldSchema(compileField(field, field.JSONName(), "", hidden, readonly, sortable))
}

func responseEnvelope(data Schema) Schema {
	return objectSchema(map[string]Schema{
		"code":    {Type: "integer", Format: "int32", Enum: []any{1000}},
		"message": {Type: "string", Enum: []any{"success"}},
		"data":    data,
	}, "code", "message", "data")
}

func responseWithoutData() Schema {
	return objectSchema(map[string]Schema{
		"code":    {Type: "integer", Format: "int32", Enum: []any{1000}},
		"message": {Type: "string", Enum: []any{"success"}},
	}, "code", "message")
}

func genericResponseEnvelope() Schema {
	return objectSchema(map[string]Schema{
		"code":    {Type: "integer", Format: "int32", Enum: []any{1000}},
		"message": {Type: "string", Enum: []any{"success"}},
		"data":    {Type: "object", Nullable: true},
	}, "code", "message")
}

func errorSchema() Schema {
	return objectSchema(map[string]Schema{
		"code":    {Type: "integer", Format: "int32", Enum: []any{1001, 1002, 1003}},
		"message": {Type: "string"},
	}, "code", "message")
}

func operationResponses(success Schema, authenticated bool, permission string) map[string]OpenAPIResponse {
	responses := map[string]OpenAPIResponse{
		"200": responseWithSchema("成功或业务错误", Schema{OneOf: []Schema{success, refSchema("ErrorResponse")}}),
		"500": responseWithSchema("未处理的服务错误", refSchema("ErrorResponse")),
	}
	if authenticated {
		responses["401"] = responseWithSchema("身份未验证", refSchema("ErrorResponse"))
	}
	if permission != "" {
		responses["403"] = responseWithSchema("权限不足", refSchema("ErrorResponse"))
	}

	return responses
}

func responseWithSchema(description string, schema Schema) OpenAPIResponse {
	return OpenAPIResponse{
		Description: description,
		Content:     map[string]MediaType{jsonMediaType: {Schema: schema}},
	}
}

func apiErrors(authenticated bool, permission string) []Error {
	errors := []Error{
		{Status: http.StatusOK, Code: 1001, Description: "业务失败"},
		{Status: http.StatusOK, Code: 1002, Description: "参数校验失败"},
		{Status: http.StatusOK, Code: 1003, Description: "核心服务失败"},
	}
	if authenticated {
		errors = append(errors, Error{Status: http.StatusUnauthorized, Code: 1001, Description: "身份未验证"})
	}
	if permission != "" {
		errors = append(errors, Error{Status: http.StatusForbidden, Code: 1001, Description: "权限不足"})
	}
	errors = append(errors, Error{Status: http.StatusInternalServerError, Code: 1001, Description: "未处理的服务错误"})

	return errors
}

func schemaParameters(schema Schema) []Parameter {
	if schema.Ref != "" {
		return nil
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}
	result := make([]Parameter, 0, len(names))
	for _, name := range names {
		value := schema.Properties[name]
		result = append(result, Parameter{
			Name:        name,
			In:          "query",
			Description: value.Description,
			Required:    required[name],
			Schema:      value,
		})
	}

	return result
}

func resolveSchema(schema Schema, schemas map[string]Schema) Schema {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(schema.Ref, prefix) {
		return schema
	}
	if resolved, exists := schemas[strings.TrimPrefix(schema.Ref, prefix)]; exists {
		return resolved
	}

	return schema
}

func setOperation(item *PathItem, method string, operation *Operation) error {
	var target **Operation
	switch method {
	case http.MethodConnect:
		target = &item.Connect
	case http.MethodDelete:
		target = &item.Delete
	case http.MethodGet:
		target = &item.Get
	case http.MethodHead:
		target = &item.Head
	case http.MethodOptions:
		target = &item.Options
	case http.MethodPatch:
		target = &item.Patch
	case http.MethodPost:
		target = &item.Post
	case http.MethodPut:
		target = &item.Put
	case http.MethodTrace:
		target = &item.Trace
	default:
		return exception.Core(fmt.Sprintf("OpenAPI 不支持 HTTP Method %s", method))
	}
	if *target != nil {
		return exception.Core(fmt.Sprintf("OpenAPI 路径存在重复操作 %s", method))
	}
	*target = operation

	return nil
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

func setSchemaFormat(schema *Schema, logicalType coreentity.LogicalType) {
	switch logicalType {
	case coreentity.LogicalInt, coreentity.LogicalUint:
		schema.Type = "integer"
		schema.Format = "int64"
	case coreentity.LogicalFloat:
		schema.Type = "number"
		schema.Format = "double"
	case coreentity.LogicalBytes:
		schema.Type = "string"
		schema.Format = "byte"
	case coreentity.LogicalTime:
		schema.Type = "string"
		schema.Format = "date-time"
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

func objectSchema(properties map[string]Schema, required ...string) Schema {
	additionalProperties := false
	if properties == nil {
		properties = map[string]Schema{}
	}

	return Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             append([]string(nil), required...),
		AdditionalProperties: &additionalProperties,
	}
}

func arraySchema(items Schema) Schema {
	return Schema{Type: "array", Items: &items}
}

func refSchema(name string) Schema {
	return Schema{Ref: "#/components/schemas/" + name}
}

func schemaPrefix(key, fallback string) string {
	value := key + "." + fallback
	var result strings.Builder
	upper := true
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			if upper {
				character = unicode.ToUpper(character)
			}
			result.WriteRune(character)
			upper = false
			continue
		}
		upper = true
	}
	if result.Len() == 0 {
		return "Controller"
	}

	return result.String()
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

func cloneProperties(values map[string]Schema) map[string]Schema {
	result := make(map[string]Schema, len(values))
	for name, value := range values {
		result[name] = value
	}

	return result
}

func stringValues(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}

	return result
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
