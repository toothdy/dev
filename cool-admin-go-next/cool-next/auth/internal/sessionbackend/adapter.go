package sessionbackend

import (
	"context"
	"errors"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessioncontract"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// Store 的认证端口适配器
type Adapter struct {
	store Store
}

// 创建认证端口适配器
func NewAdapter(store Store) (*Adapter, error) {
	if store == nil {
		return nil, exception.Core("Session Store 不能为空")
	}

	return &Adapter{store: store}, nil
}

// 读取 Session 快照
func (adapter *Adapter) Get(ctx context.Context, sessionID string) (sessioncontract.SessionSnapshot, bool, error) {
	if adapter == nil || adapter.store == nil {
		return sessioncontract.SessionSnapshot{}, false, exception.Core("Session Adapter 未初始化")
	}
	value, exists, err := adapter.store.Get(ctx, sessionID)
	if err != nil || !exists {
		return sessioncontract.SessionSnapshot{}, exists, err
	}

	return toSnapshot(value), true, nil
}

// 保存 Session 快照
func (adapter *Adapter) Save(ctx context.Context, snapshot sessioncontract.SessionSnapshot) error {
	if adapter == nil || adapter.store == nil {
		return exception.Core("Session Adapter 未初始化")
	}
	value, err := fromSnapshot(snapshot)
	if err != nil {
		return err
	}

	return adapter.store.Save(ctx, value)
}

// 原子轮换 Session 快照
func (adapter *Adapter) RotateRefresh(
	ctx context.Context,
	sessionID string,
	expectedRefreshJTI string,
	next sessioncontract.SessionSnapshot,
) error {
	if adapter == nil || adapter.store == nil {
		return exception.Core("Session Adapter 未初始化")
	}
	value, err := fromSnapshot(next)
	if err != nil {
		return err
	}
	err = adapter.store.RotateRefresh(ctx, sessionID, expectedRefreshJTI, value)
	switch {
	case errors.Is(err, ErrNotFound):
		return sessioncontract.ErrSessionNotFound
	case errors.Is(err, ErrRefreshReplay):
		return sessioncontract.ErrRefreshReplay
	default:
		return err
	}
}

// 撤销 Session
func (adapter *Adapter) Revoke(ctx context.Context, sessionID string) error {
	if adapter == nil || adapter.store == nil {
		return exception.Core("Session Adapter 未初始化")
	}

	return adapter.store.Revoke(ctx, sessionID)
}

// 按身份种类和用户 ID 撤销全部 Session
func (adapter *Adapter) RevokeUser(ctx context.Context, subject sessioncontract.Kind, userID uint64) error {
	return adapter.RevokeUsers(ctx, subject, []uint64{userID})
}

// 按身份种类批量撤销用户的全部 Session
func (adapter *Adapter) RevokeUsers(ctx context.Context, subject sessioncontract.Kind, userIDs []uint64) error {
	if adapter == nil || adapter.store == nil {
		return exception.Core("Session Adapter 未初始化")
	}

	return adapter.store.RevokeUsers(ctx, subject, userIDs)
}

// 转换为认证 Session 快照
func toSnapshot(value Session) sessioncontract.SessionSnapshot {
	return sessioncontract.SessionSnapshot{
		TokenSubject: sessioncontract.TokenSubject{
			SessionID: value.ID(),
			Subject:   value.Subject(),
			UserID:    value.UserID(),
			Username:  value.Username(),
			RoleIDs:   value.RoleIDs(),
			PasswordV: value.PasswordV(),
		},
		AccessJTI:  value.AccessJTI(),
		RefreshJTI: value.RefreshJTI(),
		ExpiresAt:  value.ExpiresAt(),
	}
}

// 转换并校验认证 Session 快照
func fromSnapshot(snapshot sessioncontract.SessionSnapshot) (Session, error) {
	switch snapshot.Subject {
	case sessioncontract.AdminKind:
		return NewAdmin(
			snapshot.SessionID,
			snapshot.UserID,
			snapshot.Username,
			snapshot.PasswordV,
			snapshot.RoleIDs,
			snapshot.AccessJTI,
			snapshot.RefreshJTI,
			snapshot.ExpiresAt,
		)
	case sessioncontract.AppKind:
		return NewApp(
			snapshot.SessionID,
			snapshot.UserID,
			snapshot.AccessJTI,
			snapshot.RefreshJTI,
			snapshot.ExpiresAt,
		)
	default:
		return Session{}, exception.Core("Session 快照身份种类无效")
	}
}
