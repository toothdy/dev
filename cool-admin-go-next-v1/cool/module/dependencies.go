package module

import (
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
)

// ControllerProvider 延迟提供应用的完整 Controller 集合。
type ControllerProvider func() []controller.Definition

// Controllers 读取当前 Controller 集合。
func (p ControllerProvider) Controllers() []controller.Definition {
	if p == nil {
		return nil
	}
	return p()
}

// UploadDirectory 表示应用上传文件的根目录。
type UploadDirectory string

// String 返回上传目录路径。
func (d UploadDirectory) String() string {
	return string(d)
}

// AuthOptions 保存应用级认证配置快照。
type AuthOptions struct {
	SSO bool
}

// I18nOptions 保存应用级国际化配置快照。
type I18nOptions struct {
	Enabled   bool
	Languages []string
}

// CRUDOptions 保存应用级 CRUD 配置快照。
type CRUDOptions struct {
	SoftDelete bool
}

// RedisDefaultConfig 保存 redis.default 的强类型配置快照。
type RedisDefaultConfig struct {
	Configured bool
	Config     gredis.Config
}
