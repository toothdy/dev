package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// 认证与刷新内核
type Service struct {
	tokens     TokenCodec
	sessions   SessionStore
	authorizer Authorizer
	now        func() time.Time
	newID      func() (string, error)
}

// 创建认证服务
func NewService(tokens TokenCodec, sessions SessionStore, authorizer Authorizer) (*Service, error) {
	return newService(tokens, sessions, authorizer, time.Now, randomID)
}

// 创建新登录态
func (service *Service) Create(ctx context.Context, principal Principal) (TokenPair, error) {
	if err := service.validateReady(); err != nil {
		return TokenPair{}, err
	}
	if err := validatePrincipal(principal); err != nil {
		return TokenPair{}, err
	}
	sessionID, err := service.newID()
	if err != nil {
		return TokenPair{}, exception.WrapCore(err, "生成 Session ID 失败")
	}
	subject := TokenSubject{
		SessionID: sessionID,
		Subject:   principal.Subject,
		UserID:    principal.UserID,
		Username:  principal.Username,
		RoleIDs:   append([]uint64(nil), principal.RoleIDs...),
		PasswordV: principal.PasswordV,
	}
	pair, err := service.tokens.IssuePair(subject)
	if err != nil {
		return TokenPair{}, exception.WrapCore(err, "签发 Token 失败")
	}
	if err = service.sessions.Save(ctx, snapshotFromPair(subject, pair)); err != nil {
		return TokenPair{}, exception.WrapCore(err, "保存登录 Session 失败")
	}

	return pair, nil
}

// 验证 Access Token 并写入可信身份
func (service *Service) Access(ctx context.Context, token string) (context.Context, error) {
	claims, current, err := service.verify(ctx, token, false)
	if err != nil {
		return ctx, err
	}

	return contextWithSession(ctx, claims, current), nil
}

// 刷新并原子轮换 Token 对
func (service *Service) Refresh(ctx context.Context, token string) (TokenPair, error) {
	return service.RefreshWith(ctx, token, func(_ context.Context, principal Principal) (Principal, error) {
		return principal, nil
	})
}

// RefreshWith 使用重新解析的权威身份原子轮换 Token 对。
func (service *Service) RefreshWith(
	ctx context.Context,
	token string,
	resolve func(context.Context, Principal) (Principal, error),
) (TokenPair, error) {
	if resolve == nil {
		return TokenPair{}, exception.Core("刷新身份解析器不能为空")
	}
	claims, current, err := service.verify(ctx, token, true)
	if err != nil {
		return TokenPair{}, err
	}
	principal, err := resolve(ctx, principalFromSnapshot(current))
	if err != nil {
		return TokenPair{}, err
	}
	if err = validatePrincipal(principal); err != nil {
		return TokenPair{}, err
	}
	if principal.Subject != current.Subject || principal.UserID != current.UserID {
		return TokenPair{}, exception.Core("刷新不能改变 Session 身份")
	}
	subject := tokenSubjectFromPrincipal(current.SessionID, principal)
	pair, err := service.tokens.IssuePair(subject)
	if err != nil {
		return TokenPair{}, exception.WrapCore(err, "签发刷新 Token 失败")
	}
	next := snapshotFromPair(subject, pair)
	if err = service.sessions.RotateRefresh(ctx, current.SessionID, claims.JTI, next); err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			return TokenPair{}, unauthenticatedError()
		case errors.Is(err, ErrRefreshReplay):
			return TokenPair{}, service.revokeReplay(ctx, current.SessionID)
		default:
			return TokenPair{}, exception.WrapCore(err, "轮换刷新 Session 失败")
		}
	}

	return pair, nil
}

// 按路由规则执行认证和授权
func (service *Service) Authenticate(ctx context.Context, token string, rule Rule) (context.Context, error) {
	if rule.IgnoreToken {
		return ctx, nil
	}
	verified, err := service.Access(ctx, token)
	if err != nil {
		return ctx, err
	}
	if strings.TrimSpace(rule.Permission) == "" {
		return verified, nil
	}
	if err = Authorize(verified, service.authorizer, rule.Permission, rule.Resource); err != nil {
		return ctx, err
	}

	return withPermission(verified, PermissionState{
		Permission: rule.Permission,
		Resource:   rule.Resource,
	}), nil
}

// 验证 Token 和服务端 Session
func (service *Service) verify(
	ctx context.Context,
	token string,
	isRefresh bool,
) (TokenClaims, SessionSnapshot, error) {
	if err := service.validateReady(); err != nil {
		return TokenClaims{}, SessionSnapshot{}, err
	}
	claims, err := service.tokens.Parse(token, isRefresh)
	if err != nil {
		if errors.Is(err, ErrInvalidCredential) {
			return TokenClaims{}, SessionSnapshot{}, invalidCredentialError()
		}
		return TokenClaims{}, SessionSnapshot{}, exception.WrapCore(err, "验证 Token 失败")
	}
	current, exists, err := service.sessions.Get(ctx, claims.SessionID)
	if err != nil {
		return TokenClaims{}, SessionSnapshot{}, exception.WrapCore(err, "读取鉴权 Session 失败")
	}
	if !exists || !current.ExpiresAt.After(service.now()) {
		return TokenClaims{}, SessionSnapshot{}, unauthenticatedError()
	}
	if err = checkClaims(claims, current); err != nil {
		if isRefresh && claims.SessionID != "" && claims.JTI != current.RefreshJTI {
			return TokenClaims{}, SessionSnapshot{}, service.revokeReplay(ctx, claims.SessionID)
		}
		return TokenClaims{}, SessionSnapshot{}, unauthenticatedError()
	}

	return claims, current, nil
}

