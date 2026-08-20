package entity

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// 校验一组 Descriptor 的内部一致性和物理名称冲突
func ValidateSet(descriptors ...Metadata) error {
	seenTables := make(map[string]Metadata, len(descriptors))
	seenIndexes := make(map[string]string)
	for _, descriptor := range descriptors {
		if isNilInterface(descriptor) {
			return exception.Core("Descriptor 不能为 nil")
		}
		if err := validateMetadata(descriptor); err != nil {
			return err
		}

		table := descriptor.Table()
		if previous, exists := seenTables[table]; exists {
			return exception.Core(fmt.Sprintf("表名 %s 冲突: %s 与 %s", table, previous.Table(), table))
		}
		seenTables[table] = descriptor

		for _, index := range descriptor.Indexes() {
			if previousTable, exists := seenIndexes[index.Name]; exists {
				return exception.Core(fmt.Sprintf("物理索引名 %s 冲突: 表 %s 与表 %s",
					index.Name,
					previousTable,
					table),
				)
			}
			seenIndexes[index.Name] = table
		}
	}

	return nil
}

// 单 Descriptor 内部一致性校验
func validateMetadata(metadata Metadata) error {
	table := metadata.Table()
	if !tableNamePattern.MatchString(table) {
		return exception.Core(fmt.Sprintf("Descriptor 表名 %q 无效", table))
	}
	if strings.TrimSpace(metadata.Description()) == "" {
		return exception.Core(fmt.Sprintf("Descriptor 表 %s 的描述不能为空", table))
	}

	fields := metadata.Fields()
	if len(fields) == 0 {
		return exception.Core(fmt.Sprintf("Descriptor 表 %s 没有字段", table))
	}
	seenNames := make(map[string]bool, len(fields))
	seenJSON := make(map[string]bool, len(fields))
	seenColumns := make(map[string]bool, len(fields))
	for _, field := range fields {
		if isNilInterface(field) {
			return exception.Core(fmt.Sprintf("Descriptor 表 %s 包含 nil 字段", table))
		}
		if !lowerCamelNamePattern.MatchString(field.Name()) || seenNames[field.Name()] {
			return exception.Core(fmt.Sprintf("Descriptor 表 %s 的逻辑字段名 %q 无效或重复", table, field.Name()))
		}
		if !lowerCamelNamePattern.MatchString(field.JSONName()) || seenJSON[field.JSONName()] {
			return exception.Core(fmt.Sprintf("Descriptor 表 %s 的 JSON 名 %q 无效或重复", table, field.JSONName()))
		}
		if !lowerCamelNamePattern.MatchString(field.Column()) || seenColumns[field.Column()] {
			return exception.Core(fmt.Sprintf("Descriptor 表 %s 的列名 %q 无效或重复", table, field.Column()))
		}
		seenNames[field.Name()] = true
		seenJSON[field.JSONName()] = true
		seenColumns[field.Column()] = true
	}
	primary := metadata.Primary()
	if isNilInterface(primary) || !primary.Primary() || !seenNames[primary.Name()] {
		return exception.Core(fmt.Sprintf("Descriptor 表 %s 的主键无效", table))
	}

	seenIndexNames := make(map[string]bool)
	for _, index := range metadata.Indexes() {
		if !indexNamePattern.MatchString(index.Name) || seenIndexNames[index.Name] {
			return exception.Core(fmt.Sprintf("Descriptor 表 %s 的索引名 %q 无效或重复", table, index.Name))
		}
		if len(index.Fields) == 0 {
			return exception.Core(fmt.Sprintf("Descriptor 表 %s 的索引 %s 没有字段", table, index.Name))
		}
		seenIndexFields := make(map[string]bool, len(index.Fields))
		for _, field := range index.Fields {
			if !seenNames[field] {
				return exception.Core(fmt.Sprintf("Descriptor 表 %s 的索引 %s 引用未知字段 %s", table, index.Name, field))
			}
			if seenIndexFields[field] {
				return exception.Core(fmt.Sprintf("Descriptor 表 %s 的索引 %s 包含重复字段 %s", table, index.Name, field))
			}
			seenIndexFields[field] = true
		}
		seenIndexNames[index.Name] = true
	}

	return nil
}

// 涵盖接口装箱的 nil
func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
