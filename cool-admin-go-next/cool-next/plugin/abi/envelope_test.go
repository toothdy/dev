package abi

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestSuccessEnvelopeRoundTrip(t *testing.T) {
	payload, err := EncodeSuccess(json.RawMessage(`{"requestId":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"requestId":"test"}` {
		t.Fatalf("data = %s", data)
	}

	payload, err = EncodeSuccess(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err = Decode(payload)
	if err != nil || string(data) != "null" {
		t.Fatalf("nil data = %s, error = %v", data, err)
	}
}

func TestFailureEnvelopeRoundTrip(t *testing.T) {
	payload, err := EncodeFailure(ErrorInvalidInput, "参数无效")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decode(payload)
	var pluginError *PluginError
	if !errors.As(err, &pluginError) {
		t.Fatalf("Decode() error = %T %v", err, err)
	}
	if pluginError.Code != ErrorInvalidInput || pluginError.Message != "参数无效" {
		t.Fatalf("plugin error = %#v", pluginError)
	}

	cause := errors.New("cause")
	wrapped := WrapError(ErrorTrap, "插件执行失败", cause)
	if !errors.Is(wrapped, cause) {
		t.Fatal("WrapError() 未保留 cause")
	}
	if NewError(ErrorDisabled, "插件已禁用").Error() != "PLUGIN_DISABLED: 插件已禁用" {
		t.Fatal("PluginError.Error() 文本错误")
	}
}

func TestEnvelopeRejectsInvalidPayload(t *testing.T) {
	testCases := []struct {
		name    string
		payload string
	}{
		{name: "empty", payload: ""},
		{name: "missing ok", payload: `{"data":null}`},
		{name: "unknown field", payload: `{"ok":true,"data":null,"extra":1}`},
		{name: "success missing data", payload: `{"ok":true}`},
		{name: "success with error", payload: `{"ok":true,"data":null,"error":{"code":"PLUGIN_TRAP","message":"失败"}}`},
		{name: "failure missing error", payload: `{"ok":false}`},
		{name: "failure with data", payload: `{"ok":false,"data":null,"error":{"code":"PLUGIN_TRAP","message":"失败"}}`},
		{name: "failure empty code", payload: `{"ok":false,"error":{"code":"","message":"失败"}}`},
		{name: "trailing", payload: `{"ok":true,"data":null} {"ok":true,"data":null}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Decode(json.RawMessage(testCase.payload)); err == nil {
				t.Fatal("Decode() error = nil")
			}
		})
	}

	if _, err := EncodeSuccess(json.RawMessage("{")); err == nil {
		t.Fatal("EncodeSuccess() accepted invalid JSON")
	}
	if _, err := EncodeFailure("", "失败"); err == nil {
		t.Fatal("EncodeFailure() accepted empty code")
	}
	if _, err := EncodeFailure(ErrorTrap, " "); err == nil {
		t.Fatal("EncodeFailure() accepted empty message")
	}
}

func TestErrorCodesAreStable(t *testing.T) {
	want := []ErrorCode{
		"PLUGIN_NOT_FOUND",
		"PLUGIN_DISABLED",
		"PLUGIN_METHOD_NOT_FOUND",
		"PLUGIN_INVALID_INPUT",
		"PLUGIN_INVALID_OUTPUT",
		"PLUGIN_ABI_UNSUPPORTED",
		"PLUGIN_INIT_FAILED",
		"PLUGIN_TIMEOUT",
		"PLUGIN_TRAP",
		"PLUGIN_RESOURCE_EXHAUSTED",
		"PLUGIN_HOST_CALL_FAILED",
		"PLUGIN_CALL_CYCLE",
		"PLUGIN_CALL_DEPTH_EXCEEDED",
	}
	actual := []ErrorCode{
		ErrorNotFound,
		ErrorDisabled,
		ErrorMethodNotFound,
		ErrorInvalidInput,
		ErrorInvalidOutput,
		ErrorABIUnsupported,
		ErrorInitFailed,
		ErrorTimeout,
		ErrorTrap,
		ErrorResourceExhausted,
		ErrorHostCallFailed,
		ErrorCallCycle,
		ErrorCallDepthExceeded,
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("error codes = %#v, want %#v", actual, want)
	}
}
