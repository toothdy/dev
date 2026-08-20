package crud

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/service"
)

// CRUD 运行时
type Runtime struct {
	db            gdb.DB
	registry      *Registry
	deleteManager recycle.DeleteManager
}

/**
 * 创建 CRUD 运行时
 * @param db GoFrame 数据库实例
 * @param registry 资源注册表
 * @param deleteManagers 可选回收站删除协调器
 * @returns *Runtime
 */
func NewRuntime(db gdb.DB, registry *Registry, deleteManagers ...recycle.DeleteManager) *Runtime {
	runtime := &Runtime{db: db, registry: registry}
	if len(deleteManagers) > 0 {
		runtime.deleteManager = deleteManagers[0]
	}
	return runtime
}

/**
 * 获取资源注册表
 * @returns *Registry
 */
func (r *Runtime) Registry() *Registry {
	return r.registry
}

/**
 * 新增数据
 * @param ctx 上下文
 * @param resource 资源定义
 * @param input 输入数据
 * @returns 新增结果
 */
func (r *Runtime) Add(ctx context.Context, resource Resource, input map[string]interface{}) (interface{}, error) {
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	input = sanitizeTenantMutationInput(resource, mergeInsertParam(ctx, resource, input))
	svc := resourceService(resource)
	if handler, ok := svc.(AddHandler); ok {
		return handler.Add(ctx, AddRequest{Data: input})
	}
	var data map[string]interface{}
	err := r.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		data, err = addDefault(ctx, tx, resource, svc, input, scope)
		return err
	})
	return data, err
}

// 执行 Node 兼容的顶层数组批量新增并返回 ID 数组
func (r *Runtime) AddMany(ctx context.Context, resource Resource, inputs []map[string]interface{}) (interface{}, error) {
	if len(inputs) == 0 || len(inputs) > MaxBatchSize {
		return nil, gerror.Newf("批量新增数量必须在 1-%d 之间", MaxBatchSize)
	}
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	svc := resourceService(resource)
	if handler, custom := svc.(AddHandler); custom {
		ids := make([]interface{}, 0, len(inputs))
		for _, input := range inputs {
			input = sanitizeTenantMutationInput(resource, mergeInsertParam(ctx, resource, input))
			result, err := handler.Add(ctx, AddRequest{Data: input})
			if err != nil {
				return nil, err
			}
			if values, ok := result.(map[string]interface{}); ok {
				ids = append(ids, values[resource.PrimaryField.JSONName])
				continue
			}
			ids = append(ids, result)
		}
		return map[string]interface{}{resource.PrimaryField.JSONName: ids}, nil
	}
	ids := make([]interface{}, 0, len(inputs))
	err := r.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, input := range inputs {
			input = sanitizeTenantMutationInput(resource, mergeInsertParam(ctx, resource, input))
			result, err := addDefault(ctx, tx, resource, svc, input, scope)
			if err != nil {
				return err
			}
			ids = append(ids, result[resource.PrimaryField.JSONName])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{resource.PrimaryField.JSONName: ids}, nil
}

/**
 * 删除数据
 * @param ctx 上下文
 * @param resource 资源定义
 * @param ids ID 列表
 * @returns 删除结果
 */
func (r *Runtime) Delete(ctx context.Context, resource Resource, ids []interface{}) (interface{}, error) {
	return r.DeleteWithData(ctx, resource, ids, nil)
}

// 删除数据，并把删除请求的附加参数传给业务重写
func (r *Runtime) DeleteWithData(ctx context.Context, resource Resource, ids []interface{}, input map[string]interface{}) (interface{}, error) {
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	ids = normalizeMutationIDs(ids)
	input = sanitizeTenantMutationInput(resource, input)
	svc := resourceService(resource)
	if handler, ok := svc.(DeleteHandler); ok {
		return handler.Delete(ctx, DeleteRequest{IDs: ids, Data: cloneMap(input)})
	}
	deleteWork := func(ctx context.Context, tx gdb.TX, deleteScope *recycle.DeleteScope) error {
		if err := requireVisibleMutationRows(ctx, tx, resource, ids, scope); err != nil {
			return err
		}
		if err := runModifyBefore(ctx, svc, Delete, append([]interface{}{}, ids...)); err != nil {
			return err
		}
		query, err := buildDeleteQuery(resource, ids, scope)
		if err != nil {
			return err
		}
		result, err := tx.Ctx(ctx).Exec(query.SQL, query.Args...)
		if err != nil {
			return gerror.Wrapf(err, "删除 %s 失败", resource.Spec.Name)
		}
		if err = requireAffectedRows(result, int64(len(ids)), false, resource); err != nil {
			return err
		}
		if deleteScope != nil {
			affected, affectedErr := result.RowsAffected()
			if affectedErr != nil {
				return gerror.Wrapf(affectedErr, "读取 %s 删除行数失败", resource.Spec.Name)
			}
			if markErr := deleteScope.MarkDeleted(affected); markErr != nil {
				return markErr
			}
		}
		return runModifyAfter(ctx, svc, Delete, append([]interface{}{}, ids...))
	}
	if r.deleteManager != nil {
		params := cloneMap(input)
		if params == nil {
			params = map[string]interface{}{}
		}
		params["ids"] = append([]interface{}{}, ids...)
		err := r.deleteManager.RunDelete(ctx, recycle.DeleteRequest{
			Resource: resource.Spec.Name,
			Entity:   resource.Spec.Model.Name,
			Model:    resource.Spec.Model,
			IDs:      ids,
			Params:   params,
		}, func(workCtx context.Context, deleteScope *recycle.DeleteScope) error {
			return deleteWork(workCtx, deleteScope.TX(), deleteScope)
		})
		return nil, err
	}
	err := r.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return deleteWork(ctx, tx, nil)
	})
	return nil, err
}

