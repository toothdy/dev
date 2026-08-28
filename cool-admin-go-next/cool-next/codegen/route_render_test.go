package codegen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderEmitsCompilableStaticRoutes(t *testing.T) {
	files := controllerRouteWorkspace()
	files["modules/demo/service/product.go"] += `
func (*ProductService) Page(context.Context, *dto.DeleteReq) (map[string]int, error) {
	return map[string]int{"total": 1}, nil
}
`
	files["modules/demo/controller/admin/coding.go"] = `package admin
import (
	"context"
	controller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
)
type CodingHandler struct{}
func NewCodingHandler() *CodingHandler { return &CodingHandler{} }
func (*CodingHandler) Ping(context.Context) error { return nil }
func CodingController(handler *CodingHandler) controller.Definition {
	return controller.Admin("coding").Route(controller.Route{Method: "GET", Path: "/ping", Handler: controller.Handle(handler.Ping)}).Build()
}
`
	root := writeWorkspace(t, files)
	model, err := Analyze(context.Background(), Options{Dir: root, ModulesRoot: "modules"})
	if err != nil {
		t.Fatal(err)
	}
	descriptors, err := CompileDescriptors(model)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := BuildGraphWithDescriptors(model, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	source, err := Render(model, graph, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	content := string(source)
	for _, want := range []string{
		"func controllerdemoexample_test_app_modules_demo_controller_admin_sysGoodsController",
		"Controllers: []coreroute.ControllerDefinition",
		"Routes: []coreroute.Definition",
		"Kind: coreroute.KindCRUD",
		"Kind: coreroute.KindCustom",
		"Bind: coreroute.BindPath",
		"Path: \"/demo/sys/goods/disable/{id}\"",
		", \"/demo/sys/goods/disable/{id}\", false)",
		", \"/demo/sys/goods/info\", true)",
		"eps.CompileViews(eps.Input{",
		"Controllers: []eps.ControllerInput",
		"Definition: controller_demoexample_test_app_modules_demo_controller_adminCodingController",
		"Descriptors: []coreentity.RuntimeDescriptor",
		"gmode.IsDevelop()",
		"eps.PublishViews(epsViews)",
		"generatedHTTPInstaller := func(server *ghttp.Server) error",
		"if gmode.IsDevelop()",
		"server.BindMiddleware(\"POST:/demo/sys/goods/disable/{id}\"",
		"server.BindHandler(\"GET:/demo/sys/goods/health\"",
		"corecontroller.HandleDTO[dto.DisableReq]",
		"corecontroller.HandleCRUDDTO[dto.DeleteReq]",
		"corecontroller.HandleNoDTO",
		"component_demoexample_test_app_modules_demo_controller_admin_sysNewGoodsHandler.Delete(scopeCtx, input)",
		"return instance.Base.Delete(ctx, input)",
		"return adapterdemoexample_test_app_modules_demo_serviceProductServicePage(scopeCtx, component_demoexample_test_app_modules_demo_serviceNewProductService, input)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Render() output missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Permission:") || strings.Contains(content, "auth.Rule") {
		t.Fatalf("Render() output contains static permission declarations:\n%s", content)
	}
	if strings.Contains(content, "func generatedHTTPInstaller(*ghttp.Server) error { return nil }") {
		t.Fatalf("Render() output contains empty HTTP installer:\n%s", content)
	}
	if !strings.Contains(content, "controller_demoexample_test_app_modules_demo_controller_adminCodingController :=") {
		t.Fatalf("Render() output does not create pure custom controller definition for EPS:\n%s", content)
	}
	writeTestFile(t, root, "modules/modules_gen.go", content)
	writeTestFile(t, root, "modules/routes_gen_test.go", `package modules
import "testing"
func TestGeneratedRoutes(t *testing.T) {
	table := Generated().Routes()
	if len(table.Controllers()) != 2 || len(table.Routes()) != 10 {
		t.Fatalf("routes = %#v, %#v", table.Controllers(), table.Routes())
	}
}
`)
	command := exec.Command("go", "test", "-mod=mod", "./modules", "-count=1")
	command.Dir = root
	command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(root, ".cache", "go-build"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated route test error = %v\n%s\n%s", err, output, content)
	}
}
