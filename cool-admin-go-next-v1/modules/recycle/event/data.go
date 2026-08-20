package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
)

type dataRecord struct {
	ID             int64  `orm:"id"`
	EntityInfo     string `orm:"entityInfo"`
	UserID         *int64 `orm:"userId"`
	Data           string `orm:"data"`
	URL            string `orm:"url"`
	Params         string `orm:"params"`
	Count          int    `orm:"count"`
	RestoreStatus  string `orm:"restoreStatus"`
	RemainingCount int    `orm:"remainingCount"`
	TenantID       *int64 `orm:"tenantId"`
}

type dataDO struct {
	g.Meta         `orm:"do:true"`
	CreateTime     interface{} `orm:"createTime"`
	UpdateTime     interface{} `orm:"updateTime"`
	EntityInfo     interface{} `orm:"entityInfo"`
	UserID         interface{} `orm:"userId"`
	Data           interface{} `orm:"data"`
	URL            interface{} `orm:"url"`
	Params         interface{} `orm:"params"`
	Count          interface{} `orm:"count"`
	RestoreStatus  interface{} `orm:"restoreStatus"`
	RemainingCount interface{} `orm:"remainingCount"`
	TenantID       interface{} `orm:"tenantId"`
}

type itemRecord struct {
	ID           int64  `orm:"id"`
	RecycleID    int64  `orm:"recycleId"`
	Resource     string `orm:"resource"`
	TableName    string `orm:"tableName"`
	PrimaryKey   string `orm:"primaryKey"`
	Data         string `orm:"data"`
	BranchKey    string `orm:"branchKey"`
	ParentItemID *int64 `orm:"parentItemId"`
	RestoreOrder int    `orm:"restoreOrder"`
	Status       string `orm:"status"`
	Error        string `orm:"error"`
	TenantID     *int64 `orm:"tenantId"`
}

type itemDO struct {
	g.Meta       `orm:"do:true"`
	CreateTime   interface{} `orm:"createTime"`
	UpdateTime   interface{} `orm:"updateTime"`
	RecycleID    interface{} `orm:"recycleId"`
	Resource     interface{} `orm:"resource"`
	TableName    interface{} `orm:"tableName"`
	PrimaryKey   interface{} `orm:"primaryKey"`
	Data         interface{} `orm:"data"`
	BranchKey    interface{} `orm:"branchKey"`
	ParentItemID interface{} `orm:"parentItemId"`
	RestoreOrder interface{} `orm:"restoreOrder"`
	Status       interface{} `orm:"status"`
	Error        interface{} `orm:"error"`
	TenantID     interface{} `orm:"tenantId"`
}

// Store 实现 Recycle 核心持久化契约。
type Store struct {
	db        gdb.DB
	dataModel entity.Definition
	itemModel entity.Definition
}

// NewStore 创建 Recycle 持久化 Store。
func NewStore(
	db gdb.DB,
	dataModel entity.Definition,
	itemModel entity.Definition,
) (*Store, error) {
	if db == nil {
		return nil, gerror.New("Recycle 数据库不能为空")
	}
	if dataModel.TableName == "" || itemModel.TableName == "" {
		return nil, gerror.New("Recycle 模型定义不完整")
	}
	return &Store{db: db, dataModel: dataModel, itemModel: itemModel}, nil
}

