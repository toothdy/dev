package sys

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

const (
	CodeLength    = 4          // 验证码长度
	DefaultTTL    = 30         // 验证码过期时间
	DefaultWidth  = 150        // 默认验证码宽度
	DefaultHeight = 50         // 默认验证码高度
	KeyPrefix     = "captcha:" // 验证码缓存键前缀
	DefaultColor  = "#fff"     // 默认验证码文字颜色
)

// 登录服务
type BaseSysLoginService struct {
	DB        gdb.DB
	Manager   *security.Manager
	Cache     *gcache.Cache
	Sessions  security.SessionStore
	SSO       bool
	captchaMu sync.Mutex
}

// 图片验证码响应
type Captcha struct {
	CaptchaID string `json:"captchaId"`
	Data      string `json:"data"`
}

/**
 * 创建认证服务
 * @param db 数据库实例
 * @param manager token 管理器
 * @param sessions 会话存储
 * @param options 应用级认证配置
 * @returns *AuthService
 */
func NewAuthService(db gdb.DB, manager *security.Manager, sessions security.SessionStore, options module.AuthOptions) *BaseSysLoginService {
	return AuthServiceWithDependencies(db, manager, gcache.New(), sessions, options)
}

/**
 * 创建带缓存的认证服务
 * @param db 数据库实例
 * @param manager token 管理器
 * @param cache 验证码缓存
 * @returns *AuthService
 */
func AuthServiceWithCache(db gdb.DB, manager *security.Manager, cache *gcache.Cache) *BaseSysLoginService {
	return AuthServiceWithDependencies(db, manager, cache, security.NewMemorySessionStore(), module.AuthOptions{})
}

// AuthServiceWithDependencies 创建共享验证码缓存和登录会话的认证服务。
func AuthServiceWithDependencies(db gdb.DB, manager *security.Manager, cache *gcache.Cache, sessions security.SessionStore, options module.AuthOptions) *BaseSysLoginService {
	if cache == nil {
		cache = gcache.New()
	}
	if sessions == nil {
		sessions = security.NewMemorySessionStore()
	}
	return &BaseSysLoginService{
		DB:       db,
		Manager:  manager,
		Cache:    cache,
		Sessions: sessions,
		SSO:      options.SSO,
	}
}

/**
 * 生成图片验证码
 * @param ctx 上下文
 * @param height 图片高度
 * @param width 图片宽度
 * @param color 文字颜色
 * @returns Captcha
 */
func (s *BaseSysLoginService) Captcha(ctx context.Context, height int, width int, color string) (Captcha, error) {
	height, width, color = GetDefaultCaptchaOptions(height, width, color)
	code, err := GetRandomCode(CodeLength)
	if err != nil {
		return Captcha{}, gerror.Wrap(err, "生成验证码失败")
	}
	captchaID, err := randomCaptchaID()
	if err != nil {
		return Captcha{}, gerror.Wrap(err, "生成验证码标识失败")
	}
	svg, err := buildCaptchaSVG(code, width, height, color)
	if err != nil {
		return Captcha{}, gerror.Wrap(err, "生成验证码图片失败")
	}
	if err = s.Cache.Set(ctx, captchaCacheKey(captchaID), strings.ToLower(code), DefaultTTL*time.Minute); err != nil {
		return Captcha{}, gerror.Wrap(err, "保存验证码失败")
	}
	return Captcha{
		CaptchaID: captchaID,
		Data:      "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)),
	}, nil
}

