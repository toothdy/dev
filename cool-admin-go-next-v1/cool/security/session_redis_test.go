package security

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
)

type redisCall struct {
	command string
	args    []any
}

type redisCommanderStub struct {
	calls  []redisCall
	result *gvar.Var
	err    error
}

func (s *redisCommanderStub) Do(_ context.Context, command string, args ...any) (*gvar.Var, error) {
	s.calls = append(s.calls, redisCall{command: command, args: append([]any(nil), args...)})
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return gvar.New(1), nil
}

func TestRedisSessionStorePayloadOmitsAuthorizationClaims(t *testing.T) {
	payload, err := json.Marshal(storedFromSession(Session{UserID: 7}, "generation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"username", "roleIds", "tenantId"} {
		if strings.Contains(string(payload), `"`+field+`"`) {
			t.Fatalf("session payload should not contain %s: %s", field, payload)
		}
	}
}

func TestRedisSessionStoreDeleteUsersUsesSingleAtomicCommand(t *testing.T) {
	client := &redisCommanderStub{}
	store, err := NewRedisSessionStore(client, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteUsers(context.Background(), []int64{3, 2, 3, 0}); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("expected one Redis command, got %#v", client.calls)
	}
	call := client.calls[0]
	if call.command != "EVAL" {
		t.Fatalf("expected atomic EVAL, got %s", call.command)
	}
	if len(call.args) != 6 || call.args[1] != 2 ||
		call.args[2] != "test:user:2:generation" ||
		call.args[3] != "test:user:3:generation" {
		t.Fatalf("unexpected normalized batch args: %#v", call.args)
	}
	if err = store.DeleteUsers(context.Background(), nil); err != nil {
		t.Fatalf("empty delete should succeed: %v", err)
	}
	if len(client.calls) != 1 {
		t.Fatal("empty delete should not call Redis")
	}
}

func TestRedisSessionStoreDeleteUsersFailsClosed(t *testing.T) {
	wantErr := errors.New("redis unavailable")
	client := &redisCommanderStub{err: wantErr}
	store, err := NewRedisSessionStore(client, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteUsers(context.Background(), []int64{1, 2}); !errors.Is(err, wantErr) {
		t.Fatalf("expected Redis error, got %v", err)
	}
}

func TestRedisSessionStoreRotateChecksCurrentGeneration(t *testing.T) {
	client := &redisCommanderStub{}
	store, err := NewRedisSessionStore(client, "test")
	if err != nil {
		t.Fatal(err)
	}
	next := Session{
		ID:                  "session-id",
		UserID:              7,
		RefreshTokenExpires: time.Now().Add(time.Hour),
	}
	rotated, err := store.Rotate(context.Background(), next.ID, "old-refresh-hash", next)
	if err != nil || !rotated {
		t.Fatalf("expected successful Redis rotate command, rotated=%v err=%v", rotated, err)
	}
	if len(client.calls) != 1 {
		t.Fatalf("expected one Redis command, got %#v", client.calls)
	}
	call := client.calls[0]
	if call.command != "EVAL" || len(call.args) < 5 || call.args[1] != 2 ||
		call.args[2] != "test:session:session-id" ||
		call.args[3] != "test:user:7:generation" {
		t.Fatalf("Redis rotate must atomically read session and generation keys: %#v", call.args)
	}
	if !strings.Contains(call.args[0].(string), "current.generation ~= generation") {
		t.Fatalf("Redis rotate script does not reject a revoked generation: %s", call.args[0])
	}
}
