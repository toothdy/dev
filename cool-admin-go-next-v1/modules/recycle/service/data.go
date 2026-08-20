package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	recycleModule "github.com/toothdy/cool-admin-go-next/modules/recycle"
	recycleEvent "github.com/toothdy/cool-admin-go-next/modules/recycle/event"
)

// Catalog 表示启动期冻结的可恢复模型目录。
type Catalog struct {
	*recycle.Catalog
}

// NewCatalog 创建冻结模型目录。
func NewCatalog(models []entity.Definition) (*Catalog, error) {
	catalog, err := recycle.NewCatalog(models)
	if err != nil {
		return nil, err
	}
	return &Catalog{Catalog: catalog}, nil
}

// NewManager 创建模块唯一的回收站协调器。
func NewManager(
	db gdb.DB,
	store *recycleEvent.Store,
	catalog *Catalog,
	options module.CRUDOptions,
) (*recycle.Manager, error) {
	var coreCatalog *recycle.Catalog
	if catalog != nil {
		coreCatalog = catalog.Catalog
	}
	return recycle.NewManager(db, store, coreCatalog, recycle.Options{Enabled: options.SoftDelete})
}

// DataService 提供回收站查询、恢复和过期清理能力。
type DataService struct {
	*service.Base
	db           gdb.DB
	manager      *recycle.Manager
	store        *recycleEvent.Store
	confModel    entity.Definition
	cleanupBatch int
	lockName     string
}

type dataViewRecord struct {
	ID             int64  `orm:"id"`
	CreateTime     string `orm:"createTime"`
	UpdateTime     string `orm:"updateTime"`
	TenantID       *int64 `orm:"tenantId"`
	EntityInfo     string `orm:"entityInfo"`
	UserID         *int64 `orm:"userId"`
	UserName       string `orm:"user_name"`
	Data           string `orm:"data"`
	URL            string `orm:"url"`
	Params         string `orm:"params"`
	Count          int    `orm:"count"`
	RestoreStatus  string `orm:"restoreStatus"`
	RemainingCount int    `orm:"remainingCount"`
}

// DataView 表示 Node 兼容的回收站分页项。
type DataView struct {
	ID             int64           `json:"id"`
	CreateTime     string          `json:"createTime"`
	UpdateTime     string          `json:"updateTime"`
	TenantID       *int64          `json:"tenantId"`
	EntityInfo     json.RawMessage `json:"entityInfo"`
	UserID         *int64          `json:"userId"`
	UserName       string          `json:"userName"`
	Data           json.RawMessage `json:"data"`
	URL            string          `json:"url"`
	Params         json.RawMessage `json:"params"`
	Count          int             `json:"count"`
	RestoreStatus  string          `json:"restoreStatus"`
	RemainingCount int             `json:"remainingCount"`
}

// DataPage 表示回收站分页响应。
type DataPage struct {
	List       []DataView      `json:"list"`
	Pagination crud.Pagination `json:"pagination"`
}

// NewDataService 创建回收站数据服务。
func NewDataService(
	db gdb.DB,
	manager *recycle.Manager,
	store *recycleEvent.Store,
	dataModel entity.Definition,
	catalog *Catalog,
	config recycleModule.Config,
) (*DataService, error) {
	if db == nil || manager == nil || store == nil || catalog == nil {
		return nil, gerror.New("Recycle 服务依赖不完整")
	}
	confMetadata, hasConfModel := catalog.ModelByTable("base_sys_conf")
	if dataModel.TableName == "" || !hasConfModel ||
		config.CleanupBatch <= 0 || strings.TrimSpace(config.LockName) == "" {
		return nil, gerror.New("Recycle 服务配置不完整")
	}
	return &DataService{
		Base: service.NewBase(db, dataModel),
		db:          db, manager: manager, store: store, confModel: confMetadata.Definition,
		cleanupBatch: config.CleanupBatch, lockName: strings.TrimSpace(config.LockName),
	}, nil
}

// Restore 恢复指定回收批次，普通冲突由核心静默保留。
func (s *DataService) Restore(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return gerror.New("回收批次 ID 不能为空")
	}
	return s.manager.RestoreMany(ctx, ids)
}

// Info 查询单个回收批次并保留 JSON 字段结构。
func (s *DataService) Info(ctx context.Context, request crud.InfoRequest) (interface{}, error) {
	query, err := s.dataViewQuery(ctx)
	if err != nil {
		return nil, err
	}
	var record dataViewRecord
	err = query.Fields("a.*", "b.name AS user_name").Where("a.id", request.ID).Scan(&record)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "查询回收站详情失败")
	}
	if record.ID == 0 {
		return nil, nil
	}
	return dataViewFromRecord(record), nil
}

// Page 查询回收批次并联表返回操作人名称。
func (s *DataService) Page(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	query, err := s.dataViewQuery(ctx)
	if err != nil {
		return nil, err
	}
	if keyword := strings.TrimSpace(request.Keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("b.name LIKE ? OR a.url LIKE ?", pattern, pattern)
	}
	total, err := query.Count()
	if err != nil {
		return nil, gerror.Wrap(err, "查询回收站总数失败")
	}
	page, size := normalizePage(request)
	orderColumn := recycleOrderColumn(request.Sort)
	if strings.EqualFold(request.Order, "ASC") {
		query = query.OrderAsc(orderColumn)
	} else {
		query = query.OrderDesc(orderColumn)
	}
	var records []dataViewRecord
	err = query.Fields("a.*", "b.name AS user_name").Page(page, size).Scan(&records)
	if err != nil {
		return nil, gerror.Wrap(err, "查询回收站分页失败")
	}
	items := make([]DataView, 0, len(records))
	for _, record := range records {
		items = append(items, dataViewFromRecord(record))
	}
	return DataPage{
		List:       items,
		Pagination: crud.Pagination{Page: page, Size: size, Total: total},
	}, nil
}