/**
 * 更新数据
 * @param ctx 上下文
 * @param resource 资源定义
 * @param input 输入数据
 * @returns 更新结果
 */
func (r *Runtime) Update(ctx context.Context, resource Resource, input map[string]interface{}) (interface{}, error) {
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	input = normalizeUpdateMutationInput(resource, input)
	svc := resourceService(resource)
	if handler, ok := svc.(UpdateHandler); ok {
		return handler.Update(ctx, UpdateRequest{Data: input})
	}
	err := r.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return updateDefault(ctx, tx, resource, svc, input, scope, true)
	})
	return nil, err
}

// 执行 Node 兼容的顶层数组批量更新
func (r *Runtime) UpdateMany(ctx context.Context, resource Resource, inputs []map[string]interface{}) (interface{}, error) {
	if len(inputs) == 0 || len(inputs) > MaxBatchSize {
		return nil, gerror.Newf("批量更新数量必须在 1-%d 之间", MaxBatchSize)
	}
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	svc := resourceService(resource)
	if handler, custom := svc.(UpdateHandler); custom {
		for _, input := range inputs {
			input = normalizeUpdateMutationInput(resource, input)
			if _, err := handler.Update(ctx, UpdateRequest{Data: input}); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	preparedInputs := make([]map[string]interface{}, 0, len(inputs))
	lockIDs := make([]interface{}, 0, len(inputs))
	for _, input := range inputs {
		input = normalizeUpdateMutationInput(resource, input)
		idValue, ok := input[resource.PrimaryField.JSONName]
		if !ok || isEmptyValue(idValue) {
			return nil, exception.Validate("更新数据缺少主键")
		}
		preparedInputs = append(preparedInputs, input)
		lockIDs = append(lockIDs, idValue)
	}
	lockIDs = normalizeMutationIDs(lockIDs)
	err := r.db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := requireVisibleMutationRows(ctx, tx, resource, lockIDs, scope); err != nil {
			return err
		}
		for _, input := range preparedInputs {
			if err := updateDefault(ctx, tx, resource, svc, input, scope, false); err != nil {
				return err
			}
		}
		return nil
	})
	return nil, err
}

