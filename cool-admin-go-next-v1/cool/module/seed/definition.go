package seed

import "fmt"

// 初始化数据类型
type Kind string

const (
	// db.json 初始化数据
	KindDB Kind = "db"
	// menu.json 初始化数据
	KindMenu Kind = "menu"
)

// 模块 seed 文件声明
type Definition struct {
	DBPath   string
	MenuPath string
}

// 创建 seed 定义
// @param dbPath DB 初始化文件路径
// @param menuPath 菜单初始化文件路径
// @returns seed 定义
func NewDefinition(dbPath string, menuPath string) Definition {
	return Definition{
		DBPath:   dbPath,
		MenuPath: menuPath,
	}
}

// 生成初始化标记键
// @param kind 初始化类型
// @param moduleName 模块名
// @returns 初始化标记键
func MarkerKey(kind Kind, moduleName string) string {
	return fmt.Sprintf("init_%s_%s", kind, moduleName)
}
