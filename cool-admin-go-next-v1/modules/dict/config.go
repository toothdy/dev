package dict

import "github.com/toothdy/cool-admin-go-next/cool/module"

// Config 表示 Dict 模块的纯业务配置。
type Config struct{}

// ModuleConfig 声明 Dict 模块元信息和默认配置。
func ModuleConfig() module.Declaration[Config] {
	return module.Declaration[Config]{
		Name:        "字典管理",
		Description: "数据字典等",
		Defaults:    Config{},
	}
}