func addDefault(ctx context.Context, tx gdb.TX, resource Resource, svc interface{}, input map[string]interface{}, scope tenant.Scope) (map[string]interface{}, error) {
	if err := runModifyBefore(ctx, svc, Add, input); err != nil {
		return nil, err
	}
	query, err := buildInsertQuery(resource, input, scope)
	if err != nil {
		return nil, err
	}
	result, err := tx.Ctx(ctx).Exec(query.SQL, query.Args...)
	if err != nil {
		return nil, gerror.Wrapf(err, "新增 %s 失败", resource.Spec.Name)
	}
	insertID, err := insertedID(result, resource, input)
	if err != nil {
		return nil, gerror.Wrapf(err, "获取 %s 新增 ID 失败", resource.Spec.Name)
	}
	if err = runModifyAfter(ctx, svc, Add, input); err != nil {
		return nil, err
	}
	return map[string]interface{}{resource.PrimaryField.JSONName: insertID}, nil
}

func updateDefault(ctx context.Context, tx gdb.TX, resource Resource, svc interface{}, input map[string]interface{}, scope tenant.Scope, lockRow bool) error {
	idValue, ok := input[resource.PrimaryField.JSONName]
	if !ok || isEmptyValue(idValue) {
		return exception.Validate("更新数据缺少主键")
	}
	if lockRow {
		if err := requireVisibleMutationRows(ctx, tx, resource, []interface{}{idValue}, scope); err != nil {
			return err
		}
	}
	if err := runModifyBefore(ctx, svc, Update, input); err != nil {
		return err
	}
	query, queryID, err := buildUpdateQuery(resource, input, scope)
	if err != nil {
		return err
	}
	if fmt.Sprint(queryID) != fmt.Sprint(idValue) {
		return exception.Validate("更新数据主键不能修改")
	}
	result, err := tx.Ctx(ctx).Exec(query.SQL, query.Args...)
	if err != nil {
		return gerror.Wrapf(err, "更新 %s 失败", resource.Spec.Name)
	}
	if err = requireAffectedRows(result, 1, true, resource); err != nil {
		return err
	}
	return runModifyAfter(ctx, svc, Update, input)
}

/**
 * 查询详情
 * @param ctx 上下文
 * @param resource 资源定义
 * @param id 主键值
 * @returns 记录数据
 */
func (r *Runtime) Info(ctx context.Context, resource Resource, id interface{}) (interface{}, error) {
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	svc := resourceService(resource)
	if handler, ok := svc.(InfoHandler); ok {
		return handler.Info(ctx, InfoRequest{ID: id})
	}
	query, err := buildInfoQuery(resource, id, scope)
	if err != nil {
		return nil, err
	}
	row, err := r.db.GetOne(ctx, query.SQL, query.Args...)
	if err != nil {
		return nil, gerror.Wrapf(err, "查询 %s 详情失败", resource.Spec.Name)
	}
	if len(row) == 0 {
		return nil, nil
	}
	return mapRecord(resource, row, true), nil
}

/**
 * 查询列表
 * @param ctx 上下文
 * @param resource 资源定义
 * @param request 查询请求
 * @returns 记录列表
 */
func (r *Runtime) List(ctx context.Context, resource Resource, request QueryRequest) (interface{}, error) {
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	svc := resourceService(resource)
	if handler, ok := svc.(ListHandler); ok {
		return handler.List(ctx, request)
	}
	resource = queryResource(resource, resource.ListQuery)
	query, err := buildListQuery(resource, request, scope)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.GetAll(ctx, query.SQL, query.Args...)
	if err != nil {
		return nil, gerror.Wrapf(err, "查询 %s 列表失败", resource.Spec.Name)
	}
	return mapRows(resource, rows), nil
}

