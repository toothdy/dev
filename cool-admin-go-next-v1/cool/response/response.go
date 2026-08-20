package response

import "github.com/toothdy/cool-admin-go-next/cool/exception"

// 响应体
type Body struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

/**
 * 成功响应
 * @param data 响应数据
 * @returns Body
 */
func OK(data interface{}) Body {
	return Body{
		Code:    exception.CodeSuccess,
		Message: "success",
		Data:    data,
	}
}

/**
 * 失败响应
 * @param message 错误消息
 * @param code 错误码
 * @returns Body
 */
func Fail(message string, code ...int) Body {
	failCode := exception.CodeCommFail
	if len(code) > 0 {
		failCode = code[0]
	}

	return Body{
		Code:    failCode,
		Message: message,
	}
}