// 撤销发生 Refresh 重放的 Session
func (service *Service) revokeReplay(ctx context.Context, sessionID string) error {
	if err := service.sessions.Revoke(ctx, sessionID); err != nil {
		return exception.WrapCore(err, "撤销重放 Session 失败")
	}

	return unauthenticatedError()
}

// 校验服务已初始化
func (service *Service) validateReady() error {
	if service == nil || service.tokens == nil || service.sessions == nil || service.now == nil || service.newID == nil {
		return exception.Core("认证服务未初始化")
	}

	return nil
}

// 创建可替换时钟和 ID 源的认证服务
func newService(
	tokens TokenCodec,
	sessions SessionStore,
	authorizer Authorizer,
	now func() time.Time,
	newID func() (string, error),
) (*Service, error) {
	service := &Service{tokens: tokens, sessions: sessions, authorizer: authorizer, now: now, newID: newID}
	if err := service.validateReady(); err != nil {
		return nil, err
	}

	return service, nil
}

// 校验新登录态身份
func validatePrincipal(principal Principal) error {
	if principal.UserID == 0 {
		return exception.Core("登录身份 ID 不能为空")
	}
	switch principal.Subject {
	case AdminKind:
		if strings.TrimSpace(principal.Username) == "" || principal.PasswordV <= 0 {
			return exception.Core("管理端登录身份无效")
		}
		for _, roleID := range principal.RoleIDs {
			if roleID == 0 {
				return exception.Core("管理端角色 ID 必须为正数")
			}
		}
	case AppKind:
		if principal.Username != "" || principal.RoleIDs != nil || principal.PasswordV != 0 {
			return exception.Core("应用端登录身份不能携带管理端字段")
		}
	default:
		return exception.Core("登录身份种类无效")
	}

	return nil
}

func principalFromSnapshot(snapshot SessionSnapshot) Principal {
	return Principal{
		Subject:   snapshot.Subject,
		UserID:    snapshot.UserID,
		Username:  snapshot.Username,
		RoleIDs:   append([]uint64(nil), snapshot.RoleIDs...),
		PasswordV: snapshot.PasswordV,
	}
}

func tokenSubjectFromPrincipal(sessionID string, principal Principal) TokenSubject {
	return TokenSubject{
		SessionID: sessionID,
		Subject:   principal.Subject,
		UserID:    principal.UserID,
		Username:  principal.Username,
		RoleIDs:   append([]uint64(nil), principal.RoleIDs...),
		PasswordV: principal.PasswordV,
	}
}

// 核对 Token 与服务端 Session
func checkClaims(claims TokenClaims, current SessionSnapshot) error {
	if claims.SessionID != current.SessionID || claims.Subject != current.Subject || claims.UserID != current.UserID {
		return ErrInvalidCredential
	}
	if claims.Subject == AdminKind && claims.PasswordV != current.PasswordV {
		return ErrInvalidCredential
	}
	wantJTI := current.AccessJTI
	if claims.IsRefresh {
		wantJTI = current.RefreshJTI
	}
	if claims.JTI != wantJTI {
		return ErrInvalidCredential
	}

	return nil
}

// 从 Session 构造可信身份 Context
func contextWithSession(ctx context.Context, claims TokenClaims, current SessionSnapshot) context.Context {
	ctx = withSession(ctx, SessionState{
		ID:        current.SessionID,
		Subject:   current.Subject,
		AccessJTI: current.AccessJTI,
		ExpiresAt: current.ExpiresAt,
	})
	if claims.Subject == AdminKind {
		return withAdmin(ctx, current.UserID, current.Username, current.PasswordV, current.RoleIDs)
	}

	return withApp(ctx, current.UserID)
}

// 从 Token 对构造 Session 快照
func snapshotFromPair(subject TokenSubject, pair TokenPair) SessionSnapshot {
	subject.RoleIDs = append([]uint64(nil), subject.RoleIDs...)
	return SessionSnapshot{
		TokenSubject: subject,
		AccessJTI:    pair.AccessJTI,
		RefreshJTI:   pair.RefreshJTI,
		ExpiresAt:    pair.ExpiresAt,
	}
}

// 生成密码学随机标识符
func randomID() (string, error) {
	content := make([]byte, 32)
	if _, err := rand.Read(content); err != nil {
		return "", err
	}

	return hex.EncodeToString(content), nil
}

// 无效凭证异常
func invalidCredentialError() error {
	return exception.Comm("凭证无效", http.StatusUnauthorized)
}
