// Package seed 提供模块初始化种子数据的通用执行原语：记录解析、树形同步、
// 幂等插入。业务模块负责编排（哪些表、什么顺序、字段变换），本包只提供
// 与具体业务无关的通用机制——迁自 modules/base/service/initializer.go，
// 语义未变，仅去除对 Base 类型的依赖。
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// Model 是种子写入所需的最小 gdb 事务能力。
type Model interface {
	Model(...any) *gdb.Model
}

// Record 是一条尚未解码的种子记录，键为业务字段的 JSON 名。
type Record map[string]json.RawMessage

// TreeNode 是一条带层级归属的种子记录。
type TreeNode struct {
	Record    Record
	Key       string // 本节点的种子内唯一键
	ParentKey string // 父节点的种子内唯一键，根节点为空
}

// NewDO 按 Descriptor 构造插入/更新用 DO，自动补齐 createTime/updateTime。
func NewDO(descriptor coreentity.RuntimeDescriptor, values map[string]any, isInsert bool) (coreentity.DOValue, error) {
	do := descriptor.NewDO()
	now := gtime.Now()
	if isInsert {
		if err := do.SetColumn("createTime", *now); err != nil {
			return nil, exception.WrapCore(err, "设置初始化创建时间失败")
		}
	}
	if err := do.SetColumn("updateTime", *now); err != nil {
		return nil, exception.WrapCore(err, "设置初始化更新时间失败")
	}
	for field, value := range values {
		if err := do.SetColumn(field, value); err != nil {
			return nil, exception.WrapCore(err, "构造初始化数据失败")
		}
	}

	return do, nil
}

// InsertMissing 按唯一字段补齐缺失记录，已存在则跳过。
func InsertMissing(
	ctx context.Context,
	transaction Model,
	descriptor coreentity.RuntimeDescriptor,
	records []Record,
	uniqueField string,
) error {
	for _, record := range records {
		value, err := record.String(uniqueField)
		if err != nil {
			return err
		}
		existing, err := transaction.Model(descriptor.Table()).Ctx(ctx).Where(uniqueField, value).One()
		if err != nil {
			return exception.WrapCore(err, "查询初始化记录失败")
		}
		if existing != nil {
			continue
		}
		data, dataErr := record.Data(descriptor)
		if dataErr != nil {
			return dataErr
		}
		if _, err = transaction.Model(descriptor.Table()).Ctx(ctx).Data(data).Insert(); err != nil {
			return exception.WrapCore(err, "写入初始化记录失败")
		}
	}

	return nil
}

// SyncTree 按父子依赖顺序补齐或更新树形记录，返回种子键到实际 ID 的映射。
func SyncTree(
	ctx context.Context,
	transaction Model,
	descriptor coreentity.RuntimeDescriptor,
	nodes []TreeNode,
) (map[string]uint64, error) {
	ids := make(map[string]uint64, len(nodes))
	for len(ids) < len(nodes) {
		progressed := false
		for _, node := range nodes {
			if _, exists := ids[node.Key]; exists {
				continue
			}
			parentID, ready := uint64(0), node.ParentKey == ""
			if !ready {
				parentID, ready = ids[node.ParentKey]
			}
			if !ready {
				continue
			}
			values, valuesErr := node.Record.Values(descriptor)
			if valuesErr != nil {
				return nil, valuesErr
			}
			if node.ParentKey == "" {
				values["parentId"] = nil
			} else {
				values["parentId"] = parentID
			}
			model := transaction.Model(descriptor.Table()).Ctx(ctx)
			existing, err := model.Where("seedKey", node.Key).One()
			if err != nil {
				return nil, exception.WrapCore(err, "查询初始化树节点失败")
			}
			if existing == nil {
				data, dataErr := NewDO(descriptor, values, true)
				if dataErr != nil {
					return nil, dataErr
				}
				id, insertErr := model.Data(data.DBData()).InsertAndGetId()
				if insertErr != nil {
					return nil, exception.WrapCore(insertErr, "写入初始化树节点失败")
				}
				if id <= 0 {
					return nil, exception.Core("初始化树节点返回了无效 ID")
				}
				ids[node.Key] = uint64(id)
			} else {
				id := existing["id"].Uint64()
				if id == 0 {
					return nil, exception.Core("初始化树节点缺少 ID")
				}
				data, dataErr := NewDO(descriptor, values, false)
				if dataErr != nil {
					return nil, dataErr
				}
				if _, err = model.Where("id", id).Data(data.DBData()).Update(); err != nil {
					return nil, exception.WrapCore(err, "同步初始化树节点失败")
				}
				ids[node.Key] = id
			}
			progressed = true
		}
		if !progressed {
			return nil, exception.Core("初始化树节点存在无法解析的父子依赖")
		}
	}

	return ids, nil
}

