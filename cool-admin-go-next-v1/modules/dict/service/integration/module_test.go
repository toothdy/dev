package dict_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	dictModule "github.com/toothdy/cool-admin-go-next/modules/dict"
	dictAdmin "github.com/toothdy/cool-admin-go-next/modules/dict/controller/admin"
	dictEntity "github.com/toothdy/cool-admin-go-next/modules/dict/entity"
)

func TestDictModuleDeclaration(t *testing.T) {
	declaration := dictModule.ModuleConfig()
	if declaration.Name != "字典管理" || declaration.Description != "数据字典等" || declaration.Order != 0 {
		t.Fatalf("unexpected Dict declaration: %#v", declaration)
	}
	if len(declaration.Middlewares) != 0 || len(declaration.GlobalMiddlewares) != 0 {
		t.Fatalf("Dict must not declare middleware: %#v", declaration)
	}
}

func TestDictModelsAndControllersKeepContract(t *testing.T) {
	resources := []string{
		dictEntity.DictInfo().TableName + ":" + dictEntity.DictInfo().ResourceKey(),
		dictEntity.DictType().TableName + ":" + dictEntity.DictType().ResourceKey(),
	}
	sort.Strings(resources)
	if !reflect.DeepEqual(resources, []string{"dict_info:dict.info", "dict_type:dict.type"}) {
		t.Fatalf("unexpected Dict models: %#v", resources)
	}
	definitions := []controller.Definition{
		dictAdmin.InfoController(nil, dictEntity.DictInfo()),
		dictAdmin.TypeController(nil, dictEntity.DictType()),
	}
	if definitions[0].Prefix != "/admin/dict/info" || definitions[1].Prefix != "/admin/dict/type" {
		t.Fatalf("unexpected Dict controllers: %#v", definitions)
	}
}