/**
 * 查询分页
 * @param ctx 上下文
 * @param resource 资源定义
 * @param request 查询请求
 * @returns 分页结果
 */
func (r *Runtime) Page(ctx context.Context, resource Resource, request QueryRequest) (interface{}, error) {
	scope := tenant.Resolve(ctx)
	if err := tenant.ValidateScope(scope, resource.Tenant); err != nil {
		return nil, err
	}
	svc := resourceService(resource)
	if handler, ok := svc.(PageHandler); ok {
		return handler.Page(ctx, request)
	}
	resource = queryResource(resource, resource.PageQuery)
	dataQuery, countQuery, normalized, err := buildPageQuery(resource, request, scope)
	if err != nil {
		return nil, err
	}
	total, err := r.db.GetCount(ctx, countQuery.SQL, countQuery.Args...)
	if err != nil {
		return nil, gerror.Wrapf(err, "查询 %s 总数失败", resource.Spec.Name)
	}
	rows, err := r.db.GetAll(ctx, dataQuery.SQL, dataQuery.Args...)
	if err != nil {
		return nil, gerror.Wrapf(err, "查询 %s 分页失败", resource.Spec.Name)
	}
	return PageResult{
		List: mapRows(resource, rows),
		Pagination: Pagination{
			Page:  normalized.Page,
			Size:  normalized.Size,
			Total: total,
		},
	}, nil
}

/**
 * 获取资源业务服务
 * @param resource 资源定义
 * @returns 业务服务
 */
func resourceService(resource Resource) interface{} {
	if resource.Service != nil {
		return resource.Service
	}
	return resource.Spec.Service
}

/**
 * 合并新增默认参数
 * @param ctx 上下文
 * @param resource 资源定义
 * @param input 输入数据
 * @returns 合并后的输入数据
 */
func mergeInsertParam(ctx context.Context, resource Resource, input map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for key, value := range input {
		merged[key] = value
	}
	insertParam := resource.InsertParam
	if insertParam == nil {
		insertParam = resource.Spec.InsertParam
	}
	if insertParam == nil {
		return merged
	}
	for key, value := range insertParam(ctx) {
		merged[key] = value
	}
	return merged
}

/**
 * 净化租户写入数据
 * @param resource 资源定义
 * @param input 原始输入
 * @returns 不包含客户端租户字段的副本
 */
func sanitizeTenantMutationInput(resource Resource, input map[string]interface{}) map[string]interface{} {
	data := cloneMap(input)
	if resource.Tenant.IsAware() {
		delete(data, resource.Tenant.JSONField())
	}
	return data
}

/**
 * 归一化更新写入数据
 * @param resource 资源定义
 * @param input 原始输入
 * @returns 保留主键且不包含其他只读字段的副本
 */
func normalizeUpdateMutationInput(resource Resource, input map[string]interface{}) map[string]interface{} {
	data := sanitizeTenantMutationInput(resource, input)
	for fieldName := range resource.ReadonlyFields {
		if fieldName == resource.PrimaryField.JSONName {
			continue
		}
		if _, ok := resource.FieldsByJSON[fieldName]; !ok {
			continue
		}
		delete(data, fieldName)
	}
	return data
}

/**
 * 归一化批量写入 ID
 * @param ids 原始 ID 列表
 * @returns 保持首次出现顺序的去重 ID
 */
