package admin

import (
	"net/http"
	"testing"

	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
)

func TestInfoControllerMatchesNodeRoutesAndPermissions(t *testing.T) {
	definition := InfoController(nil, entity.TaskInfo())
	if definition.Prefix != "/admin/task/info" || definition.CRUD == nil {
		t.Fatalf("unexpected Task controller: %#v", definition)
	}
	if len(definition.CRUD.HiddenFields) != 1 || definition.CRUD.HiddenFields[0] != "lockOwner" {
		t.Fatalf("expected lockOwner to be hidden: %#v", definition.CRUD.HiddenFields)
	}
	hasReadonlyOwner := false
	for _, field := range definition.CRUD.ReadonlyFields {
		if field == "lockOwner" {
			hasReadonlyOwner = true
		}
	}
	if !hasReadonlyOwner {
		t.Fatalf("expected lockOwner to be readonly: %#v", definition.CRUD.ReadonlyFields)
	}
	routes := map[string]string{}
	for _, route := range definition.Routes {
		routes[route.Name] = route.Method
	}
	want := map[string]string{"once": http.MethodPost, "stop": http.MethodPost, "start": http.MethodPost, "log": http.MethodGet}
	for name, method := range want {
		if routes[name] != method {
			t.Fatalf("missing route %s %s: %#v", method, name, routes)
		}
	}
}
