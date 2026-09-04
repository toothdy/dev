package runtime

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/abi"
)

func TestRealGoPluginExportsMatchABI(t *testing.T) {
	ctx := t.Context()
	wazeroRuntime := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = wazeroRuntime.Close(context.Background()) })
	compiled, err := wazeroRuntime.CompileModule(ctx, buildEchoPlugin(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiled.Close(context.Background()) })

	if err = validateCompiled(compiled); err != nil {
		t.Fatal(err)
	}
	exports := compiled.ExportedFunctions()
	for _, signature := range abi.GuestExports() {
		if _, exists := exports[signature.Name]; !exists {
			t.Errorf("missing export %s", signature.Name)
		}
	}
	imports := compiled.ImportedFunctions()
	hostImports := make(map[string]bool)
	for _, imported := range imports {
		moduleName, name, _ := imported.Import()
		if moduleName == abi.HostModule {
			hostImports[name] = true
		}
	}
	for _, signature := range abi.HostImports() {
		if !hostImports[signature.Name] {
			t.Errorf("missing host import %s", signature.Name)
		}
	}
}
