package security_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

func TestMemorySessionStoreExpiresRotatesAndDeletesUserSessions(t *testing.T) {
	store := security.NewMemorySessionStore()
	ctx := context.Background()
	expired := security.Session{ID: "expired", UserID: 1, RefreshTokenExpires: time.Now().Add(-time.Second)}
	if err := store.Save(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Get(ctx, expired.ID); err != nil || ok {
		t.Fatalf("expected expired session removed, ok=%v err=%v", ok, err)
	}

	session := security.Session{
		ID: "sid-1", UserID: 1, RefreshJTIHash: security.HashTokenID("old"),
		RefreshTokenExpires: time.Now().Add(time.Hour),
	}
	if err := store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	next := session
	next.RefreshJTIHash = security.HashTokenID("next")
	ok, err := store.Rotate(ctx, session.ID, security.HashTokenID("old"), next)
	if err != nil || !ok {
		t.Fatalf("expected first rotation to succeed, ok=%v err=%v", ok, err)
	}
	if ok, err = store.Rotate(ctx, session.ID, security.HashTokenID("old"), next); err != nil || ok {
		t.Fatalf("expected refresh replay rejected, ok=%v err=%v", ok, err)
	}

	if err = store.Save(ctx, security.Session{
		ID: "sid-2", UserID: 1, RefreshTokenExpires: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteUser(ctx, 1); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{"sid-1", "sid-2"} {
		if _, exists, getErr := store.Get(ctx, sid); getErr != nil || exists {
			t.Fatalf("expected %s deleted, exists=%v err=%v", sid, exists, getErr)
		}
	}
}

func TestMemorySessionStoreAllowsOnlyOneConcurrentRefresh(t *testing.T) {
	store := security.NewMemorySessionStore()
	session := security.Session{
		ID: "sid", UserID: 2, RefreshJTIHash: security.HashTokenID("old"),
		RefreshTokenExpires: time.Now().Add(time.Hour),
	}
	if err := store.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	next := session
	next.RefreshJTIHash = security.HashTokenID("new")
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.Rotate(context.Background(), session.ID, security.HashTokenID("old"), next)
			if err != nil {
				t.Errorf("rotate failed: %v", err)
				return
			}
			if ok {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("expected exactly one successful refresh, got %d", success)
	}
}

func TestMemorySessionStoreDeleteUsersNormalizesIDs(t *testing.T) {
	store := security.NewMemorySessionStore()
	ctx := context.Background()
	for _, session := range []security.Session{
		{ID: "user-2-a", UserID: 2, RefreshTokenExpires: time.Now().Add(time.Hour)},
		{ID: "user-2-b", UserID: 2, RefreshTokenExpires: time.Now().Add(time.Hour)},
		{ID: "user-3", UserID: 3, RefreshTokenExpires: time.Now().Add(time.Hour)},
		{ID: "user-4", UserID: 4, RefreshTokenExpires: time.Now().Add(time.Hour)},
	} {
		if err := store.Save(ctx, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteUsers(ctx, []int64{3, 2, 3, 0, -1}); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"user-2-a", "user-2-b", "user-3"} {
		if _, exists, err := store.Get(ctx, sessionID); err != nil || exists {
			t.Fatalf("expected %s revoked, exists=%v err=%v", sessionID, exists, err)
		}
	}
	if _, exists, err := store.Get(ctx, "user-4"); err != nil || !exists {
		t.Fatalf("unrelated session should remain, exists=%v err=%v", exists, err)
	}
	if err := store.DeleteUsers(ctx, nil); err != nil {
		t.Fatalf("empty delete should succeed: %v", err)
	}
}

func TestMiddlewareScopesAdminAndValidatesSession(t *testing.T) {
	manager := security.NewManager("middleware-secret", 3600, 7200)
	store := security.NewMemorySessionStore()
	claims := security.Claims{UserId: 7, Username: "tester", PasswordVersion: 3, TenantId: security.PlatformTenant()}
	pair, err := manager.GenerateTokenPair(claims)
	if err != nil {
		t.Fatal(err)
	}
	session := tokenSession(t, manager, pair)
	if err = store.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}

	server := newAuthServer(t, security.MiddlewareOptions{
		Manager: manager, Sessions: store, IgnorePaths: []string{"/admin/open"},
		ProtectedPrefixes: []string{"/admin/"},
	})
	assertHTTPStatus(t, server, http.MethodGet, "/app/free", "", http.StatusOK)
	assertHTTPStatus(t, server, http.MethodGet, "/admin/open", "", http.StatusOK)
	assertHTTPStatus(t, server, http.MethodGet, "/admin/protected", "", http.StatusUnauthorized)
	assertHTTPStatus(t, server, http.MethodGet, "/admin/protected", pair.RefreshToken, http.StatusUnauthorized)
	assertHTTPStatus(t, server, http.MethodGet, "/admin/protected", pair.Token, http.StatusOK)

	if err = store.Delete(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	assertHTTPStatus(t, server, http.MethodGet, "/admin/protected", pair.Token, http.StatusUnauthorized)
}

func TestMiddlewareSSORejectsOlderAccessJTI(t *testing.T) {
	manager := security.NewManager("sso-secret", 3600, 7200)
	store := security.NewMemorySessionStore()
	claims := security.Claims{UserId: 8, Username: "tester", PasswordVersion: 1, TenantId: security.PlatformTenant()}
	oldPair, err := manager.GenerateTokenPair(claims)
	if err != nil {
		t.Fatal(err)
	}
	oldSession := tokenSession(t, manager, oldPair)
	claims.SessionID = oldSession.ID
	newPair, err := manager.GenerateTokenPair(claims)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(context.Background(), tokenSession(t, manager, newPair)); err != nil {
		t.Fatal(err)
	}

	server := newAuthServer(t, security.MiddlewareOptions{
		Manager: manager, Sessions: store, ProtectedPrefixes: []string{"/admin/"}, SSO: true,
	})
	assertHTTPStatus(t, server, http.MethodGet, "/admin/protected", oldPair.Token, http.StatusUnauthorized)
	assertHTTPStatus(t, server, http.MethodGet, "/admin/protected", newPair.Token, http.StatusOK)
}

func TestMiddlewareBuildsUserContextFromVerifiedClaims(t *testing.T) {
	manager := security.NewManager("claims-source-secret", 3600, 7200)
	store := security.NewMemorySessionStore()
	claims := security.Claims{
		UserId:          12,
		Username:        "jwt-user",
		RoleIds:         []int64{7, 9},
		TenantId:        mustTenantIdentity(t, 5),
		PasswordVersion: 4,
	}
	pair, err := manager.GenerateTokenPair(claims)
	if err != nil {
		t.Fatal(err)
	}
	session := tokenSession(t, manager, pair)
	if err = store.Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}

	var received security.UserContext
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	definitions = append(definitions, middleware.Definition{
		Name: "test.auth", Order: 200,
		Handler: security.NewMiddleware(security.MiddlewareOptions{
			Manager: manager, Sessions: store, ProtectedPrefixes: []string{"/admin/"},
		}),
	})
	if err = middleware.Register(server, definitions); err != nil {
		t.Fatal(err)
	}
	server.BindHandler("/admin/context", func(r *ghttp.Request) {
		received, _ = security.UserFromContext(r.Context())
		r.Response.Write("ok")
	})
	if err = server.Start(); err != nil {
		t.Fatalf("start auth test server failed: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })

	assertHTTPStatus(t, server, http.MethodGet, "/admin/context", pair.Token, http.StatusOK)
	if received.UserId != claims.UserId ||
		received.Username != claims.Username ||
		received.TenantId != claims.TenantId ||
		received.PasswordVersion != claims.PasswordVersion ||
		!reflect.DeepEqual(received.RoleIds, claims.RoleIds) {
		t.Fatalf("expected JWT claims in user context, got %#v", received)
	}
}

/**
 * 创建测试租户身份
 * @param t 测试上下文
 * @param tenantID 租户 ID
 * @returns 租户身份
 */
func mustTenantIdentity(t *testing.T, tenantID int64) security.TenantIdentity {
	t.Helper()
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	return identity
}

func tokenSession(t *testing.T, manager *security.Manager, pair security.TokenPair) security.Session {
	t.Helper()
	access, err := manager.ParseAccessToken(pair.Token)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := manager.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	return security.Session{
		ID: access.SessionID, UserID: access.UserId,
		AccessJTIHash: security.HashTokenID(access.JTI), RefreshJTIHash: security.HashTokenID(refresh.JTI),
		PasswordVersion: access.PasswordVersion, RefreshTokenExpires: time.Unix(refresh.ExpiresAt, 0),
	}
}

func newAuthServer(t *testing.T, options security.MiddlewareOptions) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	definitions = append(definitions, middleware.Definition{Name: "test.auth", Order: 200, Handler: security.NewMiddleware(options)})
	if err := middleware.Register(server, definitions); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/app/free", "/admin/open", "/admin/protected"} {
		server.BindHandler(path, func(r *ghttp.Request) { r.Response.Write("ok") })
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start auth test server failed: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	return server
}

func assertHTTPStatus(t *testing.T, server *ghttp.Server, method string, path string, token string, want int) {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, recorder.Code, want, recorder.Body.String())
	}
}
