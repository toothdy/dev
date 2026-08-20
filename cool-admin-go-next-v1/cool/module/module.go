package module

import (
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

// 模块配置骨架
type Config struct {
	Description string
	Order       int
}

// cool 模块接口
type Module interface {
	Key() string
	NameText() string
	OrderValue() int
	ModuleConfig() Config
	ModuleModels() []entity.Definition
	ModuleSeeds() seed.Definition
	ModuleControllers() []controller.Definition
}

// 默认模块定义
type Definition struct {
	key         string
	name        string
	config      Config
	models      []entity.Definition
	seeds       seed.Definition
	controllers []controller.Definition
}

/**
 * 创建模块定义
 * @param key 模块标识
 * @returns *Definition
 */
func New(key string) *Definition {
	return &Definition{
		key: key,
		config: Config{
			Order: 0,
		},
	}
}

/**
 * 设置模块名称
 * @param name 模块名称
 * @returns *Definition
 */
func (d *Definition) Name(name string) *Definition {
	d.name = name
	return d
}

/**
 * 设置模块排序
 * @param order 排序值
 * @returns *Definition
 */
func (d *Definition) Order(order int) *Definition {
	d.config.Order = order
	return d
}

/**
 * 设置模块配置
 * @param config 模块配置
 * @returns *Definition
 */
func (d *Definition) Config(config Config) *Definition {
	d.config = config
	return d
}

/**
 * 设置模块模型
 * @param models 模型列表
 * @returns *Definition
 */
func (d *Definition) Models(models []entity.Definition) *Definition {
	d.models = append([]entity.Definition{}, models...)
	return d
}

/**
 * 设置模块 seed 文件
 * @param dbPath DB 初始化文件路径
 * @param menuPath 菜单初始化文件路径
 * @returns *Definition
 */
func (d *Definition) Seeds(dbPath string, menuPath string) *Definition {
	d.seeds = seed.NewDefinition(dbPath, menuPath)
	return d
}

/**
 * 模块标识
 * @returns string
 */
func (d *Definition) Key() string {
	return d.key
}

/**
 * 模块名称
 * @returns string
 */
func (d *Definition) NameText() string {
	return d.name
}

/**
 * 模块排序
 * @returns int
 */
func (d *Definition) OrderValue() int {
	return d.config.Order
}

/**
 * 模块配置
 * @returns Config
 */
func (d *Definition) ModuleConfig() Config {
	return d.config
}

/**
 * 模块模型
 * @returns []entity.Definition
 */
func (d *Definition) ModuleModels() []entity.Definition {
	return append([]entity.Definition{}, d.models...)
}

/**
 * 模块 seed 定义
 * @returns seed.Definition
 */
func (d *Definition) ModuleSeeds() seed.Definition {
	return d.seeds
}

/**
 * 设置模块 controller 元数据
 * @param controllers controller 元数据
 * @returns *Definition
 */
func (d *Definition) Controllers(controllers []controller.Definition) *Definition {
	d.controllers = cloneControllers(controllers)
	return d
}

/**
 * 模块 controller 元数据
 * @returns []controller.Definition
 */
func (d *Definition) ModuleControllers() []controller.Definition {
	return cloneControllers(d.controllers)
}

/**
 * 深复制 controller 元数据列表
 * @param controllers controller 元数据列表
 * @returns []controller.Definition
 */
func cloneControllers(controllers []controller.Definition) []controller.Definition {
	cloned := make([]controller.Definition, len(controllers))
	for index, definition := range controllers {
		cloned[index] = controller.CloneDefinition(definition)
	}
	return cloned
}

/**
 * 收集模块 controller 元数据
 * @param modules 模块列表
 * @returns []controller.Definition
 */
func CollectControllers(modules []Module) []controller.Definition {
	controllers := make([]controller.Definition, 0)
	for _, mod := range modules {
		controllers = append(controllers, mod.ModuleControllers()...)
	}
	return controllers
}