// 登录
func (s *BaseSysLoginService) Login(ctx context.Context, r dto.LoginReq) (security.TokenPair, error) {
	if err := s.verifyCaptcha(ctx, r.CaptchaID, r.VerifyCode); err != nil {
		return security.TokenPair{}, err
	}
	candidate, err := s.userByUsername(ctx, r.Username)
	if err != nil {
		return security.TokenPair{}, err
	}
	if len(candidate) == 0 {
		return security.TokenPair{}, exception.Comm("账户或密码不正确~")
	}
	userID := int64Value(candidate["id"])
	var pair security.TokenPair
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if lockErr := lockAuthorizationUsers(ctx, tx, []int64{userID}); lockErr != nil {
			if errors.Is(lockErr, errAuthorizationUserMissing) {
				return exception.Comm("账户或密码不正确~")
			}
			return lockErr
		}
		user, queryErr := userByIDFromTX(ctx, tx, userID)
		if queryErr != nil {
			return queryErr
		}
		validPassword := security.VerifyPassword(r.Password, stringValue(user["password"]))
		if stringValue(user["username"]) != r.Username || int64Value(user["status"]) != 1 || !validPassword {
			return exception.Comm("账户或密码不正确~")
		}
		roleIDs, roleErr := roleIDsByUserIDFromTX(ctx, tx, userID)
		if roleErr != nil {
			return roleErr
		}
		claims, claimsErr := claimsFromUserSnapshot(user, roleIDs)
		if errors.Is(claimsErr, errAuthorizationUserRoleless) {
			return exception.Comm("该用户未设置任何角色，无法登录~")
		}
		if claimsErr != nil {
			return exception.Comm("账户或密码不正确~")
		}
		pair, queryErr = s.Manager.GenerateTokenPair(claims)
		if queryErr != nil {
			return queryErr
		}
		return s.saveSession(ctx, pair, s.SSO)
	})
	if err != nil {
		return security.TokenPair{}, err
	}
	return pair, nil
}

/**
 * 刷新 token
 * @param ctx 上下文
 * @param refreshToken 刷新 token
 * @returns security.TokenPair
 */
func (s *BaseSysLoginService) RefreshToken(ctx context.Context, refreshToken string) (security.TokenPair, error) {
	if s == nil || s.Manager == nil || s.Sessions == nil {
		return security.TokenPair{}, exception.Internal(nil, "登录服务不可用")
	}
	claims, err := s.Manager.ParseRefreshToken(refreshToken)
	if err != nil {
		return security.TokenPair{}, exception.Unauthorized()
	}
	session, ok, sessionErr := s.Sessions.Get(ctx, claims.SessionID)
	if sessionErr != nil {
		return security.TokenPair{}, exception.Internal(sessionErr, "读取登录会话失败")
	}
	if !ok ||
		session.UserID != claims.UserId ||
		session.PasswordVersion != claims.PasswordVersion ||
		session.RefreshJTIHash != security.HashTokenID(claims.JTI) {
		return security.TokenPair{}, exception.Unauthorized()
	}
	var pair security.TokenPair
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if lockErr := lockAuthorizationUsers(ctx, tx, []int64{claims.UserId}); lockErr != nil {
			if errors.Is(lockErr, errAuthorizationUserMissing) {
				return exception.Unauthorized()
			}
			return lockErr
		}
		user, queryErr := userByIDFromTX(ctx, tx, claims.UserId)
		if queryErr != nil {
			return queryErr
		}
		roleIDs, roleErr := roleIDsByUserIDFromTX(ctx, tx, claims.UserId)
		if roleErr != nil {
			return roleErr
		}
		newClaims, claimsErr := claimsFromUserSnapshot(user, roleIDs)
		if claimsErr != nil {
			return exception.Unauthorized()
		}
		newClaims.SessionID = claims.SessionID
		pair, queryErr = s.Manager.GenerateTokenPair(newClaims)
		if queryErr != nil {
			return queryErr
		}
		next, sessionErr := sessionFromPair(s.Manager, pair)
		if sessionErr != nil {
			return exception.Internal(sessionErr, "解析新登录会话失败")
		}
		rotated, rotateErr := s.Sessions.Rotate(ctx, claims.SessionID, security.HashTokenID(claims.JTI), next)
		if rotateErr != nil {
			return exception.Internal(rotateErr, "轮换登录会话失败")
		}
		if !rotated {
			return exception.Unauthorized()
		}
		return nil
	})
	if err != nil {
		return security.TokenPair{}, err
	}
	return pair, nil
}

// 使当前用户会话失效
func (s *BaseSysLoginService) Logout(ctx context.Context, sessionID string) error {
	if s == nil || s.Sessions == nil {
		return nil
	}
	if err := s.Sessions.Delete(ctx, sessionID); err != nil {
		return exception.Internal(err, "注销登录会话失败")
	}
	return nil
}

