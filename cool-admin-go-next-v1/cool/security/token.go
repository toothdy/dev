package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

// JWT payload
type Claims struct {
	IsRefresh       bool           `json:"isRefresh"`
	TokenType       string         `json:"tokenType"`
	SessionID       string         `json:"sid"`
	JTI             string         `json:"jti"`
	RoleIds         []int64        `json:"roleIds"`
	Username        string         `json:"username"`
	UserId          int64          `json:"userId"`
	PasswordVersion int64          `json:"passwordVersion"`
	TenantId        TenantIdentity `json:"tenantId"`
	ExpiresAt       int64          `json:"exp"`
}

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// 登录和刷新接口返回结构
type TokenPair struct {
	Token         string `json:"token"`
	Expire        int64  `json:"expire"`
	RefreshToken  string `json:"refreshToken"`
	RefreshExpire int64  `json:"refreshExpire"`
}

// JWT token 管理器
type Manager struct {
	Secret        []byte
	Expire        int64
	RefreshExpire int64
}

// 创建 token 管理器
func NewManager(secret string, expire int64, refreshExpire int64) *Manager {
	return &Manager{
		Secret:        []byte(secret),
		Expire:        expire,
		RefreshExpire: refreshExpire,
	}
}

// 生成 token pair
func (m *Manager) GenerateTokenPair(claims Claims) (TokenPair, error) {
	if claims.TenantId.IsMissing() {
		return TokenPair{}, gerror.New("租户身份缺失")
	}
	now := time.Now().Unix()
	sessionID := claims.SessionID
	if sessionID == "" {
		var err error
		sessionID, err = randomTokenID()
		if err != nil {
			return TokenPair{}, gerror.Wrap(err, "生成 session id 失败")
		}
	}
	accessJTI, err := randomTokenID()
	if err != nil {
		return TokenPair{}, gerror.Wrap(err, "生成 access jti 失败")
	}
	refreshJTI, err := randomTokenID()
	if err != nil {
		return TokenPair{}, gerror.Wrap(err, "生成 refresh jti 失败")
	}
	accessClaims := claims
	accessClaims.IsRefresh = false
	accessClaims.TokenType = TokenTypeAccess
	accessClaims.SessionID = sessionID
	accessClaims.JTI = accessJTI
	accessClaims.ExpiresAt = now + m.Expire

	refreshClaims := claims
	refreshClaims.IsRefresh = true
	refreshClaims.TokenType = TokenTypeRefresh
	refreshClaims.SessionID = sessionID
	refreshClaims.JTI = refreshJTI
	refreshClaims.ExpiresAt = now + m.RefreshExpire

	token, err := m.sign(accessClaims)
	if err != nil {
		return TokenPair{}, gerror.Wrap(err, "生成 access token 失败")
	}
	refreshToken, err := m.sign(refreshClaims)
	if err != nil {
		return TokenPair{}, gerror.Wrap(err, "生成 refresh token 失败")
	}

	return TokenPair{
		Token:         token,
		Expire:        m.Expire,
		RefreshToken:  refreshToken,
		RefreshExpire: m.RefreshExpire,
	}, nil
}

// 解析 access token
func (m *Manager) ParseAccessToken(token string) (Claims, error) {
	claims, err := m.parse(token)
	if err != nil {
		return Claims{}, err
	}
	if claims.TokenType != TokenTypeAccess || claims.IsRefresh {
		return Claims{}, gerror.New("refresh token 不能作为 access token 使用")
	}
	return claims, nil
}

// 解析 refresh token
func (m *Manager) ParseRefreshToken(token string) (Claims, error) {
	claims, err := m.parse(token)
	if err != nil {
		return Claims{}, err
	}
	if claims.TokenType != TokenTypeRefresh || !claims.IsRefresh {
		return Claims{}, gerror.New("access token 不能作为 refresh token 使用")
	}
	return claims, nil
}

func (m *Manager) sign(claims Claims) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerData, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadData, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerData) + "." + base64.RawURLEncoding.EncodeToString(payloadData)
	signature := m.signature(signingInput)
	return signingInput + "." + signature, nil
}

func (m *Manager) parse(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, gerror.New("token 格式错误")
	}
	signingInput := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(m.signature(signingInput))) {
		return Claims{}, gerror.New("token 签名无效")
	}

	payloadData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, gerror.Wrap(err, "解析 token payload 失败")
	}
	claims := Claims{}
	if err = json.Unmarshal(payloadData, &claims); err != nil {
		return Claims{}, gerror.Wrap(err, "反序列化 token payload 失败")
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		return Claims{}, gerror.New("token 已过期")
	}
	if claims.SessionID == "" || claims.JTI == "" {
		return Claims{}, gerror.New("token 会话标识无效")
	}
	if claims.TenantId.IsMissing() {
		return Claims{}, gerror.New("token 租户身份缺失")
	}
	return claims, nil
}

func (m *Manager) signature(signingInput string) string {
	mac := hmac.New(sha256.New, m.Secret)
	_, _ = mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomTokenID() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
