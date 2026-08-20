package sys

import (
	"context"
	"database/sql"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
)

type managedDeleteWork func(context.Context, gdb.TX, *recycle.DeleteScope) error

// runManagedDelete 统一复用回收站事务；未注入 Manager 时保持原物理删除行为。
func runManagedDelete(
	ctx context.Context,
	db gdb.DB,
	manager *recycle.Manager,
	definition entity.Definition,
	ids []interface{},
	params interface{},
	work managedDeleteWork,
) error {
	if manager == nil {
		return db.Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
			return work(txCtx, tx, nil)
		})
	}
	return manager.RunDelete(ctx, recycle.DeleteRequest{
		Resource: definition.ResourceKey(),
		Entity:   definition.Name,
		Model:    definition,
		IDs:      ids,
		Params:   params,
	}, func(txCtx context.Context, scope *recycle.DeleteScope) error {
		return work(txCtx, scope.TX(), scope)
	})
}

func markManagedDeleted(scope *recycle.DeleteScope, result sql.Result, message string) error {
	if scope == nil {
		return nil
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, message)
	}
	return scope.MarkDeleted(affected)
}
