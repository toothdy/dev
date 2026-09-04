package auth

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
)

func TestRedisStoreUsesVersionedUserHashAndKeepsLatestExpiry(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	backend := newRedisBackendStub(func() time.Time { return now })
	store := newRedisStoreForTest(t, backend, func() time.Time { return now })
	backend.strings["test:legacy"] = "legacy-session"

	late := redisSnapshot("late", AdminKind, 7, now.Add(2*time.Hour))
	early := redisSnapshot("early", AdminKind, 7, now.Add(time.Hour))
	if err := store.Save(t.Context(), late); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), early); err != nil {
		t.Fatal(err)
	}

	if store.prefix != "test:v2:" {
		t.Fatalf("prefix = %q", store.prefix)
	}
	if backend.strings["test:v2:session:late"] != "admin:7" || backend.strings["test:v2:session:early"] != "admin:7" {
		t.Fatalf("locator keys = %#v", backend.strings)
	}
	userKey := "test:v2:user:admin:7"
	if len(backend.hashes[userKey]) != 2 {
		t.Fatalf("user hash = %#v", backend.hashes[userKey])
	}
	if backend.expires[userKey] != late.ExpiresAt.UnixMilli() {
		t.Fatalf("user hash expiry = %d, want %d", backend.expires[userKey], late.ExpiresAt.UnixMilli())
	}

	value, exists, err := store.Get(t.Context(), early.SessionID)
	if err != nil || !exists || value.SessionID != early.SessionID {
		t.Fatalf("Get() = %#v, %t, %v", value, exists, err)
	}
	if _, exists, err = store.Get(t.Context(), "legacy"); err != nil || exists {
		t.Fatalf("legacy Get() exists = %t, err = %v", exists, err)
	}
	if backend.strings["test:legacy"] != "legacy-session" {
		t.Fatal("legacy Session should remain for its original TTL")
	}
}

