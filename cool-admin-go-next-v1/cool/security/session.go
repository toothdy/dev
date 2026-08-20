package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

// 不保存 token 明文的 admin 登录会话
type Session struct {
	ID                  string
	UserID              int64
	AccessJTIHash       string
	RefreshJTIHash      string
	PasswordVersion     int64
	RefreshTokenExpires time.Time
}

// 可替换的 admin 会话存储
type SessionStore interface {
	Save(ctx context.Context, session Session) error
	ReplaceUser(ctx context.Context, userID int64, session Session) error
	Get(ctx context.Context, sessionID string) (Session, bool, error)
	Rotate(ctx context.Context, sessionID string, oldRefreshJTIHash string, next Session) (bool, error)
	Delete(ctx context.Context, sessionID string) error
	DeleteUser(ctx context.Context, userID int64) error
	DeleteUsers(ctx context.Context, userIDs []int64) error
}

// 返回 JTI 的不可逆存储值
func HashTokenID(jti string) string {
	sum := sha256.Sum256([]byte(jti))
	return hex.EncodeToString(sum[:])
}

// 支持多会话和原子 refresh 轮换的进程内存储
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
	users    map[int64]map[string]struct{}
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: map[string]Session{},
		users:    map[int64]map[string]struct{}{},
	}
}

func (s *MemorySessionStore) Save(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveLocked(session)
	return nil
}

func (s *MemorySessionStore) ReplaceUser(_ context.Context, userID int64, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteUserLocked(userID)
	s.saveLocked(session)
	return nil
}

func (s *MemorySessionStore) Get(_ context.Context, sessionID string) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, false, nil
	}
	if !session.RefreshTokenExpires.IsZero() && !session.RefreshTokenExpires.After(time.Now()) {
		s.deleteLocked(sessionID)
		return Session{}, false, nil
	}
	return session, true, nil
}

func (s *MemorySessionStore) Rotate(_ context.Context, sessionID string, oldRefreshJTIHash string, next Session) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionID]
	if !ok || current.RefreshJTIHash != oldRefreshJTIHash {
		return false, nil
	}
	if !current.RefreshTokenExpires.IsZero() && !current.RefreshTokenExpires.After(time.Now()) {
		s.deleteLocked(sessionID)
		return false, nil
	}
	if next.ID != sessionID || next.UserID != current.UserID {
		return false, nil
	}
	s.saveLocked(next)
	return true, nil
}

func (s *MemorySessionStore) Delete(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(sessionID)
	return nil
}

func (s *MemorySessionStore) DeleteUser(ctx context.Context, userID int64) error {
	return s.DeleteUsers(ctx, []int64{userID})
}

func (s *MemorySessionStore) DeleteUsers(_ context.Context, userIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, userID := range normalizeUserIDs(userIDs) {
		s.deleteUserLocked(userID)
	}
	return nil
}

func (s *MemorySessionStore) saveLocked(session Session) {
	if previous, ok := s.sessions[session.ID]; ok && previous.UserID != session.UserID {
		s.removeUserIndexLocked(previous.UserID, session.ID)
	}
	s.sessions[session.ID] = session
	if s.users[session.UserID] == nil {
		s.users[session.UserID] = map[string]struct{}{}
	}
	s.users[session.UserID][session.ID] = struct{}{}
}

func (s *MemorySessionStore) deleteLocked(sessionID string) {
	session, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	delete(s.sessions, sessionID)
	s.removeUserIndexLocked(session.UserID, sessionID)
}

func (s *MemorySessionStore) deleteUserLocked(userID int64) {
	for sessionID := range s.users[userID] {
		delete(s.sessions, sessionID)
	}
	delete(s.users, userID)
}

func (s *MemorySessionStore) removeUserIndexLocked(userID int64, sessionID string) {
	delete(s.users[userID], sessionID)
	if len(s.users[userID]) == 0 {
		delete(s.users, userID)
	}
}

func normalizeUserIDs(userIDs []int64) []int64 {
	unique := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID > 0 {
			unique[userID] = struct{}{}
		}
	}
	normalized := make([]int64, 0, len(unique))
	for userID := range unique {
		normalized = append(normalized, userID)
	}
	sort.Slice(normalized, func(left int, right int) bool {
		return normalized[left] < normalized[right]
	})
	return normalized
}
