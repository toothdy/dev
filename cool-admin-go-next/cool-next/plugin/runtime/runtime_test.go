package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/abi"
)

var echoBuild struct {
	sync.Once
	wasm []byte
	err  error
}

func TestRuntimeInvokesRealGoPlugin(t *testing.T) {
	wasm := buildEchoPlugin(t)
	var hostCalls int
	host := func(_ context.Context, invocationID int64, operation string, input json.RawMessage) (json.RawMessage, error) {
		hostCalls++
		if invocationID != 3 || operation != "echo.prefix" || string(input) != `{"value":"call"}` {
			t.Fatalf("host call = %d %q %s", invocationID, operation, input)
		}

		return json.RawMessage(`{"value":"host:call"}`), nil
	}
	runtime, err := New(t.Context(), DefaultConfig(), host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	compiled, err := runtime.Compile(t.Context(), wasm)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiled.Close(context.Background()) })

	instance, err := compiled.Instantiate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close(context.Background()) })
	if err = instance.Initialize(t.Context(), 1, json.RawMessage(`{"prefix":"plugin:"}`)); err != nil {
		t.Fatal(err)
	}
	result, err := instance.Invoke(t.Context(), 2, "echo", json.RawMessage(`{"value":"call"}`))
	if err != nil || string(result) != `{"value":"plugin:call"}` {
		t.Fatalf("echo result = %s, error = %v", result, err)
	}
	result, err = instance.Invoke(t.Context(), 3, "host", json.RawMessage(`{"value":"call"}`))
	if err != nil || string(result) != `{"value":"host:call"}` {
		t.Fatalf("host result = %s, error = %v", result, err)
	}
	if hostCalls != 1 {
		t.Fatalf("host calls = %d", hostCalls)
	}
	if _, err = instance.Invoke(t.Context(), 4, "missing", json.RawMessage(`{}`)); pluginErrorCode(err) != abi.ErrorMethodNotFound {
		t.Fatalf("missing method error = %v", err)
	}
	if _, err = instance.Invoke(t.Context(), 5, "panic", json.RawMessage(`{}`)); pluginErrorCode(err) != abi.ErrorTrap {
		t.Fatalf("panic error = %v", err)
	}
	if err = instance.Shutdown(t.Context(), 6); err != nil {
		t.Fatal(err)
	}
}

func TestCompiledCreatesIsolatedInstances(t *testing.T) {
	runtime, err := New(t.Context(), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	compiled, err := runtime.Compile(t.Context(), buildEchoPlugin(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiled.Close(context.Background()) })

	first, err := compiled.Instantiate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := compiled.Instantiate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = first.Close(context.Background())
		_ = second.Close(context.Background())
	})
	if err = first.Initialize(t.Context(), 1, json.RawMessage(`{"prefix":"first:"}`)); err != nil {
		t.Fatal(err)
	}
	if err = second.Initialize(t.Context(), 2, json.RawMessage(`{"prefix":"second:"}`)); err != nil {
		t.Fatal(err)
	}
	firstResult, firstErr := first.Invoke(t.Context(), 3, "echo", json.RawMessage(`{"value":"value"}`))
	secondResult, secondErr := second.Invoke(t.Context(), 4, "echo", json.RawMessage(`{"value":"value"}`))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("invoke errors = %v, %v", firstErr, secondErr)
	}
	if string(firstResult) != `{"value":"first:value"}` || string(secondResult) != `{"value":"second:value"}` {
		t.Fatalf("results = %s, %s", firstResult, secondResult)
	}
}

