package dto

// 刷新 token 请求
type RefreshTokenReq struct {
	RefreshToken string `json:"refreshToken" v:"required#Token为空"`
}

// 图形验证码请求
type CaptchaReq struct {
	Height int    `json:"height"`
	Width  int    `json:"width"`
	Color  string `json:"color"`
}
