package service

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// 业务服务基类
type Base struct {
	DB    gdb.DB
	Model entity.Definition
}

// 默认 CRUD 修改前 hook
type ModifyBeforeHook interface {
	ModifyBefore(ctx context.Context, action string, data interface{}) error
}

// 默认 CRUD 修改后 hook
type ModifyAfterHook interface {
	ModifyAfter(ctx context.Context, action string, data interface{}) error
}

/**
 * 创建业务服务基类
 * @param db 数据库实例
 * @param definition 模型定义
 * @returns *Base
 */
func NewBase(db gdb.DB, definition entity.Definition) *Base {
	return &Base{
		DB:    db,
		Model: definition,
	}
}
