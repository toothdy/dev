package service

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth/bcrypt"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/seed"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

const initialAdminPassword = "123456"

// initializerLockKey 是 Base 在框架种子导入幂等表中的标记键。
const initializerLockKey = "base"

// Initializer 按业务唯一键补齐 Base 内置数据。通用的解析/树同步/幂等插入机制
// 由 cool-next/seed 提供；这里只保留无法通用化的编排：哪些表、什么顺序、
// 管理员初始口令哈希、用户与角色的名称级关系解析。
type Initializer struct {
	runtime    *coredb.Runtime
	lock       *seed.Store
	password   *bcrypt.Verifier
	conf       coreentity.Descriptor[entity.Conf, uint64]
	department coreentity.Descriptor[entity.Department, uint64]
	menu       coreentity.Descriptor[entity.Menu, uint64]
	param      coreentity.Descriptor[entity.Param, uint64]
	role       coreentity.Descriptor[entity.Role, uint64]
	user       coreentity.Descriptor[entity.User, uint64]
	userRole   coreentity.Descriptor[entity.UserRole, uint64]
}

// NewInitializer 创建 Base 幂等初始化器。
func NewInitializer(
	runtime *coredb.Runtime,
	conf coreentity.Descriptor[entity.Conf, uint64],
	department coreentity.Descriptor[entity.Department, uint64],
	menu coreentity.Descriptor[entity.Menu, uint64],
	param coreentity.Descriptor[entity.Param, uint64],
	role coreentity.Descriptor[entity.Role, uint64],
	user coreentity.Descriptor[entity.User, uint64],
	userRole coreentity.Descriptor[entity.UserRole, uint64],
) (*Initializer, error) {
	if runtime == nil || runtime.DB() == nil || runtime.Runner() == nil ||
		conf == nil || department == nil || menu == nil || param == nil || role == nil || user == nil || userRole == nil {
		return nil, exception.Core("Base 初始化器依赖无效")
	}
	lock, err := seed.NewStore(runtime)
	if err != nil {
		return nil, exception.WrapCore(err, "创建种子导入守卫失败")
	}
	password, err := bcrypt.New(bcrypt.Config{})
	if err != nil {
		return nil, exception.WrapCore(err, "创建初始化密码适配器失败")
	}

	return &Initializer{
		runtime: runtime, lock: lock, password: password, conf: conf, department: department,
		menu: menu, param: param, role: role, user: user, userRole: userRole,
	}, nil
}

// OnInit 在应用启动时补齐缺失的 Base 种子数据，整体幂等且事务化。
func (initializer *Initializer) OnInit(ctx context.Context) error {
	if initializer == nil || initializer.runtime == nil || initializer.lock == nil {
		return exception.Core("Base 初始化器未初始化")
	}
	seeds, err := parseInitialSeeds(base.DBSeed(), base.MenuSeed())
	if err != nil {
		return err
	}
	if err = initializer.lock.Prepare(ctx); err != nil {
		return err
	}

	return initializer.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		return initializer.lock.Guard(txCtx, initializerLockKey, func(guardCtx context.Context) error {
			return initializer.apply(guardCtx, seeds)
		})
	})
}

func (initializer *Initializer) apply(ctx context.Context, seeds initialSeeds) error {
	transaction, exists, err := initializer.runtime.Current(ctx)
	if err != nil || !exists || transaction == nil {
		return exception.WrapCore(err, "获取 Base 初始化事务失败")
	}

	if err = seed.InsertMissing(ctx, transaction, initializer.param, seeds.params, "keyName"); err != nil {
		return err
	}
	if err = seed.InsertMissing(ctx, transaction, initializer.conf, seeds.confs, "cKey"); err != nil {
		return err
	}
	departmentIDs, err := seed.SyncTree(ctx, transaction, initializer.department, seeds.departments)
	if err != nil {
		return err
	}
	if _, err = seed.SyncTree(ctx, transaction, initializer.menu, seeds.menus); err != nil {
		return err
	}
	if err = seed.InsertMissing(ctx, transaction, initializer.role, seeds.roles, "label"); err != nil {
		return err
	}
	departmentSourceIDs := make(map[uint64]uint64, len(seeds.departments))
	for _, department := range seeds.departments {
		oldID, exists := department.Record.Uint64("id")
		if exists {
			departmentSourceIDs[oldID] = departmentIDs[department.Key]
		}
	}
	if err = initializer.insertUsers(ctx, transaction, seeds.users, departmentSourceIDs); err != nil {
		return err
	}

	return initializer.insertUserRoles(ctx, transaction, seeds.userRoles, seeds.users, seeds.roles)
}

