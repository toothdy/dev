package service

import (
	"context"
	"database/sql"
	"fmt"
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

// 字典信息服务
type DictInfoService struct {
	*service.Base
	typeModel      entity.Definition
	recycleManager recycle.DeleteManager
}

/**
 * 创建字典信息服务
 * @param db 数据库实例
 * @param dictInfoModel 字典信息模型
 * @param dictTypeModel 字典类型模型
 * @param recycleManager 回收站 Manager
 * @returns *DictInfoService
 */
func NewDictInfoService(
	db gdb.DB,
	dictInfoModel entity.Definition,
	dictTypeModel entity.Definition,
	recycleManager *recycle.Manager,
) *DictInfoService {
	return &DictInfoService{
		Base:    service.NewBase(db, dictInfoModel),
		typeModel:      dictTypeModel,
		recycleManager: recycleManager,
	}
}

/**
 * 获得所有字典类型
 * @param ctx 上下文
 * @returns 字典类型列表
 */
func (s *DictInfoService) Types(ctx context.Context) ([]map[string]interface{}, error) {
	if s == nil || s.DB == nil {
		return []map[string]interface{}{}, nil
	}
	dbModel, err := tenant.ScopedModel(ctx, s.DB, s.typeModel, "dt")
	if err != nil {
		return nil, err
	}
	return queryDictTypes(dbModel)
}

/**
 * 获得公开的平台字典类型
 * @param ctx 上下文
 * @returns 平台字典类型列表
 */
func (s *DictInfoService) GlobalTypes(ctx context.Context) ([]map[string]interface{}, error) {
	if s == nil || s.DB == nil {
		return []map[string]interface{}{}, nil
	}
	metadata, err := tenant.CompileMetadata(s.typeModel)
	if err != nil {
		return nil, err
	}
	condition, err := tenant.GlobalOnlyPredicate(metadata, "dt")
	if err != nil {
		return nil, err
	}
	dbModel := s.DB.Model(s.typeModel.TableName).Ctx(ctx).As("dt")
	if condition.SQL != "" {
		dbModel = dbModel.Where(condition.SQL, condition.Args...)
	}
	return queryDictTypes(dbModel)
}

/**
 * 查询字典类型列表
 * @param dbModel 已应用读取策略的模型
 * @returns 字典类型列表
 */
func queryDictTypes(dbModel *gdb.Model) ([]map[string]interface{}, error) {
	rows, err := dbModel.
		Fields("dt.id AS id", "dt.name AS name", "dt.`key` AS `key`", "dt.createTime AS createTime", "dt.updateTime AS updateTime", "dt.tenantId AS tenantId").
		OrderAsc("dt.id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询字典类型失败")
	}
	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Map())
	}
	return result, nil
}

/**
 * 获得字典数据
 * @param ctx 上下文
 * @param types 字典类型标识，为空时返回全部
 * @returns 字典数据映射
 */
func (s *DictInfoService) Data(ctx context.Context, types []string) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	if s == nil || s.DB == nil {
		return result, nil
	}

	typeModel, err := tenant.ScopedModel(ctx, s.DB, s.typeModel, "dt")
	if err != nil {
		return nil, err
	}
	typeModel = typeModel.Fields("dt.id AS id", "dt.`key` AS `key`")
	var typeRows gdb.Result
	if len(types) > 0 {
		typeModel = typeModel.WhereIn("dt.`key`", types)
	}
	typeRows, err = typeModel.All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询字典类型失败")
	}
	if len(typeRows) == 0 {
		return result, nil
	}

	typeIDs := make([]interface{}, 0, len(typeRows))
	for _, row := range typeRows {
		typeIDs = append(typeIDs, row["id"].Val())
	}
	infoModel, err := tenant.ScopedModel(ctx, s.DB, s.Model, "di")
	if err != nil {
		return nil, err
	}
	infoRows, err := infoModel.
		Fields("di.id AS id", "di.typeId AS typeId", "di.name AS name", "di.parentId AS parentId", "di.orderNum AS orderNum", "di.value AS value").
		WhereIn("di.typeId", typeIDs).
		OrderAsc("di.orderNum").
		OrderAsc("di.createTime").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询字典信息失败")
	}

	for _, typeRow := range typeRows {
		key := typeRow["key"].String()
		typeID := typeRow["id"].Val()
		items := make([]map[string]interface{}, 0)
		for _, infoRow := range infoRows {
			if infoRow["typeId"].Val() != typeID {
				continue
			}
			items = append(items, map[string]interface{}{
				"id":       infoRow["id"].Val(),
				"typeId":   infoRow["typeId"].Val(),
				"name":     infoRow["name"].String(),
				"parentId": infoRow["parentId"].Val(),
				"orderNum": infoRow["orderNum"].Int(),
				"value":    convertDictValue(infoRow["value"].String()),
			})
		}
		result[key] = items
	}
	return result, nil
}

