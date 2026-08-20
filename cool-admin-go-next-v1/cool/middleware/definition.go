package middleware

import "github.com/gogf/gf/v2/net/ghttp"

// 中间件元数据
type Definition struct {
	Name    string
	Order   int
	Handler ghttp.HandlerFunc
	core    bool
}