func (s *BaseSysLoginService) saveSession(ctx context.Context, pair security.TokenPair, replaceUser bool) error {
	if s.Sessions == nil {
		return exception.Internal(nil, "登录会话存储不可用")
	}
	session, err := sessionFromPair(s.Manager, pair)
	if err != nil {
		return exception.Internal(err, "解析登录会话失败")
	}
	if replaceUser {
		err = s.Sessions.ReplaceUser(ctx, session.UserID, session)
	} else {
		err = s.Sessions.Save(ctx, session)
	}
	if err != nil {
		return exception.Internal(err, "保存登录会话失败")
	}
	return nil
}

func sessionFromPair(manager *security.Manager, pair security.TokenPair) (security.Session, error) {
	access, err := manager.ParseAccessToken(pair.Token)
	if err != nil {
		return security.Session{}, err
	}
	refresh, err := manager.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		return security.Session{}, err
	}
	if access.SessionID != refresh.SessionID || access.UserId != refresh.UserId {
		return security.Session{}, gerror.New("token pair 会话不一致")
	}
	return security.Session{
		ID:                  access.SessionID,
		UserID:              access.UserId,
		AccessJTIHash:       security.HashTokenID(access.JTI),
		RefreshJTIHash:      security.HashTokenID(refresh.JTI),
		PasswordVersion:     access.PasswordVersion,
		RefreshTokenExpires: time.Unix(refresh.ExpiresAt, 0),
	}, nil
}

// 验证验证码
func (s *BaseSysLoginService) verifyCaptcha(ctx context.Context, captchaID string, verifyCode string) error {
	if captchaID == "" || verifyCode == "" {
		return exception.Comm("验证码不正确")
	}
	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()

	value, err := s.Cache.Get(ctx, captchaCacheKey(captchaID))
	if err != nil {
		return gerror.Wrap(err, "读取验证码失败")
	}
	if value == nil || !strings.EqualFold(value.String(), verifyCode) {
		return exception.Comm("验证码不正确")
	}
	if _, err = s.Cache.Remove(ctx, captchaCacheKey(captchaID)); err != nil {
		return gerror.Wrap(err, "删除验证码失败")
	}
	return nil
}

func captchaCacheKey(captchaID string) string {
	return KeyPrefix + captchaID
}

// 默认验证码参数
func GetDefaultCaptchaOptions(height int, width int, color string) (int, int, string) {
	if height <= 1 {
		height = DefaultHeight
	}
	if width < 30 {
		width = DefaultWidth
	}
	if !IsHexColor(color) {
		color = DefaultColor
	}
	return height, width, color
}

