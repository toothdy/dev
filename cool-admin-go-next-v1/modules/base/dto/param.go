package dto

// 网页参数值请求
type HTMLReq struct {
	Key string `json:"key" v:"required#key不能为空"`
}

// 应用端参数请求
type ParamReq struct {
	Key string `json:"key" v:"required#key不能为空"`
}
