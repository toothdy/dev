package service

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

/**
 * 测试基础服务保存模型定义
 * @param t 测试对象
 * @returns null
 */
func TestNewBaseStoresModel(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user")
	service := NewBase(nil, definition)

	if service.DB != nil {
		t.Fatal("expected nil DB to be stored as nil")
	}
	if service.Model.Name != "BaseSysUser" {
		t.Fatalf("expected model name BaseSysUser, got %s", service.Model.Name)
	}
	if service.Model.TableName != "base_sys_user" {
		t.Fatalf("expected table base_sys_user, got %s", service.Model.TableName)
	}
}
