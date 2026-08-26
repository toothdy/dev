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

func TestHandleCRUDOverrideIgnoresBaseCallbacks(t *testing.T) {
	dispatcher, err := crud.NewDispatcher(&pipelineRunner{})
	if err != nil {
		t.Fatal(err)
	}
	bindCalls := 0
	invokeCalls := 0

	err = HandleCRUD(
		t.Context(),
		nil,
		crud.ActionUpdate,
		crud.ActionModeOverride,
		dispatcher,
		func(context.Context) (*crud.QueryRequest, error) {
			bindCalls++
			return &crud.QueryRequest{}, nil
		},
		func(context.Context) error {
			t.Fatal("override 不应执行 Base 请求增强")
			return nil
		},
		func(context.Context, *crud.QueryRequest) (*crud.ActionPlan, error) {
			t.Fatal("override 不应编译 Base 动作计划")
			return nil, nil
		},
		func(context.Context) error {
			invokeCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bindCalls != 1 || invokeCalls != 1 {
		t.Fatalf("HandleCRUD() 调用次数 = bind:%d invoke:%d", bindCalls, invokeCalls)
	}
}
