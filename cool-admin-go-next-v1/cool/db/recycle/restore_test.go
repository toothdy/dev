package recycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

type restoreStubDB struct {
	gdb.DB
	insertCount  int
	rollbackTo   int
	conflictAt   int
	transactions int
}

func (d *restoreStubDB) Transaction(ctx context.Context, callback func(context.Context, gdb.TX) error) error {
	d.transactions++
	return callback(ctx, &restoreStubTX{db: d})
}

type restoreStubTX struct {
	gdb.TX
	db *restoreStubDB
}

func (tx *restoreStubTX) Ctx(context.Context) gdb.TX {
	return tx
}

func (tx *restoreStubTX) SavePoint(string) error {
	return nil
}

func (tx *restoreStubTX) RollbackTo(string) error {
	tx.db.rollbackTo++
	return nil
}

func (tx *restoreStubTX) Exec(string, ...interface{}) (sql.Result, error) {
	tx.db.insertCount++
	if tx.db.insertCount == tx.db.conflictAt {
		return nil, &mysql.MySQLError{Number: 1062, Message: "duplicate"}
	}
	return restoreSQLResult{}, nil
}

type restoreSQLResult struct{}

func (restoreSQLResult) LastInsertId() (int64, error) {
	return 1, nil
}

func (restoreSQLResult) RowsAffected() (int64, error) {
	return 1, nil
}

type restoreStubStore struct {
	archive     *Archive
	saveCount   int
	deleteCount int
}

func (s *restoreStubStore) SaveArchive(context.Context, gdb.TX, *Archive) error {
	return nil
}

func (s *restoreStubStore) LockArchive(context.Context, gdb.TX, int64, *int64) (*Archive, error) {
	return s.archive, nil
}

func (s *restoreStubStore) SaveRestoreState(_ context.Context, _ gdb.TX, archive *Archive) error {
	s.saveCount++
	s.archive = archive
	return nil
}

func (s *restoreStubStore) DeleteArchive(context.Context, gdb.TX, int64, *int64) error {
	s.deleteCount++
	return nil
}

func TestBuildRestoreInsertPreservesUnsignedBigInt(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, err := NewCatalog([]entity.Definition{definition})
	if err != nil {
		t.Fatalf("compile recycle catalog failed: %v", err)
	}
	metadata, _ := catalog.Model("demo.type")
	query, args, err := buildRestoreInsert(metadata, json.RawMessage(`{"id":18446744073709551615,"name":"root"}`))
	if err != nil {
		t.Fatalf("build restore insert failed: %v", err)
	}
	if !strings.Contains(query, "INSERT INTO `demo_type`") || len(args) != 2 {
		t.Fatalf("unexpected restore insert: %s %#v", query, args)
	}
	if value, ok := args[0].(uint64); !ok || value != math.MaxUint64 {
		t.Fatalf("unsigned bigint lost precision: %#v", args[0])
	}
}

func TestOrderRestoreItemsRejectsCycleAndSortsParentsFirst(t *testing.T) {
	parentID := int64(1)
	items := []*ArchiveItem{
		{ID: 2, BranchKey: "a", ParentItemID: &parentID, RestoreOrder: 1, Status: ItemStatusPending},
		{ID: 1, BranchKey: "a", RestoreOrder: 10, Status: ItemStatusPending},
	}
	ordered, err := orderRestoreItems(items)
	if err != nil {
		t.Fatalf("order restore items failed: %v", err)
	}
	if ordered[0].ID != 1 || ordered[1].ID != 2 {
		t.Fatalf("parent was not restored before child: %#v", ordered)
	}

	secondID := int64(2)
	items[1].ParentItemID = &secondID
	if _, err = orderRestoreItems(items); err == nil || !strings.Contains(err.Error(), "成环") {
		t.Fatalf("expected restore dependency cycle rejected, got %v", err)
	}
}

