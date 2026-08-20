package event

import (
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

var _ func(gdb.DB, entity.Definition, entity.Definition) (*Store, error) = NewStore
