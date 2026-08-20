package dto

import "github.com/gogf/gf/v2/net/ghttp"

// 文件上传请求(admin/app 共用)
type UploadReq struct {
	File *ghttp.UploadFile `type:"file" v:"required#上传文件为空"`
	Key  string            `json:"key" form:"key"`
}
