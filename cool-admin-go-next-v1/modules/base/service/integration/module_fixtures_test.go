package base_test

import (
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func baseDefinitions() []entity.Definition {
	return moduleSpec("base").Models
}

func baseAndRecycleDefinitions() []entity.Definition {
	definitions := append([]entity.Definition{}, moduleSpec("base").Models...)
	return append(definitions, moduleSpec("recycle").Models...)
}

func applicationSpecs() []module.Spec {
	return modules.Specs()
}

func moduleSpec(key string) module.Spec {
	for _, spec := range modules.Specs() {
		if spec.Key == key {
			return spec
		}
	}
	panic("module spec not found: " + key)
}