// SaveArchive 在删除事务内保存完整归档。
func (s *Store) SaveArchive(ctx context.Context, tx gdb.TX, archive *recycle.Archive) error {
	if tx == nil || archive == nil {
		return gerror.New("回收归档写入参数不完整")
	}
	entityInfo, err := json.Marshal(archive.EntityInfo)
	if err != nil {
		return gerror.Wrap(err, "编码回收实体信息失败")
	}
	params := nullableJSON(archive.Params)
	now := time.Now().Format("2006-01-02 15:04:05")
	id, err := tx.Model(s.dataModel.TableName).Ctx(ctx).Data(dataDO{
		CreateTime: now, UpdateTime: now,
		EntityInfo: string(entityInfo), UserID: archive.UserID, Data: string(archive.Data), URL: nullableString(archive.URL),
		Params: params, Count: archive.Count, RestoreStatus: archive.RestoreStatus,
		RemainingCount: archive.RemainingCount, TenantID: archive.TenantID,
	}).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "写入回收批次失败")
	}
	archive.ID = id
	itemIDs := make(map[string]int64, len(archive.Items))
	for _, item := range archive.Items {
		if item == nil {
			return gerror.New("回收归档项不能为空")
		}
		identity, encodeErr := json.Marshal(item.Identity)
		if encodeErr != nil {
			return gerror.Wrap(encodeErr, "编码回收数据身份失败")
		}
		var parentItemID *int64
		if item.ParentKey != "" {
			parentID, ok := itemIDs[item.ParentKey]
			if !ok {
				return gerror.Newf("回收归档父项尚未写入: %s", item.ParentKey)
			}
			parentItemID = &parentID
		}
		itemID, insertErr := tx.Model(s.itemModel.TableName).Ctx(ctx).Data(itemDO{
			CreateTime: now, UpdateTime: now,
			RecycleID: id, Resource: item.Resource, TableName: item.TableName, PrimaryKey: string(identity),
			Data: string(item.Data), BranchKey: item.BranchKey, ParentItemID: parentItemID,
			RestoreOrder: item.RestoreOrder, Status: item.Status, Error: nullableString(item.Error), TenantID: item.TenantID,
		}).InsertAndGetId()
		if insertErr != nil {
			return gerror.Wrap(insertErr, "写入回收数据项失败")
		}
		item.ID = itemID
		item.RecycleID = id
		item.ParentItemID = parentItemID
		itemIDs[item.Key] = itemID
	}
	return nil
}

// LockArchive 锁定并读取一个待恢复批次。
func (s *Store) LockArchive(ctx context.Context, tx gdb.TX, id int64, tenantID *int64) (*recycle.Archive, error) {
	if tx == nil || id <= 0 {
		return nil, gerror.New("回收批次参数无效")
	}
	query := tx.Model(s.dataModel.TableName).Ctx(ctx).Where("id", id)
	if tenantID != nil {
		query = query.Where("tenantId", *tenantID)
	}
	var record dataRecord
	if err := query.LockUpdate().Scan(&record); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "读取回收批次失败")
	}
	if record.ID == 0 {
		return nil, nil
	}
	archive, err := decodeArchiveRecord(record)
	if err != nil {
		return nil, err
	}
	itemQuery := tx.Model(s.itemModel.TableName).Ctx(ctx).Where("recycleId", id)
	if tenantID != nil {
		itemQuery = itemQuery.Where("tenantId", *tenantID)
	}
	var items []itemRecord
	if err = itemQuery.OrderAsc("restoreOrder").OrderAsc("id").LockUpdate().Scan(&items); err != nil {
		return nil, gerror.Wrap(err, "读取回收数据项失败")
	}
	archive.Items = make([]*recycle.ArchiveItem, 0, len(items))
	for _, item := range items {
		decoded, decodeErr := decodeArchiveItem(item)
		if decodeErr != nil {
			return nil, decodeErr
		}
		archive.Items = append(archive.Items, decoded)
	}
	return archive, nil
}

