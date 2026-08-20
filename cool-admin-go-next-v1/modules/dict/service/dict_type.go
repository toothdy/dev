package service

import (
	"context"
	"strconv"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

// 字典类型服务
type DictTypeService struct {
	*service.Base
	infoModel      entity.Definition
	recycleManager recycle.DeleteManager
}

/**
 * 创建字典类型服务
 * @param db 数据库实例
 * @param dictTypeModel 字典类型模型
 * @param dictInfoModel 字典信息模型
 * @param recycleManager 回收站 Manager
 * @returns *DictTypeService
 */
func NewDictTypeService(
	db gdb.DB,
	dictTypeModel entity.Definition,
	dictInfoModel entity.Definition,
	recycleManager *recycle.Manager,
) *DictTypeService {
	return &DictTypeService{
		Base:    service.NewBase(db, dictTypeModel),
		infoModel:      dictInfoModel,
		recycleManager: recycleManager,
	}
}

/**
 * 删除字典类型并级联删除字典信息
 * @param ctx 上下文
 * @param request 删除请求
 * @returns 删除结果
 */
func (s *DictTypeService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	ids := normalizeDictIDs(request.IDs)
	if len(ids) == 0 {
		return map[string]interface{}{}, nil
	}
	var err error
	if s.recycleManager == nil {
		err = s.deleteLegacy(ctx, ids)
	} else {
		err = s.recycleManager.RunDelete(ctx, recycle.DeleteRequest{
			Resource: s.Model.ResourceKey(), Entity: s.Model.Name, Model: s.Model, IDs: ids, Params: request,
		}, func(txCtx context.Context, scope *recycle.DeleteScope) error {
			return s.deleteManaged(txCtx, scope, ids)
		})
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{}, nil
}

// deleteLegacy 保留未注入回收站 Manager 时的原删除行为。
func (s *DictTypeService) deleteLegacy(ctx context.Context, ids []interface{}) error {
	return s.DB.Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		typeModel, err := tenant.ScopedModel(txCtx, tx, s.Model, "")
		if err != nil {
			return err
		}
		types, err := typeModel.Fields("id").WhereIn("id", ids).OrderAsc("id").LockUpdate().All()
		if err != nil {
			return gerror.Wrap(err, "查询字典类型失败")
		}
		if len(types) != len(ids) {
			return exception.Comm("字典类型不存在")
		}
		infoModel, err := tenant.ScopedModel(txCtx, tx, s.infoModel, "")
		if err != nil {
			return err
		}
		if _, err = infoModel.WhereIn("typeId", ids).Delete(); err != nil {
			return gerror.Wrap(err, "删除字典信息失败")
		}
		typeModel, err = tenant.ScopedModel(txCtx, tx, s.Model, "")
		if err != nil {
			return err
		}
		if _, err = typeModel.WhereIn("id", ids).Delete(); err != nil {
			return gerror.Wrap(err, "删除字典类型失败")
		}
		return nil
	})
}

// deleteManaged 在 Manager 事务内归档并删除字典类型聚合。
func (s *DictTypeService) deleteManaged(ctx context.Context, scope *recycle.DeleteScope, ids []interface{}) error {
	if scope == nil || scope.TX() == nil {
		return gerror.New("字典类型删除缺少回收站事务")
	}
	typeLockModel, err := tenant.ScopedModel(ctx, scope.TX(), s.Model, "")
	if err != nil {
		return err
	}
	types, err := typeLockModel.Fields("id").WhereIn("id", ids).OrderAsc("id").LockUpdate().All()
	if err != nil {
		return gerror.Wrap(err, "锁定字典类型失败")
	}
	if len(types) != len(ids) {
		return exception.Comm("字典类型不存在")
	}
	infoLockModel, err := tenant.ScopedModel(ctx, scope.TX(), s.infoModel, "")
	if err != nil {
		return err
	}
	infos, err := infoLockModel.WhereIn("typeId", ids).OrderAsc("typeId").OrderAsc("id").LockUpdate().All()
	if err != nil {
		return gerror.Wrap(err, "锁定字典信息失败")
	}
	if scope.IsArchiving() {
		if err = s.archiveTypeInfos(scope, infos); err != nil {
			return err
		}
	}
	infoDeleteModel, err := tenant.ScopedModel(ctx, scope.TX(), s.infoModel, "")
	if err != nil {
		return err
	}
	infoResult, err := infoDeleteModel.WhereIn("typeId", ids).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除字典信息失败")
	}
	if err = markDictDeleted(scope, infoResult, len(infos), "字典信息"); err != nil {
		return err
	}
	typeDeleteModel, err := tenant.ScopedModel(ctx, scope.TX(), s.Model, "")
	if err != nil {
		return err
	}
	typeResult, err := typeDeleteModel.WhereIn("id", ids).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除字典类型失败")
	}
	return markDictDeleted(scope, typeResult, len(types), "字典类型")
}

type dictTypeInfoNode struct {
	record   gdb.Record
	id       int64
	typeID   int64
	parentID int64
}

// archiveTypeInfos 按真实父链归档各类型的字典信息。
func (s *DictTypeService) archiveTypeInfos(scope *recycle.DeleteScope, rows gdb.Result) error {
	nodes, err := orderDictTypeInfoNodes(rows)
	if err != nil {
		return err
	}
	itemKeys := make(map[int64]string, len(nodes))
	for _, node := range nodes {
		parentKey, exists := scope.RootKey(node.typeID)
		if !exists {
			return gerror.Newf("字典类型 %d 缺少回收站根归档项", node.typeID)
		}
		if node.parentID > 0 {
			if parent := itemKeys[node.parentID]; parent != "" {
				parentKey = parent
			}
		}
		itemKey, addErr := scope.AddRecord(s.infoModel, node.record.Map(), recycle.ItemOptions{
			BranchKey: strconv.FormatInt(node.typeID, 10), ParentKey: parentKey, RestoreOrder: 10,
		})
		if addErr != nil {
			return addErr
		}
		itemKeys[node.id] = itemKey
	}
	return nil
}

// orderDictTypeInfoNodes 按父先子后校验并排序字典信息。
func orderDictTypeInfoNodes(rows gdb.Result) ([]*dictTypeInfoNode, error) {
	nodes := make(map[int64]*dictTypeInfoNode, len(rows))
	orderedIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		id, hasID := dictRecordInt64(row, "id")
		typeID, hasTypeID := dictRecordInt64(row, "typeId")
		if !hasID || id <= 0 || !hasTypeID || typeID <= 0 {
			return nil, gerror.New("字典信息归档身份无效")
		}
		parentID, _ := dictRecordInt64(row, "parentId")
		nodes[id] = &dictTypeInfoNode{record: row, id: id, typeID: typeID, parentID: parentID}
		orderedIDs = append(orderedIDs, id)
	}
	state := make(map[int64]uint8, len(nodes))
	ordered := make([]*dictTypeInfoNode, 0, len(nodes))
	var visit func(int64) error
	visit = func(id int64) error {
		node := nodes[id]
		if node == nil {
			return gerror.Newf("字典信息归档节点不存在: %d", id)
		}
		if state[id] == 1 {
			return gerror.New("字典信息父级关系不能成环")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		if parent := nodes[node.parentID]; parent != nil {
			if parent.typeID != node.typeID {
				return gerror.Newf("字典信息 %d 的父级不属于同一类型", node.id)
			}
			if err := visit(parent.id); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, node)
		return nil
	}
	for _, id := range orderedIDs {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
