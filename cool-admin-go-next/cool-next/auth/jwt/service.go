package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// JWT 签发与验证服务
type Service struct {
	config Config
	keys   map[string][]byte
	now    func() time.Time
	newJTI func() (string, error)
}

// 创建 JWT 服务
func New(config Config) (*Service, error) {
	return newService(config, time.Now, randomJTI)
}

// 签发 Access 与 Refresh Token 对
func (service *Service) IssuePair(subject auth.TokenSubject) (auth.TokenPair, error) {
	if err := service.validateReady(); err != nil {
		return auth.TokenPair{}, err
	}
	if err := validateSubject(subject); err != nil {
		return auth.TokenPair{}, err
	}
	accessJTI, err := service.newJTI()
	if err != nil {
		return auth.TokenPair{}, exception.WrapCore(err, "生成 Access JTI 失败")
	}
	refreshJTI, err := service.newJTI()
	if err != nil {
		return auth.TokenPair{}, exception.WrapCore(err, "生成 Refresh JTI 失败")
	}
	if strings.TrimSpace(accessJTI) == "" || strings.TrimSpace(refreshJTI) == "" || accessJTI == refreshJTI {
		return auth.TokenPair{}, exception.Core("JWT JTI 无效或重复")
	}
	now := service.now().UTC()
	pair := auth.TokenPair{
		AccessJTI:       accessJTI,
		RefreshJTI:      refreshJTI,
		AccessExpiresAt: now.Add(service.config.AccessTTL),
		ExpiresAt:       now.Add(service.config.RefreshTTL),
	}
	pair.AccessToken, err = service.sign(subject, accessJTI, false, now, pair.AccessExpiresAt)
	if err != nil {
		return auth.TokenPair{}, err
	}
	pair.RefreshToken, err = service.sign(subject, refreshJTI, true, now, pair.ExpiresAt)
	if err != nil {
		return auth.TokenPair{}, err
	}

	return pair, nil
}

// 解析并严格校验指定类型的 Token
func (service *Service) Parse(encoded string, expectedRefresh bool) (auth.TokenClaims, error) {
	if err := service.validateReady(); err != nil {
		return auth.TokenClaims{}, err
	}
	if strings.TrimSpace(encoded) == "" || strings.TrimSpace(encoded) != encoded {
		return auth.TokenClaims{}, auth.ErrInvalidCredential
	}
	subject, err := service.readSubject(encoded)
	if err != nil {
		return auth.TokenClaims{}, auth.ErrInvalidCredential
	}
	switch subject {
	case auth.AdminKind:
		claims := &AdminClaims{}
		if err = service.parse(encoded, claims); err != nil {
			return auth.TokenClaims{}, auth.ErrInvalidCredential
		}
		return adminTokenClaims(claims, expectedRefresh)
	case auth.AppKind:
		claims := &AppClaims{}
		if err = service.parse(encoded, claims); err != nil {
			return auth.TokenClaims{}, auth.ErrInvalidCredential
		}
		return appTokenClaims(claims, expectedRefresh)
	default:
		return auth.TokenClaims{}, auth.ErrInvalidCredential
	}
}

// 创建可替换时钟和 JTI 源的 JWT 服务
func newService(config Config, now func() time.Time, newJTI func() (string, error)) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if now == nil || newJTI == nil {
		return nil, exception.Core("JWT 运行依赖不能为空")
	}
	keys := make(map[string][]byte, len(config.Keys))
	for keyID, secret := range config.Keys {
		keys[keyID] = append([]byte(nil), []byte(secret)...)
	}
	config.Keys = nil

	return &Service{config: config, keys: keys, now: now, newJTI: newJTI}, nil
}

