package auth

import (
	"context"
	"slices"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
)

// 授权关系变更的统一入口：先锁目标行再写入，必要时撤销 Session
type Boundary struct {
	runtime  *coredb.Runtime
	sessions SessionStore
}

// 授权变更边界
func NewBoundary(runtime *coredb.Runtime, sessions SessionStore) (*Boundary, error) {
	if runtime == nil || runtime.Runner() == nil || sessions == nil {
		return nil, exception.Core("授权变更边界依赖无效")
	}

	return &Boundary{runtime: runtime, sessions: sessions}, nil
}

// 授权变更使用的行锁与存在性校验
func (boundary *Boundary) LockTable(ctx context.Context, table string, ids []uint64, message string) error {
	if boundary == nil || boundary.runtime == nil {
		return exception.Core("授权变更边界未初始化")
	}
	ids = NormalizeIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	locked, err := boundary.runtime.LockRows(ctx, table, ids)
	if err != nil {
		return exception.WrapCore(err, message)
	}
	if !slices.Equal(ids, locked) {
		return exception.Validate(message + ": 目标记录不存在")
	}

	return nil
}

// 授权变更后让目标用户的旧 Token 失效
func (boundary *Boundary) LockUsersAndRevoke(ctx context.Context, table string, userIDs []uint64, kind Kind, message string) error {
	if boundary == nil || boundary.sessions == nil {
		return exception.Core("授权变更边界未初始化")
	}
	userIDs = NormalizeIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}
	if err := boundary.LockTable(ctx, table, userIDs, message); err != nil {
		return err
	}
	if err := boundary.sessions.RevokeUsers(ctx, kind, userIDs); err != nil {
		return exception.WrapCore(err, "撤销用户 Session 失败")
	}

	return nil
}

// 调用方已加锁时跳过重复加锁的 Session 撤销
func (boundary *Boundary) RevokeUsers(ctx context.Context, kind Kind, userIDs []uint64) error {
	if boundary == nil || boundary.sessions == nil {
		return exception.Core("授权变更边界未初始化")
	}
	userIDs = NormalizeIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}
	if err := boundary.sessions.RevokeUsers(ctx, kind, userIDs); err != nil {
		return exception.WrapCore(err, "撤销用户 Session 失败")
	}

	return nil
}

// 授权变更提交前的并发一致性校验
func ValidateSnapshot(before, after []uint64, message string) error {
	if !slices.Equal(NormalizeIDs(before), NormalizeIDs(after)) {
		return exception.Comm(message)
	}

	return nil
}

// 为交叉加锁准备一致的 ID 顺序
func NormalizeIDs(ids []uint64) []uint64 {
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			result = append(result, id)
		}
	}
	slices.Sort(result)

	return slices.Compact(result)
}
