package dto

import "github.com/toothdy/cool-admin-go-next/cool/exception"

// 日志保留天数更新请求
type LogKeepRequest struct {
   Value int64 `json:"value" v:"required#日志保留天数不能为空|min:1"`
}

/**
 * 校验日志保留天数请求
 * @returns 校验错误
 */
func (r LogKeepRequest) Validate() error {
   if r.Value <= 0 {
      return exception.Validate("日志保留天数必须大于0")
   }
   return nil
}