func TestManagerRestoreSilentlyKeepsConflictAndRestoresIndependentParent(t *testing.T) {
	rootModel := recycleTestRootModel()
	childModel := recycleTestRelationModel()
	catalog, err := NewCatalog([]entity.Definition{rootModel, childModel})
	if err != nil {
		t.Fatalf("compile restore catalog failed: %v", err)
	}
	parentID := int64(11)
	archive := &Archive{
		ID: 7,
		Items: []*ArchiveItem{
			{
				ID: 11, RecycleID: 7, Resource: rootModel.ResourceKey(), TableName: rootModel.TableName,
				Identity: Identity{Fields: []IdentityField{{JSONName: "id", ColumnName: "id", Value: json.RawMessage(`1`)}}},
				Data:     json.RawMessage(`{"id":1,"name":"root"}`), BranchKey: "a", Status: ItemStatusPending,
			},
			{
				ID: 12, RecycleID: 7, Resource: childModel.ResourceKey(), TableName: childModel.TableName,
				Identity: Identity{Fields: []IdentityField{
					{JSONName: "typeId", ColumnName: "typeId", Value: json.RawMessage(`1`)},
					{JSONName: "itemId", ColumnName: "item_id", Value: json.RawMessage(`2`)},
				}},
				Data: json.RawMessage(`{"typeId":1,"itemId":2}`), BranchKey: "a", ParentItemID: &parentID, Status: ItemStatusPending,
			},
		},
	}
	db := &restoreStubDB{conflictAt: 2}
	store := &restoreStubStore{archive: archive}
	manager, err := NewManager(db, store, catalog, Options{Enabled: true})
	if err != nil {
		t.Fatalf("create restore manager failed: %v", err)
	}
	ctx := security.ContextWithUser(context.Background(), security.UserContext{UserId: 9, TenantId: security.PlatformTenant()})
	if err = manager.Restore(ctx, 7); err != nil {
		t.Fatalf("ordinary restore conflict must stay silent: %v", err)
	}
	if db.transactions != 1 || db.rollbackTo != 1 || store.saveCount != 1 || store.deleteCount != 0 {
		t.Fatalf("unexpected partial restore lifecycle: db=%#v store=%#v", db, store)
	}
	if archive.RemainingCount != 1 || archive.RestoreStatus != RestoreStatusPartial ||
		archive.Items[0].Status != ItemStatusRestored || archive.Items[1].Status != ItemStatusConflict {
		t.Fatalf("unexpected partial restore state: %#v", archive)
	}
}

func TestManagerRestoreRejectsTenantSnapshotMismatch(t *testing.T) {
	definition := entity.NewDefinition("demo", "DemoTenant", "demo_tenant").
		WithResource("demo.tenant").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary(),
			entity.NewField("tenantId", "tenantId", "bigint").Unsigned().Nullable(),
			entity.NewField("name", "name", "varchar").NotNull(),
		})
	catalog, err := NewCatalog([]entity.Definition{definition})
	if err != nil {
		t.Fatalf("compile tenant restore catalog failed: %v", err)
	}
	tenantID := int64(7)
	archive := &Archive{
		ID: 8, TenantID: &tenantID,
		Items: []*ArchiveItem{{
			ID: 21, RecycleID: 8, TenantID: &tenantID, Resource: definition.ResourceKey(), TableName: definition.TableName,
			Identity: Identity{Fields: []IdentityField{{JSONName: "id", ColumnName: "id", Value: json.RawMessage(`1`)}}},
			Data:     json.RawMessage(`{"id":1,"tenantId":9,"name":"wrong tenant"}`), BranchKey: "a", Status: ItemStatusPending,
		}},
	}
	db := &restoreStubDB{}
	manager, _ := NewManager(db, &restoreStubStore{archive: archive}, catalog, Options{Enabled: true})
	ctx := security.ContextWithUser(context.Background(), security.UserContext{UserId: 9, TenantId: security.PlatformTenant()})

	err = manager.Restore(ctx, archive.ID)
	if err == nil || !strings.Contains(err.Error(), "租户快照不一致") || db.insertCount != 0 {
		t.Fatalf("tenant snapshot mismatch must fail before insert: err=%v db=%#v", err, db)
	}
}
