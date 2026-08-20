package recycle

import (
	"context"
	"net/url"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

const (
	maximumOperatorTypeLength = 32
	maximumOperatorIDLength   = 128
)

// 删除来源输入
type AuditInput struct {
	Source       string
	Params       url.Values
	OperatorType string
	OperatorID   string
}

// 已脱敏删除来源
type Audit struct {
	source       string
	params       string
	operatorType string
	operatorID   string
}

type auditContextKey struct{}

// 构造已脱敏删除来源
func NewAudit(input AuditInput) (Audit, error) {
	source, err := sanitizeSource(input.Source)
	if err != nil {
		return Audit{}, err
	}
	operatorType := strings.TrimSpace(input.OperatorType)
	operatorID := strings.TrimSpace(input.OperatorID)
	if len(operatorType) > maximumOperatorTypeLength {
		return Audit{}, gerror.New("操作者类型过长")
	}
	if len(operatorID) > maximumOperatorIDLength {
		return Audit{}, gerror.New("操作者 ID 过长")
	}

	return Audit{
		source:       source,
		params:       sanitizeParams(input.Params).Encode(),
		operatorType: operatorType,
		operatorID:   operatorID,
	}, nil
}

// 将删除来源写入上下文
func WithAudit(ctx context.Context, audit Audit) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, auditContextKey{}, audit)
}

func currentAudit(ctx context.Context) Audit {
	if ctx == nil {
		return Audit{}
	}
	audit, _ := ctx.Value(auditContextKey{}).(Audit)

	return audit
}

func sanitizeSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}
	parsed, err := url.ParseRequestURI(source)
	if err != nil {
		return "", gerror.Wrap(err, "删除来源 URL 无效")
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""

	return parsed.String(), nil
}

func sanitizeParams(params url.Values) url.Values {
	sanitized := make(url.Values, len(params))
	for key, values := range params {
		if isSensitiveKey(key) {
			continue
		}
		sanitized[key] = append([]string(nil), values...)
	}

	return sanitized
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	for _, sensitive := range []string{"authorization", "cookie", "password", "passwd", "secret", "token"} {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}

	return false
}
