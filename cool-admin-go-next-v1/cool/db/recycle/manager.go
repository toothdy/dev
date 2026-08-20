package recycle

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

const maxRootItems = 500

// RestoreHook 表示恢复事务提交后的资源对账动作
type RestoreHook func(ctx context.Context, resource string) error

// Options 表示回收站 Manager 启动配置
type Options struct {
	Enabled bool
}

// DeleteRequest 表示一次受管删除请求
type DeleteRequest struct {
	Resource string
	Entity   string
	Model    entity.Definition
	IDs      []interface{}
	Params   interface{}
}

// DeleteWork 表示在 Manager 事务内执行的业务删除
type DeleteWork func(ctx context.Context, scope *DeleteScope) error

// DeleteManager 定义 CRUD 依赖的最小删除协调契约
type DeleteManager interface {
	RunDelete(ctx context.Context, request DeleteRequest, work DeleteWork) error
}

// DeleteScope 表示受 Manager 控制的事务删除作用域
type DeleteScope struct {
	tx           gdb.TX
	batch        *Batch
	isArchiving  bool
	deletedCount int
	afterCommit  []func(context.Context) error
}

// Manager 协调归档、物理删除和恢复事务
type Manager struct {
	db       gdb.DB
	store    Store
	catalog  *Catalog
	enabled  bool
	hookMu   sync.RWMutex
	hooks    map[string]RestoreHook
	isFrozen bool
}

/**
 * 创建回收站协调器
 * @param db GoFrame 数据库实例
 * @param store 归档持久化实现
 * @param catalog 冻结模型目录
 * @param options 启动配置
 * @returns *Manager 和校验错误
 */
func NewManager(db gdb.DB, store Store, catalog *Catalog, options Options) (*Manager, error) {
	if db == nil {
		return nil, gerror.New("回收站数据库不能为空")
	}
	if store == nil {
		return nil, gerror.New("回收站 Store 不能为空")
	}
	if catalog == nil {
		return nil, gerror.New("回收站模型目录不能为空")
	}
	return &Manager{
		db: db, store: store, catalog: catalog, enabled: options.Enabled, hooks: map[string]RestoreHook{},
	}, nil
}

/**
 * 返回是否开启新增删除归档
 * @returns bool
 */
func (m *Manager) Enabled() bool {
	return m != nil && m.enabled
}

/**
 * 返回冻结模型目录
 * @returns *Catalog
 */
func (m *Manager) Catalog() *Catalog {
	if m == nil {
		return nil
	}
	return m.catalog
}

/**
 * 注册资源恢复后对账动作
 * @param resource 稳定资源名
 * @param hook 恢复动作
 * @returns error
 */
func (m *Manager) RegisterRestoreHook(resource string, hook RestoreHook) error {
	if m == nil {
		return gerror.New("回收站 Manager 不能为空")
	}
	if _, ok := m.catalog.Model(resource); !ok {
		return gerror.Newf("回收站恢复 Hook 资源未注册: %s", resource)
	}
	if hook == nil {
		return gerror.Newf("回收站恢复 Hook 不能为空: %s", resource)
	}
	m.hookMu.Lock()
	defer m.hookMu.Unlock()
	if m.isFrozen {
		return gerror.New("回收站恢复 Hook 已冻结")
	}
	if _, ok := m.hooks[resource]; ok {
		return gerror.Newf("回收站恢复 Hook 重复: %s", resource)
	}
	m.hooks[resource] = hook
	return nil
}

/**
 * 冻结恢复 Hook 注册表
 * @returns null
 */
func (m *Manager) FreezeRestoreHooks() {
	if m == nil {
		return
	}
	m.hookMu.Lock()
	m.isFrozen = true
	m.hookMu.Unlock()
}

/**
 * 执行同步归档与业务删除
 * @param ctx 上下文
 * @param request 删除请求
 * @param work 事务内删除回调
 * @returns error
 */
