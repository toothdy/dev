package eps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
	coreroute "github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

type epsItem struct {
	g.Meta `orm:"table:eps_item" description:"EPS 项目"`
	coreentity.Base
	Name    string  `json:"name" orm:"name" description:"名称" cool:"size=80"`
	Secret  string  `json:"secret" orm:"secret" description:"密钥"`
	Status  int32   `json:"status" orm:"status" description:"状态" cool:"default=1"`
	GroupID *uint64 `json:"groupId" orm:"groupId" description:"分组"`
}

type epsGroup struct {
	g.Meta `orm:"table:eps_group" description:"EPS 分组"`
	coreentity.Base
	Name string `json:"name" orm:"name" description:"名称"`
}

type epsColumnTypes struct {
	g.Meta `orm:"table:eps_column_types" description:"EPS 字段类型"`
	coreentity.Base
	Score   float64        `json:"score" orm:"score" description:"评分"`
	Price   float64        `json:"price" orm:"price" description:"价格" cool:"precision=10,scale=2"`
	RunAt   time.Time      `json:"runAt" orm:"runAt" description:"执行时间"`
	Payload []byte         `json:"payload" orm:"payload" description:"内容"`
	Config  map[string]any `json:"config" orm:"config" description:"配置" cool:"json=true"`
}

type epsService struct{}

