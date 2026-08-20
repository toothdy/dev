package auth

import (
	"context"
	"slices"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
)

// Boundary 统一授权关系变更的数据库锁和 Session 撤销顺序：业务模块改角色/菜单/
// 部门/用户这类授权关系前，先按主键升序锁定目标行、校验其确实存在，改的是用户
// 本身时随后撤销其全部已签发 Session，防止旧 Token 在权限变更后继续生效。
type Boundary struct {
	runtime  *coredb.Runtime
	sessions SessionStore
}

// NewBoundary 创建授权变更边界。
func NewBoundary(runtime *coredb.Runtime, sessions SessionStore) (*Boundary, error) {
	if runtime == nil || runtime.Runner() == nil || sessions == nil {
		return nil, exception.Core("授权变更边界依赖无效")
	}

	return &Boundary{runtime: runtime, sessions: sessions}, nil
}

// LockTable 在当前框架事务内按主键升序锁定 table 中的 ids，并校验请求的记录全部
// 存在；message 用作失败时的错误文案前缀。ids 为空时直接返回 nil，不发起查询。
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

// LockUsersAndRevoke 锁定 table 中的 userIDs，成功后按 kind 批量撤销这些用户的
// 已签发 Session。用于"改了某人的授权关系，其旧登录必须失效"这类语义。
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

// RevokeUsers 按 kind 批量撤销 userIDs 的已签发 Session，不加锁。调用方已在别处
// 完成加锁（例如与其他授权变更共用一次锁定）时用这个跳过重复加锁。
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

// ValidateSnapshot 校验 before 与 after 归一化后的 ID 集合相同，用于锁定后重新
// 查询当前授权关系、确认加锁期间未被其他事务并发改动。
func ValidateSnapshot(before, after []uint64, message string) error {
	if !slices.Equal(NormalizeIDs(before), NormalizeIDs(after)) {
		return exception.Comm(message)
	}

	return nil
}

// NormalizeIDs 去零、去重并升序排列 ID，调用顺序一致可避免多事务交叉加锁产生死锁。
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
