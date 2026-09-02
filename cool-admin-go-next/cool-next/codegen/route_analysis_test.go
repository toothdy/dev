package codegen

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/route"
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
		bind   route.BindSource
		kind   route.Kind
		path   string
		symbol string
	}{
		"POST add":     {bind: route.BindJSON, kind: route.KindCRUD, path: "/demo/sys/goods/add"},
		"POST delete":  {bind: route.BindJSON, kind: route.KindCustom, path: "/demo/sys/goods/delete", symbol: "GoodsHandler"},
		"POST update":  {bind: route.BindJSON, kind: route.KindCRUD, path: "/demo/sys/goods/update"},
		"GET info":     {bind: route.BindQuery, kind: route.KindCRUD, path: "/demo/sys/goods/info"},
		"POST list":    {bind: route.BindJSON, kind: route.KindCRUD, path: "/demo/sys/goods/list"},
		"POST page":    {bind: route.BindJSON, kind: route.KindCRUD, path: "/demo/sys/goods/page"},
		"POST disable": {bind: route.BindPath, kind: route.KindCustom, path: "/demo/sys/goods/disable/{id}"},
		"GET health":   {bind: route.BindQuery, kind: route.KindCustom, path: "/demo/sys/goods/health"},
		"POST ping":    {bind: route.BindJSON, kind: route.KindCustom, path: "/demo/sys/goods/ping"},
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
func ProductSchema() gnentity.Schema { return gnentity.Schema{} }
`
	files["modules/demo/dto/page.go"] = `package dto
type ProductPageReq struct { Page int }
type ProductPageResult struct { Total int }
`
	files["modules/demo/service/product.go"] = `package service
import (
	"context"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnservice"
	"example.test/app/modules/demo/dto"
	"example.test/app/modules/demo/entity"
)
type ProductService struct { *gnservice.Base[entity.Product, uint64] }
func (*ProductService) Info(context.Context, uint64) (map[string]string, error) {
	return map[string]string{"name": "product"}, nil
}
func (*ProductService) List(context.Context, gnservice.Query) ([]string, error) {
	return []string{"product"}, nil
}
func (*ProductService) Page(context.Context, *dto.ProductPageReq) (dto.ProductPageResult, error) {
	return dto.ProductPageResult{Total: 1}, nil
}
`
	files["modules/demo/controller/admin/product.go"] = `package admin
import (
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
	"example.test/app/modules/demo/entity"
	demoservice "example.test/app/modules/demo/service"
)
func ProductController(service *demoservice.ProductService) gnctrl.Definition {
	return gnctrl.Admin("product").Curd(gnctrl.CurdOption{
		API: gnctrl.AllAPI(),
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
	for _, routeDeclaration := range routes {
		if routeDeclaration.Kind() != route.KindCRUD {
			t.Fatalf("Route kind = %s", routeDeclaration.Kind())
		}
		if routeDeclaration.handler.Method == "Page" {
			if !routeDeclaration.handler.HasRequest || routeDeclaration.handler.RequestType != "ProductPageReq" ||
				routeDeclaration.handler.RequestPackagePath != "example.test/app/modules/demo/dto" {
				t.Fatalf("Page handler = %#v", routeDeclaration.handler)
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
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
	"example.test/app/modules/demo/entity"
	demoservice "example.test/app/modules/demo/service"
)
func GoodsController(service *demoservice.ProductService) gnctrl.Definition {
	return gnctrl.Admin("").Curd(gnctrl.CurdOption{
		API: gnctrl.API(gnctrl.Delete), Entity: entity.Product{}, Service: service,
	}).Build()
}
`,
			code: "CG102",
		},
		{
			name: "invalid handler signature",
			controller: `package admin
import "github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
import demoservice "example.test/app/modules/demo/service"
func GoodsController(service *demoservice.ProductService) gnctrl.Definition {
	return gnctrl.Admin("").Route(gnctrl.Route{Method: "POST", Path: "/disable", Handler: gnctrl.Handle(service.Invalid)}).Build()
}
`,
			service: `func (service *ProductService) Invalid(context.Context, *dto.DisableReq, string) error { return nil }
`,
			code: "CG102",
		},
		{
			name: "ambiguous bind",
			controller: `package admin
import "github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
import demoservice "example.test/app/modules/demo/service"
func GoodsController(service *demoservice.ProductService) gnctrl.Definition {
	return gnctrl.Admin("").Route(gnctrl.Route{Method: "POST", Path: "/disable/{id}", Handler: gnctrl.Handle(service.Ambiguous)}).Build()
}
`,
			service: `func (service *ProductService) Ambiguous(context.Context, *dto.AmbiguousReq) error { return nil }
`,
			code: "CG101",
		},
		{
			name: "duplicate route",
			controller: `package admin
import "github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
import demoservice "example.test/app/modules/demo/service"
func GoodsController(service *demoservice.ProductService) gnctrl.Definition {
	route := gnctrl.Route{Method: "POST", Path: "/disable/{id}", Handler: gnctrl.Handle(service.Disable)}
	return gnctrl.Admin("").Route(route, route).Build()
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
func ProductSchema() gnentity.Schema { return gnentity.Schema{} }
func OtherSchema() gnentity.Schema { return gnentity.Schema{} }
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
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnservice"
	"example.test/app/modules/demo/dto"
	"example.test/app/modules/demo/entity"
)
type ProductService struct { *gnservice.Base[entity.Product, uint64] }
func NewProductService() *ProductService { return &ProductService{} }
func (service *ProductService) Delete(context.Context, dto.DeleteReq) error { return nil }
func (service *ProductService) Disable(context.Context, *dto.DisableReq) error { return nil }
func (service *ProductService) Health(context.Context) (map[string]string, error) { return map[string]string{"status": "ok"}, nil }
func (service *ProductService) Ping(context.Context) error { return nil }
`
	files["modules/demo/controller/admin/sys/goods.go"] = `package sys
import (
	"context"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
	"example.test/app/modules/demo/dto"
	"example.test/app/modules/demo/entity"
	demoservice "example.test/app/modules/demo/service"
)
type GoodsHandler struct{}
func NewGoodsHandler() *GoodsHandler { return &GoodsHandler{} }
func (*GoodsHandler) Delete(context.Context, *dto.DeleteReq) error { return nil }
func GoodsController(service *demoservice.ProductService, handler *GoodsHandler) gnctrl.Definition {
	return gnctrl.Admin("").
		Options(gnctrl.RouterOptions{
			Alias: []string{"goods"},
			DevelopmentOnly: true,
			Middleware: []gnctrl.MiddlewareRef{module.Ref("middleware.NewAudit")},
			Description: "商品管理",
			TagName: "商品",
			IgnoreGlobalPrefix: true,
		}).
		Curd(gnctrl.CurdOption{
			API: gnctrl.API(gnctrl.Add, gnctrl.Update, gnctrl.Info, gnctrl.List, gnctrl.Page),
			Entity: entity.Product{},
			Service: service,
			URLTag: &gnctrl.URLTag{Name: gnctrl.TagIgnoreToken, URL: gnctrl.API(gnctrl.Info)},
		}).
		Route(
			gnctrl.Route{
				Method: "POST",
				Path: "/delete",
				Handler: gnctrl.Handle(handler.Delete),
			},
			gnctrl.Route{
				Method: "POST",
				Path: "/disable/{id}",
				Handler: gnctrl.Handle(service.Disable),
				Middleware: []gnctrl.MiddlewareRef{module.Ref("middleware.NewRoute")},
				Tags: []gnctrl.URLTag{{Name: "audit"}},
				DevelopmentOnly: true,
			},
		).
		Route(
			gnctrl.Route{Method: "GET", Path: "/health", Handler: gnctrl.Handle(service.Health), Transaction: gnctrl.NonTransactional()},
			gnctrl.Route{Method: "POST", Path: "/ping", Handler: gnctrl.Handle(service.Ping), Transaction: gnctrl.NonTransactional()},
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