func TestCompileViewsProjectsFinalContract(t *testing.T) {
	itemDescriptor, groupDescriptor := epsDescriptors(t)
	dynamicCalled := false
	definitions := []ControllerInput{
		{Key: "demo:item", Definition: itemDefinition()},
		{Key: "demo:dynamic", Definition: dynamicDefinition(&dynamicCalled)},
		{Key: "demo:app", Definition: appDefinition()},
	}
	views, err := CompileViews(Input{
		Graph:       epsGraph(t),
		Controllers: definitions,
		Descriptors: []coreentity.RuntimeDescriptor{itemDescriptor, groupDescriptor},
	}, false)
	if err != nil {
		fatalError(t, err)
	}
	if dynamicCalled {
		t.Fatal("CompileViews 执行了 DynamicQuery")
	}
	if len(views.Admin["demo"]) != 3 || len(views.App["demo"]) != 1 {
		t.Fatalf("views = %#v", views)
	}

	crudController := findController(t, views.Admin["demo"], "/admin/demo/archive/items")
	customController := findController(t, views.Admin["demo"], "/admin/demo/public/items")
	if crudController.Name != "EpsItemEntity" || len(crudController.API) != 1 || crudController.API[0].Method != "post" || crudController.API[0].Path != "/page" {
		t.Fatalf("CRUD Controller = %#v", crudController)
	}
	if customController.Name != "" || len(customController.API) != 2 || customController.API[0].Path != "/export" || !customController.API[0].IgnoreToken {
		t.Fatalf("custom Controller = %#v", customController)
	}
	if !findAPI(customController.API, "/v1/export") {
		t.Fatalf("后缀重叠路由 Prefix 错误: %#v", customController.API)
	}
	if findAPI(customController.API, "/detail/{id}") {
		t.Fatalf("参数化 API 未过滤: %#v", customController.API)
	}

	columns := columnNames(crudController.Columns)
	if columns["secret"] {
		t.Fatalf("隐藏字段 secret 出现在 columns: %#v", crudController.Columns)
	}
	if !columns["status"] {
		t.Fatalf("Readonly 字段未保留: %#v", crudController.Columns)
	}
	last := crudController.Columns[len(crudController.Columns)-2:]
	if last[0].PropertyName != "createTime" || last[1].PropertyName != "updateTime" {
		t.Fatalf("时间字段顺序错误: %#v", crudController.Columns)
	}
	status := findColumn(t, crudController.Columns, "status")
	if status.Type != "number" || status.DefaultValue != int64(1) {
		t.Fatalf("status column = %#v", status)
	}
	if findColumn(t, crudController.Columns, "name").Type != "string" || findColumn(t, crudController.PageColumns, "groupName").Type != "text" {
		t.Fatalf("字符串类型未按 Node 契约投影: %#v / %#v", crudController.Columns, crudController.PageColumns)
	}
	if findColumn(t, crudController.Columns, "createTime").Type != "varchar" {
		t.Fatalf("系统时间类型未按 Node 契约投影: %#v", crudController.Columns)
	}
	assertOptionalJSONFields(t, customController, findColumn(t, crudController.Columns, "id"))

	if len(crudController.PageQueryOp.KeyWordLikeFields) != 1 || crudController.PageQueryOp.KeyWordLikeFields[0] != "a.name" {
		t.Fatalf("keyword = %#v", crudController.PageQueryOp)
	}
	encoded, err := json.Marshal(crudController.PageQueryOp)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"fieldEq":["a.status"]`) ||
		!strings.Contains(text, `"fieldLike":[{"column":"a.name","requestParam":"query"}]`) {
		t.Fatalf("pageQueryOp JSON = %s", text)
	}
	if countSource(crudController.PageColumns, "g.name") != 3 {
		t.Fatalf("同源不同 alias 未保留: %#v", crudController.PageColumns)
	}
	if countSource(crudController.PageColumns, "a.secret") != 0 {
		t.Fatalf("隐藏字段出现在 pageColumns: %#v", crudController.PageColumns)
	}
	lastPageColumns := crudController.PageColumns[len(crudController.PageColumns)-2:]
	if lastPageColumns[0].PropertyName != "createTime" || lastPageColumns[0].Source != "g.name" ||
		lastPageColumns[1].PropertyName != "updateTime" {
		t.Fatalf("pageColumns 时间字段未按最终别名移尾: %#v", crudController.PageColumns)
	}
	if findColumn(t, crudController.PageColumns, "groupCreated").Source != "g.createTime" {
		t.Fatalf("非时间别名被错误移尾: %#v", crudController.PageColumns)
	}

	dynamic := findController(t, views.Admin["demo"], "/admin/demo/dynamic")
	if len(dynamic.PageQueryOp.FieldEq) != 0 || len(dynamic.PageColumns) != 0 {
		t.Fatalf("DynamicQuery 投影不为空: %#v", dynamic)
	}
	app := views.App["demo"][0]
	if app.Prefix != "/app/demo/open" || app.Name != "" || app.API[0].Path != "/ping" {
		t.Fatalf("App Controller = %#v", app)
	}
}

func TestColumnTypeMatchesNodeContract(t *testing.T) {
	descriptor, err := coreentity.Compile[epsColumnTypes, uint64](coreentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{
		"score":   "number",
		"price":   "decimal",
		"runAt":   "date",
		"payload": "text",
		"config":  "json",
	}
	for name, want := range wants {
		field, exists := descriptor.Field(name)
		if !exists {
			t.Fatalf("字段 %s 不存在", name)
		}
		if columnType(field) != want {
			t.Fatalf("columnType(%s) = %q, want %q", name, columnType(field), want)
		}
	}
}

func assertOptionalJSONFields(t *testing.T, custom Controller, column Column) {
	t.Helper()
	encoded, err := json.Marshal(custom)
	if err != nil {
		t.Fatal(err)
	}
	var controllerJSON map[string]any
	if err = json.Unmarshal(encoded, &controllerJSON); err != nil {
		t.Fatal(err)
	}
	if _, exists := controllerJSON["name"]; exists {
		t.Fatalf("纯自定义 Controller 不应输出 name: %s", encoded)
	}

	encoded, err = json.Marshal(column)
	if err != nil {
		t.Fatal(err)
	}
	var columnJSON map[string]any
	if err = json.Unmarshal(encoded, &columnJSON); err != nil {
		t.Fatal(err)
	}
	if _, exists := columnJSON["defaultValue"]; exists {
		t.Fatalf("无默认值字段不应输出 defaultValue: %s", encoded)
	}
	if _, exists := columnJSON["dict"]; exists {
		t.Fatalf("无字典字段不应输出 dict: %s", encoded)
	}
}

func TestCompileViewsIncludesDevelopmentRoutesOnDemand(t *testing.T) {
	itemDescriptor, groupDescriptor := epsDescriptors(t)
	views, err := CompileViews(Input{
		Graph: epsGraph(t),
		Controllers: []ControllerInput{
			{Key: "demo:item", Definition: itemDefinition()},
			{Key: "demo:dynamic", Definition: dynamicDefinition(new(bool))},
			{Key: "demo:app", Definition: appDefinition()},
		},
		Descriptors: []coreentity.RuntimeDescriptor{itemDescriptor, groupDescriptor},
	}, true)
	if err != nil {
		fatalError(t, err)
	}
	custom := findController(t, views.Admin["demo"], "/admin/demo/public/items")
	if !findAPI(custom.API, "/preview") {
		t.Fatalf("开发路由未包含: %#v", custom.API)
	}
}

func TestCompileViewsRejectsIncompleteRuntimeInput(t *testing.T) {
	itemDescriptor, groupDescriptor := epsDescriptors(t)
	input := Input{
		Graph: epsGraph(t),
		Controllers: []ControllerInput{
			{Key: "demo:item", Definition: itemDefinition()},
			{Key: "demo:dynamic", Definition: dynamicDefinition(new(bool))},
		},
		Descriptors: []coreentity.RuntimeDescriptor{itemDescriptor, groupDescriptor},
	}
	if _, err := CompileViews(input, false); err == nil || !strings.Contains(err.Error(), "缺少运行时 Definition") {
		t.Fatalf("missing definition error = %v", err)
	}
	input.Controllers = append(input.Controllers, ControllerInput{Key: "demo:app", Definition: appDefinition()})
	input.Descriptors = input.Descriptors[:1]
	if _, err := CompileViews(input, false); err == nil || !strings.Contains(err.Error(), "运行时 Descriptor") {
		t.Fatalf("missing descriptor error = %v", err)
	}
}

func itemDefinition() controller.Definition {
	return controller.Admin("public/items").
		Options(controller.RouterOptions{Description: "项目"}).
		Curd(controller.CurdOption{
			Prefix:  "archive/items",
			API:     controller.API(controller.Page),
			Entity:  epsItem{},
			Service: &epsService{},
			PageQueryOp: controller.StaticQuery(controller.QueryOp{
				KeyWordLikeFields: []controller.ColumnRef{controller.Field("name")},
				FieldEq:           []controller.FieldEq{controller.Eq(controller.Field("status"))},
				FieldLike:         []controller.FieldLike{controller.LikeFrom(controller.Field("name"), "query")},
				Join: []controller.JoinOp{controller.LeftJoin(
					epsGroup{},
					"g",
					controller.On(controller.FieldOf[epsGroup]("id").Of("g"), controller.FieldOf[epsItem]("groupId")),
				)},
				Select: []controller.SelectField{
					controller.As(controller.FieldOf[epsGroup]("name").Of("g"), "groupName"),
					controller.As(controller.FieldOf[epsGroup]("name").Of("g"), "groupLabel"),
					controller.As(controller.FieldOf[epsGroup]("createTime").Of("g"), "groupCreated"),
					controller.As(controller.FieldOf[epsGroup]("name").Of("g"), "createTime"),
					controller.As(controller.FieldOf[epsGroup]("updateTime").Of("g"), "updateTime"),
					controller.As(controller.Field("secret"), "secretAlias"),
				},
			}),
			HiddenFields:   []controller.ColumnRef{controller.Field("secret")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("status")},
		}).
		Route(
			controller.Route{Method: http.MethodGet, Path: "/export", Handler: controller.Handle(func() {}), Tags: []controller.URLTag{{Name: controller.TagIgnoreToken}}},
			controller.Route{Method: http.MethodGet, Path: "/v1/export", Handler: controller.Handle(func() {})},
			controller.Route{Method: http.MethodGet, Path: "/detail/{id}", Handler: controller.Handle(func() {})},
			controller.Route{Method: http.MethodGet, Path: "/preview", DevelopmentOnly: true, Handler: controller.Handle(func() {})},
		).
		Build()
}

func dynamicDefinition(called *bool) controller.Definition {
	return controller.Admin("dynamic").
		Curd(controller.CurdOption{
			API:     controller.API(controller.Page),
			Entity:  epsItem{},
			Service: &epsService{},
			PageQueryOp: controller.DynamicQuery(func(context.Context) (controller.QueryOp, error) {
				*called = true
				return controller.QueryOp{FieldEq: []controller.FieldEq{controller.Eq(controller.Field("status"))}}, nil
			}),
		}).
		Build()
}

func appDefinition() controller.Definition {
	return controller.App("open").
		Route(controller.Route{Method: http.MethodGet, Path: "/ping", Handler: controller.Handle(func() {})}).
		Build()
}

func epsDescriptors(t *testing.T) (coreentity.RuntimeDescriptor, coreentity.RuntimeDescriptor) {
	t.Helper()
	item, err := coreentity.Compile[epsItem, uint64](coreentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	group, err := coreentity.Compile[epsGroup, uint64](coreentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}

	return item, group
}

func epsGraph(t *testing.T) module.Graph {
	t.Helper()
	providers := []module.ProviderDefinition{
		{Kind: module.ProviderKindDescriptor, Module: "demo", PackagePath: "example.test/entity", Name: "ItemDescriptor", Type: "entity.Descriptor[epsItem, uint64]"},
		{Kind: module.ProviderKindDescriptor, Module: "demo", PackagePath: "example.test/entity", Name: "GroupDescriptor", Type: "entity.Descriptor[epsGroup, uint64]"},
	}
	controllers := []coreroute.ControllerDefinition{
		graphController("demo:item", "/admin/demo/public/items"),
		graphController("demo:dynamic", "/admin/demo/dynamic"),
		graphController("demo:app", "/app/demo/open"),
	}
	routes := []coreroute.Definition{
		graphRoute("demo:item", coreroute.KindCRUD, http.MethodPost, "/admin/demo/archive/items/page", false, nil),
		graphRoute("demo:item", coreroute.KindCustom, http.MethodGet, "/admin/demo/public/items/export", false, []string{controller.TagIgnoreToken}),
		graphRoute("demo:item", coreroute.KindCustom, http.MethodGet, "/admin/demo/public/items/v1/export", false, nil),
		graphRoute("demo:item", coreroute.KindCustom, http.MethodGet, "/admin/demo/public/items/detail/{id}", false, nil),
		graphRoute("demo:item", coreroute.KindCustom, http.MethodGet, "/admin/demo/public/items/preview", true, nil),
		graphRoute("demo:dynamic", coreroute.KindCRUD, http.MethodPost, "/admin/demo/dynamic/page", false, nil),
		graphRoute("demo:app", coreroute.KindCustom, http.MethodGet, "/app/demo/open/ping", false, nil),
	}
	graph, err := module.BuildGraph(module.GraphInput{
		Modules:     []module.ModuleDefinition{{Key: "demo", Name: "示例", Description: "示例模块"}},
		Providers:   providers,
		Descriptors: []module.DescriptorDefinition{{Module: "demo", Provider: providers[0], Table: "eps_item"}, {Module: "demo", Provider: providers[1], Table: "eps_group"}},
		Controllers: controllers,
		Routes:      routes,
	})
	if err != nil {
		t.Fatal(err)
	}

	return graph
}

func graphController(key, routePath string) coreroute.ControllerDefinition {
	return coreroute.ControllerDefinition{
		Key:         key,
		Module:      "demo",
		Path:        routePath,
		Factory:     coreroute.CallableRef{PackagePath: "example.test/controller", Symbol: strings.ReplaceAll(key, ":", ""), Type: "func() controller.Definition"},
		Description: "项目",
	}
}

func graphRoute(controllerKey string, kind coreroute.Kind, method, routePath string, development bool, tags []string) coreroute.Definition {
	return coreroute.Definition{
		Controller:      controllerKey,
		Kind:            kind,
		Method:          method,
		Path:            routePath,
		Bind:            coreroute.BindJSON,
		DevelopmentOnly: development,
		Tags:            tags,
		Handler: coreroute.CallableRef{
			Method:      "Handle",
			PackagePath: "example.test/controller",
			Symbol:      "Handler",
			Type:        "*controller.Handler",
		},
	}
}

func findController(t *testing.T, controllers []Controller, prefix string) Controller {
	t.Helper()
	for _, current := range controllers {
		if current.Prefix == prefix {
			return current
		}
	}
	t.Fatalf("找不到 Prefix %s: %#v", prefix, controllers)

	return Controller{}
}

func findAPI(apis []API, routePath string) bool {
	for _, api := range apis {
		if api.Path == routePath {
			return true
		}
	}

	return false
}

func findColumn(t *testing.T, columns []Column, name string) Column {
	t.Helper()
	for _, column := range columns {
		if column.PropertyName == name {
			return column
		}
	}
	t.Fatalf("找不到 Column %s: %#v", name, columns)

	return Column{}
}

func columnNames(columns []Column) map[string]bool {
	result := make(map[string]bool, len(columns))
	for _, column := range columns {
		result[column.PropertyName] = true
	}

	return result
}

func countSource(columns []Column, source string) int {
	count := 0
	for _, column := range columns {
		if column.Source == source {
			count++
		}

	}

	return count
}

func fatalError(t *testing.T, err error) {
	t.Helper()
	for err != nil {
		t.Logf("%T: %v", err, err)
		err = errors.Unwrap(err)
	}
	t.FailNow()
}
