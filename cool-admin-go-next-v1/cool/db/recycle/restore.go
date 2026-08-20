package recycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

type restoreConflictError struct {
	err error
}

func (e *restoreConflictError) Error() string {
	if e == nil || e.err == nil {
		return "数据恢复冲突"
	}
	return e.err.Error()
}

func (e *restoreConflictError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

/**
 * 按批次恢复归档数据
 * @param ctx 上下文
 * @param id 回收批次 ID
 * @returns error
 */
func (m *Manager) Restore(ctx context.Context, id int64) error {
	return m.RestoreMany(ctx, []int64{id})
}

/**
 * 按批次列表恢复归档数据
 * @param ctx 上下文
 * @param ids 回收批次 ID
 * @returns error
 */
func (m *Manager) RestoreMany(ctx context.Context, ids []int64) error {
	if m == nil || m.db == nil || m.store == nil || m.catalog == nil {
		return gerror.New("回收站恢复协调器不可用")
	}
	normalizedIDs, err := normalizeRestoreIDs(ids)
	if err != nil {
		return err
	}
	tenantID, err := restoreTenantID(ctx)
	if err != nil {
		return err
	}
	for _, id := range normalizedIDs {
		restoredResources := map[string]struct{}{}
		err = m.db.Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
			archive, lockErr := m.store.LockArchive(txCtx, tx, id, tenantID)
			if lockErr != nil {
				return gerror.Wrapf(lockErr, "锁定回收批次 %d 失败", id)
			}
			if archive == nil {
				return nil
			}
			if validateErr := validateArchiveScope(archive, id, tenantID); validateErr != nil {
				return validateErr
			}
			ordered, orderErr := orderRestoreItems(archive.Items)
			if orderErr != nil {
				return orderErr
			}
			statusByID := make(map[int64]ItemStatus, len(ordered))
			for _, item := range ordered {
				statusByID[item.ID] = item.Status
			}
			for _, item := range ordered {
				if item.Status == ItemStatusRestored {
					continue
				}
				if item.ParentItemID != nil && statusByID[*item.ParentItemID] != ItemStatusRestored {
					item.Status = ItemStatusConflict
					item.Error = "父级数据未恢复"
					statusByID[item.ID] = item.Status
					continue
				}
				point := fmt.Sprintf("recycle_item_%d", item.ID)
				if saveErr := tx.SavePoint(point); saveErr != nil {
					return gerror.Wrapf(saveErr, "创建回收站恢复保存点 %d 失败", item.ID)
				}
				insertErr := m.restoreItem(txCtx, tx, archive, item)
				if insertErr != nil {
					if rollbackErr := tx.RollbackTo(point); rollbackErr != nil {
						return gerror.Wrapf(rollbackErr, "回滚回收站恢复保存点 %d 失败", item.ID)
					}
					if !isRestoreConflict(insertErr) {
						return insertErr
					}
					item.Status = ItemStatusConflict
					item.Error = insertErr.Error()
					statusByID[item.ID] = item.Status
					g.Log().Warningf(
						txCtx, "回收站批次 %d 资源 %s 归档项 %d 恢复冲突: %v",
						archive.ID, item.Resource, item.ID, insertErr,
					)
					continue
				}
				item.Status = ItemStatusRestored
				item.Error = ""
				item.RestoredInRun = true
				statusByID[item.ID] = item.Status
				restoredResources[item.Resource] = struct{}{}
			}
			remaining := 0
			for _, item := range archive.Items {
				if item.Status != ItemStatusRestored {
					remaining++
				}
			}
			archive.RemainingCount = remaining
			if remaining == 0 {
				return m.store.DeleteArchive(txCtx, tx, archive.ID, tenantID)
			}
			archive.RestoreStatus = RestoreStatusPartial
			return m.store.SaveRestoreState(txCtx, tx, archive)
		})
		if err != nil {
			return err
		}
		m.runRestoreHooks(ctx, restoredResources)
	}
	return nil
}