// 是否为十六进制颜色
func IsHexColor(color string) bool {
	if (len(color) != 4 && len(color) != 7) || color[0] != '#' {
		return false
	}
	for _, char := range color[1:] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

// 随机验证码
func GetRandomCode(length int) (string, error) {
	if length <= 0 {
		return "", gerror.New("验证码长度错误")
	}
	// 随机验证码字符集
	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var builder strings.Builder
	builder.Grow(length)

	for index := 0; index < length; index++ {
		randomIndex, err := RandomInt(len(characters))
		if err != nil {
			return "", err
		}
		builder.WriteByte(characters[randomIndex])
	}

	return builder.String(), nil
}

func randomCaptchaID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// 随机整数
func RandomInt(max int) (int, error) {
	if max <= 0 {
		return 0, gerror.New("随机上限必须大于零")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func buildCaptchaSVG(text string, width int, height int, color string) (string, error) {
	var builder strings.Builder
	builder.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	builder.WriteString(strconv.Itoa(width))
	builder.WriteString(`" height="`)
	builder.WriteString(strconv.Itoa(height))
	builder.WriteString(`" viewBox="0 0 `)
	builder.WriteString(strconv.Itoa(width))
	builder.WriteByte(' ')
	builder.WriteString(strconv.Itoa(height))
	builder.WriteString(`">`)

	for index := 0; index < 3; index++ {
		x1, err := RandomInt(width)
		if err != nil {
			return "", err
		}
		y1, err := RandomInt(height)
		if err != nil {
			return "", err
		}
		x2, err := RandomInt(width)
		if err != nil {
			return "", err
		}
		y2, err := RandomInt(height)
		if err != nil {
			return "", err
		}
		controlX, err := RandomInt(width)
		if err != nil {
			return "", err
		}
		controlY, err := RandomInt(height)
		if err != nil {
			return "", err
		}
		grey, err := randomGreyColor()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, `<path d="M %d %d Q %d %d %d %d" stroke="%s" fill="none" stroke-width="1"/>`, x1, y1, controlX, controlY, x2, y2, grey)
	}

	fontSize := height * 7 / 10
	if fontSize < 1 {
		fontSize = 1
	}
	for index, digit := range text {
		baseX := (index + 1) * width / (len(text) + 1)
		xJitterLimit := width / 20
		if xJitterLimit < 1 {
			xJitterLimit = 1
		}
		xJitter, err := RandomInt(xJitterLimit)
		if err != nil {
			return "", err
		}
		rotation, err := RandomInt(21)
		if err != nil {
			return "", err
		}
		grey, err := randomGreyColor()
		if err != nil {
			return "", err
		}
		x := baseX + xJitter - xJitterLimit/2
		y := height/2 + fontSize/2
		fmt.Fprintf(&builder, `<text x="%d" y="%d" fill="%s" font-size="%d" text-anchor="middle" transform="rotate(%d %d %d)">%c</text>`, x, y, color, fontSize, rotation-10, x, y, digit)
		for noiseIndex := 0; noiseIndex < 2; noiseIndex++ {
			noiseXLimit := width / 12
			if noiseXLimit < 1 {
				noiseXLimit = 1
			}
			noiseX, err := RandomInt(noiseXLimit)
			if err != nil {
				return "", err
			}
			noiseY, err := RandomInt(height)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&builder, `<path d="M %d %d h 1" stroke="%s"/>`, x+noiseX-noiseXLimit/2, noiseY, grey)
		}
	}
	builder.WriteString(`</svg>`)
	return builder.String(), nil
}

func randomGreyColor() (string, error) {
	value, err := RandomInt(256)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("#%02x%02x%02x", value, value, value), nil
}

func (s *BaseSysLoginService) userByUsername(ctx context.Context, username string) (map[string]interface{}, error) {
	record, err := s.DB.GetOne(ctx, "SELECT id, departmentId AS departmentId, userId AS userId, name, username, password, passwordV AS passwordV, nickName AS nickName, headImg AS headImg, phone, email, remark, status, socketId AS socketId, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId FROM base_sys_user WHERE username = ? LIMIT 1", username)
	if err != nil {
		return nil, gerror.Wrap(err, "查询登录用户失败")
	}
	return recordValues(record), nil
}

func userByIDFromTX(ctx context.Context, tx gdb.TX, userID int64) (map[string]interface{}, error) {
	record, err := tx.Ctx(ctx).GetOne("SELECT id, departmentId AS departmentId, userId AS userId, name, username, password, passwordV AS passwordV, nickName AS nickName, headImg AS headImg, phone, email, remark, status, socketId AS socketId, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId FROM base_sys_user WHERE id = ? LIMIT 1 FOR UPDATE", userID)
	if err != nil {
		return nil, gerror.Wrap(err, "查询当前用户失败")
	}
	return recordValues(record), nil
}

func roleIDsByUserIDFromTX(ctx context.Context, tx gdb.TX, userID int64) ([]int64, error) {
	result, err := tx.Ctx(ctx).GetAll("SELECT roleId AS roleId FROM base_sys_user_role WHERE userId = ?", userID)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户角色失败")
	}
	roleIDs := make([]int64, 0, len(result))
	for _, record := range result {
		roleIDs = append(roleIDs, int64Value(recordValue(record, "roleId", "roleId")))
	}
	return roleIDs, nil
}

func recordValues(record gdb.Record) map[string]interface{} {
	values := make(map[string]interface{}, len(record))
	for key, value := range record {
		values[key] = value.Val()
	}
	return values
}

func recordValue(record gdb.Record, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return value.Val()
		}
	}
	return nil
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func int64Value(value interface{}) int64 {
	switch item := value.(type) {
	case int64:
		return item
	case int:
		return int64(item)
	case uint64:
		return int64(item)
	case []byte:
		var result int64
		_, _ = fmt.Sscan(string(item), &result)
		return result
	default:
		var result int64
		_, _ = fmt.Sscan(fmt.Sprintf("%v", item), &result)
		return result
	}
}
