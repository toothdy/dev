package runtime

import (
	"context"
	"errors"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/abi"
)

// 已编译并通过 ABI 校验的插件
type Compiled struct {
	mu       sync.Mutex
	runtime  *Runtime
	module   wazero.CompiledModule
	isClosed bool
}

// 创建独立插件实例
func (compiled *Compiled) Instantiate(ctx context.Context) (*Instance, error) {
	compiled.mu.Lock()
	defer compiled.mu.Unlock()
	if compiled.isClosed || compiled.runtime == nil || compiled.module == nil {
		return nil, errors.New("已编译插件已关闭")
	}
	module, err := compiled.runtime.wazero.InstantiateModule(
		ctx,
		compiled.module,
		wazero.NewModuleConfig().WithName("").WithStartFunctions(abi.InitializeExport),
	)
	if err != nil {
		return nil, mapRuntimeError(err, abi.ErrorInitFailed, "实例化 WASM 插件失败")
	}
	bucket := compiled.runtime.host.register(module)
	instance := newInstance(compiled.runtime, module, bucket)
	version, err := instance.abiVersion(ctx)
	if err != nil {
		_ = instance.Close(ctx)
		return nil, err
	}
	if version != abi.Version {
		_ = instance.Close(ctx)
		return nil, abi.NewError(abi.ErrorABIUnsupported, "插件 ABI 版本不兼容")
	}

	return instance, nil
}

// 释放编译产物
func (compiled *Compiled) Close(ctx context.Context) error {
	compiled.mu.Lock()
	defer compiled.mu.Unlock()
	if compiled.isClosed {
		return nil
	}
	compiled.isClosed = true

	return compiled.module.Close(ctx)
}
