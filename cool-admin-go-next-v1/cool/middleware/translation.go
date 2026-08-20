package middleware

// 模块可共享的响应翻译端口
type Translator interface {
	Translate(kind string, language string, text string) string
}
