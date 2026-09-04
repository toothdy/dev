package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"unsafe"

	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/abi"
)

func TestGuestRuntimeLifecycleAndHostCall(t *testing.T) {
	var readyCount int
	var shutdownCount int
	runtime := newGuestRuntime(func(_ context.Context, invocationID int64, operation string, input json.RawMessage) (json.RawMessage, error) {
		if invocationID != 12 || operation != "echo.prefix" || string(input) != `{"value":"hello"}` {
			t.Fatalf("host call = %d %q %s", invocationID, operation, input)
		}
		return json.RawMessage(`{"value":"host:hello"}`), nil
	})
	definition := Define(
		Ready(func(ctx context.Context) error {
			readyCount++
			if id, exists := InvocationID(ctx); !exists || id != 10 {
				t.Fatalf("ready invocation = %d, exists = %t", id, exists)
			}
			return nil
		}),
		Method("echo", func(ctx context.Context, request echoRequest) (echoResponse, error) {
			config, err := Config[echoRequest](ctx)
			if err != nil {
				return echoResponse{}, err
			}
			return echoResponse{Value: config.Value + request.Value}, nil
		}),
		RawMethod("host", func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return HostCall(ctx, "echo.prefix", input)
		}),
		Shutdown(func(ctx context.Context) error {
			shutdownCount++
			if id, exists := InvocationID(ctx); !exists || id != 13 {
				t.Fatalf("shutdown invocation = %d, exists = %t", id, exists)
			}
			return nil
		}),
	)
	if err := runtime.register(definition); err != nil {
		t.Fatal(err)
	}
	assertResponseData(t, runtime, runtime.initialize(10, json.RawMessage(`{"value":"prefix:"}`)), "null")
	assertResponseData(t, runtime, runtime.invoke(11, "echo", json.RawMessage(`{"value":"hello"}`)), `{"value":"prefix:hello"}`)
	assertResponseData(t, runtime, runtime.invoke(12, "host", json.RawMessage(`{"value":"hello"}`)), `{"value":"host:hello"}`)
	assertResponseData(t, runtime, runtime.shutdown(13), "null")
	assertResponseData(t, runtime, runtime.shutdown(14), "null")
	if readyCount != 1 || shutdownCount != 1 {
		t.Fatalf("ready = %d, shutdown = %d", readyCount, shutdownCount)
	}
	assertResponseError(t, runtime, runtime.invoke(15, "echo", json.RawMessage(`{}`)), abi.ErrorDisabled)
}

func TestGuestRuntimeValidatesStateAndErrors(t *testing.T) {
	runtime := newGuestRuntime(nil)
	assertResponseError(t, runtime, runtime.initialize(1, json.RawMessage(`{}`)), abi.ErrorInitFailed)
	if err := runtime.register(Definition{}); err == nil {
		t.Fatal("register() accepted zero Definition")
	}
	if err := runtime.register(Define(
		RawMethod("invalid", func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage("{"), nil
		}),
		RawMethod("panic", func(context.Context, json.RawMessage) (json.RawMessage, error) {
			panic("secret")
		}),
		RawMethod("error", func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, abi.NewError(abi.ErrorInvalidInput, "业务参数无效")
		}),
	)); err != nil {
		t.Fatal(err)
	}
	assertResponseError(t, runtime, runtime.initialize(2, json.RawMessage("null")), abi.ErrorInvalidInput)
	assertResponseData(t, runtime, runtime.initialize(3, json.RawMessage(`{}`)), "null")
	assertResponseError(t, runtime, runtime.initialize(4, json.RawMessage(`{}`)), abi.ErrorInitFailed)
	assertResponseError(t, runtime, runtime.invoke(5, "missing", json.RawMessage(`{}`)), abi.ErrorMethodNotFound)
	assertResponseError(t, runtime, runtime.invoke(6, "invalid", json.RawMessage(`{}`)), abi.ErrorInvalidOutput)
	assertResponseError(t, runtime, runtime.invoke(7, "panic", json.RawMessage(`{}`)), abi.ErrorTrap)
	assertResponseError(t, runtime, runtime.invoke(8, "error", json.RawMessage(`{}`)), abi.ErrorInvalidInput)
}

func TestGuestMemoryOwnership(t *testing.T) {
	runtime := newGuestRuntime(nil)
	if err := runtime.register(Define(RawMethod("echo", emptyRawHandler))); err != nil {
		t.Fatal(err)
	}
	configPointer := writeAllocation(t, runtime, []byte(`{}`))
	handle := runtime.initializeMemory(1, configPointer, 2)
	assertResponseData(t, runtime, handle, "null")
	runtime.allocations.free(configPointer, 1)
	if _, err := runtime.allocations.read(configPointer, 2); err != nil {
		t.Fatal("wrong-size free released allocation")
	}
	runtime.allocations.free(configPointer, 2)
	if _, err := runtime.allocations.read(configPointer, 2); err == nil {
		t.Fatal("free did not release allocation")
	}

	invalidValue := byte(0)
	invalidHandle := runtime.initializeMemory(2, unsafe.Pointer(&invalidValue), 1)
	assertResponseError(t, runtime, invalidHandle, abi.ErrorInvalidInput)
}

func TestReadyPanicDoesNotInitializeRuntime(t *testing.T) {
	runtime := newGuestRuntime(nil)
	if err := runtime.register(Define(Ready(func(context.Context) error {
		panic("secret")
	}))); err != nil {
		t.Fatal(err)
	}
	assertResponseError(t, runtime, runtime.initialize(1, json.RawMessage(`{}`)), abi.ErrorInitFailed)
	assertResponseError(t, runtime, runtime.invoke(2, "missing", json.RawMessage(`{}`)), abi.ErrorDisabled)
}

func assertResponseData(t *testing.T, runtime *guestRuntime, handle int64, want string) {
	t.Helper()
	payload := readResponse(t, runtime, handle)
	data, err := abi.Decode(payload)
	if err != nil {
		t.Fatalf("Decode() error = %v, payload = %s", err, payload)
	}
	if string(data) != want {
		t.Fatalf("response data = %s, want %s", data, want)
	}
}

func assertResponseError(t *testing.T, runtime *guestRuntime, handle int64, code abi.ErrorCode) {
	t.Helper()
	payload := readResponse(t, runtime, handle)
	_, err := abi.Decode(payload)
	assertPluginError(t, err, code)
}

func assertPluginError(t *testing.T, err error, code abi.ErrorCode) {
	t.Helper()
	var pluginError *abi.PluginError
	if !errors.As(err, &pluginError) || pluginError.Code != code {
		t.Fatalf("error = %T %v, want code %s", err, err, code)
	}
}

func readResponse(t *testing.T, runtime *guestRuntime, handle int64) json.RawMessage {
	t.Helper()
	size := runtime.responses.length(handle)
	if size < 0 {
		t.Fatalf("response handle %d not found", handle)
	}
	pointer := runtime.responses.pointer(handle)
	var result []byte
	if size > 0 {
		result = append([]byte(nil), unsafe.Slice((*byte)(pointer), int(size))...)
	}
	runtime.responses.drop(handle)
	if runtime.responses.length(handle) != -1 {
		t.Fatalf("response handle %d not dropped", handle)
	}

	return result
}

func writeAllocation(t *testing.T, runtime *guestRuntime, value []byte) unsafe.Pointer {
	t.Helper()
	pointer := runtime.allocations.allocate(int32(len(value)))
	copy(unsafe.Slice((*byte)(pointer), len(value)), value)

	return pointer
}
