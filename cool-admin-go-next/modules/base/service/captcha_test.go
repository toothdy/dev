package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

type captchaTestSVG struct {
	XMLName xml.Name             `xml:"svg"`
	Width   int                  `xml:"width,attr"`
	Height  int                  `xml:"height,attr"`
	ViewBox string               `xml:"viewBox,attr"`
	Paths   []captchaTestSVGPath `xml:"path"`
	Texts   []struct{}           `xml:"text"`
	Circles []struct{}           `xml:"circle"`
}

type captchaTestSVGPath struct {
	Fill          string `xml:"fill,attr"`
	Data          string `xml:"d,attr"`
	Stroke        string `xml:"stroke,attr"`
	StrokeWidth   string `xml:"stroke-width,attr"`
	StrokeOpacity string `xml:"stroke-opacity,attr"`
}

func TestCaptchaGenerateUsesDefaultsAndThirtyMinuteTTL(t *testing.T) {
	service := newCaptchaTestService(t)
	result, err := service.Generate(context.Background(), dto.CaptchaQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if decoded, decodeErr := hex.DecodeString(result.CaptchaID); decodeErr != nil || len(decoded) != captchaIDBytes {
		t.Fatalf("captchaId = %q, decode error = %v", result.CaptchaID, decodeErr)
	}

	decoded, content := decodeCaptchaSVG(t, result.Data)
	if decoded.Width != captchaDefaultWidth || decoded.Height != captchaDefaultHeight || decoded.ViewBox != "0 0 150 50" {
		t.Fatalf("captcha size = %dx%d, viewBox = %q", decoded.Width, decoded.Height, decoded.ViewBox)
	}
	value, err := service.cache.Get(context.Background(), captchaCacheKey(result.CaptchaID))
	if err != nil || value == nil || len(value.String()) != captchaCodeLength {
		t.Fatalf("cached captcha = %#v, error = %v", value, err)
	}
	if bytes.Contains(content, []byte("<text")) {
		t.Fatal("SVG contains text")
	}
	assertCaptchaPaths(t, decoded, captchaDefaultColor)
	expire, err := service.cache.GetExpire(context.Background(), captchaCacheKey(result.CaptchaID))
	if err != nil || expire < captchaTTL-time.Second || expire > captchaTTL {
		t.Fatalf("captcha TTL = %s, error = %v", expire, err)
	}
}

func TestCaptchaGenerateUsesCustomOptionsAndRejectsInvalidValues(t *testing.T) {
	service := newCaptchaTestService(t)
	minimum, err := service.Generate(context.Background(), dto.CaptchaQuery{
		Width:  captchaMinWidth,
		Height: captchaMinHeight,
		Color:  "#000",
	})
	if err != nil {
		t.Fatal(err)
	}
	minimumSVG, _ := decodeCaptchaSVG(t, minimum.Data)
	if minimumSVG.Width != captchaMinWidth || minimumSVG.Height != captchaMinHeight {
		t.Fatalf("minimum captcha size = %dx%d", minimumSVG.Width, minimumSVG.Height)
	}
	assertCaptchaPaths(t, minimumSVG, "#000")

	custom, err := service.Generate(context.Background(), dto.CaptchaQuery{
		Width:  150,
		Height: 45,
		Color:  "#2c3142",
	})
	if err != nil {
		t.Fatal(err)
	}
	customSVG, _ := decodeCaptchaSVG(t, custom.Data)
	if customSVG.Width != 150 || customSVG.Height != 45 {
		t.Fatalf("custom captcha size = %dx%d", customSVG.Width, customSVG.Height)
	}
	assertCaptchaPaths(t, customSVG, "#2c3142")

	invalid, err := service.Generate(context.Background(), dto.CaptchaQuery{
		Width:  captchaMaxWidth + 1,
		Height: captchaMaxHeight + 1,
		Color:  `"/><script>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidSVG, invalidContent := decodeCaptchaSVG(t, invalid.Data)
	if invalidSVG.Width != captchaDefaultWidth || invalidSVG.Height != captchaDefaultHeight ||
		bytes.Contains(invalidContent, []byte("<script>")) {
		t.Fatalf("fallback captcha size = %dx%d", invalidSVG.Width, invalidSVG.Height)
	}
	assertCaptchaPaths(t, invalidSVG, captchaDefaultColor)
}

func TestBuildCaptchaSVGDoesNotExposeCodeAndKeepsOneCurve(t *testing.T) {
	content, err := buildCaptchaSVG("Te5T", captchaDefaultWidth, captchaDefaultHeight, captchaDefaultColor)
	if err != nil {
		t.Fatal(err)
	}
	var decoded captchaTestSVG
	if err = xml.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(content)), "te5t") {
		t.Fatal("SVG contains plaintext captcha code")
	}
	assertCaptchaPaths(t, decoded, captchaDefaultColor)
}

func TestCaptchaVerifyRetainsFailureAndConsumesSuccess(t *testing.T) {
	service := newCaptchaTestService(t)
	ctx := context.Background()
	key := captchaCacheKey("verify-once")
	if err := service.cache.Set(ctx, key, "ab12", time.Minute); err != nil {
		t.Fatal(err)
	}

	matched, err := service.Verify(ctx, "verify-once", "wrong")
	if err != nil || matched {
		t.Fatalf("wrong code matched = %t, error = %v", matched, err)
	}
	value, err := service.cache.Get(ctx, key)
	if err != nil || value == nil || value.String() != "ab12" {
		t.Fatalf("captcha after failure = %#v, error = %v", value, err)
	}

	matched, err = service.Verify(ctx, "verify-once", "AB12")
	if err != nil || !matched {
		t.Fatalf("correct code matched = %t, error = %v", matched, err)
	}
	value, err = service.cache.Get(ctx, key)
	if err != nil || value != nil {
		t.Fatalf("captcha after success = %#v, error = %v", value, err)
	}
	matched, err = service.Verify(ctx, "verify-once", "ab12")
	if err != nil || matched {
		t.Fatalf("replayed code matched = %t, error = %v", matched, err)
	}
}

func TestCaptchaVerifyEmptyInputDoesNotConsume(t *testing.T) {
	service := newCaptchaTestService(t)
	ctx := context.Background()
	key := captchaCacheKey("empty-input")
	if err := service.cache.Set(ctx, key, "1234", time.Minute); err != nil {
		t.Fatal(err)
	}

	for _, input := range [][2]string{{"", "1234"}, {"empty-input", ""}} {
		matched, err := service.Verify(ctx, input[0], input[1])
		if err != nil || matched {
			t.Fatalf("Verify(%q, %q) = %t, %v", input[0], input[1], matched, err)
		}
	}
	value, err := service.cache.Get(ctx, key)
	if err != nil || value == nil || value.String() != "1234" {
		t.Fatalf("captcha after empty input = %#v, error = %v", value, err)
	}
}

func TestCaptchaConcurrentVerifyAllowsOneSuccess(t *testing.T) {
	service := newCaptchaTestService(t)
	ctx := context.Background()
	if err := service.cache.Set(ctx, captchaCacheKey("concurrent"), "a1b2", time.Minute); err != nil {
		t.Fatal(err)
	}

	const workers = 32
	var (
		ready     sync.WaitGroup
		start     = make(chan struct{})
		done      sync.WaitGroup
		successes atomic.Int32
		errors    = make(chan error, workers)
	)
	ready.Add(workers)
	done.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			matched, err := service.Verify(ctx, "concurrent", "A1B2")
			if err != nil {
				errors <- err
				return
			}
			if matched {
				successes.Add(1)
			}
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if successes.Load() != 1 {
		t.Fatalf("successful verifications = %d, want 1", successes.Load())
	}
}

func newCaptchaTestService(t *testing.T) *CaptchaService {
	t.Helper()

	service, err := NewCaptcha()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.cache.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})

	return service
}

func decodeCaptchaSVG(t *testing.T, data string) (captchaTestSVG, []byte) {
	t.Helper()

	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(data, prefix) {
		t.Fatalf("captcha data URL = %q", data)
	}
	content, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, prefix))
	if err != nil {
		t.Fatal(err)
	}
	var decoded captchaTestSVG
	if err = xml.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}

	return decoded, content
}

func assertCaptchaPaths(t *testing.T, value captchaTestSVG, expectedColor string) {
	t.Helper()

	if len(value.Paths) != captchaCodeLength+1 {
		t.Fatalf("captcha paths = %d, want %d", len(value.Paths), captchaCodeLength+1)
	}
	if len(value.Texts) != 0 || len(value.Circles) != 0 {
		t.Fatalf("captcha text elements = %d, circles = %d", len(value.Texts), len(value.Circles))
	}
	curve := value.Paths[0]
	if curve.Fill != "none" || curve.Data == "" || curve.Stroke != expectedColor ||
		curve.StrokeWidth != captchaCurveWidth || curve.StrokeOpacity != captchaCurveOpacity {
		t.Fatalf("captcha curve = %#v", curve)
	}
	for index, path := range value.Paths[1:] {
		if path.Fill != expectedColor || path.Data == "" || path.Stroke != "" {
			t.Fatalf("captcha path %d = %#v", index, path)
		}
	}
}