/**
 * 删除字典信息并递归清理子项
 * @param ctx 上下文
 * @param request 删除请求
 * @returns 删除结果
 */
func (s *DictInfoService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
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
func (s *DictInfoService) deleteLegacy(ctx context.Context, ids []interface{}) error {
	return s.DB.Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		targetModel, err := tenant.ScopedModel(txCtx, tx, s.Model, "")
		if err != nil {
			return err
		}
		targets, err := targetModel.Fields("id").WhereIn("id", ids).OrderAsc("id").LockUpdate().All()
		if err != nil {
			return gerror.Wrap(err, "查询字典信息失败")
		}
		if len(targets) != len(ids) {
			return exception.Comm("字典信息不存在")
		}
		allIDs, err := s.collectDescendantIDs(txCtx, tx, ids)
		if err != nil {
			return err
		}
		deleteModel, err := tenant.ScopedModel(txCtx, tx, s.Model, "")
		if err != nil {
			return err
		}
		if _, err = deleteModel.WhereIn("id", allIDs).Delete(); err != nil {
			return gerror.Wrap(err, "删除字典信息失败")
		}
		return nil
	})
}

// deleteManaged 在 Manager 事务内归档并删除字典信息树。
func (s *DictInfoService) deleteManaged(ctx context.Context, scope *recycle.DeleteScope, ids []interface{}) error {
	if scope == nil || scope.TX() == nil {
		return gerror.New("字典信息删除缺少回收站事务")
	}
	targetModel, err := tenant.ScopedModel(ctx, scope.TX(), s.Model, "")
	if err != nil {
		return err
	}
	targets, err := targetModel.Fields("id").WhereIn("id", ids).OrderAsc("id").LockUpdate().All()
	if err != nil {
		return gerror.Wrap(err, "锁定字典信息失败")
	}
	if len(targets) != len(ids) {
		return exception.Comm("字典信息不存在")
	}
	descendants, descendantIDs, err := s.collectDescendantRows(ctx, scope.TX(), ids)
	if err != nil {
		return err
	}
	if scope.IsArchiving() {
		if err = s.archiveInfoDescendants(scope, targets, descendants); err != nil {
			return err
		}
	}
	allIDs := append(append([]interface{}{}, ids...), descendantIDs...)
	deleteModel, err := tenant.ScopedModel(ctx, scope.TX(), s.Model, "")
	if err != nil {
		return err
	}
	result, err := deleteModel.WhereIn("id", allIDs).Delete()
	if err != nil {
		return gerror.Wrap(err, "删除字典信息失败")
	}
	return markDictDeleted(scope, result, len(targets)+len(descendants), "字典信息")
}

/**
 * 递归收集后代字典信息ID
 * @param ctx 上下文
 * @param tx 事务
 * @param ids 起始ID列表
 * @returns 全部后代ID列表
 */
func (s *DictInfoService) collectDescendantIDs(ctx context.Context, tx gdb.TX, ids []interface{}) ([]interface{}, error) {
	collected := append([]interface{}{}, ids...)
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		seen[fmt.Sprint(id)] = struct{}{}
	}
	current := append([]interface{}{}, ids...)
	for len(current) > 0 {
		dbModel, err := tenant.ScopedModel(ctx, tx, s.Model, "")
		if err != nil {
			return nil, err
		}
		rows, err := dbModel.Fields("id").WhereIn("parentId", current).OrderAsc("id").LockUpdate().All()
		if err != nil {
			return nil, gerror.Wrap(err, "查询子字典信息失败")
		}
		next := make([]interface{}, 0, len(rows))
		for _, row := range rows {
			id := row["id"].Val()
			key := fmt.Sprint(id)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			next = append(next, id)
		}
		if len(next) == 0 {
			break
		}
		collected = append(collected, next...)
		current = next
	}
	return collected, nil
}

