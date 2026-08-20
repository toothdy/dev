package recycle

import (
	"context"
	"errors"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

type managerStubDB struct {
	gdb.DB
	rows             gdb.Result
	transactionCount int
	rollbackCount    int
	queryCount       int
}

func (d *managerStubDB) Transaction(ctx context.Context, callback func(context.Context, gdb.TX) error) error {
	d.transactionCount++
	err := callback(ctx, &managerStubTX{db: d})
	if err != nil {
		d.rollbackCount++
	}
	return err
}

type managerStubTX struct {
	gdb.TX
	db *managerStubDB
}

func (tx *managerStubTX) Ctx(context.Context) gdb.TX {
	return tx
}

func (tx *managerStubTX) GetAll(string, ...interface{}) (gdb.Result, error) {
	tx.db.queryCount++
	return tx.db.rows, nil
}

type managerStubStore struct {
	archive   *Archive
	saveCount int
	saveErr   error
}

func (s *managerStubStore) SaveArchive(_ context.Context, _ gdb.TX, archive *Archive) error {
	s.saveCount++
	s.archive = archive
	return s.saveErr
}

func (s *managerStubStore) LockArchive(context.Context, gdb.TX, int64, *int64) (*Archive, error) {
	return nil, nil
}

func (s *managerStubStore) SaveRestoreState(context.Context, gdb.TX, *Archive) error {
	return nil
}

func (s *managerStubStore) DeleteArchive(context.Context, gdb.TX, int64, *int64) error {
	return nil
}

func managerTestContext() context.Context {
	return security.ContextWithUser(context.Background(), security.UserContext{UserId: 9, TenantId: security.PlatformTenant()})
}

func managerTestRows() gdb.Result {
	return gdb.Result{
		gdb.Record{"id": gvar.New(uint64(1)), "name": gvar.New("root")},
	}
}

func TestManagerArchivesAndDeletesInOneTransaction(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, err := NewCatalog([]entity.Definition{definition})
	if err != nil {
		t.Fatalf("compile recycle catalog failed: %v", err)
	}
	db := &managerStubDB{rows: managerTestRows()}
	store := &managerStubStore{}
	manager, err := NewManager(db, store, catalog, Options{Enabled: true})
	if err != nil {
		t.Fatalf("create recycle manager failed: %v", err)
	}
	workCalled := false
	err = manager.RunDelete(managerTestContext(), DeleteRequest{
		Resource: "demo/type", Entity: "DemoTypeEntity", Model: definition, IDs: []interface{}{uint64(1)},
	}, func(_ context.Context, scope *DeleteScope) error {
		workCalled = true
		return scope.MarkDeleted(1)
	})
	if err != nil {
		t.Fatalf("run managed delete failed: %v", err)
	}
	if !workCalled || db.transactionCount != 1 || db.rollbackCount != 0 || db.queryCount != 1 || store.saveCount != 1 {
		t.Fatalf("unexpected managed delete lifecycle: db=%#v store=%#v", db, store)
	}
	if store.archive == nil || store.archive.Count != 1 || store.archive.EntityInfo.Resource != "demo.type" || store.archive.UserID == nil || *store.archive.UserID != 9 {
		t.Fatalf("unexpected archived delete metadata: %#v", store.archive)
	}
}

func TestManagerKeepsSuccessWhenAfterCommitActionFails(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, _ := NewCatalog([]entity.Definition{definition})
	db := &managerStubDB{rows: managerTestRows()}
	manager, _ := NewManager(db, &managerStubStore{}, catalog, Options{Enabled: true})
	actionCalled := false

	err := manager.RunDelete(managerTestContext(), DeleteRequest{
		Resource: "demo/type", Model: definition, IDs: []interface{}{uint64(1)},
	}, func(_ context.Context, scope *DeleteScope) error {
		if err := scope.AfterCommit(func(context.Context) error {
			actionCalled = true
			return errors.New("scheduler unavailable")
		}); err != nil {
			return err
		}
		return scope.MarkDeleted(1)
	})
	if err != nil || !actionCalled || db.rollbackCount != 0 {
		t.Fatalf("after-commit failure must not turn a committed delete into failure: err=%v called=%v db=%#v", err, actionCalled, db)
	}
}

func TestManagerRollsBackDeleteWhenArchiveWriteFails(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, _ := NewCatalog([]entity.Definition{definition})
	db := &managerStubDB{rows: managerTestRows()}
	store := &managerStubStore{saveErr: errors.New("archive unavailable")}
	manager, _ := NewManager(db, store, catalog, Options{Enabled: true})

	err := manager.RunDelete(managerTestContext(), DeleteRequest{
		Resource: "demo/type", Model: definition, IDs: []interface{}{uint64(1)},
	}, func(_ context.Context, scope *DeleteScope) error {
		return scope.MarkDeleted(1)
	})
	if err == nil || db.rollbackCount != 1 || store.saveCount != 1 {
		t.Fatalf("archive failure must roll back managed delete: err=%v db=%#v store=%#v", err, db, store)
	}
}

func TestManagerBypassAndDisabledSkipArchiveButKeepTransaction(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, _ := NewCatalog([]entity.Definition{definition})
	for _, test := range []struct {
		name    string
		enabled bool
		ctx     context.Context
	}{
		{name: "bypass", enabled: true, ctx: WithBypass(context.Background())},
		{name: "disabled", enabled: false, ctx: context.Background()},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &managerStubDB{rows: managerTestRows()}
			store := &managerStubStore{}
			manager, _ := NewManager(db, store, catalog, Options{Enabled: test.enabled})
			wasArchiving := true
			err := manager.RunDelete(test.ctx, DeleteRequest{
				Resource: "demo/type", Model: definition, IDs: []interface{}{uint64(1)},
			}, func(_ context.Context, scope *DeleteScope) error {
				wasArchiving = scope.IsArchiving()
				return nil
			})
			if err != nil {
				t.Fatalf("run direct delete failed: %v", err)
			}
			if wasArchiving || db.transactionCount != 1 || db.queryCount != 0 || store.saveCount != 0 {
				t.Fatalf("direct delete unexpectedly archived: db=%#v store=%#v", db, store)
			}
		})
	}
}

func TestManagerFailsClosedWithoutOperator(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, _ := NewCatalog([]entity.Definition{definition})
	db := &managerStubDB{rows: managerTestRows()}
	manager, _ := NewManager(db, &managerStubStore{}, catalog, Options{Enabled: true})

	err := manager.RunDelete(context.Background(), DeleteRequest{
		Resource: "demo/type", Model: definition, IDs: []interface{}{uint64(1)},
	}, func(context.Context, *DeleteScope) error {
		return nil
	})
	if err == nil || db.transactionCount != 0 {
		t.Fatalf("missing operator must fail before transaction: err=%v db=%#v", err, db)
	}
}

func TestManagerRejectsArchiveAndDeleteCountMismatch(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, _ := NewCatalog([]entity.Definition{definition})
	db := &managerStubDB{rows: managerTestRows()}
	store := &managerStubStore{}
	manager, _ := NewManager(db, store, catalog, Options{Enabled: true})

	err := manager.RunDelete(managerTestContext(), DeleteRequest{
		Resource: "demo/type", Model: definition, IDs: []interface{}{uint64(1)},
	}, func(_ context.Context, scope *DeleteScope) error {
		return scope.MarkDeleted(0)
	})
	if err == nil || db.rollbackCount != 1 || store.saveCount != 0 {
		t.Fatalf("delete count mismatch must roll back before archive write: err=%v db=%#v store=%#v", err, db, store)
	}
}
