package dto

// 登录请求
type LoginReq struct {
	Username   string `json:"username" v:"required#用户名不能为空"`
	Password   string `json:"password" v:"required#密码不能为空"`
	CaptchaID  string `json:"captchaId" v:"required#非法操作"`
	VerifyCode string `json:"verifyCode" v:"required#验证码不能为空"`
}