// collectDescendantRows 分层锁定并返回全部非根后代完整记录。
func (s *DictInfoService) collectDescendantRows(
	ctx context.Context,
	tx gdb.TX,
	ids []interface{},
) (gdb.Result, []interface{}, error) {
	rows := gdb.Result{}
	descendantIDs := make([]interface{}, 0)
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		seen[fmt.Sprint(id)] = struct{}{}
	}
	current := append([]interface{}{}, ids...)
	for len(current) > 0 {
		dbModel, err := tenant.ScopedModel(ctx, tx, s.Model, "")
		if err != nil {
			return nil, nil, err
		}
		children, err := dbModel.WhereIn("parentId", current).OrderAsc("id").LockUpdate().All()
		if err != nil {
			return nil, nil, gerror.Wrap(err, "锁定子字典信息失败")
		}
		next := make([]interface{}, 0, len(children))
		for _, row := range children {
			id := row["id"].Val()
			key := fmt.Sprint(id)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, row)
			descendantIDs = append(descendantIDs, id)
			next = append(next, id)
		}
		current = next
	}
	return rows, descendantIDs, nil
}

// archiveInfoDescendants 将后代绑定到最近的真实父项或请求根。
func (s *DictInfoService) archiveInfoDescendants(
	scope *recycle.DeleteScope,
	roots gdb.Result,
	descendants gdb.Result,
) error {
	itemKeys := make(map[int64]string, len(roots)+len(descendants))
	branchKeys := make(map[int64]string, len(roots)+len(descendants))
	for _, root := range roots {
		id, exists := dictRecordInt64(root, "id")
		if !exists || id <= 0 {
			return gerror.New("字典信息根归档身份无效")
		}
		itemKey, hasRoot := scope.RootKey(id)
		if !hasRoot {
			return gerror.Newf("字典信息 %d 缺少回收站根归档项", id)
		}
		itemKeys[id] = itemKey
		branchKeys[id] = strconv.FormatInt(id, 10)
	}
	for _, row := range descendants {
		id, hasID := dictRecordInt64(row, "id")
		parentID, hasParentID := dictRecordInt64(row, "parentId")
		if !hasID || id <= 0 || !hasParentID || parentID <= 0 {
			return gerror.New("字典信息后代归档身份无效")
		}
		parentKey, hasParent := itemKeys[parentID]
		branchKey, hasBranch := branchKeys[parentID]
		if !hasParent || !hasBranch {
			return gerror.Newf("字典信息 %d 缺少已归档父项 %d", id, parentID)
		}
		itemKey, err := scope.AddRecord(s.Model, row.Map(), recycle.ItemOptions{
			BranchKey: branchKey, ParentKey: parentKey, RestoreOrder: 10,
		})
		if err != nil {
			return err
		}
		itemKeys[id] = itemKey
		branchKeys[id] = branchKey
	}
	return nil
}

// dictRecordInt64 读取记录中的可选整数。
func dictRecordInt64(record gdb.Record, name string) (int64, bool) {
	value, exists := record[name]
	if !exists || value == nil || value.IsNil() || value.Val() == nil {
		return 0, false
	}
	return value.Int64(), true
}

// markDictDeleted 校验并累计受管物理删除行数。
func markDictDeleted(scope *recycle.DeleteScope, result sql.Result, expected int, label string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrapf(err, "读取%s删除结果失败", label)
	}
	if affected != int64(expected) {
		return gerror.Newf("%s删除数量异常: 期望 %d，实际 %d", label, expected, affected)
	}
	return scope.MarkDeleted(affected)
}

/**
 * 归一化字典删除 ID
 * @param ids 原始 ID 列表
 * @returns 去重后的 ID 列表
 */
func normalizeDictIDs(ids []interface{}) []interface{} {
	normalized := make([]interface{}, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		key := fmt.Sprint(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}

/**
 * 将字典值转换为数字或字符串
 * @param value 字典值
 * @returns 转换后的值
 */
func convertDictValue(value string) interface{} {
	if value == "" {
		return value
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}
