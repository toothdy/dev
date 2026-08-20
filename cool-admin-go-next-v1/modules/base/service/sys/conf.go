package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/service"
)

// 系统配置服务
type ConfService struct {
	*service.Base
}

type confValueRow struct {
	CValue     interface{} `orm:"cValue"`
	UpdateTime string      `orm:"updateTime"`
}

// 创建系统配置服务
func NewConfService(db gdb.DB, baseSysConfModel entity.Definition) *ConfService {
	return &ConfService{Base: service.NewBase(db, baseSysConfModel)}
}

// 根据键读取配置值
func (s *ConfService) GetValue(ctx context.Context, key string) (interface{}, error) {
	value, err := s.DB.Model(s.Model.TableName).Ctx(ctx).Where("cKey", key).Value("cValue")
	if err != nil {
		return nil, gerror.Wrap(err, "查询系统配置失败")
	}
	if value == nil {
		return nil, nil
	}
	return value.Val(), nil
}

// 更新配置值
func (s *ConfService) UpdateValue(ctx context.Context, key string, value interface{}) error {
	row := confValueRow{
		CValue:     value,
		UpdateTime: mutationTimestamp(),
	}
	if _, err := s.DB.Model(s.Model.TableName).Ctx(ctx).Where("cKey", key).Data(row).Update(); err != nil {
		return gerror.Wrap(err, "更新系统配置失败")
	}
	return nil
}