func TestRedisStoreRotatesAndRevokesSession(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	backend := newRedisBackendStub(func() time.Time { return now })
	store := newRedisStoreForTest(t, backend, func() time.Time { return now })
	original := redisSnapshot("session", AdminKind, 7, now.Add(time.Hour))
	if err := store.Save(t.Context(), original); err != nil {
		t.Fatal(err)
	}

	next := original
	next.AccessJTI = "access-next"
	next.RefreshJTI = "refresh-next"
	next.ExpiresAt = now.Add(2 * time.Hour)
	if err := store.RotateRefresh(t.Context(), original.SessionID, original.RefreshJTI, next); err != nil {
		t.Fatal(err)
	}
	if err := store.RotateRefresh(t.Context(), original.SessionID, original.RefreshJTI, next); !errors.Is(err, ErrRefreshReplay) {
		t.Fatalf("replayed RotateRefresh() error = %v", err)
	}
	value, exists, err := store.Get(t.Context(), original.SessionID)
	if err != nil || !exists || value.RefreshJTI != next.RefreshJTI {
		t.Fatalf("rotated Get() = %#v, %t, %v", value, exists, err)
	}

	if err = store.Revoke(t.Context(), original.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, exists, err = store.Get(t.Context(), original.SessionID); err != nil || exists {
		t.Fatalf("revoked Get() exists = %t, err = %v", exists, err)
	}
	if _, exists = backend.strings["test:v2:session:session"]; exists {
		t.Fatal("revoked locator still exists")
	}
	if _, exists = backend.hashes["test:v2:user:admin:7"]; exists {
		t.Fatal("revoked user hash still exists")
	}
}

func TestRedisStoreRevokesOnlyTargetUsersWithoutScanning(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	backend := newRedisBackendStub(func() time.Time { return now })
	store := newRedisStoreForTest(t, backend, func() time.Time { return now })
	snapshots := []Snapshot{
		redisSnapshot("admin-1-a", AdminKind, 1, now.Add(time.Hour)),
		redisSnapshot("admin-1-b", AdminKind, 1, now.Add(2*time.Hour)),
		redisSnapshot("admin-2", AdminKind, 2, now.Add(time.Hour)),
		redisSnapshot("app-1", AppKind, 1, now.Add(time.Hour)),
	}
	for _, snapshot := range snapshots {
		if err := store.Save(t.Context(), snapshot); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.RevokeUsers(t.Context(), AdminKind, []uint64{1, 1}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.unlinkCalls, [][]string{{"test:v2:user:admin:1"}}) {
		t.Fatalf("UNLINK calls = %#v", backend.unlinkCalls)
	}
	for _, sessionID := range []string{"admin-1-a", "admin-1-b"} {
		if _, exists, err := store.Get(t.Context(), sessionID); err != nil || exists {
			t.Fatalf("target Get(%q) exists = %t, err = %v", sessionID, exists, err)
		}
		if _, exists := backend.strings[store.sessionKey(sessionID)]; exists {
			t.Fatalf("stale locator %q was not cleaned", sessionID)
		}
	}
	for _, sessionID := range []string{"admin-2", "app-1"} {
		if _, exists, err := store.Get(t.Context(), sessionID); err != nil || !exists {
			t.Fatalf("unrelated Get(%q) exists = %t, err = %v", sessionID, exists, err)
		}
	}
}

func TestRedisStoreRejectsSessionIDOwnedByAnotherUser(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	backend := newRedisBackendStub(func() time.Time { return now })
	store := newRedisStoreForTest(t, backend, func() time.Time { return now })
	if err := store.Save(t.Context(), redisSnapshot("same", AdminKind, 1, now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), redisSnapshot("same", AdminKind, 2, now.Add(time.Hour))); err == nil {
		t.Fatal("Save() error = nil")
	}
}

func newRedisStoreForTest(t *testing.T, backend redisBackend, now func() time.Time) *RedisStore {
	t.Helper()
	store, err := newRedisStore(t.Context(), backend, "test:", now)
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func redisSnapshot(sessionID string, subject Kind, userID uint64, expiresAt time.Time) Snapshot {
	value := Snapshot{
		TokenSubject: TokenSubject{SessionID: sessionID, Subject: subject, UserID: userID},
		AccessJTI:    "access-" + sessionID,
		RefreshJTI:   "refresh-" + sessionID,
		ExpiresAt:    expiresAt,
	}
	if subject == AdminKind {
		value.Username = "admin"
		value.RoleIDs = []uint64{1}
		value.PasswordV = 1
	}

	return value
}

type redisBackendStub struct {
	mutex       sync.Mutex
	strings     map[string]string
	hashes      map[string]map[string]string
	expires     map[string]int64
	unlinkCalls [][]string
	now         func() time.Time
}

func newRedisBackendStub(now func() time.Time) *redisBackendStub {
	return &redisBackendStub{
		strings: make(map[string]string),
		hashes:  make(map[string]map[string]string),
		expires: make(map[string]int64),
		now:     now,
	}
}

func (backend *redisBackendStub) Do(_ context.Context, command string, _ ...any) (*gvar.Var, error) {
	if command == "PING" {
		return gvar.New("PONG"), nil
	}

	return nil, errors.New("unexpected Redis command: " + command)
}

func (backend *redisBackendStub) Get(_ context.Context, key string) (*gvar.Var, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	backend.expire(key)
	value, exists := backend.strings[key]
	if !exists {
		return gvar.New(nil), nil
	}

	return gvar.New(value), nil
}

func (backend *redisBackendStub) HGet(_ context.Context, key, field string) (*gvar.Var, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	backend.expire(key)
	value, exists := backend.hashes[key][field]
	if !exists {
		return gvar.New(nil), nil
	}

	return gvar.New(value), nil
}

func (backend *redisBackendStub) Eval(
	_ context.Context,
	script string,
	_ int64,
	keys []string,
	args []any,
) (*gvar.Var, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	for _, key := range keys {
		backend.expire(key)
	}

	switch script {
	case saveSessionScript:
		return backend.save(keys, args), nil
	case rotateRefreshScript:
		return backend.rotate(keys, args), nil
	case revokeSessionScript:
		return backend.revoke(keys, args), nil
	case deleteSessionIfUnchangedScript:
		return backend.deleteIfUnchanged(keys, args), nil
	case deleteLocatorIfUnchangedScript:
		return backend.deleteLocatorIfUnchanged(keys, args), nil
	default:
		return nil, errors.New("unexpected Redis script")
	}
}

func (backend *redisBackendStub) Unlink(_ context.Context, keys ...string) (int64, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	backend.unlinkCalls = append(backend.unlinkCalls, append([]string(nil), keys...))
	var removed int64
	for _, key := range keys {
		if _, exists := backend.strings[key]; exists {
			removed++
		}
		if _, exists := backend.hashes[key]; exists {
			removed++
		}
		backend.delete(key)
	}

	return removed, nil
}

func (backend *redisBackendStub) save(keys []string, args []any) *gvar.Var {
	locator := redisString(args[0])
	if current, exists := backend.strings[keys[0]]; exists && current != locator {
		return gvar.New(0)
	}
	backend.strings[keys[0]] = locator
	backend.expires[keys[0]] = redisInt64(args[3])
	backend.setHash(keys[1], redisString(args[1]), redisString(args[2]), redisInt64(args[3]))

	return gvar.New(1)
}

func (backend *redisBackendStub) rotate(keys []string, args []any) *gvar.Var {
	if backend.strings[keys[0]] != redisString(args[0]) {
		return gvar.New(0)
	}
	field := redisString(args[1])
	current, exists := backend.hashes[keys[1]][field]
	if !exists {
		return gvar.New(0)
	}
	if current != redisString(args[2]) {
		return gvar.New(2)
	}
	backend.strings[keys[0]] = redisString(args[0])
	backend.expires[keys[0]] = redisInt64(args[4])
	backend.setHash(keys[1], field, redisString(args[3]), redisInt64(args[4]))

	return gvar.New(1)
}

func (backend *redisBackendStub) revoke(keys []string, args []any) *gvar.Var {
	if backend.strings[keys[0]] != redisString(args[0]) {
		return gvar.New(0)
	}
	backend.deleteHashField(keys[1], redisString(args[1]))
	backend.delete(keys[0])

	return gvar.New(1)
}

func (backend *redisBackendStub) deleteIfUnchanged(keys []string, args []any) *gvar.Var {
	field := redisString(args[1])
	if backend.hashes[keys[1]][field] != redisString(args[2]) {
		return gvar.New(0)
	}
	if backend.strings[keys[0]] == redisString(args[0]) {
		backend.delete(keys[0])
	}
	backend.deleteHashField(keys[1], field)

	return gvar.New(1)
}

func (backend *redisBackendStub) deleteLocatorIfUnchanged(keys []string, args []any) *gvar.Var {
	if backend.strings[keys[0]] != redisString(args[0]) {
		return gvar.New(0)
	}
	backend.delete(keys[0])

	return gvar.New(1)
}

func (backend *redisBackendStub) setHash(key, field, value string, expiresAt int64) {
	if backend.hashes[key] == nil {
		backend.hashes[key] = make(map[string]string)
	}
	backend.hashes[key][field] = value
	if backend.expires[key] < expiresAt {
		backend.expires[key] = expiresAt
	}
}

func (backend *redisBackendStub) deleteHashField(key, field string) {
	delete(backend.hashes[key], field)
	if len(backend.hashes[key]) == 0 {
		backend.delete(key)
	}
}

func (backend *redisBackendStub) expire(key string) {
	if expiresAt := backend.expires[key]; expiresAt > 0 && expiresAt <= backend.now().UnixMilli() {
		backend.delete(key)
	}
}

func (backend *redisBackendStub) delete(key string) {
	delete(backend.strings, key)
	delete(backend.hashes, key)
	delete(backend.expires, key)
}

func redisString(value any) string {
	switch converted := value.(type) {
	case string:
		return converted
	case []byte:
		return string(converted)
	default:
		return gvar.New(value).String()
	}
}

func redisInt64(value any) int64 {
	return gvar.New(value).Int64()
}

var _ redisBackend = (*redisBackendStub)(nil)