func (m *Manager) restoreItem(ctx context.Context, tx gdb.TX, archive *Archive, item *ArchiveItem) error {
	metadata, ok := m.catalog.Model(item.Resource)
	if !ok {
		return gerror.Newf("回收站归档项模型未注册: %s", item.Resource)
	}
	if metadata.Definition.TableName != item.TableName {
		return gerror.Newf("回收站归档项表名与模型不一致: %s", item.Resource)
	}
	if item.RecycleID != archive.ID {
		return gerror.Newf("回收站归档项批次不一致: %d", item.ID)
	}
	if !sameOptionalInt64(item.TenantID, archive.TenantID) {
		return gerror.Newf("回收站归档项租户不一致: %d", item.ID)
	}
	if err := validateItemIdentity(metadata, item); err != nil {
		return err
	}
	if err := validateSnapshotTenant(metadata, archive, item); err != nil {
		return err
	}
	query, args, err := buildRestoreInsert(metadata, item.Data)
	if err != nil {
		return err
	}
	if _, err = tx.Ctx(ctx).Exec(query, args...); err != nil {
		if isMySQLRestoreConflict(err) {
			return &restoreConflictError{err: gerror.Wrapf(err, "恢复回收站归档项 %d 冲突", item.ID)}
		}
		return gerror.Wrapf(err, "恢复回收站归档项 %d 失败", item.ID)
	}
	return nil
}

