package recycle

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/database/gdb"
)

// RestoreStatus 表示回收批次的恢复状态
type RestoreStatus string

const (
	RestoreStatusPending RestoreStatus = "pending"
	RestoreStatusPartial RestoreStatus = "partial"
)

// ItemStatus 表示单条归档项的恢复状态
type ItemStatus string

const (
	ItemStatusPending  ItemStatus = "pending"
	ItemStatusRestored ItemStatus = "restored"
	ItemStatusConflict ItemStatus = "conflict"
)

// EntityInfo 表示 Node 兼容的根实体信息
type EntityInfo struct {
	DataSourceName string `json:"dataSourceName"`
	Entity         string `json:"entity"`
	Resource       string `json:"resource"`
}

// Archive 表示一次删除操作的完整归档
type Archive struct {
	ID             int64
	EntityInfo     EntityInfo
	UserID         *int64
	Data           json.RawMessage
	URL            string
	Params         json.RawMessage
	Count          int
	RestoreStatus  RestoreStatus
	RemainingCount int
	TenantID       *int64
	Items          []*ArchiveItem
}

// ArchiveItem 表示一条可恢复的数据快照
type ArchiveItem struct {
	ID            int64
	Key           string
	RecycleID     int64
	Resource      string
	TableName     string
	Identity      Identity
	Data          json.RawMessage
	BranchKey     string
	ParentKey     string
	ParentItemID  *int64
	RestoreOrder  int
	Status        ItemStatus
	Error         string
	TenantID      *int64
	RestoredInRun bool
}

// Store 定义事务内归档与恢复持久化契约
type Store interface {
	SaveArchive(ctx context.Context, tx gdb.TX, archive *Archive) error
	LockArchive(ctx context.Context, tx gdb.TX, id int64, tenantID *int64) (*Archive, error)
	SaveRestoreState(ctx context.Context, tx gdb.TX, archive *Archive) error
	DeleteArchive(ctx context.Context, tx gdb.TX, id int64, tenantID *int64) error
}
