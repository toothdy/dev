package db

import (
	"context"
	"slices"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/driver"
)

// 行锁回读列
type lockedRow struct {
	ID uint64 `orm:"id"`
}

// 空更新载荷：把主键写回自身，不触碰任何业务列
type writeLockData struct {
	ID any `orm:"id"`
}

// LockRows 在当前框架事务内按主键升序锁定目标表记录，返回实际锁定的 ID。
//
// 调用方负责比对返回值与请求 ID 以判定记录是否存在，本方法只保证加锁与回读。
// ID 会去零、去重并升序排列，调用顺序一致可避免多事务交叉加锁产生死锁。
//
// MySQL 与 PostgreSQL 使用 SELECT ... FOR UPDATE。SQLite 没有行级锁，
// 改为先执行不触碰业务列的空更新把事务提升为写事务，再回读确认；
// 该空更新走 Unscoped，避免框架的 updateTime 自动写入把加锁变成数据变更。
func (r *Runtime) LockRows(ctx context.Context, table string, ids []uint64) ([]uint64, error) {
	if r == nil || r.database == nil {
		return nil, exception.Core("框架数据库 Runtime 未初始化")
	}
	if table == "" {
		return nil, exception.Core("行锁目标表不能为空")
	}
	ids = normalizeLockIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	transaction, exists, err := r.Current(ctx)
	if err != nil {
		return nil, err
	}
	if !exists || transaction == nil {
		return nil, exception.Core("当前上下文不存在框架事务")
	}
	model := func() *gdb.Model { return transaction.Model(table).Ctx(ctx) }
	if r.dialect.Kind() == driver.SQLite {
		if _, err = model().Unscoped().
			Data(writeLockData{ID: gdb.Raw("id")}).
			WhereIn("id", ids).
			Update(); err != nil {
			return nil, exception.WrapCore(err, "提升 SQLite 写锁失败")
		}
	}
	read := model().Fields("id").WhereIn("id", ids).OrderAsc("id")
	if r.dialect.Kind() != driver.SQLite {
		read = read.LockUpdate()
	}
	var rows []lockedRow
	if err = read.Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "锁定目标记录失败")
	}
	locked := make([]uint64, len(rows))
	for index, row := range rows {
		locked[index] = row.ID
	}

	return normalizeLockIDs(locked), nil
}

// 去零、去重并升序排列行锁 ID
func normalizeLockIDs(ids []uint64) []uint64 {
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			result = append(result, id)
		}
	}
	slices.Sort(result)

	return slices.Compact(result)
}
