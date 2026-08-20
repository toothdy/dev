package crud

import (
	"encoding/json"
	"fmt"
)

// 从删除请求中提取 ID 列表
func RequestIDs(input map[string]interface{}) ([]interface{}, error) {
	value, ok := input["ids"]
	if !ok {
		return nil, fmt.Errorf("删除 ID 不能为空")
	}
	switch ids := value.(type) {
	case []interface{}:
		if len(ids) == 0 {
			return nil, fmt.Errorf("删除 ID 不能为空")
		}
		return ids, nil
	case []string:
		items := make([]interface{}, 0, len(ids))
		for _, id := range ids {
			items = append(items, id)
		}
		return items, nil
	case []int:
		items := make([]interface{}, 0, len(ids))
		for _, id := range ids {
			items = append(items, id)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("ids 必须是数组")
	}
}

// 将 Node 兼容的动态请求 map 转换为 CRUD 查询请求
func NewQueryRequest(resource Resource, metadata QueryMetadata, input map[string]interface{}) QueryRequest {
	metadata = effectiveQueryMetadata(resource, metadata)
	request := QueryRequest{
		Page:           intValue(input["page"]),
		Size:           intValue(input["size"]),
		Keyword:        stringValue(firstValue(input, "keyword", "keyWord")),
		IsExport:       boolValue(input["isExport"]),
		MaxExportLimit: intValue(input["maxExportLimit"]),
		Sort:           stringValue(input["order"]),
		Order:          stringValue(input["sort"]),
		FieldEq:        mapValue(input["fieldEq"]),
		FieldLike:      mapValue(input["fieldLike"]),
		Raw:            cloneMap(input),
	}
	for fieldName := range metadata.EqualFields {
		if value, ok := input[fieldName]; ok {
			request.FieldEq[fieldName] = value
		}
	}
	for fieldName := range metadata.LikeFields {
		if value, ok := input[fieldName]; ok {
			request.FieldLike[fieldName] = value
		}
	}
	return request
}

func boolValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true" || typed == "1"
	case json.Number:
		return typed.String() == "1"
	case float64:
		return typed == 1
	case int:
		return typed == 1
	case int64:
		return typed == 1
	default:
		return false
	}
}

func cloneMap(values map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// 保留包内测试和旧扩展的兼容入口
func requestQuery(resource Resource, metadata QueryMetadata, input map[string]interface{}) QueryRequest {
	return NewQueryRequest(resource, metadata, input)
}

func firstValue(input map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := input[key]; ok {
			return value
		}
	}
	return nil
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func intValue(value interface{}) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	case string:
		var parsed int
		_, _ = fmt.Sscan(number, &parsed)
		return parsed
	default:
		return 0
	}
}

func mapValue(value interface{}) map[string]interface{} {
	items := map[string]interface{}{}
	values, ok := value.(map[string]interface{})
	if !ok {
		return items
	}
	for key, item := range values {
		items[key] = item
	}
	return items
}
