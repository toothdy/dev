package auth

import "github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessioncontract"

// 身份种类
type Kind = sessioncontract.Kind

const (
	AdminKind = sessioncontract.AdminKind // 管理端身份
	AppKind   = sessioncontract.AppKind   // 应用端身份
)

// 管理端已验证身份
type AdminIdentity struct {
	UserID    uint64 // 用户 ID
	Username  string // 用户名
	PasswordV int    // 密码版本
	roleIDs   []uint64
}

// 返回角色 ID 副本
func (identity AdminIdentity) RoleIDs() []uint64 {
	return append([]uint64(nil), identity.roleIDs...)
}

// 应用端已验证身份
type AppIdentity struct {
	ID uint64 // 应用用户 ID
}

// 构造管理端已验证身份
func newAdminIdentity(userID uint64, username string, passwordV int, roleIDs []uint64) AdminIdentity {
	return AdminIdentity{
		UserID:    userID,
		Username:  username,
		PasswordV: passwordV,
		roleIDs:   append([]uint64(nil), roleIDs...),
	}
}