func validateSnapshotTenant(metadata ModelMetadata, archive *Archive, item *ArchiveItem) error {
	if !metadata.Tenant.IsAware() {
		return nil
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(item.Data, &values); err != nil {
		return gerror.Wrapf(err, "解析回收站归档项 %d 租户快照失败", item.ID)
	}
	raw, ok := values[metadata.Tenant.JSONField()]
	if !ok {
		return gerror.Newf("回收站归档项租户快照缺失: %d", item.ID)
	}
	expected := json.RawMessage("null")
	if archive.TenantID != nil {
		encoded, err := json.Marshal(*archive.TenantID)
		if err != nil {
			return gerror.Wrapf(err, "编码回收站归档项 %d 租户失败", item.ID)
		}
		expected = encoded
	}
	if !sameJSONValue(raw, expected) {
		return gerror.Newf("回收站归档项租户快照不一致: %d", item.ID)
	}
	return nil
}

func validateItemIdentity(metadata ModelMetadata, item *ArchiveItem) error {
	if len(item.Identity.Fields) == 0 {
		return gerror.Newf("回收站归档项数据身份为空: %d", item.ID)
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(item.Data, &values); err != nil {
		return gerror.Wrapf(err, "解析回收站归档项 %d 快照失败", item.ID)
	}
	seen := map[string]struct{}{}
	for _, identityField := range item.Identity.Fields {
		field, ok := metadata.FieldsByJSON[identityField.JSONName]
		if !ok || field.ColumnName != identityField.ColumnName {
			return gerror.Newf("回收站归档项数据身份字段无效: %d", item.ID)
		}
		if _, exists := seen[field.JSONName]; exists {
			return gerror.Newf("回收站归档项数据身份字段重复: %d", item.ID)
		}
		seen[field.JSONName] = struct{}{}
		dataValue, exists := values[field.JSONName]
		if !exists || !sameJSONValue(dataValue, identityField.Value) {
			return gerror.Newf("回收站归档项数据身份与快照不一致: %d", item.ID)
		}
	}
	if len(metadata.IdentityFields) > 0 {
		if len(item.Identity.Fields) != len(metadata.IdentityFields) {
			return gerror.Newf("回收站归档项数据身份与模型不一致: %d", item.ID)
		}
		for index, field := range metadata.IdentityFields {
			identityField := item.Identity.Fields[index]
			if identityField.JSONName != field.JSONName || identityField.ColumnName != field.ColumnName {
				return gerror.Newf("回收站归档项数据身份顺序与模型不一致: %d", item.ID)
			}
		}
	}
	return nil
}

func sameJSONValue(left json.RawMessage, right json.RawMessage) bool {
	var (
		leftValue  interface{}
		rightValue interface{}
	)
	leftDecoder := json.NewDecoder(bytes.NewReader(left))
	leftDecoder.UseNumber()
	rightDecoder := json.NewDecoder(bytes.NewReader(right))
	rightDecoder.UseNumber()
	if leftDecoder.Decode(&leftValue) != nil || rightDecoder.Decode(&rightValue) != nil {
		return false
	}
	leftJSON, leftErr := json.Marshal(leftValue)
	rightJSON, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func buildRestoreInsert(metadata ModelMetadata, data json.RawMessage) (string, []interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	values := map[string]json.RawMessage{}
	if err := decoder.Decode(&values); err != nil {
		return "", nil, gerror.Wrap(err, "解析回收站数据快照失败")
	}
	if len(values) == 0 {
		return "", nil, &restoreConflictError{err: gerror.New("回收站数据快照为空")}
	}
	for name := range values {
		if _, ok := metadata.FieldsByJSON[name]; !ok {
			return "", nil, &restoreConflictError{err: gerror.Newf("回收站数据快照字段已移除: %s", name)}
		}
	}
	columns := make([]string, 0, len(metadata.Definition.FieldsValue))
	placeholders := make([]string, 0, len(metadata.Definition.FieldsValue))
	args := make([]interface{}, 0, len(metadata.Definition.FieldsValue))
	for _, field := range metadata.Definition.FieldsValue {
		raw, ok := values[field.JSONName]
		if !ok {
			if field.IsNullable || field.HasDefault || field.IsAutoIncrement {
				continue
			}
			return "", nil, &restoreConflictError{err: gerror.Newf("回收站数据快照缺少必填字段: %s", field.JSONName)}
		}
		value, err := decodeFieldValue(field, raw)
		if err != nil {
			return "", nil, &restoreConflictError{err: err}
		}
		columns = append(columns, quoteIdentifier(field.ColumnName))
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	if len(columns) == 0 {
		return "", nil, &restoreConflictError{err: gerror.New("回收站数据快照没有可恢复字段")}
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		quoteIdentifier(metadata.Definition.TableName), strings.Join(columns, ", "), strings.Join(placeholders, ", "),
	), args, nil
}

func decodeFieldValue(field entity.Field, raw json.RawMessage) (interface{}, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if !field.IsNullable {
			return nil, gerror.Newf("回收站数据快照非空字段为 null: %s", field.JSONName)
		}
		return nil, nil
	}
	typeName := strings.ToLower(field.DataType)
	switch typeName {
	case "json":
		if !json.Valid(raw) {
			return nil, gerror.Newf("回收站 JSON 字段无效: %s", field.JSONName)
		}
		return string(raw), nil
	case "bigint", "int", "integer", "smallint", "tinyint", "mediumint":
		var number json.Number
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&number); err != nil {
			return nil, gerror.Wrapf(err, "解析回收站整数字段 %s 失败", field.JSONName)
		}
		if field.IsUnsigned {
			value, err := strconv.ParseUint(number.String(), 10, 64)
			if err != nil {
				return nil, gerror.Wrapf(err, "解析回收站无符号字段 %s 失败", field.JSONName)
			}
			return value, nil
		}
		value, err := strconv.ParseInt(number.String(), 10, 64)
		if err != nil {
			return nil, gerror.Wrapf(err, "解析回收站整数字段 %s 失败", field.JSONName)
		}
		return value, nil
	default:
		var value interface{}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, gerror.Wrapf(err, "解析回收站字段 %s 失败", field.JSONName)
		}
		return value, nil
	}
}

func orderRestoreItems(items []*ArchiveItem) ([]*ArchiveItem, error) {
	if len(items) == 0 || len(items) > MaxBatchItems {
		return nil, gerror.Newf("回收站归档项数量必须在 1-%d 之间", MaxBatchItems)
	}
	byID := make(map[int64]*ArchiveItem, len(items))
	for _, item := range items {
		if item == nil || item.ID <= 0 {
			return nil, gerror.New("回收站归档项 ID 无效")
		}
		if _, ok := byID[item.ID]; ok {
			return nil, gerror.Newf("回收站归档项 ID 重复: %d", item.ID)
		}
		if item.Status != ItemStatusPending && item.Status != ItemStatusConflict && item.Status != ItemStatusRestored {
			return nil, gerror.Newf("回收站归档项状态无效: %d", item.ID)
		}
		byID[item.ID] = item
	}
	state := make(map[int64]uint8, len(items))
	depth := make(map[int64]int, len(items))
	var visit func(int64) (int, error)
	visit = func(id int64) (int, error) {
		switch state[id] {
		case 1:
			return 0, gerror.New("回收站恢复依赖不能成环")
		case 2:
			return depth[id], nil
		}
		item := byID[id]
		state[id] = 1
		itemDepth := 0
		if item.ParentItemID != nil {
			parent, ok := byID[*item.ParentItemID]
			if !ok {
				return 0, gerror.Newf("回收站父归档项不存在: %d", *item.ParentItemID)
			}
			if parent.BranchKey != item.BranchKey {
				return 0, gerror.New("回收站归档项不能跨分支依赖")
			}
			parentDepth, err := visit(*item.ParentItemID)
			if err != nil {
				return 0, err
			}
			itemDepth = parentDepth + 1
		}
		state[id] = 2
		depth[id] = itemDepth
		return itemDepth, nil
	}
	for id := range byID {
		if _, err := visit(id); err != nil {
			return nil, err
		}
	}
	ordered := append([]*ArchiveItem{}, items...)
	sort.SliceStable(ordered, func(left int, right int) bool {
		if depth[ordered[left].ID] != depth[ordered[right].ID] {
			return depth[ordered[left].ID] < depth[ordered[right].ID]
		}
		if ordered[left].RestoreOrder != ordered[right].RestoreOrder {
			return ordered[left].RestoreOrder < ordered[right].RestoreOrder
		}
		if ordered[left].BranchKey != ordered[right].BranchKey {
			return ordered[left].BranchKey < ordered[right].BranchKey
		}
		return ordered[left].ID < ordered[right].ID
	})
	return ordered, nil
}

func normalizeRestoreIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > maxRootItems {
		return nil, gerror.Newf("恢复数量必须在 1-%d 之间", maxRootItems)
	}
	seen := map[int64]struct{}{}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, gerror.New("恢复 ID 必须大于 0")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left] < result[right]
	})
	return result, nil
}