func (initializer *Initializer) insertUsers(
	ctx context.Context,
	transaction interface{ Model(...any) *gdb.Model },
	users []seed.Record,
	departmentIDs map[uint64]uint64,
) error {
	for _, record := range users {
		username, err := record.String("username")
		if err != nil {
			return err
		}
		model := transaction.Model(initializer.user.Table()).Ctx(ctx)
		existing, err := model.Where("username", username).One()
		if err != nil {
			return exception.WrapCore(err, "查询初始化用户失败")
		}
		if existing != nil {
			continue
		}
		values, err := record.Values(initializer.user)
		if err != nil {
			return err
		}
		password, err := initializer.password.Hash(initialAdminPassword)
		if err != nil {
			return exception.WrapCore(err, "生成初始管理员密码失败")
		}
		values["password"] = password
		if oldDepartment, exists := record.Uint64("departmentId"); exists {
			departmentID, found := departmentIDs[oldDepartment]
			if !found {
				return exception.Core("初始用户引用了未知部门")
			}
			values["departmentId"] = departmentID
		}
		data, dataErr := seed.NewDO(initializer.user, values, true)
		if dataErr != nil {
			return dataErr
		}
		if _, err = model.Data(data.DBData()).Insert(); err != nil {
			return exception.WrapCore(err, "写入初始化用户失败")
		}
	}

	return nil
}

func (initializer *Initializer) insertUserRoles(
	ctx context.Context,
	transaction interface{ Model(...any) *gdb.Model },
	relations []seed.Record,
	users []seed.Record,
	roles []seed.Record,
) error {
	userNames := make(map[uint64]string, len(users))
	for _, user := range users {
		id, ok := user.Uint64("id")
		if !ok {
			return exception.Core("初始用户缺少 ID")
		}
		name, err := user.String("username")
		if err != nil {
			return err
		}
		userNames[id] = name
	}
	roleLabels := make(map[uint64]string, len(roles))
	for _, role := range roles {
		id, ok := role.Uint64("id")
		if !ok {
			return exception.Core("初始角色缺少 ID")
		}
		label, err := role.String("label")
		if err != nil {
			return err
		}
		roleLabels[id] = label
	}
	for _, relation := range relations {
		oldUserID, hasUserID := relation.Uint64("userId")
		oldRoleID, hasRoleID := relation.Uint64("roleId")
		if !hasUserID || !hasRoleID {
			return exception.Core("初始用户角色关系无效")
		}
		userID, err := seed.FindID(ctx, transaction, initializer.user.Table(), "username", userNames[oldUserID])
		if err != nil {
			return err
		}
		roleID, err := seed.FindID(ctx, transaction, initializer.role.Table(), "label", roleLabels[oldRoleID])
		if err != nil {
			return err
		}
		model := transaction.Model(initializer.userRole.Table()).Ctx(ctx)
		existing, err := model.Where("userId", userID).Where("roleId", roleID).One()
		if err != nil {
			return exception.WrapCore(err, "查询初始化用户角色关系失败")
		}
		if existing != nil {
			continue
		}
		data, dataErr := seed.NewDO(initializer.userRole, map[string]any{"userId": userID, "roleId": roleID}, true)
		if dataErr != nil {
			return dataErr
		}
		if _, err = model.Data(data.DBData()).Insert(); err != nil {
			return exception.WrapCore(err, "写入初始化用户角色关系失败")
		}
	}

	return nil
}

