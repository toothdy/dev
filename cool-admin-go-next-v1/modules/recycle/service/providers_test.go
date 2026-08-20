package service

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	recycleEntity "github.com/toothdy/cool-admin-go-next/modules/recycle/entity"
	recycleEvent "github.com/toothdy/cool-admin-go-next/modules/recycle/event"
)

var (
	_ func([]entity.Definition) (*Catalog, error)                                                  = NewCatalog
	_ func(gdb.DB, *recycleEvent.Store, *Catalog, module.CRUDOptions) (*recycle.Manager, error) = NewManager
)

func TestNewManagerUsesCRUDOptions(t *testing.T) {
	db, err := gdb.New(gdb.ConfigNode{
		Type: "mysql", Host: "127.0.0.1", Port: "3306", User: "test", Pass: "test", Name: "test", DryRun: true,
	})
	if err != nil {
		t.Fatalf("create recycle provider test database failed: %v", err)
	}
	defer db.Close(context.Background())
	store, err := recycleEvent.NewStore(db, recycleEntity.Data(), recycleEntity.Item())
	if err != nil {
		t.Fatalf("create recycle store failed: %v", err)
	}
	catalog, err := NewCatalog([]entity.Definition{recycleEntity.Data(), recycleEntity.Item()})
	if err != nil {
		t.Fatalf("create recycle catalog failed: %v", err)
	}
	manager, err := NewManager(db, store, catalog, module.CRUDOptions{SoftDelete: true})
	if err != nil {
		t.Fatalf("create recycle manager failed: %v", err)
	}
	if !manager.Enabled() {
		t.Fatal("recycle manager must use application CRUD options")
	}
}