func TestRuntimeStopsInfiniteGuestOnDeadline(t *testing.T) {
	runtime, err := New(t.Context(), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	compiled, err := runtime.Compile(t.Context(), buildEchoPlugin(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiled.Close(context.Background()) })
	instance, err := compiled.Instantiate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close(context.Background()) })
	if err = instance.Initialize(t.Context(), 1, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = instance.Invoke(ctx, 2, "loop", json.RawMessage(`{}`))
	if pluginErrorCode(err) != abi.ErrorTimeout {
		t.Fatalf("loop error = %T %v", err, err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("deadline termination took %s", time.Since(started))
	}
	if _, err = instance.Invoke(t.Context(), 3, "echo", json.RawMessage(`{}`)); err == nil {
		t.Fatal("timed out instance remained callable")
	}
}

func TestRuntimeValidatesInputAndABI(t *testing.T) {
	configCases := []Config{
		{},
		{MemoryLimitPages: 65537, MaxPayloadBytes: 1},
		{MemoryLimitPages: 1, MaxPayloadBytes: wasmPageBytes + 1},
	}
	for _, config := range configCases {
		if _, err := New(t.Context(), config, nil); err == nil {
			t.Fatalf("New(%#v) error = nil", config)
		}
	}

	runtime, err := New(t.Context(), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	if _, err = runtime.Compile(t.Context(), []byte("not wasm")); pluginErrorCode(err) != abi.ErrorABIUnsupported {
		t.Fatalf("invalid wasm error = %v", err)
	}
	wasm := buildEchoPlugin(t)
	invalidExport := bytes.Replace(wasm, []byte(abi.ExportABIVersion), []byte("xool_abi_version"), 1)
	if bytes.Equal(invalidExport, wasm) {
		t.Fatal("ABI export marker not found")
	}
	if _, err = runtime.Compile(t.Context(), invalidExport); pluginErrorCode(err) != abi.ErrorABIUnsupported {
		t.Fatalf("missing export error = %v", err)
	}

	smallConfig := DefaultConfig()
	smallConfig.MaxPayloadBytes = 64
	smallRuntime, err := New(t.Context(), smallConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = smallRuntime.Close(context.Background()) })
	smallCompiled, err := smallRuntime.Compile(t.Context(), wasm)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = smallCompiled.Close(context.Background()) })
	instance, err := smallCompiled.Instantiate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close(context.Background()) })
	if err = instance.Initialize(t.Context(), 1, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{"value":"payload-too-large-payload-too-large-payload-too-large-payload-too-large"}`)
	if _, err = instance.Invoke(t.Context(), 2, "echo", input); pluginErrorCode(err) != abi.ErrorResourceExhausted {
		t.Fatalf("large payload error = %v", err)
	}
}

func TestRuntimeCloseIsIdempotent(t *testing.T) {
	runtime, err := New(t.Context(), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Compile(t.Context(), buildEchoPlugin(t)); err == nil {
		t.Fatal("closed runtime compiled module")
	}
}

func buildEchoPlugin(t *testing.T) []byte {
	t.Helper()
	echoBuild.Do(func() {
		root, err := filepath.Abs(filepath.Join("..", "..", ".."))
		if err != nil {
			echoBuild.err = err
			return
		}
		directory, err := os.MkdirTemp("", "cool-plugin-echo-")
		if err != nil {
			echoBuild.err = err
			return
		}
		defer os.RemoveAll(directory)
		output := filepath.Join(directory, "echo.wasm")
		command := exec.CommandContext(context.Background(), "go", "build", "-buildmode=c-shared", "-trimpath", "-o", output, "./cool-next/plugin/testdata/echo")
		command.Dir = root
		command.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
		combined, err := command.CombinedOutput()
		if err != nil {
			echoBuild.err = errors.New("build echo plugin: " + err.Error() + "\n" + string(combined))
			return
		}
		echoBuild.wasm, echoBuild.err = os.ReadFile(output)
	})
	if echoBuild.err != nil {
		t.Fatal(echoBuild.err)
	}

	return bytes.Clone(echoBuild.wasm)
}

func pluginErrorCode(err error) abi.ErrorCode {
	var pluginError *abi.PluginError
	if errors.As(err, &pluginError) {
		return pluginError.Code
	}

	return ""
}

func TestHostErrorsAreRedacted(t *testing.T) {
	host := func(context.Context, int64, string, json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("password=secret")
	}
	runtime, err := New(t.Context(), DefaultConfig(), host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	compiled, err := runtime.Compile(t.Context(), buildEchoPlugin(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiled.Close(context.Background()) })
	instance, err := compiled.Instantiate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close(context.Background()) })
	if err = instance.Initialize(t.Context(), 1, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	_, err = instance.Invoke(t.Context(), 2, "host", json.RawMessage(`{}`))
	if pluginErrorCode(err) != abi.ErrorHostCallFailed || strings.Contains(err.Error(), "secret") {
		t.Fatalf("host error = %v", err)
	}
}
