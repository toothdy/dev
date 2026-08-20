package exception

import "github.com/gogf/gf/v2/errors/gerror"

// 返回仅由 cool/errors 构造的稳定错误分类
func KindOf(err error) (Kind, bool) {
	if err == nil {
		return "", false
	}
	detail, ok := gerror.Code(err).Detail().(codeDetail)
	if !ok {
		return "", false
	}
	return detail.Kind, true
}

// 错误边界可直接渲染的安全结果
type Resolved struct {
	Kind         Kind
	HTTPStatus   int
	BusinessCode int
	Message      string
	LogLevel     LogLevel
}

// 将任意错误解析为安全的传输结果
func Resolve(err error) Resolved {
	detail := internalDetail()
	if err != nil {
		if typed, ok := gerror.Code(err).Detail().(codeDetail); ok {
			detail = typed
		}
	}
	message := detail.DefaultMessage
	if detail.Public && err != nil && err.Error() != "" {
		// GoFrame Error() 包含完整 cause 链；公开响应只使用当前层消息。
		message = gerror.Current(err).Error()
	}
	if message == "" {
		message = "操作失败"
	}
	return Resolved{
		Kind:         detail.Kind,
		HTTPStatus:   detail.HTTPStatus,
		BusinessCode: detail.BusinessCode,
		Message:      message,
		LogLevel:     detail.LogLevel,
	}
}

func internalDetail() codeDetail {
	return codeInternal.Detail().(codeDetail)
}
