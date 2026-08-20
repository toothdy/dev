// Package sessionbackend 提供鉴权专用 Session 持久状态
package sessionbackend

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessioncontract"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

var (
	// Session 不存在
	ErrNotFound = errors.New("session 不存在")
	// Refresh Token 重放
	ErrRefreshReplay = errors.New("refresh token 重放")
)

// 鉴权 Session 的只读值
type Session struct {
	sessionID  string
	subject    sessioncontract.Kind
	userID     uint64
	username   string
	roleIDs    []uint64
	passwordV  int
	accessJTI  string
	refreshJTI string
	expiresAt  time.Time
}

// 鉴权专用 Session 存储
type Store interface {
	Get(context.Context, string) (Session, bool, error)
	Save(context.Context, Session) error
	RotateRefresh(context.Context, string, string, Session) error
	Revoke(context.Context, string) error
	RevokeUser(context.Context, sessioncontract.Kind, uint64) error
	RevokeUsers(context.Context, sessioncontract.Kind, []uint64) error
}

// 构造管理端 Session
func NewAdmin(
	sessionID string,
	userID uint64,
	username string,
	passwordV int,
	roleIDs []uint64,
	accessJTI string,
	refreshJTI string,
	expiresAt time.Time,
) (Session, error) {
	value := Session{
		sessionID:  sessionID,
		subject:    sessioncontract.AdminKind,
		userID:     userID,
		username:   username,
		roleIDs:    append([]uint64(nil), roleIDs...),
		passwordV:  passwordV,
		accessJTI:  accessJTI,
		refreshJTI: refreshJTI,
		expiresAt:  expiresAt,
	}
	if err := validate(value, time.Now()); err != nil {
		return Session{}, err
	}

	return value, nil
}

// 构造应用端 Session
func NewApp(
	sessionID string,
	userID uint64,
	accessJTI string,
	refreshJTI string,
	expiresAt time.Time,
) (Session, error) {
	value := Session{
		sessionID:  sessionID,
		subject:    sessioncontract.AppKind,
		userID:     userID,
		accessJTI:  accessJTI,
		refreshJTI: refreshJTI,
		expiresAt:  expiresAt,
	}
	if err := validate(value, time.Now()); err != nil {
		return Session{}, err
	}

	return value, nil
}

// 返回 Session ID
func (value Session) ID() string {
	return value.sessionID
}

// 返回身份种类
func (value Session) Subject() sessioncontract.Kind {
	return value.subject
}

// 返回用户 ID
func (value Session) UserID() uint64 {
	return value.userID
}

// 返回管理端用户名
func (value Session) Username() string {
	return value.username
}

// 返回管理端角色 ID 副本
func (value Session) RoleIDs() []uint64 {
	return append([]uint64(nil), value.roleIDs...)
}

// 返回管理端密码版本
func (value Session) PasswordV() int {
	return value.passwordV
}

// 返回当前 Access JTI
func (value Session) AccessJTI() string {
	return value.accessJTI
}

// 返回当前 Refresh JTI
func (value Session) RefreshJTI() string {
	return value.refreshJTI
}

// 返回 Refresh Session 过期时间
func (value Session) ExpiresAt() time.Time {
	return value.expiresAt
}

// 返回防御性副本
func (value Session) clone() Session {
	value.roleIDs = append([]uint64(nil), value.roleIDs...)
	return value
}

// 校验批量撤销身份并生成用户集合
func revokeUserSet(subject sessioncontract.Kind, userIDs []uint64) (map[uint64]struct{}, error) {
	if subject != sessioncontract.AdminKind && subject != sessioncontract.AppKind {
		return nil, exception.Core("Session 用户身份无效")
	}
	targets := make(map[uint64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			return nil, exception.Core("Session 用户身份无效")
		}
		targets[userID] = struct{}{}
	}

	return targets, nil
}

// 校验 Session 完整性
func validate(value Session, now time.Time) error {
	if !validIdentifier(value.sessionID) {
		return exception.Core("Session ID 无效")
	}
	if value.userID == 0 {
		return exception.Core("Session 用户 ID 不能为空")
	}
	if !validIdentifier(value.accessJTI) || !validIdentifier(value.refreshJTI) {
		return exception.Core("Session JTI 无效")
	}
	if value.expiresAt.IsZero() || !value.expiresAt.After(now) {
		return exception.Core("Session 已过期")
	}

	switch value.subject {
	case sessioncontract.AdminKind:
		if strings.TrimSpace(value.username) == "" {
			return exception.Core("管理端 Session 用户名不能为空")
		}
		if value.passwordV <= 0 {
			return exception.Core("管理端 Session 密码版本必须为正数")
		}
		for _, roleID := range value.roleIDs {
			if roleID == 0 {
				return exception.Core("管理端 Session 角色 ID 必须为正数")
			}
		}
	case sessioncontract.AppKind:
		if value.username != "" || value.roleIDs != nil || value.passwordV != 0 {
			return exception.Core("应用端 Session 不能携带管理端字段")
		}
	default:
		return exception.Core("Session 身份种类无效")
	}

	return nil
}

// 校验 Session 标识符
func validIdentifier(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}

	return true
}
