package service

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/os/gcache"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

const (
	captchaCodeLength    = 4
	captchaIDBytes       = 16
	captchaTTL           = 30 * time.Minute
	captchaDefaultWidth  = 150
	captchaDefaultHeight = 50
	captchaDefaultColor  = "#fff"
	captchaMinWidth      = 30
	captchaMaxWidth      = 1000
	captchaMinHeight     = 20
	captchaMaxHeight     = 500
	captchaCachePrefix   = "captcha:"
	captchaCharacters    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// CaptchaService 提供 Base 模块的进程内验证码。
type CaptchaService struct {
	cache  *gcache.Cache
	color  string
	height int
	mu     sync.Mutex
	random io.Reader
	ttl    time.Duration
	width  int
}

// NewCaptcha 按 Base 配置创建使用私有内存缓存的验证码服务。
func NewCaptcha(config base.Config) (*CaptchaService, error) {
	captcha := config.Captcha
	if captcha.TTL <= 0 || captcha.Width < captchaMinWidth || captcha.Width > captchaMaxWidth ||
		captcha.Height < captchaMinHeight || captcha.Height > captchaMaxHeight || !isCaptchaColor(captcha.Color) {
		return nil, exception.Core("验证码配置无效")
	}

	return &CaptchaService{
		cache:  gcache.New(),
		color:  captcha.Color,
		height: captcha.Height,
		random: cryptorand.Reader,
		ttl:    captcha.TTL,
		width:  captcha.Width,
	}, nil
}

// Generate 生成图片验证码并缓存答案。
func (service *CaptchaService) Generate(ctx context.Context, query dto.CaptchaQuery) (dto.CaptchaResult, error) {
	if service == nil || service.cache == nil || service.random == nil {
		return dto.CaptchaResult{}, exception.Core("验证码服务未初始化")
	}
	width, height, color := service.normalizeOptions(query)
	code, err := service.randomString(captchaCodeLength, captchaCharacters)
	if err != nil {
		return dto.CaptchaResult{}, exception.WrapCore(err, "生成验证码失败")
	}
	captchaID, err := service.randomHex(captchaIDBytes)
	if err != nil {
		return dto.CaptchaResult{}, exception.WrapCore(err, "生成验证码标识失败")
	}
	if err = service.cache.Set(ctx, captchaCacheKey(captchaID), strings.ToLower(code), service.ttl); err != nil {
		return dto.CaptchaResult{}, exception.WrapCore(err, "保存验证码失败")
	}

	svg := buildCaptchaSVG(code, width, height, color)
	return dto.CaptchaResult{
		CaptchaID: captchaID,
		Data:      "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)),
	}, nil
}

// Verify 校验验证码，成功后立即消费。
func (service *CaptchaService) Verify(ctx context.Context, captchaID, verifyCode string) (bool, error) {
	if captchaID == "" || verifyCode == "" {
		return false, nil
	}
	if service == nil || service.cache == nil {
		return false, exception.Core("验证码服务未初始化")
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	value, err := service.cache.Get(ctx, captchaCacheKey(captchaID))
	if err != nil {
		return false, exception.WrapCore(err, "读取验证码失败")
	}
	if value == nil || !strings.EqualFold(value.String(), verifyCode) {
		return false, nil
	}
	if _, err = service.cache.Remove(ctx, captchaCacheKey(captchaID)); err != nil {
		return false, exception.WrapCore(err, "消费验证码失败")
	}

	return true, nil
}

func (service *CaptchaService) randomString(length int, characters string) (string, error) {
	var builder strings.Builder
	builder.Grow(length)
	limit := big.NewInt(int64(len(characters)))
	for index := 0; index < length; index++ {
		value, err := cryptorand.Int(service.random, limit)
		if err != nil {
			return "", err
		}
		builder.WriteByte(characters[value.Int64()])
	}

	return builder.String(), nil
}

func (service *CaptchaService) randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(service.random, value); err != nil {
		return "", err
	}

	return hex.EncodeToString(value), nil
}

func (service *CaptchaService) normalizeOptions(query dto.CaptchaQuery) (int, int, string) {
	width := query.Width
	if width < captchaMinWidth || width > captchaMaxWidth {
		width = service.width
	}
	height := query.Height
	if height < captchaMinHeight || height > captchaMaxHeight {
		height = service.height
	}
	color := query.Color
	if !isCaptchaColor(color) {
		color = service.color
	}

	return width, height, color
}

func isCaptchaColor(color string) bool {
	if (len(color) != 4 && len(color) != 7) || color[0] != '#' {
		return false
	}
	for _, character := range color[1:] {
		isNumber := character >= '0' && character <= '9'
		isLower := character >= 'a' && character <= 'f'
		isUpper := character >= 'A' && character <= 'F'
		if !isNumber && !isLower && !isUpper {
			return false
		}
	}

	return true
}

func buildCaptchaSVG(code string, width, height int, color string) string {
	fontSize := height * 3 / 5
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><text x="50%%" y="50%%" fill="%s" font-family="monospace" font-size="%d" font-weight="700" text-anchor="middle" dominant-baseline="middle">%s</text></svg>`,
		width,
		height,
		width,
		height,
		color,
		fontSize,
		code,
	)
}

func captchaCacheKey(captchaID string) string {
	return captchaCachePrefix + captchaID
}
