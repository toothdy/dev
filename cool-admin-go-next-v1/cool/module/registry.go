package module

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	coolMiddleware "github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	coolTask "github.com/toothdy/cool-admin-go-next/cool/task"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

// Runtime 表示随应用启动和停止的模块运行时
type Runtime interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// 构建模块 Runtime 时注入的依赖
type RuntimeDeps struct {
	Context      context.Context
	DB           gdb.DB
	Models       []entity.Definition
	Recycle      *recycle.Manager
	TaskHandlers []coolTask.HandlerDefinition
	AuthOptions  AuthOptions
	I18nOptions  I18nOptions
	CRUDOptions  CRUDOptions
	RedisDefault RedisDefaultConfig
}

// RecycleProvider 表示应用级唯一的回收站运行时提供器
type RecycleProvider func(RuntimeDeps) (*recycle.Manager, Runtime, error)

// 构建 controller 时注入的运行时依赖
type Deps struct {
	Context      context.Context
	DB           gdb.DB
	AuthManager  *security.Manager
	SessionStore security.SessionStore
	SSO          bool
	EPSProvider  ControllerProvider
	UploadDir    string
	// UploadDirectory 是供自动装配使用的强类型上传目录。
	UploadDirectory UploadDirectory
	Runtime         Runtime
	Recycle         *recycle.Manager
	Models          []entity.Definition
	AuthOptions     AuthOptions
	I18nOptions     I18nOptions
	CRUDOptions     CRUDOptions
	RedisDefault    RedisDefaultConfig
}

// 构建模块中间件时注入的运行时依赖
type MiddlewareDeps struct {
	Context       context.Context
	DB            gdb.DB
	AuthManager   *security.Manager
	SessionStore  security.SessionStore
	SSO           bool
	I18nEnabled   bool
	I18nLanguages []string
	Translator    coolMiddleware.Translator
	Controllers   []controller.Definition
	Runtime       Runtime
	Recycle       *recycle.Manager
	Models        []entity.Definition
	AuthOptions   AuthOptions
	I18nOptions   I18nOptions
	CRUDOptions   CRUDOptions
	RedisDefault  RedisDefaultConfig
}

// MiddlewareFactory 构建模块声明的中间件。
type MiddlewareFactory func(MiddlewareDeps) ([]coolMiddleware.Definition, error)

// ControllerFactory 构建模块声明的 Controller。
type ControllerFactory func(Deps) ([]controller.Definition, error)

// Spec 描述应用显式装配的模块声明。
type Spec struct {
	Key               string
	Name              string
	Description       string
	Order             int
	Configure         func(context.Context) error
	Models            []entity.Definition
	Controllers       ControllerFactory
	Runtime           func(RuntimeDeps) (Runtime, error)
	RecycleProvider   RecycleProvider
	GlobalMiddlewares MiddlewareFactory
	Middlewares       MiddlewareFactory
	TaskHandlers      []coolTask.HandlerDefinition
	DB                string
	Menu              string
}
