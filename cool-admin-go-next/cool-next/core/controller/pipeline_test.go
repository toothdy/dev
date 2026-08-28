package controller

import (
	"context"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	dbtx "github.com/toothdy/cool-admin-go-next/cool-next/db/tx"
)

type pipelineRunner struct{}

func (*pipelineRunner) Group() string {
	return "default"
}

func (*pipelineRunner) Within(ctx context.Context, callback dbtx.Callback) error {
	return callback(ctx)
}

func TestHandleCRUDOverrideRunsDeclaredPipelineWithoutBaseHooks(t *testing.T) {
	dispatcher, err := crud.NewDispatcher(&pipelineRunner{})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]string, 0, 5)
	definition := Admin("pipeline").Curd(CurdOption{
		API:     API(Update),
		Entity:  projectionEntity{},
		Service: &projectionService{},
		Before: func(context.Context) error {
			events = append(events, "before")
			return nil
		},
	}).Build()
	request := &crud.QueryRequest{}
	plan, err := crud.CompilePlan(t.Context(), nil, crud.PlanInput{Action: crud.ActionUpdate}, request)
	if err != nil {
		t.Fatal(err)
	}

	err = HandleCRUD(
		t.Context(),
		definition,
		crud.ActionUpdate,
		crud.ActionModeOverride,
		dispatcher,
		func(context.Context) (*crud.QueryRequest, error) {
			events = append(events, "bind")
			return request, nil
		},
		func(context.Context) error {
			events = append(events, "enhance")
			return nil
		},
		func(_ context.Context, current *crud.QueryRequest) (*crud.ActionPlan, error) {
			if current != request {
				t.Fatal("计划编译收到的请求已变化")
			}
			events = append(events, "compile")
			return plan, nil
		},
		func(ctx context.Context) error {
			events = append(events, "invoke")
			operation, exists := crud.CurrentOperation(ctx)
			if !exists || operation.Plan() != plan {
				t.Fatalf("override operation = %#v, %t", operation, exists)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"before", "bind", "enhance", "compile", "invoke"}
	if len(events) != len(want) {
		t.Fatalf("HandleCRUD() events = %#v", events)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("HandleCRUD() events = %#v", events)
		}
	}
}
