package module

// ComponentRef 表示生成期解析的组件函数引用。
type ComponentRef string

// Declaration 描述模块元信息、中间件与强类型默认配置。
type Declaration[T any] struct {
	Name              string
	Description       string
	Order             int
	Middlewares       []ComponentRef
	GlobalMiddlewares []ComponentRef
	Defaults          T
}