// FindID 按唯一字段查找记录 ID，未命中视为错误。
func FindID(ctx context.Context, transaction Model, table, field, value string) (uint64, error) {
	if value == "" {
		return 0, exception.Core("初始化关系引用无效")
	}
	record, err := transaction.Model(table).Ctx(ctx).Where(field, value).One()
	if err != nil {
		return 0, exception.WrapCore(err, "查询初始化关系失败")
	}
	if record == nil || record["id"].Uint64() == 0 {
		return 0, exception.Core("初始化关系引用不存在")
	}

	return record["id"].Uint64(), nil
}

// Data 按 Descriptor 解码并构造插入用 DO 数据。
func (record Record) Data(descriptor coreentity.RuntimeDescriptor) (any, error) {
	values, err := record.Values(descriptor)
	if err != nil {
		return nil, err
	}
	do, err := NewDO(descriptor, values, true)
	if err != nil {
		return nil, err
	}

	return do.DBData(), nil
}

// Values 按 Descriptor 把 JSON 字段解码为可写入的列值。
func (record Record) Values(descriptor coreentity.RuntimeDescriptor) (map[string]any, error) {
	values := make(map[string]any, len(record))
	for name, raw := range record {
		field, exists := descriptor.JSON(name)
		if !exists || field.Primary() || field.SystemMaintained() {
			continue
		}
		value, err := DecodeValue(raw, field)
		if err != nil {
			return nil, err
		}
		values[field.Name()] = value
	}

	return values, nil
}

// String 读取字符串字段，缺失或为空视为错误。
func (record Record) String(name string) (string, error) {
	raw, exists := record[name]
	if !exists {
		return "", exception.Core("初始化数据缺少 " + name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", exception.Core("初始化数据 " + name + " 无效")
	}

	return value, nil
}

// Uint64 读取无符号整型字段，缺失、null 或零值返回 false。
func (record Record) Uint64(name string) (uint64, bool) {
	raw, exists := record[name]
	if !exists || string(raw) == "null" {
		return 0, false
	}
	var value uint64
	if json.Unmarshal(raw, &value) != nil || value == 0 {
		return 0, false
	}

	return value, true
}

// DecodeValue 按字段的逻辑类型和 Go 类型解码一段 JSON 原始值。
func DecodeValue(raw json.RawMessage, field coreentity.Field) (any, error) {
	target := reflect.New(field.GoType())
	if field.LogicalType() == coreentity.LogicalBool {
		if err := json.Unmarshal(raw, target.Interface()); err == nil {
			return target.Elem().Interface(), nil
		}
		var number int
		if err := json.Unmarshal(raw, &number); err == nil && (number == 0 || number == 1) {
			return number == 1, nil
		}
	}
	if field.LogicalType() == coreentity.LogicalJSON {
		var encoded string
		if json.Unmarshal(raw, &encoded) == nil && strings.EqualFold(strings.TrimSpace(encoded), "null") {
			return reflect.MakeSlice(field.GoType(), 0, 0).Interface(), nil
		}
	}
	if err := json.Unmarshal(raw, target.Interface()); err != nil {
		return nil, exception.WrapCore(err, fmt.Sprintf("解析初始化字段 %s 失败", field.Name()))
	}
	value := target.Elem()
	if field.GoType().Kind() == reflect.Pointer && !value.IsNil() {
		return value.Elem().Interface(), nil
	}

	return value.Interface(), nil
}
