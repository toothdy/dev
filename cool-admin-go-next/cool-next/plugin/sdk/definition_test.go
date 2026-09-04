package sdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/abi"
)

type echoRequest struct {
	Value string `json:"value"`
}

type echoResponse struct {
	Value string `json:"value"`
}

func TestMethodConvertsTypedHandler(t *testing.T) {
	definition := Define(Method("echo", func(_ context.Context, request echoRequest) (echoResponse, error) {
		return echoResponse{Value: request.Value}, nil
	}))
	handler := definition.methods["echo"]
	result, err := handler(t.Context(), json.RawMessage(`{"value":"test"}`))
	if err != nil || string(result) != `{"value":"test"}` {
		t.Fatalf("typed handler result = %s, error = %v", result, err)
	}

	_, err = handler(t.Context(), json.RawMessage(`{"value":`))
	assertPluginError(t, err, abi.ErrorInvalidInput)
}

func TestDefineRejectsInvalidOptions(t *testing.T) {
	testCases := []struct {
		name    string
		options []Option
	}{
		{name: "nil option", options: []Option{nil}},
		{name: "invalid name", options: []Option{RawMethod("Echo", func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil })}},
		{name: "nil raw handler", options: []Option{RawMethod("echo", nil)}},
		{name: "nil typed handler", options: []Option{Method[echoRequest, echoResponse]("echo", nil)}},
		{name: "duplicate method", options: []Option{RawMethod("echo", emptyRawHandler), RawMethod("echo", emptyRawHandler)}},
		{name: "nil ready", options: []Option{Ready(nil)}},
		{name: "duplicate ready", options: []Option{Ready(emptyLifecycle), Ready(emptyLifecycle)}},
		{name: "nil shutdown", options: []Option{Shutdown(nil)}},
		{name: "duplicate shutdown", options: []Option{Shutdown(emptyLifecycle), Shutdown(emptyLifecycle)}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Define() did not panic")
				}
			}()
			Define(testCase.options...)
		})
	}
}

func emptyRawHandler(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage("null"), nil
}

func emptyLifecycle(context.Context) error {
	return nil
}
