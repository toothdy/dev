package exception

import (
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
)

// 错误边界使用的稳定分类
type Kind string

const (
	KindComm         Kind = "comm"
	KindValidate     Kind = "validate"
	KindCore         Kind = "core"
	KindUnauthorized Kind = "unauthorized"
	KindForbidden    Kind = "forbidden"
	KindInternal     Kind = "internal"
)

// 错误边界日志级别
type LogLevel string

const (
	LogNone  LogLevel = "none"
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

type codeDetail struct {
	Kind           Kind
	HTTPStatus     int
	BusinessCode   int
	Public         bool
	DefaultMessage string
	LogLevel       LogLevel
}

var (
	codeComm = gcode.New(CodeCommFail, "Business Error", codeDetail{
		Kind: KindComm, HTTPStatus: http.StatusOK, BusinessCode: CodeCommFail, Public: true, LogLevel: LogWarn,
	})
	codeValidate = gcode.New(CodeValidateFail, "Validation Error", codeDetail{
		Kind: KindValidate, HTTPStatus: http.StatusOK, BusinessCode: CodeValidateFail, Public: true, LogLevel: LogDebug,
	})
	codeUnsupportedMedia = gcode.New(CodeValidateFail, "Unsupported Media Type", codeDetail{
		Kind: KindValidate, HTTPStatus: http.StatusUnsupportedMediaType,
		BusinessCode: CodeValidateFail, Public: true, LogLevel: LogDebug,
	})
	codePayloadTooLarge = gcode.New(CodeValidateFail, "Payload Too Large", codeDetail{
		Kind: KindValidate, HTTPStatus: http.StatusRequestEntityTooLarge,
		BusinessCode: CodeValidateFail, Public: true, LogLevel: LogDebug,
	})
	codeCore = gcode.New(CodeCoreFail, "Core Error", codeDetail{
		Kind: KindCore, HTTPStatus: http.StatusOK, BusinessCode: CodeCoreFail, Public: true, LogLevel: LogError,
	})
	codeUnauthorized = gcode.New(CodeCommFail, "Unauthorized", codeDetail{
		Kind: KindUnauthorized, HTTPStatus: http.StatusUnauthorized, BusinessCode: CodeCommFail,
		Public: true, DefaultMessage: "登录失效~", LogLevel: LogInfo,
	})
	codeForbidden = gcode.New(CodeCommFail, "Forbidden", codeDetail{
		Kind: KindForbidden, HTTPStatus: http.StatusForbidden, BusinessCode: CodeCommFail,
		Public: true, DefaultMessage: "登录失效或无权限访问~", LogLevel: LogInfo,
	})
	codeInternal = gcode.New(CodeCommFail, "Internal Error", codeDetail{
		Kind: KindInternal, HTTPStatus: http.StatusOK, BusinessCode: CodeCommFail,
		Public: false, DefaultMessage: "操作失败", LogLevel: LogError,
	})
)

// 创建可公开的业务错误
func Comm(message string) error {
	return gerror.NewCode(codeComm, message)
}

// 创建可公开的输入校验错误
func Validate(message string) error {
	return gerror.NewCode(codeValidate, message)
}

// 创建 HTTP 415 输入错误
func UnsupportedMediaType(message string) error {
	return gerror.NewCode(codeUnsupportedMedia, message)
}

// 创建 HTTP 413 输入错误
func PayloadTooLarge(message string) error {
	return gerror.NewCode(codePayloadTooLarge, message)
}

// 保留请求体超限的底层原因
func WrapPayloadTooLarge(err error, message string) error {
	if err == nil {
		return PayloadTooLarge(message)
	}
	return gerror.WrapCode(codePayloadTooLarge, err, message)
}

// 创建登录失效错误
func Unauthorized(message ...string) error {
	return gerror.NewCode(codeUnauthorized, firstMessage("登录失效~", message))
}

// 创建权限不足错误
func Forbidden(message ...string) error {
	return gerror.NewCode(codeForbidden, firstMessage("登录失效或无权限访问~", message))
}

// 创建启动或框架配置错误
func Core(message string) error {
	return gerror.NewCode(codeCore, message)
}

// 包装基础设施错误并保留 stack
func Internal(err error, operation string) error {
	operation = strings.TrimSpace(operation)
	if err == nil {
		if operation == "" {
			operation = "internal operation failed"
		}
		return gerror.NewCode(codeInternal, operation)
	}
	return gerror.WrapCode(codeInternal, err, operation)
}

func firstMessage(fallback string, messages []string) string {
	if len(messages) > 0 && strings.TrimSpace(messages[0]) != "" {
		return messages[0]
	}
	return fallback
}