func (m *Manager) RunDelete(ctx context.Context, request DeleteRequest, work DeleteWork) error {
	if m == nil || m.db == nil || work == nil {
		return gerror.New("回收站删除协调器不可用")
	}
	ids, err := normalizeIDs(request.IDs)
	if err != nil {
		return err
	}
	isArchiving := m.enabled && !IsBypass(ctx)
	metadata := RequestMetadata{}
	if isArchiving {
		metadata, err = resolveRequestMetadata(ctx, request.Params)
		if err != nil {
			return err
		}
	}
	scope := &DeleteScope{isArchiving: isArchiving}
	err = m.db.Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		scope.tx = tx
		if isArchiving {
			batch, batchErr := m.prepareRootBatch(txCtx, tx, request, ids, metadata)
			if batchErr != nil {
				return batchErr
			}
			scope.batch = batch
		}
		if workErr := work(txCtx, scope); workErr != nil {
			return workErr
		}
		if !isArchiving {
			return nil
		}
		if batchErr := scope.batch.Validate(); batchErr != nil {
			return batchErr
		}
		if scope.deletedCount != len(scope.batch.archive.Items) {
			return gerror.Newf("回收站归档数量 %d 与物理删除数量 %d 不一致", len(scope.batch.archive.Items), scope.deletedCount)
		}
		if saveErr := m.store.SaveArchive(txCtx, tx, scope.batch.archive); saveErr != nil {
			return gerror.Wrap(saveErr, "保存回收站归档失败")
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, action := range scope.afterCommit {
		if actionErr := action(ctx); actionErr != nil {
			g.Log().Errorf(ctx, "执行回收站删除提交后动作失败: %+v", actionErr)
		}
	}
	return nil
}

/**
 * 返回 Manager 提供的数据库事务
 * @returns gdb.TX
 */
func (s *DeleteScope) TX() gdb.TX {
	if s == nil {
		return nil
	}
	return s.tx
}

/**
 * 判断当前删除是否写入回收站
 * @returns bool
 */
func (s *DeleteScope) IsArchiving() bool {
	return s != nil && s.isArchiving
}

/**
 * 追加一条关联数据快照
 * @param definition 模型定义
 * @param record 完整数据记录
 * @param options 依赖配置
 * @returns 归档项内部键和错误
 */
func (s *DeleteScope) AddRecord(definition entity.Definition, record map[string]interface{}, options ItemOptions) (string, error) {
	if s == nil || !s.isArchiving {
		return "", nil
	}
	return s.batch.AddRecord(definition, record, options)
}

/**
 * 追加关联数据查询结果
 * @param definition 模型定义
 * @param rows 完整数据记录
 * @param options 依赖配置
 * @returns 归档项内部键和错误
 */
func (s *DeleteScope) AddResult(definition entity.Definition, rows gdb.Result, options ItemOptions) ([]string, error) {
	if s == nil || !s.isArchiving {
		return nil, nil
	}
	return s.batch.AddResult(definition, rows, options)
}

/**
 * 按根身份值查找归档项内部键
 * @param value 根身份值
 * @returns 归档项内部键和是否存在
 */
func (s *DeleteScope) RootKey(value interface{}) (string, bool) {
	if s == nil || !s.isArchiving {
		return "", false
	}
	return s.batch.RootKey(value)
}

/**
 * 累加业务物理删除行数
 * @param count 删除行数
 * @returns error
 */
func (s *DeleteScope) MarkDeleted(count int64) error {
	if count < 0 {
		return gerror.New("物理删除行数不能为负数")
	}
	if s != nil && s.isArchiving {
		s.deletedCount += int(count)
	}
	return nil
}

/**
 * 注册数据库提交后动作
 * @param action 提交后动作
 * @returns error
 */
func (s *DeleteScope) AfterCommit(action func(context.Context) error) error {
	if action == nil {
		return gerror.New("删除提交后动作不能为空")
	}
	s.afterCommit = append(s.afterCommit, action)
	return nil
}

func (m *Manager) prepareRootBatch(ctx context.Context, tx gdb.TX, request DeleteRequest, ids []interface{}, metadata RequestMetadata) (*Batch, error) {
	modelMetadata, ok := m.catalog.ModelByTable(request.Model.TableName)
	if !ok || modelMetadata.Resource != request.Model.ResourceKey() {
		return nil, gerror.Newf("回收站根模型未注册: %s", request.Model.ResourceKey())
	}
	if len(modelMetadata.IdentityFields) != 1 || !modelMetadata.IdentityFields[0].IsPrimary {
		return nil, gerror.Newf("回收站根模型必须使用单主键: %s", modelMetadata.Resource)
	}
	rows, err := lockSnapshots(ctx, tx, modelMetadata, ids)
	if err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, gerror.Newf("删除 %s 失败: 数据不存在或不可见", request.Resource)
	}
	entityName := request.Entity
	if entityName == "" {
		entityName = request.Model.Name
	}
	archive := &Archive{
		EntityInfo: EntityInfo{DataSourceName: "default", Entity: entityName, Resource: modelMetadata.Resource},
		UserID:     cloneInt64Pointer(&metadata.UserID), URL: metadata.URL, Params: append(json.RawMessage(nil), metadata.Params...),
		TenantID: cloneInt64Pointer(metadata.TenantID),
	}
	batch := NewBatch(m.catalog, archive)
	rootData := make([]map[string]interface{}, 0, len(rows))
	primaryField := modelMetadata.IdentityFields[0]
	for _, row := range rows {
		record, normalizeErr := normalizeRecord(modelMetadata, row.Map())
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		rootData = append(rootData, record)
		branchKey, keyErr := canonicalValue(record[primaryField.JSONName])
		if keyErr != nil {
			return nil, keyErr
		}
		itemKey, addErr := batch.AddRecord(request.Model, record, ItemOptions{BranchKey: branchKey})
		if addErr != nil {
			return nil, addErr
		}
		if setErr := batch.SetRootKey(record[primaryField.JSONName], itemKey); setErr != nil {
			return nil, setErr
		}
	}
	archive.Data, err = json.Marshal(rootData)
	if err != nil {
		return nil, gerror.Wrap(err, "序列化回收站根数据失败")
	}
	return batch, nil
}

