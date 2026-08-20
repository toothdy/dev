package sys

import (
	"context"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/security"
)

func TestLogoutRevokesAccessAndRefreshSession(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager("logout-secret", 60, 3600)
	store := security.NewMemorySessionStore()
	service := &BaseSysLoginService{Manager: manager, Sessions: store}
	pair, err := manager.GenerateTokenPair(security.Claims{UserId: 17, Username: "tester", PasswordVersion: 4, TenantId: security.PlatformTenant()})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.saveSession(ctx, pair, false); err != nil {
		t.Fatal(err)
	}
	accessClaims, err := manager.ParseAccessToken(pair.Token)
	if err != nil {
		t.Fatal(err)
	}
	session, ok, err := store.Get(ctx, accessClaims.SessionID)
	if err != nil || !ok {
		t.Fatalf("expected saved session, ok=%v err=%v", ok, err)
	}
	if time.Until(session.RefreshTokenExpires) < 30*time.Minute {
		t.Fatalf("session should live through refresh window, expires at %s", session.RefreshTokenExpires)
	}

	if err = service.Logout(ctx, accessClaims.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = store.Get(ctx, accessClaims.SessionID); err != nil || ok {
		t.Fatalf("expected logout to delete session, ok=%v err=%v", ok, err)
	}
	if _, err = service.RefreshToken(ctx, pair.RefreshToken); err == nil || err.Error() != "登录失效~" {
		t.Fatalf("expected revoked refresh token rejected before database access, got %v", err)
	}
}
