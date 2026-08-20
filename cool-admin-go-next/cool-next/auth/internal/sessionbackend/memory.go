package sessionbackend

import (
	"context"
	"sync"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessioncontract"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// 并发安全的进程内 Session Store
type MemoryStore struct {
	mutex    sync.Mutex
	sessions map[string]Session
	now      func() time.Time
}

// 创建进程内 Session Store
func NewMemory() *MemoryStore {
	return newMemory(time.Now)
}

// 创建使用指定时钟的进程内 Session Store
func newMemory(now func() time.Time) *MemoryStore {
	return &MemoryStore{sessions: make(map[string]Session), now: now}
}

// 读取有效 Session
func (store *MemoryStore) Get(ctx context.Context, sessionID string) (Session, bool, error) {
	if store == nil {
		return Session{}, false, exception.Core("Memory Session Store 未初始化")
	}
	if err := contextError(ctx); err != nil {
		return Session{}, false, err
	}
	if !validIdentifier(sessionID) {
		return Session{}, false, exception.Core("Session ID 无效")
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cleanupExpired()
	value, exists := store.sessions[sessionID]
	return value.clone(), exists, nil
}

// 保存有效 Session
func (store *MemoryStore) Save(ctx context.Context, value Session) error {
	if store == nil {
		return exception.Core("Memory Session Store 未初始化")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validate(value, store.now()); err != nil {
		return err
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cleanupExpired()
	store.sessions[value.sessionID] = value.clone()
	return nil
}

// 原子轮换 Access 与 Refresh JTI
func (store *MemoryStore) RotateRefresh(
	ctx context.Context,
	sessionID string,
	expectedRefreshJTI string,
	next Session,
) error {
	if store == nil {
		return exception.Core("Memory Session Store 未初始化")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if !validIdentifier(sessionID) || !validIdentifier(expectedRefreshJTI) {
		return exception.Core("Session 轮换参数无效")
	}
	if err := validate(next, store.now()); err != nil {
		return err
	}
	if next.sessionID != sessionID {
		return exception.Core("轮换后的 Session ID 不一致")
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cleanupExpired()
	current, exists := store.sessions[sessionID]
	if !exists {
		return ErrNotFound
	}
	if current.refreshJTI != expectedRefreshJTI {
		return ErrRefreshReplay
	}
	if current.subject != next.subject || current.userID != next.userID {
		return exception.Core("轮换不能改变 Session 身份")
	}
	store.sessions[sessionID] = next.clone()
	return nil
}

// 撤销指定 Session
func (store *MemoryStore) Revoke(ctx context.Context, sessionID string) error {
	if store == nil {
		return exception.Core("Memory Session Store 未初始化")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if !validIdentifier(sessionID) {
		return exception.Core("Session ID 无效")
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.sessions, sessionID)
	return nil
}

// 按身份种类和用户 ID 撤销 Session
func (store *MemoryStore) RevokeUser(ctx context.Context, subject sessioncontract.Kind, userID uint64) error {
	return store.RevokeUsers(ctx, subject, []uint64{userID})
}

// 按身份种类批量撤销用户 Session
func (store *MemoryStore) RevokeUsers(ctx context.Context, subject sessioncontract.Kind, userIDs []uint64) error {
	if store == nil {
		return exception.Core("Memory Session Store 未初始化")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	targets, err := revokeUserSet(subject, userIDs)
	if err != nil || len(targets) == 0 {
		return err
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cleanupExpired()
	for sessionID, value := range store.sessions {
		if _, exists := targets[value.userID]; value.subject == subject && exists {
			delete(store.sessions, sessionID)
		}
	}

	return nil
}

// 清理过期 Session
func (store *MemoryStore) cleanupExpired() {
	now := store.now()
	for sessionID, value := range store.sessions {
		if !value.expiresAt.After(now) {
			delete(store.sessions, sessionID)
		}
	}
}

// 返回 Context 取消原因
func contextError(ctx context.Context) error {
	if ctx == nil {
		return exception.Core("Session Context 不能为空")
	}

	return ctx.Err()
}