func resolveRequestMetadata(ctx context.Context, params interface{}) (RequestMetadata, error) {
	metadata, hasMetadata := RequestMetadataFromContext(ctx)
	user, hasUser := security.UserFromContext(ctx)
	if metadata.UserID <= 0 && hasUser {
		metadata.UserID = user.UserId
	}
	if metadata.UserID <= 0 {
		return RequestMetadata{}, gerror.New("删除数据缺少操作人上下文")
	}
	scope := tenant.Resolve(ctx)
	switch scope.Kind() {
	case tenant.KindTenant:
		tenantID, _ := scope.TenantID()
		if metadata.TenantID != nil && *metadata.TenantID != tenantID {
			return RequestMetadata{}, gerror.New("删除数据的租户审计信息与当前作用域不一致")
		}
		metadata.TenantID = &tenantID
	case tenant.KindPlatform:
		if metadata.TenantID != nil {
			return RequestMetadata{}, gerror.New("平台删除不能伪造租户审计信息")
		}
	default:
		return RequestMetadata{}, gerror.New("删除数据缺少有效租户作用域")
	}
	if !hasMetadata {
		if request := ghttp.RequestFromCtx(ctx); request != nil {
			metadata.URL = request.URL.Path
			metadata.Method = request.Method
		}
	}
	if len(metadata.Params) == 0 && params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return RequestMetadata{}, gerror.Wrap(err, "序列化删除请求参数失败")
		}
		metadata.Params = encoded
	}
	return metadata, nil
}

func lockSnapshots(ctx context.Context, tx gdb.TX, metadata ModelMetadata, ids []interface{}) (gdb.Result, error) {
	columns := make([]string, 0, len(metadata.Definition.FieldsValue))
	for _, field := range metadata.Definition.FieldsValue {
		columns = append(columns, fmt.Sprintf("%s AS %s", quoteIdentifier(field.ColumnName), quoteIdentifier(field.JSONName)))
	}
	primary := metadata.IdentityFields[0]
	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	condition, err := tenant.Predicate(ctx, metadata.Tenant, "")
	if err != nil {
		return nil, err
	}
	whereSQL := fmt.Sprintf("%s IN (%s)", quoteIdentifier(primary.ColumnName), strings.Join(placeholders, ", "))
	if condition.SQL != "" {
		whereSQL += " AND " + condition.SQL
		args = append(args, condition.Args...)
	}
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY %s FOR UPDATE",
		strings.Join(columns, ", "), quoteIdentifier(metadata.Definition.TableName), whereSQL, quoteIdentifier(primary.ColumnName),
	)
	rows, err := tx.Ctx(ctx).GetAll(query, args...)
	if err != nil {
		return nil, gerror.Wrapf(err, "锁定回收站根数据 %s 失败", metadata.Resource)
	}
	return rows, nil
}

func normalizeIDs(values []interface{}) ([]interface{}, error) {
	if len(values) == 0 || len(values) > maxRootItems {
		return nil, gerror.Newf("删除数量必须在 1-%d 之间", maxRootItems)
	}
	type idValue struct {
		key   string
		value interface{}
	}
	items := make([]idValue, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == nil {
			return nil, gerror.New("删除 ID 不能为空")
		}
		key, err := canonicalValue(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, idValue{key: key, value: value})
	}
	sort.SliceStable(items, func(left int, right int) bool {
		return items[left].key < items[right].key
	})
	result := make([]interface{}, 0, len(items))
	for _, item := range items {
		result = append(result, item.value)
	}
	return result, nil
}

func quoteIdentifier(value string) string {
	return "`" + value + "`"
}