// SaveRestoreState 保存部分恢复后的批次和数据项状态。
func (s *Store) SaveRestoreState(ctx context.Context, tx gdb.TX, archive *recycle.Archive) error {
	if tx == nil || archive == nil || archive.ID <= 0 {
		return gerror.New("回收恢复状态参数不完整")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, item := range archive.Items {
		if item == nil || item.ID <= 0 {
			return gerror.New("回收恢复项参数不完整")
		}
		_, err := tx.Model(s.itemModel.TableName).Ctx(ctx).
			Where("id", item.ID).Where("recycleId", archive.ID).
			Data(itemDO{Status: item.Status, Error: nullableString(item.Error), UpdateTime: now}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新回收数据项状态失败")
		}
	}
	_, err := tx.Model(s.dataModel.TableName).Ctx(ctx).Where("id", archive.ID).Data(dataDO{
		RestoreStatus: archive.RestoreStatus, RemainingCount: archive.RemainingCount, UpdateTime: now,
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新回收批次状态失败")
	}
	return nil
}

// DeleteArchive 在恢复事务内删除已完成归档。
func (s *Store) DeleteArchive(ctx context.Context, tx gdb.TX, id int64, tenantID *int64) error {
	if tx == nil || id <= 0 {
		return gerror.New("回收批次参数无效")
	}
	itemQuery := tx.Model(s.itemModel.TableName).Ctx(ctx).Where("recycleId", id)
	dataQuery := tx.Model(s.dataModel.TableName).Ctx(ctx).Where("id", id)
	if tenantID != nil {
		itemQuery = itemQuery.Where("tenantId", *tenantID)
		dataQuery = dataQuery.Where("tenantId", *tenantID)
	}
	if _, err := itemQuery.Delete(); err != nil {
		return gerror.Wrap(err, "删除回收数据项失败")
	}
	result, err := dataQuery.Delete()
	if err != nil {
		return gerror.Wrap(err, "删除回收批次失败")
	}
	return requireAffected(result, "回收批次不存在")
}

// DeleteExpired 删除一批过期回收记录。
func (s *Store) DeleteExpired(ctx context.Context, tx gdb.TX, before time.Time, limit int) (int, error) {
	if tx == nil || limit <= 0 {
		return 0, gerror.New("回收清理参数无效")
	}
	var records []struct {
		ID int64 `orm:"id"`
	}
	if err := tx.Model(s.dataModel.TableName).Ctx(ctx).Fields("id").
		WhereLT("createTime", before).OrderAsc("createTime").OrderAsc("id").Limit(limit).LockUpdate().Scan(&records); err != nil {
		return 0, gerror.Wrap(err, "读取过期回收批次失败")
	}
	if len(records) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	if _, err := tx.Model(s.itemModel.TableName).Ctx(ctx).WhereIn("recycleId", ids).Delete(); err != nil {
		return 0, gerror.Wrap(err, "清理过期回收数据项失败")
	}
	result, err := tx.Model(s.dataModel.TableName).Ctx(ctx).WhereIn("id", ids).Delete()
	if err != nil {
		return 0, gerror.Wrap(err, "清理过期回收批次失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "读取回收清理结果失败")
	}
	return int(affected), nil
}

func decodeArchiveRecord(record dataRecord) (*recycle.Archive, error) {
	var entityInfo recycle.EntityInfo
	if err := json.Unmarshal([]byte(record.EntityInfo), &entityInfo); err != nil {
		return nil, gerror.Wrap(err, "解析回收实体信息失败")
	}
	return &recycle.Archive{
		ID: record.ID, EntityInfo: entityInfo, UserID: record.UserID, Data: json.RawMessage(record.Data),
		URL: record.URL, Params: json.RawMessage(record.Params), Count: record.Count,
		RestoreStatus: recycle.RestoreStatus(record.RestoreStatus), RemainingCount: record.RemainingCount,
		TenantID: record.TenantID,
	}, nil
}

func decodeArchiveItem(record itemRecord) (*recycle.ArchiveItem, error) {
	var identity recycle.Identity
	if err := json.Unmarshal([]byte(record.PrimaryKey), &identity); err != nil {
		return nil, gerror.Wrap(err, "解析回收数据身份失败")
	}
	return &recycle.ArchiveItem{
		ID: record.ID, RecycleID: record.RecycleID, Resource: record.Resource, TableName: record.TableName,
		Identity: identity, Data: json.RawMessage(record.Data), BranchKey: record.BranchKey,
		ParentItemID: record.ParentItemID, RestoreOrder: record.RestoreOrder,
		Status: recycle.ItemStatus(record.Status), Error: record.Error, TenantID: record.TenantID,
	}, nil
}

func nullableJSON(value json.RawMessage) interface{} {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func requireAffected(result sql.Result, message string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取回收站写入结果失败")
	}
	if affected == 0 {
		return gerror.New(message)
	}
	return nil
}
