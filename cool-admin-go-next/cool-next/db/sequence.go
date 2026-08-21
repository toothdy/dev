package db

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/driver"
)

// SyncSequence 把 table 自增主键 column 的序列推进到当前最大值，使后续自动分配的
// 主键不与已写入的显式主键冲突。
//
// 仅 PostgreSQL 需要：BIGSERIAL 背后是独立序列，写入显式主键不会推进它，种子数据
// 之类带显式 id 的写入之后，下一次自动分配会重新从 1 开始并撞上已有行。MySQL 的
// AUTO_INCREMENT 与 SQLite 的 AUTOINCREMENT 都会随显式写入自动抬高，因此直接返回。
//
// 表为空时不动序列，保持其初始状态。setval 本身不受事务回滚影响，在事务内调用只会
// 让序列多前进一段，不会造成主键冲突。
func (r *Runtime) SyncSequence(ctx context.Context, table, column string) error {
	if r == nil || r.database == nil {
		return exception.Core("框架数据库 Runtime 未初始化")
	}
	if r.dialect.Kind() != driver.PostgreSQL {
		return nil
	}
	if table == "" || column == "" {
		return exception.Core("同步自增序列缺少表名或列名")
	}
	quotedTable, err := r.dialect.Quote(table)
	if err != nil {
		return exception.WrapCore(err, "同步自增序列的表名无效")
	}
	quotedColumn, err := r.dialect.Quote(column)
	if err != nil {
		return exception.WrapCore(err, "同步自增序列的列名无效")
	}

	// pg_get_serial_sequence 对非序列列返回 NULL，此时无需（也无法）同步。
	sequence, err := r.database.GetValue(ctx, "SELECT pg_get_serial_sequence(?, ?)", table, column)
	if err != nil {
		return exception.WrapCore(err, "查询自增序列失败: "+table)
	}
	if sequence == nil || sequence.IsNil() || sequence.String() == "" {
		return nil
	}

	// 三参数 setval(seq, value, is_called) 一条语句覆盖两种情形：
	// 表非空时 value=MAX、is_called=true，下一次 nextval 返回 MAX+1；
	// 表为空时 value=1、is_called=false，下一次 nextval 仍返回 1。
	maximum := "(SELECT MAX(" + quotedColumn + ") FROM " + quotedTable + ")"
	statement := "SELECT setval(?::regclass, COALESCE(" + maximum + ", 1), " + maximum + " IS NOT NULL)"
	if _, err = r.database.Exec(ctx, statement, sequence.String()); err != nil {
		return exception.WrapCore(err, "同步自增序列失败: "+table)
	}

	return nil
}
