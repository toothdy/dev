package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

/**
 * 验证 Redis 会话在应用重启后仍可鉴权
 * @param t 测试上下文
 * @returns null
 */
func TestRedisSessionStoreSurvivesApplicationRestart(t *testing.T) {
	if os.Getenv("COOL_AUTH_REDIS_INTEGRATION") != "1" {
		t.Skip("set COOL_AUTH_REDIS_INTEGRATION=1 to run auth Redis integration test")
	}
	address := os.Getenv("COOL_AUTH_REDIS_ADDRESS")
	if address == "" {
		address = "127.0.0.1:6379"
	}
	database := 0
	if configuredDatabase := os.Getenv("COOL_AUTH_REDIS_DB"); configuredDatabase != "" {
		parsedDatabase, err := strconv.Atoi(configuredDatabase)
		if err != nil {
			t.Fatalf("invalid COOL_AUTH_REDIS_DB: %v", err)
		}
		database = parsedDatabase
	}
	setSessionStoreTestConfig(t, fmt.Sprintf(`redis:
  default:
    address: %s
    db: %d
    user: %s
    pass: %s`,
		strconv.Quote(address),
		database,
		strconv.Quote(os.Getenv("COOL_AUTH_REDIS_USER")),
		strconv.Quote(os.Getenv("COOL_AUTH_REDIS_PASS")),
	))

	ctx := context.Background()
	firstStore, err := resolveSessionStore(ctx, nil, defaultRedisSessionClient)
	if err != nil {
		t.Fatalf("create first Redis session store failed: %v", err)
	}
	manager := security.NewManager("0123456789abcdef0123456789abcdef", 7200, 1296000)
	userID := time.Now().UnixNano()
	pair, err := manager.GenerateTokenPair(security.Claims{
		UserId: userID, Username: "redis-restart-test", PasswordVersion: 1, TenantId: security.PlatformTenant(),
	})
	if err != nil {
		t.Fatalf("generate token pair failed: %v", err)
	}
	accessClaims, err := manager.ParseAccessToken(pair.Token)
	if err != nil {
		t.Fatalf("parse access token failed: %v", err)
	}
	refreshClaims, err := manager.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("parse refresh token failed: %v", err)
	}
	session := security.Session{
		ID: accessClaims.SessionID, UserID: accessClaims.UserId,
		AccessJTIHash: security.HashTokenID(accessClaims.JTI), RefreshJTIHash: security.HashTokenID(refreshClaims.JTI),
		PasswordVersion: accessClaims.PasswordVersion, RefreshTokenExpires: time.Unix(refreshClaims.ExpiresAt, 0),
	}
	if err = firstStore.Save(ctx, session); err != nil {
		t.Fatalf("save Redis session failed: %v", err)
	}
	client, err := defaultRedisSessionClient()
	if err != nil {
		t.Fatalf("get Redis cleanup client failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.Do(
			context.Background(),
			"DEL",
			fmt.Sprintf("cool-admin-go-next:auth:v1:session:%s", session.ID),
			fmt.Sprintf("cool-admin-go-next:auth:v1:user:%d:generation", session.UserID),
		)
	})

	// 重新解析 store 模拟服务重启后的应用装配。
	secondStore, err := resolveSessionStore(ctx, nil, defaultRedisSessionClient)
	if err != nil {
		t.Fatalf("create second Redis session store failed: %v", err)
	}
	server := newRedisRestartAuthServer(t, manager, secondStore)
	request := httptest.NewRequest(http.MethodGet, "/admin/restart-check", nil)
	request.Header.Set("Authorization", pair.Token)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("token should survive application restart, status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

/**
 * 创建 Redis 重启鉴权测试服务
 * @param t 测试上下文
 * @param manager JWT 管理器
 * @param sessions 会话存储
 * @returns HTTP 服务
 */
func newRedisRestartAuthServer(t *testing.T, manager *security.Manager, sessions security.SessionStore) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindMiddlewareDefault(security.NewMiddleware(security.MiddlewareOptions{
		Manager: manager, Sessions: sessions, ProtectedPrefixes: []string{"/admin/"},
	}))
	server.BindHandler("/admin/restart-check", func(r *ghttp.Request) {
		r.Response.Write("ok")
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start Redis restart auth server failed: %v", err)
	}
	t.Cleanup(func() {
		_ = server.Shutdown()
	})
	return server
}
