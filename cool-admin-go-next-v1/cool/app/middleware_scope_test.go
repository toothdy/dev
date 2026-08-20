package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestValidateControllerModules(t *testing.T) {
	tests := []struct {
		name        string
		spec        module.Spec
		controllers []controller.Definition
		wantError   string
	}{
		{
			name: "匹配",
			spec: module.Spec{Key: "task"},
			controllers: []controller.Definition{
				controller.Admin("task/info").Name("任务信息").Build(),
				controller.App("task/info").Name("应用任务信息").Build(),
			},
		},
		{
			name:        "不匹配",
			spec:        module.Spec{Key: "task"},
			controllers: []controller.Definition{controller.Admin("base/info").Name("错误归属").Build()},
			wantError:   "spec=task controller=错误归属 prefix=/admin/base/info module=base",
		},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			err := validateControllerModules(item.spec, item.controllers)
			if item.wantError == "" && err != nil {
				t.Fatalf("expected valid controller modules: %v", err)
			}
			if item.wantError != "" && (err == nil || !strings.Contains(err.Error(), item.wantError)) {
				t.Fatalf("unexpected module validation error: %v", err)
			}
		})
	}
}

func TestValidateMiddlewarePlanRejectsCrossScopeDuplicates(t *testing.T) {
	handler := func(request *ghttp.Request) { request.Middleware.Next() }
	plan := &middlewarePlan{
		global: []middleware.Definition{{Name: "plugin.audit", Order: 250, Handler: handler}},
		modules: map[string][]middleware.Definition{
			"task": {{Name: "plugin.audit", Order: 300, Handler: handler}},
		},
	}
	err := validateMiddlewarePlan(plan, []module.Spec{{Key: "task"}})
	if err == nil || !strings.Contains(err.Error(), "plugin.audit") {
		t.Fatalf("expected cross-scope duplicate rejected: %v", err)
	}
}

func TestValidateMiddlewarePlanSortsEachScopeStably(t *testing.T) {
	handler := func(request *ghttp.Request) { request.Middleware.Next() }
	definition := func(name string, order int) middleware.Definition {
		return middleware.Definition{Name: name, Order: order, Handler: handler}
	}
	plan := &middlewarePlan{
		global: []middleware.Definition{
			definition("global-last", 400),
			definition("global-first", 200),
		},
		modules: map[string][]middleware.Definition{
			"task": {
				definition("task-second-a", 300),
				definition("task-first", 200),
				definition("task-second-b", 300),
			},
		},
	}
	if err := validateMiddlewarePlan(plan, []module.Spec{{Key: "task"}}); err != nil {
		t.Fatal(err)
	}
	globalNames := []string{plan.global[0].Name, plan.global[1].Name}
	moduleNames := []string{plan.modules["task"][0].Name, plan.modules["task"][1].Name, plan.modules["task"][2].Name}
	if !reflect.DeepEqual(globalNames, []string{"global-first", "global-last"}) {
		t.Fatalf("unexpected global order: %#v", globalNames)
	}
	if !reflect.DeepEqual(moduleNames, []string{"task-first", "task-second-a", "task-second-b"}) {
		t.Fatalf("unexpected module order: %#v", moduleNames)
	}
}
