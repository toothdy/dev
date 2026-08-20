package task

import (
	"context"
	"errors"
	"testing"
)

func TestRegistryFreezesDefinitionsAndFindsHandler(t *testing.T) {
	builder := NewRegistryBuilder()
	handler := func(context.Context, Invocation) (interface{}, error) { return "ok", nil }
	if err := builder.Register(HandlerDefinition{Name: "taskDemoService.test", Handler: handler}); err != nil {
		t.Fatalf("register handler failed: %v", err)
	}
	registry, err := builder.Freeze()
	if err != nil {
		t.Fatalf("freeze registry failed: %v", err)
	}
	definition, ok := registry.Find("taskDemoService.test")
	if !ok || definition.Handler == nil {
		t.Fatalf("handler missing from registry: %#v", definition)
	}
	if err = builder.Register(HandlerDefinition{Name: "otherService.test", Handler: handler}); err == nil {
		t.Fatal("expected frozen builder to reject registration")
	}
}

func TestRegistryRejectsDuplicateAndInvalidHandlers(t *testing.T) {
	handler := func(context.Context, Invocation) (interface{}, error) { return nil, nil }
	builder := NewRegistryBuilder()
	if err := builder.Register(HandlerDefinition{Name: "taskDemoService.test", Handler: handler}); err != nil {
		t.Fatalf("register handler failed: %v", err)
	}
	if err := builder.Register(HandlerDefinition{Name: "taskDemoService.test", Handler: handler}); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if err := NewRegistryBuilder().Register(HandlerDefinition{Name: "unsafe/path", Handler: handler}); err == nil {
		t.Fatal("expected invalid name error")
	}
	if err := NewRegistryBuilder().Register(HandlerDefinition{Name: "taskDemoService.test"}); err == nil {
		t.Fatal("expected nil handler error")
	}
}

func TestPermanentErrorClassification(t *testing.T) {
	cause := errors.New("invalid business input")
	err := Permanent(cause)
	if !IsPermanent(err) || !errors.Is(err, cause) {
		t.Fatalf("unexpected permanent error: %v", err)
	}
	if IsPermanent(cause) {
		t.Fatal("ordinary errors must remain retryable")
	}
}
