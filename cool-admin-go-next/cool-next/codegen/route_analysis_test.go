package codegen

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	coreroute "github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

func TestAnalyzeDiscoversStaticRoutes(t *testing.T) {
	files := controllerRouteWorkspace()
	root := writeWorkspace(t, files)
	model, err := Analyze(context.Background(), Options{Dir: root, ModulesRoot: "modules"})
	if err != nil {
		t.Fatal(err)
	}
	authenticators := 0
	for _, constructor := range model.Modules()[0].Constructors() {
		if implementsHTTPAuthenticator(constructor.resultType) {
			authenticators++
		}
	}
	if authenticators != 1 {
		t.Fatalf("HTTP authenticators = %d, constructors = %#v", authenticators, model.Modules()[0].Constructors())
	}
	controllers := model.Modules()[0].Controllers()
	if len(controllers) != 1 {
		t.Fatalf("Controllers() = %#v", controllers)
	}
	controller := controllers[0]
	if !reflect.DeepEqual(controller.Aliases(), []string{"goods"}) ||
		!reflect.DeepEqual(controller.Middleware(), []string{"middleware.NewAudit"}) {
		t.Fatalf("Controller = %#v", controller)
	}
	routes := controller.Routes()
	if len(routes) != 9 {
		t.Fatalf("Routes() = %#v", routes)
	}
	want := map[string]struct {
		bind   coreroute.BindSource
		kind   coreroute.Kind
		path   string
		symbol string
	}{
		"POST add":     {bind: coreroute.BindJSON, kind: coreroute.KindCRUD, path: "/demo/sys/goods/add"},
		"POST delete":  {bind: coreroute.BindJSON, kind: coreroute.KindCustom, path: "/demo/sys/goods/delete", symbol: "GoodsHandler"},
		"POST update":  {bind: coreroute.BindJSON, kind: coreroute.KindCRUD, path: "/demo/sys/goods/update"},
		"GET info":     {bind: coreroute.BindQuery, kind: coreroute.KindCRUD, path: "/demo/sys/goods/info"},
		"POST list":    {bind: coreroute.BindJSON, kind: coreroute.KindCRUD, path: "/demo/sys/goods/list"},
		"POST page":    {bind: coreroute.BindJSON, kind: coreroute.KindCRUD, path: "/demo/sys/goods/page"},
		"POST disable": {bind: coreroute.BindPath, kind: coreroute.KindCustom, path: "/demo/sys/goods/disable/{id}"},
		"GET health":   {bind: coreroute.BindQuery, kind: coreroute.KindCustom, path: "/demo/sys/goods/health"},
		"POST ping":    {bind: coreroute.BindJSON, kind: coreroute.KindCustom, path: "/demo/sys/goods/ping"},
	}
	for _, route := range routes {
		name := route.Method() + " " + routeName(route.Path())
		expected, exists := want[name]
		if !exists || route.Bind() != expected.bind || route.Kind() != expected.kind || route.Path() != expected.path ||
			expected.symbol != "" && route.handler.Symbol != expected.symbol {
			t.Fatalf("Route = %#v", route)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing routes = %#v", want)
	}
	if !reflect.DeepEqual(routes[2].Tags(), []string{"ignoreToken"}) {
		t.Fatalf("Info tags = %#v", routes[2].Tags())
	}
	custom := routes[6]
	if !reflect.DeepEqual(custom.Middleware(), []string{"middleware.NewRoute"}) ||
		!reflect.DeepEqual(custom.Tags(), []string{"audit"}) {
		t.Fatalf("custom route = %#v", custom)
	}
}

func TestAnalyzeKeepsTypedServiceOverridesAsCRUDRoutes(t *testing.T) {
	files := controllerAnalysisWorkspace()
	files["modules/demo/entity/product.go"] += `
func ProductSchema() coreentity.Schema { return coreentity.Schema{} }
`
	files["modules/demo/dto/page.go"] = `package dto
type ProductPageReq struct { Page int }
type ProductPageResult struct { Total int }
`
	files["modules/demo/service/product.go"] = `package service
import (
	"context"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"example.test/app/modules/demo/dto"
	"example.test/app/modules/demo/entity"
)
type ProductService struct { *coreservice.Base[entity.Product, uint64] }
func (*ProductService) Info(context.Context, uint64) (map[string]string, error) {
	return map[string]string{"name": "product"}, nil
}
func (*ProductService) List(context.Context, coreservice.Query) ([]string, error) {
	return []string{"product"}, nil
}
func (*ProductService) Page(context.Context, *dto.ProductPageReq) (dto.ProductPageResult, error) {
	return dto.ProductPageResult{Total: 1}, nil
}
`
	files["modules/demo/controller/admin/product.go"] = `package admin
import (
	controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"example.test/app/modules/demo/entity"
	demoservice "example.test/app/modules/demo/service"
)
func ProductController(service *demoservice.ProductService) controller.Definition {
	return controller.Admin("product").Curd(controller.CurdOption{
		API: controller.AllAPI(),
		Entity: entity.Product{},
		Service: service,
	}).Build()
}
`
	root := writeWorkspace(t, files)
	model, err := Analyze(context.Background(), Options{Dir: root, ModulesRoot: "modules"})
	if err != nil {
		t.Fatal(err)
	}
	routes := model.Modules()[0].Controllers()[0].Routes()
	if len(routes) != 6 {
		t.Fatalf("Routes() = %#v", routes)
	}
	for _, route := range routes {
		if route.Kind() != coreroute.KindCRUD {
			t.Fatalf("Route kind = %s", route.Kind())
		}
		if route.handler.Method == "Page" {
			if !route.handler.HasRequest || route.handler.RequestType != "ProductPageReq" ||
				route.handler.RequestPackagePath != "example.test/app/modules/demo/dto" {
				t.Fatalf("Page handler = %#v", route.handler)
			}
		}
	}
}

func TestAnalyzeReportsRouteDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		controller string
		service    string
		code       string
	}{
		{
			name: "invalid CRUD override",
			controller: `package admin
import (
	controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"example.test/app/modules/demo/entity"
	demoservice "example.test/app/modules/demo/service"
)
func GoodsController(service *demoservice.ProductService) controller.Definition {
	return controller.Admin("").Curd(controller.CurdOption{
		API: controller.API(controller.Delete), Entity: entity.Product{}, Service: service,
	}).Build()
}
`,
			code: "CG102",
		},
		{
			name: "invalid handler signature",
			controller: `package admin
import controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
import demoservice "example.test/app/modules/demo/service"
func GoodsController(service *demoservice.ProductService) controller.Definition {
	return controller.Admin("").Route(controller.Route{Method: "POST", Path: "/disable", Handler: controller.Handle(service.Invalid)}).Build()
}
`,
			service: `func (service *ProductService) Invalid(context.Context, *dto.DisableReq, string) error { return nil }
`,
			code: "CG102",
		},
		{
			name: "ambiguous bind",
			controller: `package admin
import controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
import demoservice "example.test/app/modules/demo/service"
func GoodsController(service *demoservice.ProductService) controller.Definition {
	return controller.Admin("").Route(controller.Route{Method: "POST", Path: "/disable/{id}", Handler: controller.Handle(service.Ambiguous)}).Build()
}
`,
			service: `func (service *ProductService) Ambiguous(context.Context, *dto.AmbiguousReq) error { return nil }
`,
			code: "CG101",
		},
		{
			name: "duplicate route",
			controller: `package admin
import controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
import demoservice "example.test/app/modules/demo/service"
func GoodsController(service *demoservice.ProductService) controller.Definition {
	route := controller.Route{Method: "POST", Path: "/disable/{id}", Handler: controller.Handle(service.Disable)}
	return controller.Admin("").Route(route, route).Build()
}
`,
			code: "CG103",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := controllerRouteWorkspace()
			files["modules/demo/controller/admin/sys/goods.go"] = test.controller
			if test.service != "" {
				files["modules/demo/service/product.go"] += test.service
			}
			root := writeWorkspace(t, files)
			_, err := Analyze(context.Background(), Options{Dir: root, ModulesRoot: "modules"})
			var diagnostics *DiagnosticError
			if !errors.As(err, &diagnostics) {
				t.Fatalf("Analyze() error = %v", err)
			}
			values := diagnostics.Diagnostics()
			if len(values) != 1 || values[0].Code != test.code {
				t.Fatalf("Diagnostics() = %#v", values)
			}
		})
	}
}

