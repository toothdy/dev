package service

import (
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
)

var (
	_ func(gdb.DB, entity.Definition, entity.Definition, *recycle.Manager) *DictInfoService = NewDictInfoService
	_ func(gdb.DB, entity.Definition, entity.Definition, *recycle.Manager) *DictTypeService = NewDictTypeService
)

/**
 * 验证字典服务完整使用注入模型
 * @param t 测试上下文
 * @returns null
 */
func TestDictServicesUseInjectedModels(t *testing.T) {
	dictInfoModel := entity.Definition{TableName: "injected_dict_info"}
	dictTypeModel := entity.Definition{TableName: "injected_dict_type"}
	infoService := NewDictInfoService(nil, dictInfoModel, dictTypeModel, nil)
	if infoService.Model.TableName != dictInfoModel.TableName || infoService.typeModel.TableName != dictTypeModel.TableName {
		t.Fatalf("dict info service ignored injected models: %#v", infoService)
	}
	typeService := NewDictTypeService(nil, dictTypeModel, dictInfoModel, nil)
	if typeService.Model.TableName != dictTypeModel.TableName || typeService.infoModel.TableName != dictInfoModel.TableName {
		t.Fatalf("dict type service ignored injected models: %#v", typeService)
	}
}