func restoreTenantID(ctx context.Context) (*int64, error) {
	scope := tenant.Resolve(ctx)
	switch scope.Kind() {
	case tenant.KindPlatform:
		return nil, nil
	case tenant.KindTenant:
		tenantID, _ := scope.TenantID()
		return &tenantID, nil
	default:
		return nil, gerror.New("恢复数据缺少有效租户作用域")
	}
}

func validateArchiveScope(archive *Archive, id int64, tenantID *int64) error {
	if archive.ID != id {
		return gerror.Newf("回收批次 ID 不一致: %d", id)
	}
	// nil 调用作用域表示平台管理员，可恢复任意租户归档。
	if tenantID != nil && !sameOptionalInt64(archive.TenantID, tenantID) {
		return gerror.Newf("回收批次租户不一致: %d", id)
	}
	return nil
}

func sameOptionalInt64(left *int64, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func isRestoreConflict(err error) bool {
	var target *restoreConflictError
	return errors.As(err, &target)
}

func isMySQLRestoreConflict(err error) bool {
	var mysqlError *mysql.MySQLError
	if !errors.As(err, &mysqlError) {
		return false
	}
	switch mysqlError.Number {
	case 1048, 1062, 1264, 1265, 1292, 1364, 1406:
		return true
	default:
		return false
	}
}

func (m *Manager) runRestoreHooks(ctx context.Context, resources map[string]struct{}) {
	if len(resources) == 0 {
		return
	}
	names := make([]string, 0, len(resources))
	for resource := range resources {
		names = append(names, resource)
	}
	sort.Strings(names)
	hooks := make(map[string]RestoreHook, len(names))
	m.hookMu.RLock()
	for _, resource := range names {
		hooks[resource] = m.hooks[resource]
	}
	m.hookMu.RUnlock()
	for _, resource := range names {
		hook := hooks[resource]
		if hook == nil {
			continue
		}
		if err := hook(ctx, resource); err != nil {
			g.Log().Errorf(ctx, "执行回收站资源 %s 恢复 Hook 失败: %+v", resource, err)
		}
	}
}