func controllerRouteWorkspace() map[string]string {
	files := controllerAnalysisWorkspace()
	files["modules/demo/entity/product.go"] += `
func ProductSchema() coreentity.Schema { return coreentity.Schema{} }
func OtherSchema() coreentity.Schema { return coreentity.Schema{} }
`
	files["modules/demo/dto/route.go"] = `package dto
type DisableReq struct { ID uint64 ` + "`in:\"path\"`" + ` }
type DeleteReq struct { ID uint64 }
type AmbiguousReq struct {
	ID uint64 ` + "`in:\"path\"`" + `
	Filter string ` + "`in:\"query\"`" + `
}
`
	files["modules/demo/service/auth/auth.go"] = `package auth
import "context"
type Authenticator struct{}
func NewAuthenticator() *Authenticator { return &Authenticator{} }
func (*Authenticator) AuthenticateHTTP(ctx context.Context, _, _, _, _ string, _ bool) (context.Context, error) {
	return ctx, nil
}
`
	files["modules/demo/middleware/middleware.go"] = `package middleware
import "github.com/gogf/gf/v2/net/ghttp"
type Audit struct{}
type Route struct{}
func NewAudit() *Audit { return &Audit{} }
func NewRoute() *Route { return &Route{} }
func (*Audit) Handle(request *ghttp.Request) { request.Middleware.Next() }
func (*Route) Handle(request *ghttp.Request) { request.Middleware.Next() }
`
	files["modules/demo/service/product.go"] = `package service
import (
	"context"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"example.test/app/modules/demo/dto"
	"example.test/app/modules/demo/entity"
)
type ProductService struct { *coreservice.Base[entity.Product, uint64] }
func NewProductService() *ProductService { return &ProductService{} }
func (service *ProductService) Delete(context.Context, dto.DeleteReq) error { return nil }
func (service *ProductService) Disable(context.Context, *dto.DisableReq) error { return nil }
func (service *ProductService) Health(context.Context) (map[string]string, error) { return map[string]string{"status": "ok"}, nil }
func (service *ProductService) Ping(context.Context) error { return nil }
`
	files["modules/demo/controller/admin/sys/goods.go"] = `package sys
import (
	"context"
	controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	module "github.com/toothdy/cool-admin-go-next/cool-next/core/module"
	"example.test/app/modules/demo/dto"
	"example.test/app/modules/demo/entity"
	demoservice "example.test/app/modules/demo/service"
)
type GoodsHandler struct{}
func NewGoodsHandler() *GoodsHandler { return &GoodsHandler{} }
func (*GoodsHandler) Delete(context.Context, *dto.DeleteReq) error { return nil }
func GoodsController(service *demoservice.ProductService, handler *GoodsHandler) controller.Definition {
	return controller.Admin("").
		Options(controller.RouterOptions{
			Alias: []string{"goods"},
			DevelopmentOnly: true,
			Middleware: []controller.MiddlewareRef{module.Ref("middleware.NewAudit")},
			Description: "商品管理",
			TagName: "商品",
			IgnoreGlobalPrefix: true,
		}).
		Curd(controller.CurdOption{
			API: controller.API(controller.Add, controller.Update, controller.Info, controller.List, controller.Page),
			Entity: entity.Product{},
			Service: service,
			URLTag: &controller.URLTag{Name: controller.TagIgnoreToken, URL: controller.API(controller.Info)},
		}).
		Route(
			controller.Route{
				Method: "POST",
				Path: "/delete",
				Handler: controller.Handle(handler.Delete),
			},
			controller.Route{
				Method: "POST",
				Path: "/disable/{id}",
				Handler: controller.Handle(service.Disable),
				Middleware: []controller.MiddlewareRef{module.Ref("middleware.NewRoute")},
				Tags: []controller.URLTag{{Name: "audit"}},
				DevelopmentOnly: true,
			},
		).
		Route(
			controller.Route{Method: "GET", Path: "/health", Handler: controller.Handle(service.Health), Transaction: controller.NonTransactional()},
			controller.Route{Method: "POST", Path: "/ping", Handler: controller.Handle(service.Ping), Transaction: controller.NonTransactional()},
		).
		Build()
}
`

	return files
}

func routeName(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) > 1 && strings.HasPrefix(parts[len(parts)-1], "{") {
		return parts[len(parts)-2]
	}

	return parts[len(parts)-1]
}
