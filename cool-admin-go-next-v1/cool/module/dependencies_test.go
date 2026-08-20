package module_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestControllerProviderReturnsControllersLazily(t *testing.T) {
	calls := 0
	provider := module.ControllerProvider(func() []controller.Definition {
		calls++
		return []controller.Definition{{Name: "base.open"}}
	})

	if calls != 0 {
		t.Fatal("controller provider should not execute eagerly")
	}
	definitions := provider.Controllers()
	if calls != 1 || len(definitions) != 1 || definitions[0].Name != "base.open" {
		t.Fatalf("unexpected controller provider result: calls=%d definitions=%#v", calls, definitions)
	}
}

func TestUploadDirectoryPreservesConfiguredPath(t *testing.T) {
	directory := module.UploadDirectory("resource/public/uploads")
	if directory.String() != "resource/public/uploads" {
		t.Fatalf("unexpected upload directory: %q", directory.String())
	}
}
