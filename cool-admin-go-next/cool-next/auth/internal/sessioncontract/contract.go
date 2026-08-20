package sessioncontract

import (
	"errors"
	"time"
)

// 身份种类
type Kind string

const (
	AdminKind Kind = "admin" // 管理端身份
	AppKind   Kind = "app"   // 应用端身份
)

var (
	// Session 不存在
	ErrSessionNotFound = errors.New("session 不存在")
	// Refresh Token 重放
	ErrRefreshReplay = errors.New("refresh token 重放")
)

// JWT 签发所需身份快照
type TokenSubject struct {
	SessionID string   // Session ID
	Subject   Kind     // 身份种类
	UserID    uint64   // 用户 ID
	Username  string   // 管理端用户名
	RoleIDs   []uint64 // 管理端角色 ID
	PasswordV int      // 管理端密码版本
}

// 鉴权 Session 只读快照
type SessionSnapshot struct {
	TokenSubject           // 身份快照
	AccessJTI    string    // 当前 Access JTI
	RefreshJTI   string    // 当前 Refresh JTI
	ExpiresAt    time.Time // Refresh Session 过期时间
}