func normalizeMutationIDs(ids []interface{}) []interface{} {
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
 * 在当前事务锁定并校验可见写入行
 * @param ctx 上下文
 * @param tx 事务
 * @param resource 资源定义
 * @param ids 已归一化的 ID 列表
 * @param scope 已解析的租户作用域
 * @returns error
 */
func requireVisibleMutationRows(ctx context.Context, tx gdb.TX, resource Resource, ids []interface{}, scope tenant.Scope) error {
	query, err := buildMutationLockQuery(resource, ids, scope)
	if err != nil {
		return err
	}
	rows, err := tx.Ctx(ctx).GetAll(query.SQL, query.Args...)
	if err != nil {
		return gerror.Wrapf(err, "校验 %s 写入范围失败", resource.Spec.Name)
	}
	if len(rows) != len(ids) {
		return exception.Comm("数据不存在")
	}
	return nil
}

/**
 * 校验数据库写入影响行数
 * @param result SQL 执行结果
 * @param expected 预期影响行数
 * @param allowZero 是否允许未改变数据的零影响行
 * @param resource 资源定义
 * @returns error
 */
func requireAffectedRows(result sql.Result, expected int64, allowZero bool, resource Resource) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrapf(err, "获取 %s 影响行数失败", resource.Spec.Name)
	}
	if affected == expected || allowZero && affected == 0 {
		return nil
	}
	return exception.Comm("数据不存在")
}

/**
 * 使用查询元数据构建资源
 * @param resource 资源定义
 * @param metadata 查询元数据
 * @returns 查询资源
 */
func queryResource(resource Resource, metadata QueryMetadata) Resource {
	if metadata.KeywordFields == nil && metadata.EqualFields == nil && metadata.LikeFields == nil {
		return resource
	}
	resource.KeywordFields = metadata.KeywordFields
	resource.EqualFields = metadata.EqualFields
	resource.LikeFields = metadata.LikeFields
	return resource
}

/**
 * 获取有效查询元数据
 * @param resource 资源定义
 * @param metadata 查询元数据
 * @returns 有效查询元数据
 */
func effectiveQueryMetadata(resource Resource, metadata QueryMetadata) QueryMetadata {
	if metadata.KeywordFields != nil || metadata.EqualFields != nil || metadata.LikeFields != nil {
		return metadata
	}
	return QueryMetadata{
		KeywordFields: resource.KeywordFields,
		EqualFields:   resource.EqualFields,
		LikeFields:    resource.LikeFields,
	}
}

/**
 * 执行修改前 hook
 * @param ctx 上下文
 * @param service 业务服务
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func runModifyBefore(ctx context.Context, svc interface{}, action string, data interface{}) error {
	hook, ok := svc.(service.ModifyBeforeHook)
	if !ok {
		return nil
	}
	return hook.ModifyBefore(ctx, action, data)
}

/**
 * 执行修改后 hook
 * @param ctx 上下文
 * @param service 业务服务
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func runModifyAfter(ctx context.Context, svc interface{}, action string, data interface{}) error {
	hook, ok := svc.(service.ModifyAfterHook)
	if !ok {
		return nil
	}
	return hook.ModifyAfter(ctx, action, data)
}

/**
 * 映射记录列表
 * @param resource 资源定义
 * @param rows 数据库记录列表
 * @returns 记录列表
 */
func mapRows(resource Resource, rows gdb.Result) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRecord(resource, row))
	}
	return items
}

/**
 * 映射单条记录
 * @param resource 资源定义
 * @param row 数据库记录
 * @returns 记录数据
 */
func mapRecord(resource Resource, row gdb.Record, infoOnly ...bool) map[string]interface{} {
	item := map[string]interface{}{}
	shouldIgnoreInfoFields := len(infoOnly) > 0 && infoOnly[0]
	for key, value := range row {
		if resource.HiddenFields[key] || (shouldIgnoreInfoFields && resource.InfoIgnoreFields[key]) {
			continue
		}
		item[key] = value.Val()
	}
	return item
}

/**
 * 获取新增 ID
 * @param result SQL 结果
 * @param resource 资源定义
 * @param input 输入数据
 * @returns ID
 */
func insertedID(result sql.Result, resource Resource, input map[string]interface{}) (interface{}, error) {
	if value, ok := input[resource.PrimaryField.JSONName]; ok {
		return value, nil
	}
	return result.LastInsertId()
}