func (s *DataService) dataViewQuery(ctx context.Context) (*gdb.Model, error) {
	query, err := tenant.ScopedModel(ctx, s.db, s.Model, "a")
	if err != nil {
		return nil, err
	}
	return query.LeftJoin(
		"base_sys_user b",
		"b.id = a.userId AND (a.tenantId = b.tenantId OR (a.tenantId IS NULL AND b.tenantId IS NULL))",
	), nil
}

// ClearExpired 清理超过系统保留期的回收批次。
func (s *DataService) ClearExpired(ctx context.Context) (cleaned int, resultErr error) {
	keepDays, err := s.keepDays(recycle.WithBypass(ctx))
	if err != nil {
		return 0, err
	}
	if keepDays == 0 {
		return 0, nil
	}
	now := time.Now()
	before := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -keepDays)
	lockConn, locked, err := s.acquireCleanupLock(recycle.WithBypass(ctx))
	if err != nil || !locked {
		return 0, err
	}
	defer func() {
		if releaseErr := s.releaseCleanupLock(ctx, lockConn); releaseErr != nil {
			g.Log().Errorf(context.WithoutCancel(ctx), "释放回收站清理锁失败: %+v", releaseErr)
			if resultErr == nil {
				resultErr = releaseErr
			}
		}
		_ = lockConn.Close()
	}()

	for {
		if err = ctx.Err(); err != nil {
			return cleaned, err
		}
		count := 0
		err = s.db.Transaction(recycle.WithBypass(ctx), func(txCtx context.Context, tx gdb.TX) error {
			var deleteErr error
			count, deleteErr = s.store.DeleteExpired(txCtx, tx, before, s.cleanupBatch)
			return deleteErr
		})
		if err != nil {
			return cleaned, err
		}
		cleaned += count
		if count < s.cleanupBatch {
			return cleaned, nil
		}
	}
}

func (s *DataService) acquireCleanupLock(ctx context.Context) (*sql.Conn, bool, error) {
	db, err := s.db.Master()
	if err != nil {
		return nil, false, gerror.Wrap(err, "获取回收站清理主库失败")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, gerror.Wrap(err, "获取回收站清理连接失败")
	}
	var value sql.NullInt64
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", s.lockName).Scan(&value); err != nil {
		discardSQLConnection(conn)
		_ = conn.Close()
		return nil, false, gerror.Wrap(err, "获取回收站清理锁失败")
	}
	if !value.Valid {
		discardSQLConnection(conn)
		_ = conn.Close()
		return nil, false, gerror.New("获取回收站清理锁返回空结果")
	}
	if value.Int64 != 1 {
		_ = conn.Close()
		return nil, false, nil
	}
	return conn, true, nil
}

func (s *DataService) releaseCleanupLock(ctx context.Context, conn *sql.Conn) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var value sql.NullInt64
	err := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", s.lockName).Scan(&value)
	if err == nil && value.Valid && value.Int64 == 1 {
		return nil
	}
	discardSQLConnection(conn)
	if err != nil {
		return gerror.Wrap(err, "释放回收站清理锁失败")
	}
	return gerror.New("回收站清理锁不属于当前连接")
}

func discardSQLConnection(conn *sql.Conn) {
	if conn != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
}

func (s *DataService) keepDays(ctx context.Context) (int, error) {
	query, err := tenant.ScopedModel(tenant.WithoutTenant(ctx), s.db, s.confModel, "")
	if err != nil {
		return 0, err
	}
	var record struct {
		Value string `orm:"cValue"`
	}
	if err = query.Fields("cValue").Where("cKey", "recycleKeep").Scan(&record); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, gerror.New("系统参数 recycleKeep 不存在")
		}
		return 0, gerror.Wrap(err, "读取回收站保留期失败")
	}
	value := strings.TrimSpace(record.Value)
	keepDays, err := strconv.Atoi(value)
	if err != nil {
		return 0, gerror.Wrap(err, "系统参数 recycleKeep 必须是整数")
	}
	if keepDays < 0 {
		return 0, gerror.New("系统参数 recycleKeep 不能小于 0")
	}
	return keepDays, nil
}

func normalizePage(request crud.QueryRequest) (int, int) {
	page := request.Page
	if page <= 0 {
		page = 1
	}
	size := request.Size
	if size <= 0 {
		size = 15
	}
	if size > crud.MaxPageSize {
		size = crud.MaxPageSize
	}
	if request.IsExport {
		page = 1
		size = request.MaxExportLimit
		if size <= 0 || size > crud.MaxExportSize {
			size = crud.MaxExportSize
		}
	}
	return page, size
}

func recycleOrderColumn(field string) string {
	switch field {
	case "createTime":
		return "a.createTime"
	case "updateTime":
		return "a.updateTime"
	case "count":
		return "a.count"
	case "remainingCount":
		return "a.remainingCount"
	default:
		return "a.id"
	}
}

func dataViewFromRecord(record dataViewRecord) DataView {
	return DataView{
		ID: record.ID, CreateTime: record.CreateTime, UpdateTime: record.UpdateTime, TenantID: record.TenantID,
		EntityInfo: json.RawMessage(record.EntityInfo), UserID: record.UserID, UserName: record.UserName,
		Data: json.RawMessage(record.Data), URL: record.URL, Params: rawMessage(record.Params), Count: record.Count,
		RestoreStatus: record.RestoreStatus, RemainingCount: record.RemainingCount,
	}
}

func rawMessage(value string) json.RawMessage {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return json.RawMessage(value)
}