// 签发单个 Token
func (service *Service) sign(
	subject auth.TokenSubject,
	jti string,
	isRefresh bool,
	issuedAt time.Time,
	expiresAt time.Time,
) (string, error) {
	registered := jwtlib.RegisteredClaims{
		Issuer:    service.config.Issuer,
		Audience:  jwtlib.ClaimStrings{service.config.Audience},
		ExpiresAt: jwtlib.NewNumericDate(expiresAt),
		NotBefore: jwtlib.NewNumericDate(issuedAt),
		IssuedAt:  jwtlib.NewNumericDate(issuedAt),
		ID:        jti,
	}
	refresh := isRefresh
	var claims jwtlib.Claims
	if subject.Subject == auth.AdminKind {
		roleIDs := make([]uint64, len(subject.RoleIDs))
		copy(roleIDs, subject.RoleIDs)
		claims = &AdminClaims{
			SessionID:        subject.SessionID,
			Subject:          subject.Subject,
			IsRefresh:        &refresh,
			RoleIDs:          roleIDs,
			Username:         subject.Username,
			UserID:           subject.UserID,
			PasswordV:        subject.PasswordV,
			RegisteredClaims: registered,
		}
	} else {
		claims = &AppClaims{
			SessionID:        subject.SessionID,
			Subject:          subject.Subject,
			ID:               subject.UserID,
			IsRefresh:        &refresh,
			RegisteredClaims: registered,
		}
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	token.Header["kid"] = service.config.CurrentKeyID
	encoded, err := token.SignedString(service.keys[service.config.CurrentKeyID])
	if err != nil {
		return "", exception.WrapCore(err, "JWT 签名失败")
	}

	return encoded, nil
}

// 读取未验证的身份种类以选择严格 Claims 模型
func (service *Service) readSubject(encoded string) (auth.Kind, error) {
	claims := struct {
		Subject auth.Kind `json:"subject"`
		jwtlib.RegisteredClaims
	}{}
	parser := jwtlib.NewParser(jwtlib.WithValidMethods([]string{Algorithm}), jwtlib.WithStrictDecoding())
	if _, _, err := parser.ParseUnverified(encoded, &claims); err != nil {
		return "", err
	}

	return claims.Subject, nil
}

// 验签并校验注册 Claims
func (service *Service) parse(encoded string, claims jwtlib.Claims) error {
	parser := jwtlib.NewParser(
		jwtlib.WithValidMethods([]string{Algorithm}),
		jwtlib.WithIssuer(service.config.Issuer),
		jwtlib.WithAudience(service.config.Audience),
		jwtlib.WithExpirationRequired(),
		jwtlib.WithIssuedAt(),
		jwtlib.WithLeeway(service.config.ClockSkew),
		jwtlib.WithTimeFunc(service.now),
		jwtlib.WithStrictDecoding(),
	)
	token, err := parser.ParseWithClaims(encoded, claims, service.keyForToken)
	if err != nil || token == nil || !token.Valid {
		return auth.ErrInvalidCredential
	}

	return nil
}

// 按 kid 返回固定 HS256 验签密钥
func (service *Service) keyForToken(token *jwtlib.Token) (any, error) {
	if token == nil || token.Method != jwtlib.SigningMethodHS256 || token.Method.Alg() != Algorithm {
		return nil, auth.ErrInvalidCredential
	}
	keyID, ok := token.Header["kid"].(string)
	if !ok || strings.TrimSpace(keyID) == "" {
		return nil, auth.ErrInvalidCredential
	}
	key, exists := service.keys[keyID]
	if !exists {
		return nil, auth.ErrInvalidCredential
	}

	return key, nil
}

// 转换管理端 Claims
func adminTokenClaims(claims *AdminClaims, expectedRefresh bool) (auth.TokenClaims, error) {
	if claims == nil || claims.IsRefresh == nil || *claims.IsRefresh != expectedRefresh {
		return auth.TokenClaims{}, auth.ErrInvalidCredential
	}
	return tokenClaims(
		claims.SessionID,
		claims.Subject,
		claims.UserID,
		claims.Username,
		claims.RoleIDs,
		claims.PasswordV,
		*claims.IsRefresh,
		claims.RegisteredClaims,
	)
}

// 转换应用端 Claims
func appTokenClaims(claims *AppClaims, expectedRefresh bool) (auth.TokenClaims, error) {
	if claims == nil || claims.IsRefresh == nil || *claims.IsRefresh != expectedRefresh {
		return auth.TokenClaims{}, auth.ErrInvalidCredential
	}
	return tokenClaims(
		claims.SessionID,
		claims.Subject,
		claims.ID,
		"",
		nil,
		0,
		*claims.IsRefresh,
		claims.RegisteredClaims,
	)
}

// 转换共同 Claims
func tokenClaims(
	sessionID string,
	subject auth.Kind,
	userID uint64,
	username string,
	roleIDs []uint64,
	passwordV int,
	isRefresh bool,
	registered jwtlib.RegisteredClaims,
) (auth.TokenClaims, error) {
	if registered.IssuedAt == nil || registered.NotBefore == nil || registered.ExpiresAt == nil {
		return auth.TokenClaims{}, auth.ErrInvalidCredential
	}
	return auth.TokenClaims{
		TokenSubject: auth.TokenSubject{
			SessionID: sessionID,
			Subject:   subject,
			UserID:    userID,
			Username:  username,
			RoleIDs:   append([]uint64(nil), roleIDs...),
			PasswordV: passwordV,
		},
		JTI:       registered.ID,
		IsRefresh: isRefresh,
		IssuedAt:  registered.IssuedAt.Time,
		NotBefore: registered.NotBefore.Time,
		ExpiresAt: registered.ExpiresAt.Time,
	}, nil
}

// 校验签发身份快照
func validateSubject(subject auth.TokenSubject) error {
	if strings.TrimSpace(subject.SessionID) == "" || subject.UserID == 0 {
		return exception.Core("JWT 签发身份无效")
	}
	switch subject.Subject {
	case auth.AdminKind:
		if strings.TrimSpace(subject.Username) == "" || subject.PasswordV <= 0 {
			return exception.Core("管理端 JWT 签发身份无效")
		}
		for _, roleID := range subject.RoleIDs {
			if roleID == 0 {
				return exception.Core("管理端 JWT 角色 ID 必须为正数")
			}
		}
	case auth.AppKind:
		if subject.Username != "" || subject.RoleIDs != nil || subject.PasswordV != 0 {
			return exception.Core("应用端 JWT 不能携带管理端字段")
		}
	default:
		return exception.Core("JWT 签发身份种类无效")
	}

	return nil
}

// 校验 JWT 服务已初始化
func (service *Service) validateReady() error {
	if service == nil || len(service.keys) == 0 || service.now == nil || service.newJTI == nil {
		return exception.Core("JWT 服务未初始化")
	}

	return nil
}

// 生成密码学随机 JTI
func randomJTI() (string, error) {
	content := make([]byte, 32)
	rand.Read(content)

	return hex.EncodeToString(content), nil
}

var (
	_ jwtlib.ClaimsValidator = (*AdminClaims)(nil)
	_ jwtlib.ClaimsValidator = (*AppClaims)(nil)
	_ auth.TokenCodec        = (*Service)(nil)
)
