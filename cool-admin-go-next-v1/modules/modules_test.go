package modules

import (
	"reflect"
	"testing"
)

func TestSpecsReturnsStableSortedCopy(t *testing.T) {
	first := Specs()
	if len(first) != 4 {
		t.Fatalf("expected 4 generated modules, got %d", len(first))
	}
	keys := make([]string, 0, len(first))
	for _, spec := range first {
		keys = append(keys, spec.Key)
	}
	if !reflect.DeepEqual(keys, []string{"recycle", "base", "dict", "task"}) {
		t.Fatalf("generated module keys are not stable: %v", keys)
	}

	first[0].Key = "mutated"
	first[0].Models[0].TableName = "mutated"
	second := Specs()
	if second[0].Key != "recycle" || second[0].Models[0].TableName == "mutated" {
		t.Fatalf("Specs returned shared mutable state: %#v", second[0])
	}
}
