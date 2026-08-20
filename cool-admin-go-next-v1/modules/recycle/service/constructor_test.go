package service

import (
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	recycleModule "github.com/toothdy/cool-admin-go-next/modules/recycle"
	recycleEvent "github.com/toothdy/cool-admin-go-next/modules/recycle/event"
)

var _ func(
	gdb.DB,
	*recycle.Manager,
	*recycleEvent.Store,
	entity.Definition,
	*Catalog,
	recycleModule.Config,
) (*DataService, error) = NewDataService
