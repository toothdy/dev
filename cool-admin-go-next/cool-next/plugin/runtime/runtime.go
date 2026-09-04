package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/abi"
)

// WASM 插件运行时
type Runtime struct {
	mu       sync.RWMutex
	wazero   wazero.Runtime
	host     *responseHub
	config   Config
	isClosed bool
}

// 创建 WASM 插件运行时
func New(ctx context.Context, config Config, handler HostHandler) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeConfig := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(config.MemoryLimitPages).
		WithCloseOnContextDone(true)
	wazeroRuntime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)
	runtime := &Runtime{
		wazero: wazeroRuntime,
		host:   newResponseHub(config.MaxPayloadBytes, handler),
		config: config,
	}
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, wazeroRuntime); err != nil {
		_ = wazeroRuntime.Close(ctx)
		return nil, fmt.Errorf("实例化 WASI 失败: %w", err)
	}
	if err := runtime.instantiateHost(ctx); err != nil {
		_ = wazeroRuntime.Close(ctx)
		return nil, err
	}

	return runtime, nil
}

// 编译并校验 WASM 插件
func (runtime *Runtime) Compile(ctx context.Context, wasm []byte) (*Compiled, error) {
	if len(wasm) < 8 || string(wasm[:4]) != "\x00asm" {
		return nil, abi.NewError(abi.ErrorABIUnsupported, "插件不是合法 WebAssembly 模块")
	}
	runtime.mu.RLock()
	if runtime.isClosed {
		runtime.mu.RUnlock()
		return nil, errors.New("WASM 插件运行时已关闭")
	}
	compiled, err := runtime.wazero.CompileModule(ctx, wasm)
	runtime.mu.RUnlock()
	if err != nil {
		return nil, abi.WrapError(abi.ErrorABIUnsupported, "编译 WASM 插件失败", err)
	}
	if err = validateCompiled(compiled); err != nil {
		_ = compiled.Close(ctx)
		return nil, err
	}

	return &Compiled{runtime: runtime, module: compiled}, nil
}

// 关闭运行时及其全部模块
func (runtime *Runtime) Close(ctx context.Context) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.isClosed {
		return nil
	}
	runtime.isClosed = true

	return runtime.wazero.Close(ctx)
}

func (runtime *Runtime) instantiateHost(ctx context.Context) error {
	builder := runtime.wazero.NewHostModuleBuilder(abi.HostModule)
	builder.NewFunctionBuilder().WithFunc(runtime.host.call).Export(abi.ImportCall)
	builder.NewFunctionBuilder().WithFunc(runtime.host.length).Export(abi.ImportResponseLength)
	builder.NewFunctionBuilder().WithFunc(runtime.host.read).Export(abi.ImportResponseRead)
	builder.NewFunctionBuilder().WithFunc(runtime.host.drop).Export(abi.ImportResponseDrop)
	if _, err := builder.Instantiate(ctx); err != nil {
		return fmt.Errorf("实例化插件 Host API 失败: %w", err)
	}

	return nil
}

func validateCompiled(compiled wazero.CompiledModule) error {
	if len(compiled.ImportedMemories()) > 0 {
		return abi.NewError(abi.ErrorABIUnsupported, "插件不能导入线性内存")
	}
	if _, exists := compiled.ExportedMemories()[abi.MemoryExport]; !exists {
		return abi.NewError(abi.ErrorABIUnsupported, "插件缺少 memory 导出")
	}
	hostSignatures := make(map[string]abi.FunctionSignature)
	for _, signature := range abi.HostImports() {
		hostSignatures[signature.Name] = signature
	}
	seenHostImports := make(map[string]bool, len(hostSignatures))
	for _, imported := range compiled.ImportedFunctions() {
		moduleName, name, _ := imported.Import()
		if moduleName != wasi_snapshot_preview1.ModuleName && moduleName != abi.HostModule {
			return abi.NewError(abi.ErrorABIUnsupported, fmt.Sprintf("插件包含非法导入: %s.%s", moduleName, name))
		}
		if moduleName != abi.HostModule {
			continue
		}
		signature, exists := hostSignatures[name]
		if !exists || seenHostImports[name] {
			return abi.NewError(abi.ErrorABIUnsupported, "插件 Host API 导入无效: "+name)
		}
		if !sameValueTypes(imported.ParamTypes(), signature.Parameters) || !sameValueTypes(imported.ResultTypes(), signature.Results) {
			return abi.NewError(abi.ErrorABIUnsupported, "插件 Host API 导入签名无效: "+name)
		}
		seenHostImports[name] = true
	}
	for name := range hostSignatures {
		if !seenHostImports[name] {
			return abi.NewError(abi.ErrorABIUnsupported, "插件缺少 Host API 导入: "+name)
		}
	}
	exports := compiled.ExportedFunctions()
	for _, signature := range abi.GuestExports() {
		definition, exists := exports[signature.Name]
		if !exists {
			return abi.NewError(abi.ErrorABIUnsupported, "插件缺少导出: "+signature.Name)
		}
		if !sameValueTypes(definition.ParamTypes(), signature.Parameters) || !sameValueTypes(definition.ResultTypes(), signature.Results) {
			return abi.NewError(abi.ErrorABIUnsupported, "插件导出签名无效: "+signature.Name)
		}
	}
	initialize, exists := exports[abi.InitializeExport]
	if !exists || len(initialize.ParamTypes()) != 0 || len(initialize.ResultTypes()) != 0 {
		return abi.NewError(abi.ErrorABIUnsupported, "插件缺少合法 _initialize 导出")
	}

	return nil
}

func sameValueTypes(actual []api.ValueType, want []abi.ValueType) bool {
	if len(actual) != len(want) {
		return false
	}
	for index := range actual {
		if actual[index] != byte(want[index]) {
			return false
		}
	}

	return true
}
