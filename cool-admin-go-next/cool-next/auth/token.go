package auth

import (
	"context"
	"errors"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessioncontract"
)

var (
	// 无效凭证
	ErrInvalidCredential = errors.New("无效凭证")
	// Session 不存在
	ErrSessionNotFound = sessioncontract.ErrSessionNotFound
	// Refresh Token 重放
	ErrRefreshReplay = sessioncontract.ErrRefreshReplay
)

// JWT 签发所需身份快照
type TokenSubject = sessioncontract.TokenSubject

// 已验证 JWT 内容
type TokenClaims struct {
	TokenSubject           // 身份快照
	JTI          string    // Token 唯一标识
	IsRefresh    bool      // 是否为 Refresh Token
	IssuedAt     time.Time // 签发时间
	NotBefore    time.Time // 生效时间
	ExpiresAt    time.Time // 过期时间
}

// Access 与 Refresh Token 对
type TokenPair struct {
	AccessToken     string    // Access Token
	RefreshToken    string    // Refresh Token
	AccessJTI       string    // Access Token 唯一标识
	RefreshJTI      string    // Refresh Token 唯一标识
	AccessExpiresAt time.Time // Access Token 过期时间
	ExpiresAt       time.Time // Refresh Token 及 Session 过期时间
}

// JWT 签发与验证端口
type TokenCodec interface {
	IssuePair(TokenSubject) (TokenPair, error)
	Parse(string, bool) (TokenClaims, error)
}

// 只读快照
type SessionSnapshot = sessioncontract.SessionSnapshot

// 鉴权专用 Session 端口
type SessionStore interface {
	Get(context.Context, string) (SessionSnapshot, bool, error)
	Save(context.Context, SessionSnapshot) error
	RotateRefresh(context.Context, string, string, SessionSnapshot) error
	Revoke(context.Context, string) error
	RevokeUser(context.Context, Kind, uint64) error
	RevokeUsers(context.Context, Kind, []uint64) error
}

// 新登录态身份
type Principal struct {
	Subject   Kind     // 身份种类
	UserID    uint64   // 用户 ID
	Username  string   // 管理端用户名
	RoleIDs   []uint64 // 管理端角色 ID
	PasswordV int      // 管理端密码版本
}

// 请求鉴权规则
type Rule struct {
	IgnoreToken bool   // 是否忽略 Token
	Permission  string // 权限标识
	Resource    string // 协议资源
}
