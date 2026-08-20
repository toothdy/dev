package seed

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/schema"
)

// TableName 是种子导入幂等标记的内部表名。
const TableName = "cool_seed_lock"

// UniqueKeyIndex 是 seedKey 唯一索引名。
const UniqueKeyIndex = "uk_cool_seed_lock_key"

// lockRecord 是 cool_seed_lock 表结构，用 entity.Compile 按标准 Descriptor 机制编译，
// 与 cool-next/db/recycle 的内部表模式一致，非业务实体，不经 cool generate 发现。
type lockRecord struct {
	g.Meta `orm:"table:cool_seed_lock" description:"种子导入幂等标记"`
	coreentity.Base
	SeedKey string `json:"seedKey" orm:"seedKey" description:"模块与种子类型标识" cool:"size=191"`
}

// Store 是种子导入幂等标记的存储与守卫。
type Store struct {
	runtime    *coredb.Runtime
	descriptor coreentity.Descriptor[lockRecord, uint64]
}

// NewStore 创建种子导入幂等守卫。
func NewStore(runtime *coredb.Runtime) (*Store, error) {
	if runtime == nil || runtime.DB() == nil || runtime.Runner() == nil {
		return nil, exception.Core("种子导入守卫依赖的框架数据库 Runtime 无效")
	}
	descriptor, err := coreentity.Compile[lockRecord, uint64](coreentity.Schema{
		Indexes: []coreentity.Index{coreentity.UniqueIndexOf(UniqueKeyIndex, "seedKey")},
	})
	if err != nil {
		return nil, exception.WrapCore(err, "构建种子导入标记 Descriptor 失败")
	}

	return &Store{runtime: runtime, descriptor: descriptor}, nil
}

// Prepare 确保 cool_seed_lock 表存在且结构匹配。种子导入是框架内部记账，不受业务
// schema.mode 策略约束——固定 Sync，与 cool_outbox/cool_inbox 只校验不同（那两张表
// 目前依赖仓库外的迁移流程置备；本包自成一体，不引入同样的外部前置依赖）。
func (store *Store) Prepare(ctx context.Context) error {
	if store == nil || store.runtime == nil {
		return exception.Core("种子导入守卫未初始化")
	}
	manager, err := schema.New(store.runtime.DB(), store.runtime.Dialect())
	if err != nil {
		return exception.WrapCore(err, "创建种子导入标记 Schema 管理器失败")
	}
	if _, err = manager.Apply(ctx, schema.Sync, store.descriptor); err != nil {
		return exception.WrapCore(err, "同步种子导入标记表失败")
	}

	return nil
}

// Guard 在当前框架事务内以 key 为幂等键执行 fn：key 已存在则跳过并返回 nil；
// 否则执行 fn，成功后写入标记，随事务一并提交或回滚。调用方须已处于
// runtime.Runner().Within 开启的框架事务中。
func (store *Store) Guard(ctx context.Context, key string, fn func(context.Context) error) error {
	if store == nil || store.runtime == nil {
		return exception.Core("种子导入守卫未初始化")
	}
	if key == "" {
		return exception.Core("种子导入幂等键不能为空")
	}
	transaction, exists, err := store.runtime.Current(ctx)
	if err != nil {
		return err
	}
	if !exists || transaction == nil {
		return exception.Core("Guard 必须在框架事务内调用")
	}
	existing, err := transaction.Model(TableName).Ctx(ctx).Where("seedKey", key).One()
	if err != nil {
		return exception.WrapCore(err, "查询种子导入标记失败")
	}
	if existing != nil {
		return nil
	}
	if err = fn(ctx); err != nil {
		return err
	}
	do := store.descriptor.NewDO()
	now := gtime.Now()
	if err = do.SetColumn("createTime", *now); err != nil {
		return exception.WrapCore(err, "设置种子导入标记失败")
	}
	if err = do.SetColumn("updateTime", *now); err != nil {
		return exception.WrapCore(err, "设置种子导入标记失败")
	}
	if err = do.SetColumn("seedKey", key); err != nil {
		return exception.WrapCore(err, "设置种子导入标记失败")
	}
	if _, err = transaction.Model(TableName).Ctx(ctx).Data(do.DBData()).Insert(); err != nil {
		return exception.WrapCore(err, "写入种子导入标记失败")
	}

	return nil
}
