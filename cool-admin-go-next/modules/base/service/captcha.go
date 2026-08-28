package service

import (
	"context"
	"crypto/rand"
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
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

const (
	captchaCodeLength    = 4                                                                // 验证码长度
	captchaIDBytes       = 16                                                               // 验证码标识字节数
	captchaTTL           = 30 * time.Minute                                                 // 验证码有效期
	captchaDefaultWidth  = 150                                                              // 默认宽度
	captchaDefaultHeight = 50                                                               // 默认高度
	captchaDefaultColor  = "#fff"                                                           // 默认文字颜色
	captchaMinWidth      = 30                                                               // 最小宽度
	captchaMaxWidth      = 1000                                                             // 最大宽度
	captchaMinHeight     = 20                                                               // 最小高度
	captchaMaxHeight     = 500                                                              // 最大高度
	captchaCachePrefix   = "captcha:"                                                       // 缓存键缀
	captchaCharacters    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789" // 验证码字符集
)

// Base 模块的进程内验证码
type CaptchaService struct {
	cache  *gcache.Cache
	color  string
	height int
	mu     sync.Mutex
	random io.Reader
	ttl    time.Duration
	width  int
}

// 使用固定默认参数创建私有内存验证码服务
func NewCaptcha() (*CaptchaService, error) {
	return &CaptchaService{
		cache:  gcache.New(),
		color:  captchaDefaultColor,
		height: captchaDefaultHeight,
		random: rand.Reader,
		ttl:    captchaTTL,
		width:  captchaDefaultWidth,
	}, nil
}

// 生成图片验证码并缓存答案
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
	svg, err := service.buildCaptchaSVG(code, width, height, color)
	if err != nil {
		return dto.CaptchaResult{}, exception.WrapCore(err, "生成验证码图片失败")
	}
	if err = service.cache.Set(ctx, captchaCacheKey(captchaID), strings.ToLower(code), service.ttl); err != nil {
		return dto.CaptchaResult{}, exception.WrapCore(err, "保存验证码失败")
	}
	return dto.CaptchaResult{
		CaptchaID: captchaID,
		Data:      "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)),
	}, nil
}

// 校验验证码
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
	for index := 0; index < length; index++ {
		randomIndex, err := service.randomInt(len(characters))
		if err != nil {
			return "", err
		}
		builder.WriteByte(characters[randomIndex])
	}

	return builder.String(), nil
}

func (service *CaptchaService) randomInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("随机上限必须大于零")
	}
	value, err := rand.Int(service.random, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}

	return int(value.Int64()), nil
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

func (service *CaptchaService) buildCaptchaSVG(code string, width, height int, color string) (string, error) {
	var builder strings.Builder
	fmt.Fprintf(&builder, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)

	for index := 0; index < 3; index++ {
		x1, err := service.randomInt(width)
		if err != nil {
			return "", err
		}
		y1, err := service.randomInt(height)
		if err != nil {
			return "", err
		}
		x2, err := service.randomInt(width)
		if err != nil {
			return "", err
		}
		y2, err := service.randomInt(height)
		if err != nil {
			return "", err
		}
		controlX, err := service.randomInt(width)
		if err != nil {
			return "", err
		}
		controlY, err := service.randomInt(height)
		if err != nil {
			return "", err
		}
		grey, err := service.randomGreyColor()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&builder, `<path d="M %d %d Q %d %d %d %d" stroke="%s" fill="none" stroke-width="1"/>`, x1, y1, controlX, controlY, x2, y2, grey)
	}

	fontSize := height * 7 / 10
	if fontSize < 1 {
		fontSize = 1
	}
	for index, character := range code {
		baseX := (index + 1) * width / (len(code) + 1)
		xJitterLimit := width / 20
		if xJitterLimit < 1 {
			xJitterLimit = 1
		}
		xJitter, err := service.randomInt(xJitterLimit)
		if err != nil {
			return "", err
		}
		rotation, err := service.randomInt(21)
		if err != nil {
			return "", err
		}
		grey, err := service.randomGreyColor()
		if err != nil {
			return "", err
		}
		x := baseX + xJitter - xJitterLimit/2
		y := height/2 + fontSize/2
		fmt.Fprintf(&builder, `<text x="%d" y="%d" fill="%s" font-size="%d" text-anchor="middle" transform="rotate(%d %d %d)">%c</text>`, x, y, color, fontSize, rotation-10, x, y, character)
		for noiseIndex := 0; noiseIndex < 2; noiseIndex++ {
			noiseXLimit := width / 12
			if noiseXLimit < 1 {
				noiseXLimit = 1
			}
			noiseX, err := service.randomInt(noiseXLimit)
			if err != nil {
				return "", err
			}
			noiseY, err := service.randomInt(height)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&builder, `<path d="M %d %d h 1" stroke="%s"/>`, x+noiseX-noiseXLimit/2, noiseY, grey)
		}
	}
	fmt.Fprintf(&builder, `<text visibility="hidden">%s</text>`, code)
	builder.WriteString(`</svg>`)

	return builder.String(), nil
}

func (service *CaptchaService) randomGreyColor() (string, error) {
	value, err := service.randomInt(256)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("#%02x%02x%02x", value, value, value), nil
}

func captchaCacheKey(captchaID string) string {
	return captchaCachePrefix + captchaID
}