// initialSeeds 是 Base db.json/menu.json 解析后按表分组的种子数据。
type initialSeeds struct {
	confs       []seed.Record
	departments []seed.TreeNode
	menus       []seed.TreeNode
	params      []seed.Record
	roles       []seed.Record
	userRoles   []seed.Record
	users       []seed.Record
}

func parseInitialSeeds(dbSeed, menuSeed []byte) (initialSeeds, error) {
	var database map[string][]seed.Record
	if err := json.Unmarshal(dbSeed, &database); err != nil {
		return initialSeeds{}, exception.WrapCore(err, "解析 Base 数据库种子失败")
	}
	seeds := initialSeeds{
		confs: database["base_sys_conf"], params: database["base_sys_param"], roles: database["base_sys_role"],
		users: database["base_sys_user"], userRoles: database["base_sys_user_role"],
	}
	departments, err := parseDepartmentSeeds(database["base_sys_department"])
	if err != nil {
		return initialSeeds{}, err
	}
	seeds.departments = departments
	var menus []seed.Record
	if err = json.Unmarshal(menuSeed, &menus); err != nil {
		return initialSeeds{}, exception.WrapCore(err, "解析 Base 菜单种子失败")
	}
	seeds.menus, err = parseSeedTree(menus, "")
	if err != nil {
		return initialSeeds{}, err
	}
	if len(seeds.departments) == 0 || len(seeds.menus) == 0 || len(seeds.roles) == 0 || len(seeds.users) == 0 {
		return initialSeeds{}, exception.Core("Base 种子数据不完整")
	}

	return seeds, nil
}

// parseSeedTree 解析菜单种子：以 childMenus 字段表达嵌套子节点。
func parseSeedTree(records []seed.Record, parentSeedKey string) ([]seed.TreeNode, error) {
	result := make([]seed.TreeNode, 0, len(records))
	for _, record := range records {
		seedKey, err := record.String("seedKey")
		if err != nil || seedKey == "" {
			return nil, exception.Core("初始化树节点缺少 seedKey")
		}
		if raw, exists := record["parentId"]; exists && parentSeedKey == "" && string(raw) != "null" {
			return nil, exception.Core("初始化部门根节点父级无效")
		}
		result = append(result, seed.TreeNode{Record: record, Key: seedKey, ParentKey: parentSeedKey})
		children, exists := record["childMenus"]
		if !exists || string(children) == "null" {
			continue
		}
		var childRecords []seed.Record
		if err = json.Unmarshal(children, &childRecords); err != nil {
			return nil, exception.WrapCore(err, "解析初始化菜单子节点失败")
		}
		nested, err := parseSeedTree(childRecords, seedKey)
		if err != nil {
			return nil, err
		}
		result = append(result, nested...)
	}

	return result, nil
}

// parseDepartmentSeeds 解析部门种子：扁平列表 + parentId 数字引用，转换为种子键归属。
func parseDepartmentSeeds(records []seed.Record) ([]seed.TreeNode, error) {
	keys := make(map[uint64]string, len(records))
	for _, record := range records {
		id, hasID := record.Uint64("id")
		seedKey, err := record.String("seedKey")
		if !hasID || err != nil {
			return nil, exception.Core("初始化部门数据无效")
		}
		if _, exists := keys[id]; exists {
			return nil, exception.Core("初始化部门 ID 重复")
		}
		keys[id] = seedKey
	}
	result := make([]seed.TreeNode, 0, len(records))
	for _, record := range records {
		seedKey, _ := record.String("seedKey")
		parentSeedKey := ""
		if parentID, hasParent := record.Uint64("parentId"); hasParent {
			var exists bool
			parentSeedKey, exists = keys[parentID]
			if !exists {
				return nil, exception.Core("初始化部门父级不存在")
			}
		}
		result = append(result, seed.TreeNode{Record: record, Key: seedKey, ParentKey: parentSeedKey})
	}

	return result, nil
}
