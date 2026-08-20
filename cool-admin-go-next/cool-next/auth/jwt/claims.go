package jwt

import (
	"errors"
	"strings"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
)

// 管理端 JWT Claims
type AdminClaims struct {
	SessionID               string    `json:"sessionId"`    // Session ID
	Subject                 auth.Kind `json:"subject"`      // 身份种类
	IsRefresh               *bool     `json:"isRefresh"`    // Token 类型
	RoleIDs                 []uint64  `json:"roleIds"`      // 角色 ID
	Username                string    `json:"username"`     // 用户名
	UserID                  uint64    `json:"userId"`       // 用户 ID
	PasswordV               int       `json:"passwordV"`    // 密码版本
	AppID                   *uint64   `json:"id,omitempty"` // 禁止应用端字段
	jwtlib.RegisteredClaims           // JWT 注册 Claims
}

// 校验管理端私有 Claims
func (claims AdminClaims) Validate() error {
	if err := validateCommon(claims.SessionID, claims.Subject, claims.IsRefresh, claims.RegisteredClaims); err != nil {
		return err
	}
	if claims.Subject != auth.AdminKind || claims.UserID == 0 || strings.TrimSpace(claims.Username) == "" ||
		claims.PasswordV <= 0 || claims.RoleIDs == nil || claims.AppID != nil {
		return errors.New("管理端 claims 无效")
	}
	for _, roleID := range claims.RoleIDs {
		if roleID == 0 {
			return errors.New("管理端 roleIds 无效")
		}
	}

	return nil
}

// 应用端 JWT Claims
type AppClaims struct {
	SessionID               string    `json:"sessionId"`           // Session ID
	Subject                 auth.Kind `json:"subject"`             // 身份种类
	ID                      uint64    `json:"id"`                  // 应用用户 ID
	IsRefresh               *bool     `json:"isRefresh"`           // Token 类型
	Username                *string   `json:"username,omitempty"`  // 禁止管理端字段
	RoleIDs                 *[]uint64 `json:"roleIds,omitempty"`   // 禁止管理端字段
	UserID                  *uint64   `json:"userId,omitempty"`    // 禁止管理端字段
	PasswordV               *int      `json:"passwordV,omitempty"` // 禁止管理端字段
	jwtlib.RegisteredClaims           // JWT 注册 Claims
}

// 校验应用端私有 Claims
func (claims AppClaims) Validate() error {
	if err := validateCommon(claims.SessionID, claims.Subject, claims.IsRefresh, claims.RegisteredClaims); err != nil {
		return err
	}
	if claims.Subject != auth.AppKind || claims.ID == 0 || claims.Username != nil || claims.RoleIDs != nil ||
		claims.UserID != nil || claims.PasswordV != nil {
		return errors.New("应用端 claims 无效")
	}

	return nil
}

// 校验两类 Token 的共同必填 Claims
func validateCommon(
	sessionID string,
	subject auth.Kind,
	isRefresh *bool,
	registered jwtlib.RegisteredClaims,
) error {
	if strings.TrimSpace(sessionID) == "" || subject == "" || isRefresh == nil {
		return errors.New("jwt 私有 claims 不完整")
	}
	if strings.TrimSpace(registered.ID) == "" || strings.TrimSpace(registered.Issuer) == "" ||
		len(registered.Audience) != 1 || registered.IssuedAt == nil || registered.NotBefore == nil || registered.ExpiresAt == nil {
		return errors.New("jwt 注册 claims 不完整")
	}
	if registered.IssuedAt.After(registered.NotBefore.Time) || !registered.ExpiresAt.After(registered.NotBefore.Time) {
		return errors.New("jwt 时间 claims 顺序无效")
	}

	return nil
}
