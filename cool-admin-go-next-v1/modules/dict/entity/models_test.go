package entity_test

import (
	"testing"

	coolEntity "github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/modules/dict/entity"
)

func TestDictModelFactories(t *testing.T) {
	definitions := []coolEntity.Definition{entity.DictType(), entity.DictInfo()}
	if len(definitions) != 2 {
		t.Fatalf("expected 2 dict models, got %d", len(definitions))
	}

	tables := map[string]bool{}
	for _, definition := range definitions {
		tables[definition.TableName] = true
	}
	for _, table := range []string{"dict_type", "dict_info"} {
		if !tables[table] {
			t.Fatalf("expected dict model for table %s", table)
		}
	}
	if definitions[0].ResourceKey() != "dict.type" || definitions[1].ResourceKey() != "dict.info" {
		t.Fatalf("unexpected dict resources: %s, %s", definitions[0].ResourceKey(), definitions[1].ResourceKey())
	}
}

func TestDictTypeFields(t *testing.T) {
	definition := entity.DictType()
	for _, columnName := range []string{"id", "createTime", "updateTime", "tenantId", "name", "key"} {
		if _, ok := definition.FieldByColumn(columnName); !ok {
			t.Fatalf("missing dict_type field: %s", columnName)
		}
	}
}

func TestDictInfoFields(t *testing.T) {
	definition := entity.DictInfo()
	for _, columnName := range []string{"id", "createTime", "updateTime", "tenantId", "typeId", "name", "value", "orderNum", "remark", "parentId"} {
		if _, ok := definition.FieldByColumn(columnName); !ok {
			t.Fatalf("missing dict_info field: %s", columnName)
		}
	}
}
