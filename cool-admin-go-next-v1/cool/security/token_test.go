package security_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/security"
)

func testClaims() security.Claims {
	return security.Claims{
		RoleIds:         []int64{1, 2},
		Username:        "admin",
		UserId:          1,
		PasswordVersion: 1,
		TenantId:        security.PlatformTenant(),
	}
}

/**
 * 验证平台 Token 使用 null 租户声明
 * @param t 测试上下文
 * @returns null
 */
func TestPlatformTokenUsesNullTenantClaim(t *testing.T) {
	manager := security.NewManager("secret", 7200, 604800)
	pair, err := manager.GenerateTokenPair(testClaims())
	if err != nil {
		t.Fatalf("generate token pair failed: %v", err)
	}
	parts := strings.Split(pair.Token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode token payload failed: %v", err)
	}
	var values map[string]interface{}
	if err = json.Unmarshal(payload, &values); err != nil {
		t.Fatalf("unmarshal token payload failed: %v", err)
	}
	value, ok := values["tenantId"]
	if !ok || value != nil {
		t.Fatalf("expected explicit null tenantId, got %#v", values)
	}
}

/**
 * 验证 legacy 零租户声明解析为平台
 * @param t 测试上下文
 * @returns null
 */
func TestLegacyZeroTenantClaimIsPlatform(t *testing.T) {
	manager := security.NewManager("secret", 7200, 604800)
	token := signTestToken(t, map[string]interface{}{
		"isRefresh":       false,
		"tokenType":       security.TokenTypeAccess,
		"sid":             "session",
		"jti":             "access",
		"roleIds":         []int64{1},
		"username":        "admin",
		"userId":          1,
		"passwordVersion": 1,
		"tenantId":        0,
		"exp":             time.Now().Unix() + 60,
	}, "secret")
	claims, err := manager.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("parse legacy token failed: %v", err)
	}
	if !claims.TenantId.IsPlatform() {
		t.Fatal("expected legacy zero tenant to be platform")
	}
}

/**
 * 验证缺失租户声明的 Token 被拒绝
 * @param t 测试上下文
 * @returns null
 */
func TestMissingTenantClaimFails(t *testing.T) {
	manager := security.NewManager("secret", 7200, 604800)
	if _, err := manager.GenerateTokenPair(security.Claims{UserId: 1}); err == nil {
		t.Fatal("expected missing tenant claim rejected during generation")
	}
	token := signTestToken(t, map[string]interface{}{
		"isRefresh":       false,
		"tokenType":       security.TokenTypeAccess,
		"sid":             "session",
		"jti":             "access",
		"roleIds":         []int64{1},
		"username":        "admin",
		"userId":          1,
		"passwordVersion": 1,
		"exp":             time.Now().Unix() + 60,
	}, "secret")
	if _, err := manager.ParseAccessToken(token); err == nil {
		t.Fatal("expected signed token without tenant claim rejected")
	}
}

/**
 * 签发测试 Token
 * @param t 测试上下文
 * @param payload Token payload
 * @param secret 签名密钥
 * @returns Token 字符串
 */
func signTestToken(t *testing.T, payload map[string]interface{}, secret string) string {
	t.Helper()
	headerData, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal token header failed: %v", err)
	}
	payloadData, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal token payload failed: %v", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerData) + "." + base64.RawURLEncoding.EncodeToString(payloadData)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestTokenPairCanBeParsed(t *testing.T) {
	manager := security.NewManager("secret", 7200, 604800)
	pair, err := manager.GenerateTokenPair(testClaims())
	if err != nil {
		t.Fatalf("generate token pair failed: %v", err)
	}
	if pair.Token == "" || pair.RefreshToken == "" {
		t.Fatalf("expected token pair, got %#v", pair)
	}
	if pair.Expire != 7200 || pair.RefreshExpire != 604800 {
		t.Fatalf("unexpected expires: %#v", pair)
	}

	claims, err := manager.ParseAccessToken(pair.Token)
	if err != nil {
		t.Fatalf("parse access token failed: %v", err)
	}
	if claims.IsRefresh {
		t.Fatal("expected access token isRefresh=false")
	}
	if claims.Username != "admin" || claims.UserId != 1 {
		t.Fatalf("unexpected claims: %#v", claims)
	}

	refreshClaims, err := manager.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("parse refresh token failed: %v", err)
	}
	if !refreshClaims.IsRefresh {
		t.Fatal("expected refresh token isRefresh=true")
	}
}

func TestTokenTypeMismatchFails(t *testing.T) {
	manager := security.NewManager("secret", 7200, 604800)
	pair, err := manager.GenerateTokenPair(testClaims())
	if err != nil {
		t.Fatalf("generate token pair failed: %v", err)
	}
	if _, err = manager.ParseAccessToken(pair.RefreshToken); err == nil {
		t.Fatal("expected refresh token rejected as access token")
	}
	if _, err = manager.ParseRefreshToken(pair.Token); err == nil {
		t.Fatal("expected access token rejected as refresh token")
	}
}

func TestExpiredTokenFails(t *testing.T) {
	manager := security.NewManager("secret", -1, -1)
	pair, err := manager.GenerateTokenPair(testClaims())
	if err != nil {
		t.Fatalf("generate token pair failed: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err = manager.ParseAccessToken(pair.Token); err == nil {
		t.Fatal("expected expired access token error")
	}
}
