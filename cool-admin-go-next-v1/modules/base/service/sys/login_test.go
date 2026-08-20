package sys

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gcache"
	baseDTO "github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

func newCaptchaService() (*BaseSysLoginService, *gcache.Cache) {
	cache := gcache.New()
	return AuthServiceWithCache(nil, nil, cache), cache
}

func TestCaptchaBuildsRenderableDataURLAndCachesCode(t *testing.T) {
	service, cache := newCaptchaService()
	captcha, err := service.Captcha(context.Background(), 45, 150, "#2c3142")
	if err != nil {
		t.Fatalf("create captcha failed: %v", err)
	}
	if captcha.CaptchaID == "" || !strings.HasPrefix(captcha.Data, "data:image/svg+xml;base64,") {
		t.Fatalf("unexpected captcha: %#v", captcha)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(captcha.Data, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatalf("decode SVG failed: %v", err)
	}
	if !strings.Contains(string(svg), `width="150"`) || !strings.Contains(string(svg), `height="45"`) || !strings.Contains(string(svg), `fill="#2c3142"`) {
		t.Fatalf("unexpected SVG: %s", svg)
	}
	cached, err := cache.Get(context.Background(), captchaCacheKey(captcha.CaptchaID))
	if err != nil || cached == nil || len(cached.String()) != 4 {
		t.Fatalf("expected cached four-character code, got %#v, %v", cached, err)
	}
}

func TestCaptchaFallsBackForInvalidDimensionsAndColor(t *testing.T) {
	service, _ := newCaptchaService()
	captcha, err := service.Captcha(context.Background(), 0, -1, `"/><script>`)
	if err != nil {
		t.Fatalf("create fallback captcha failed: %v", err)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(captcha.Data, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatalf("decode SVG failed: %v", err)
	}
	if !strings.Contains(string(svg), `width="150"`) || !strings.Contains(string(svg), `height="50"`) || !strings.Contains(string(svg), `fill="#fff"`) || strings.Contains(string(svg), "<script>") {
		t.Fatalf("unexpected fallback SVG: %s", svg)
	}
}

func TestVerifyCaptchaIgnoresCaseAndConsumesMatchedCode(t *testing.T) {
	service, cache := newCaptchaService()
	if err := cache.Set(context.Background(), captchaCacheKey("captcha-id"), "aB12", time.Minute); err != nil {
		t.Fatalf("seed captcha failed: %v", err)
	}
	if err := service.verifyCaptcha(context.Background(), "captcha-id", "Ab12"); err != nil {
		t.Fatalf("expected matching captcha, got %v", err)
	}
	value, err := cache.Get(context.Background(), captchaCacheKey("captcha-id"))
	if err != nil || value != nil {
		t.Fatalf("expected consumed captcha, got %#v, %v", value, err)
	}
	if err = service.verifyCaptcha(context.Background(), "captcha-id", "aB12"); err == nil || err.Error() != "验证码不正确" {
		t.Fatalf("expected replay rejected, got %v", err)
	}
}

func TestVerifyCaptchaRejectsInvalidCodeWithoutDeletingIt(t *testing.T) {
	service, cache := newCaptchaService()
	if err := cache.Set(context.Background(), captchaCacheKey("captcha-id"), "1234", time.Minute); err != nil {
		t.Fatalf("seed captcha failed: %v", err)
	}
	if err := service.verifyCaptcha(context.Background(), "captcha-id", "4321"); err == nil || err.Error() != "验证码不正确" {
		t.Fatalf("expected invalid captcha error, got %v", err)
	}
	value, err := cache.Get(context.Background(), captchaCacheKey("captcha-id"))
	if err != nil || value == nil || value.String() != "1234" {
		t.Fatalf("expected original captcha retained, got %#v, %v", value, err)
	}
}

func TestLoginRejectsInvalidCaptchaBeforeDatabaseAccess(t *testing.T) {
	service, _ := newCaptchaService()
	_, err := service.Login(context.Background(), baseDTO.LoginReq{
		Username: "admin", Password: "123456", CaptchaID: "missing", VerifyCode: "1234",
	})
	if err == nil || err.Error() != "验证码不正确" {
		t.Fatalf("expected captcha error before database access, got %v", err)
	}
}
