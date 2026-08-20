package crud

import "testing"

func TestNewQueryRequestPreservesRawInput(t *testing.T) {
	input := map[string]interface{}{
		"page":           2,
		"departmentIds":  []interface{}{1, 2},
		"isExport":       true,
		"maxExportLimit": 1000,
	}
	request := NewQueryRequest(Resource{}, QueryMetadata{}, input)
	input["page"] = 9
	if request.Raw["page"] != 2 {
		t.Fatalf("expected cloned raw input, got %#v", request.Raw)
	}
	if len(request.Raw["departmentIds"].([]interface{})) != 2 {
		t.Fatalf("expected departmentIds in raw input, got %#v", request.Raw)
	}
	if !request.IsExport || request.MaxExportLimit != 1000 {
		t.Fatalf("expected export fields, got %#v", request)
	}
}
