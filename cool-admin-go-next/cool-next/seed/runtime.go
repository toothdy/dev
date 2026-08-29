package seed

import (
	"context"
	"encoding/json"
	"sort"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/db"
)

// 模块的嵌入种子与可写入实体
type Definition struct {
	Data        Data
	Descriptors []coreentity.RuntimeDescriptor
	Key         string
}

// 模块种子定义
func NewDefinition(key string, data Data, descriptors ...coreentity.RuntimeDescriptor) Definition {
	return Definition{Key: key, Data: data, Descriptors: append([]coreentity.RuntimeDescriptor(nil), descriptors...)}
}

// 框架启动阶段统一导入模块种子
type Runtime struct {
	definitions []Definition
	runtime     *db.Runtime
	store       *Store
}

// 框架种子运行时
func NewRuntime(runtime *db.Runtime, definitions ...Definition) (*Runtime, error) {
	store, err := NewStore(runtime)
	if err != nil {
		return nil, err
	}
	result := &Runtime{runtime: runtime, store: store, definitions: append([]Definition(nil), definitions...)}
	for _, definition := range result.definitions {
		if definition.Key == "" {
			return nil, exception.Core("模块种子定义缺少模块键")
		}
		for _, descriptor := range definition.Descriptors {
			if descriptor == nil || descriptor.Table() == "" {
				return nil, exception.Core("模块种子定义包含无效 Descriptor")
			}
		}
	}
	return result, nil
}

// 以模块和文件类型为幂等边界导入全部嵌入种子
func (runtime *Runtime) OnInit(ctx context.Context) error {
	if runtime == nil || runtime.runtime == nil || runtime.store == nil {
		return exception.Core("框架种子运行时未初始化")
	}
	if err := runtime.store.Prepare(ctx); err != nil {
		return err
	}
	for _, definition := range runtime.definitions {
		legacy, err := runtime.store.Has(ctx, definition.Key)
		if err != nil {
			return err
		}
		if legacy {
			continue
		}
		if data := definition.Data.DB(); len(data) > 0 {
			if err := runtime.importDB(ctx, definition, data); err != nil {
				return err
			}
		}
		if data := definition.Data.Menu(); len(data) > 0 {
			if err := runtime.importMenu(ctx, definition, data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (runtime *Runtime) importDB(ctx context.Context, definition Definition, data []byte) error {
	var groups map[string][]Record
	if err := json.Unmarshal(data, &groups); err != nil {
		return exception.WrapCore(err, "解析数据库种子失败")
	}
	descriptors := definitionDescriptors(definition)
	tables := make([]string, 0, len(groups))
	for table := range groups {
		if descriptors[table] == nil {
			return exception.Core("数据库种子引用了未声明表: " + table)
		}
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return runtime.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		return runtime.store.Guard(txCtx, definition.Key+":db", func(guardCtx context.Context) error {
			transaction, exists, err := runtime.runtime.Current(guardCtx)
			if err != nil || !exists || transaction == nil {
				return exception.WrapCore(err, "获取数据库种子事务失败")
			}
			for _, table := range tables {
				for _, record := range groups[table] {
					values, dataErr := record.SeedData(descriptors[table])
					if dataErr != nil {
						return dataErr
					}
					if _, insertErr := transaction.Model(table).Ctx(guardCtx).Data(values).Insert(); insertErr != nil {
						return exception.WrapCore(insertErr, "写入数据库种子失败: "+table)
					}
				}
				// SeedData 保留种子里的显式主键，PostgreSQL 的序列不会因此推进，
				// 必须显式同步，否则后续自动分配的主键会从 1 开始撞上种子行。
				if err = runtime.syncPrimarySequence(guardCtx, descriptors[table]); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (runtime *Runtime) importMenu(ctx context.Context, definition Definition, data []byte) error {
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return exception.WrapCore(err, "解析菜单种子失败")
	}
	descriptor := definitionDescriptors(definition)["base_sys_menu"]
	if descriptor == nil {
		return exception.Core("菜单种子需要 base_sys_menu Descriptor")
	}
	nodes, err := parseMenuNodes(records, "")
	if err != nil {
		return err
	}
	return runtime.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		return runtime.store.Guard(txCtx, definition.Key+":menu", func(guardCtx context.Context) error {
			transaction, exists, currentErr := runtime.runtime.Current(guardCtx)
			if currentErr != nil || !exists || transaction == nil {
				return exception.WrapCore(currentErr, "获取菜单种子事务失败")
			}
			_, syncErr := SyncTree(guardCtx, transaction, descriptor, nodes)
			return syncErr
		})
	})
}

// 种子写入显式主键后同步自增序列，非自增主键直接跳过。
func (runtime *Runtime) syncPrimarySequence(ctx context.Context, descriptor coreentity.RuntimeDescriptor) error {
	primary := descriptor.Primary()
	if primary == nil || !primary.AutoIncrement() {
		return nil
	}

	return runtime.runtime.SyncSequence(ctx, descriptor.Table(), primary.Column())
}

func definitionDescriptors(definition Definition) map[string]coreentity.RuntimeDescriptor {
	result := make(map[string]coreentity.RuntimeDescriptor, len(definition.Descriptors))
	for _, descriptor := range definition.Descriptors {
		result[descriptor.Table()] = descriptor
	}
	return result
}

func parseMenuNodes(records []Record, parentKey string) ([]TreeNode, error) {
	result := make([]TreeNode, 0, len(records))
	for _, record := range records {
		key, err := record.String("seedKey")
		if err != nil {
			return nil, exception.WrapCore(err, "菜单种子缺少 seedKey")
		}
		result = append(result, TreeNode{Record: record, Key: key, ParentKey: parentKey})
		raw, exists := record["childMenus"]
		if !exists || string(raw) == "null" {
			continue
		}
		var children []Record
		if err = json.Unmarshal(raw, &children); err != nil {
			return nil, exception.WrapCore(err, "解析菜单子节点失败")
		}
		nested, nestedErr := parseMenuNodes(children, key)
		if nestedErr != nil {
			return nil, nestedErr
		}
		result = append(result, nested...)
	}
	return result, nil
}
