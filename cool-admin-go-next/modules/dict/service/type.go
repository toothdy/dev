package service

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/dict/entity"
)

// 字典类别及关联信息删除
type TypeService struct {
	*coreservice.Base[entity.Type, uint64]
	infoBase *coreservice.Base[entity.Info, uint64]
}

// 字典类别业务服务
func NewType(
	typeBase *coreservice.Base[entity.Type, uint64],
	infoBase *coreservice.Base[entity.Info, uint64],
) (*TypeService, error) {
	if typeBase == nil || typeBase.Descriptor() == nil || infoBase == nil || infoBase.Descriptor() == nil {
		return nil, exception.Core("字典类别服务依赖无效")
	}

	return &TypeService{Base: typeBase, infoBase: infoBase}, nil
}

// 删除类别及其全部字典信息
func (service *TypeService) Delete(ctx context.Context, input coreservice.DeleteInput[uint64]) error {
	if err := service.Base.Delete(ctx, input); err != nil {
		return err
	}
	model, err := service.infoBase.Model(ctx)
	if err != nil {
		return err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields("id").WhereIn("typeId", input.IDs()).Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询类别字典信息失败")
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}
	deleteInput, err := coreservice.NewDeleteInput[entity.Info](service.infoBase.Descriptor(), ids)
	if err != nil {
		return err
	}

	return service.infoBase.Delete(ctx, deleteInput)
}
