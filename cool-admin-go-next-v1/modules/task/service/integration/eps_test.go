package integration_test

import (
	"net/http"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	eps "github.com/toothdy/cool-admin-go-next/cool/util/eps"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestTaskEPSKeepsVueServiceContract(t *testing.T) {
	application := app.New(app.Options{
		StartServer: false, UploadDir: t.TempDir(), Specs: modules.Specs(),
		SessionStore: security.NewMemorySessionStore(),
	})
	controllers := module.CollectControllers(application.Modules())
	generated := eps.GenerateAdmin(controllers)
	if len(generated["task"]) != 1 {
		t.Fatalf("expected one Task EPS controller: %#v", generated["task"])
	}
	ctrl := generated["task"][0]
	if ctrl.Name != "TaskInfoEntity" || ctrl.Prefix != "/admin/task/info" {
		t.Fatalf("unexpected Task EPS controller: %#v", ctrl)
	}
	for _, column := range ctrl.Columns {
		if column.PropertyName == "lockOwner" {
			t.Fatal("Task EPS must not expose the internal lease owner")
		}
	}
	apiByPath := map[string]string{}
	for _, api := range ctrl.API {
		apiByPath[api.Path] = api.Method
	}
	want := map[string]string{
		"/add": http.MethodPost, "/delete": http.MethodPost, "/update": http.MethodPost,
		"/info": http.MethodGet, "/page": http.MethodPost, "/start": http.MethodPost,
		"/stop": http.MethodPost, "/once": http.MethodPost, "/log": http.MethodGet,
	}
	for path, method := range want {
		if apiByPath[path] != method {
			t.Fatalf("missing Task EPS API %s %s: %#v", method, path, apiByPath)
		}
	}
	permissions, err := controller.PermissionMap(controllers)
	if err != nil {
		t.Fatalf("build permission map failed: %v", err)
	}
	for route, permission := range map[string]string{
		"POST:/admin/task/info/start": "task:info:start",
		"POST:/admin/task/info/stop":  "task:info:stop",
		"POST:/admin/task/info/once":  "task:info:once",
		"GET:/admin/task/info/log":    "task:info:log",
	} {
		if permissions[route] != permission {
			t.Fatalf("unexpected permission for %s: %q", route, permissions[route])
		}
	}
}