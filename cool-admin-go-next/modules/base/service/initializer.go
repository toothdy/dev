package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth/bcrypt"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/base/data"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

const initialAdminPassword = "123456"

// Initializer 按业务唯一键补齐 Base 内置数据。
type Initializer struct {
	runtime    *coredb.Runtime
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
	password, err := bcrypt.New(bcrypt.Config{})
	if err != nil {
		return nil, exception.WrapCore(err, "创建初始化密码适配器失败")
	}

	return &Initializer{
		runtime: runtime, password: password, conf: conf, department: department,
		menu: menu, param: param, role: role, user: user, userRole: userRole,
	}, nil
}

// OnInit 在应用启动时补齐缺失的 Base 种子数据。
func (initializer *Initializer) OnInit(ctx context.Context) error {
	if initializer == nil || initializer.runtime == nil {
		return exception.Core("Base 初始化器未初始化")
	}
	seeds, err := parseInitialSeeds(data.DBSeed(), data.MenuSeed())
	if err != nil {
		return err
	}

	return initializer.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		return initializer.apply(txCtx, seeds)
	})
}

func (initializer *Initializer) apply(ctx context.Context, seeds initialSeeds) error {
	transaction, exists, err := initializer.runtime.Current(ctx)
	if err != nil || !exists || transaction == nil {
		return exception.WrapCore(err, "获取 Base 初始化事务失败")
	}

	if err = initializer.insertMissing(ctx, transaction, initializer.param, seeds.params, "keyName"); err != nil {
		return err
	}
	if err = initializer.insertMissing(ctx, transaction, initializer.conf, seeds.confs, "cKey"); err != nil {
		return err
	}
	departmentIDs, err := initializer.syncTree(ctx, transaction, initializer.department, seeds.departments)
	if err != nil {
		return err
	}
	if _, err = initializer.syncTree(ctx, transaction, initializer.menu, seeds.menus); err != nil {
		return err
	}
	if err = initializer.insertMissing(ctx, transaction, initializer.role, seeds.roles, "label"); err != nil {
		return err
	}
	departmentSourceIDs := make(map[uint64]uint64, len(seeds.departments))
	for _, department := range seeds.departments {
		oldID, exists := department.record.uint64("id")
		if exists {
			departmentSourceIDs[oldID] = departmentIDs[department.seedKey]
		}
	}
	if err = initializer.insertUsers(ctx, transaction, seeds.users, departmentSourceIDs); err != nil {
		return err
	}

	return initializer.insertUserRoles(ctx, transaction, seeds.userRoles, seeds.users, seeds.roles)
}

func (initializer *Initializer) insertMissing(
	ctx context.Context,
	transaction interface{ Model(...any) *gdb.Model },
	descriptor coreentity.RuntimeDescriptor,
	records []seedRecord,
	uniqueField string,
) error {
	for _, record := range records {
		value, err := record.string(uniqueField)
		if err != nil {
			return err
		}
		existing, err := transaction.Model(descriptor.Table()).Ctx(ctx).Where(uniqueField, value).One()
		if err != nil {
			return exception.WrapCore(err, "查询初始化记录失败")
		}
		if existing != nil {
			continue
		}
		data, dataErr := record.data(descriptor)
		if dataErr != nil {
			return dataErr
		}
		if _, err = transaction.Model(descriptor.Table()).Ctx(ctx).Data(data).Insert(); err != nil {
			return exception.WrapCore(err, "写入初始化记录失败")
		}
	}

	return nil
}

func (initializer *Initializer) syncTree(
	ctx context.Context,
	transaction interface{ Model(...any) *gdb.Model },
	descriptor coreentity.RuntimeDescriptor,
	nodes []seedTreeNode,
) (map[string]uint64, error) {
	ids := make(map[string]uint64, len(nodes))
	for len(ids) < len(nodes) {
		progressed := false
		for _, node := range nodes {
			if _, exists := ids[node.seedKey]; exists {
				continue
			}
			parentID, ready := uint64(0), node.parentSeedKey == ""
			if !ready {
				parentID, ready = ids[node.parentSeedKey]
			}
			if !ready {
				continue
			}
			values, valuesErr := node.record.values(descriptor)
			if valuesErr != nil {
				return nil, valuesErr
			}
			if node.parentSeedKey == "" {
				values["parentId"] = nil
			} else {
				values["parentId"] = parentID
			}
			model := transaction.Model(descriptor.Table()).Ctx(ctx)
			existing, err := model.Where("seedKey", node.seedKey).One()
			if err != nil {
				return nil, exception.WrapCore(err, "查询初始化树节点失败")
			}
			if existing == nil {
				data, dataErr := newDO(descriptor, values, true)
				if dataErr != nil {
					return nil, dataErr
				}
				id, insertErr := model.Data(data.DBData()).InsertAndGetId()
				if insertErr != nil {
					return nil, exception.WrapCore(insertErr, "写入初始化树节点失败")
				}
				if id <= 0 {
					return nil, exception.Core("初始化树节点返回了无效 ID")
				}
				ids[node.seedKey] = uint64(id)
			} else {
				id := existing["id"].Uint64()
				if id == 0 {
					return nil, exception.Core("初始化树节点缺少 ID")
				}
				data, dataErr := newDO(descriptor, values, false)
				if dataErr != nil {
					return nil, dataErr
				}
				if _, err = model.Where("id", id).Data(data.DBData()).Update(); err != nil {
					return nil, exception.WrapCore(err, "同步初始化树节点失败")
				}
				ids[node.seedKey] = id
			}
			progressed = true
		}
		if !progressed {
			return nil, exception.Core("初始化树存在缺失或循环父节点")
		}
	}

	return ids, nil
}

func (initializer *Initializer) insertUsers(
	ctx context.Context,
	transaction interface{ Model(...any) *gdb.Model },
	users []seedRecord,
	departmentIDs map[uint64]uint64,
) error {
	for _, record := range users {
		username, err := record.string("username")
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
		values, err := record.values(initializer.user)
		if err != nil {
			return err
		}
		password, err := initializer.password.Hash(initialAdminPassword)
		if err != nil {
			return exception.WrapCore(err, "生成初始管理员密码失败")
		}
		values["password"] = password
		if oldDepartment, exists := record.uint64("departmentId"); exists {
			departmentID, found := departmentIDs[oldDepartment]
			if !found {
				return exception.Core("初始用户引用了未知部门")
			}
			values["departmentId"] = departmentID
		}
		data, dataErr := newDO(initializer.user, values, true)
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
	relations []seedRecord,
	users []seedRecord,
	roles []seedRecord,
) error {
	userNames := make(map[uint64]string, len(users))
	for _, user := range users {
		id, ok := user.uint64("id")
		if !ok {
			return exception.Core("初始用户缺少 ID")
		}
		name, err := user.string("username")
		if err != nil {
			return err
		}
		userNames[id] = name
	}
	roleLabels := make(map[uint64]string, len(roles))
	for _, role := range roles {
		id, ok := role.uint64("id")
		if !ok {
			return exception.Core("初始角色缺少 ID")
		}
		label, err := role.string("label")
		if err != nil {
			return err
		}
		roleLabels[id] = label
	}
	for _, relation := range relations {
		oldUserID, hasUserID := relation.uint64("userId")
		oldRoleID, hasRoleID := relation.uint64("roleId")
		if !hasUserID || !hasRoleID {
			return exception.Core("初始用户角色关系无效")
		}
		userID, err := findID(ctx, transaction, initializer.user.Table(), "username", userNames[oldUserID])
		if err != nil {
			return err
		}
		roleID, err := findID(ctx, transaction, initializer.role.Table(), "label", roleLabels[oldRoleID])
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
		data, dataErr := newDO(initializer.userRole, map[string]any{"userId": userID, "roleId": roleID}, true)
		if dataErr != nil {
			return dataErr
		}
		if _, err = model.Data(data.DBData()).Insert(); err != nil {
			return exception.WrapCore(err, "写入初始化用户角色关系失败")
		}
	}

	return nil
}

func findID(ctx context.Context, transaction interface{ Model(...any) *gdb.Model }, table, field, value string) (uint64, error) {
	if value == "" {
		return 0, exception.Core("初始化关系引用无效")
	}
	record, err := transaction.Model(table).Ctx(ctx).Where(field, value).One()
	if err != nil {
		return 0, exception.WrapCore(err, "查询初始化关系失败")
	}
	if record == nil || record["id"].Uint64() == 0 {
		return 0, exception.Core("初始化关系引用不存在")
	}

	return record["id"].Uint64(), nil
}

func newDO(descriptor coreentity.RuntimeDescriptor, values map[string]any, isInsert bool) (coreentity.DOValue, error) {
	do := descriptor.NewDO()
	now := gtime.Now()
	if isInsert {
		if err := do.SetColumn("createTime", *now); err != nil {
			return nil, exception.WrapCore(err, "设置初始化创建时间失败")
		}
	}
	if err := do.SetColumn("updateTime", *now); err != nil {
		return nil, exception.WrapCore(err, "设置初始化更新时间失败")
	}
	for field, value := range values {
		if err := do.SetColumn(field, value); err != nil {
			return nil, exception.WrapCore(err, "构造初始化数据失败")
		}
	}

	return do, nil
}

type initialSeeds struct {
	confs       []seedRecord
	departments []seedTreeNode
	menus       []seedTreeNode
	params      []seedRecord
	roles       []seedRecord
	userRoles   []seedRecord
	users       []seedRecord
}

type seedRecord map[string]json.RawMessage

type seedTreeNode struct {
	record        seedRecord
	seedKey       string
	parentSeedKey string
}

func parseInitialSeeds(dbSeed, menuSeed []byte) (initialSeeds, error) {
	var database map[string][]seedRecord
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
	var menus []seedRecord
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

func parseSeedTree(records []seedRecord, parentSeedKey string) ([]seedTreeNode, error) {
	result := make([]seedTreeNode, 0, len(records))
	for _, record := range records {
		seedKey, err := record.string("seedKey")
		if err != nil || seedKey == "" {
			return nil, exception.Core("初始化树节点缺少 seedKey")
		}
		if raw, exists := record["parentId"]; exists && parentSeedKey == "" && string(raw) != "null" {
			return nil, exception.Core("初始化部门根节点父级无效")
		}
		result = append(result, seedTreeNode{record: record, seedKey: seedKey, parentSeedKey: parentSeedKey})
		children, exists := record["childMenus"]
		if !exists || string(children) == "null" {
			continue
		}
		var childRecords []seedRecord
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

func parseDepartmentSeeds(records []seedRecord) ([]seedTreeNode, error) {
	keys := make(map[uint64]string, len(records))
	for _, record := range records {
		id, hasID := record.uint64("id")
		seedKey, err := record.string("seedKey")
		if !hasID || err != nil {
			return nil, exception.Core("初始化部门数据无效")
		}
		if _, exists := keys[id]; exists {
			return nil, exception.Core("初始化部门 ID 重复")
		}
		keys[id] = seedKey
	}
	result := make([]seedTreeNode, 0, len(records))
	for _, record := range records {
		seedKey, _ := record.string("seedKey")
		parentSeedKey := ""
		if parentID, hasParent := record.uint64("parentId"); hasParent {
			var exists bool
			parentSeedKey, exists = keys[parentID]
			if !exists {
				return nil, exception.Core("初始化部门父级不存在")
			}
		}
		result = append(result, seedTreeNode{record: record, seedKey: seedKey, parentSeedKey: parentSeedKey})
	}

	return result, nil
}

func (record seedRecord) data(descriptor coreentity.RuntimeDescriptor) (any, error) {
	values, err := record.values(descriptor)
	if err != nil {
		return nil, err
	}
	do, err := newDO(descriptor, values, true)
	if err != nil {
		return nil, err
	}

	return do.DBData(), nil
}

func (record seedRecord) values(descriptor coreentity.RuntimeDescriptor) (map[string]any, error) {
	values := make(map[string]any, len(record))
	for name, raw := range record {
		field, exists := descriptor.JSON(name)
		if !exists || field.Primary() || field.SystemMaintained() {
			continue
		}
		value, err := decodeSeedValue(raw, field)
		if err != nil {
			return nil, err
		}
		values[field.Name()] = value
	}

	return values, nil
}

func decodeSeedValue(raw json.RawMessage, field coreentity.Field) (any, error) {
	target := reflect.New(field.GoType())
	if field.LogicalType() == coreentity.LogicalBool {
		if err := json.Unmarshal(raw, target.Interface()); err == nil {
			return target.Elem().Interface(), nil
		}
		var number int
		if err := json.Unmarshal(raw, &number); err == nil && (number == 0 || number == 1) {
			return number == 1, nil
		}
	}
	if field.LogicalType() == coreentity.LogicalJSON {
		var encoded string
		if json.Unmarshal(raw, &encoded) == nil && strings.EqualFold(strings.TrimSpace(encoded), "null") {
			return reflect.MakeSlice(field.GoType(), 0, 0).Interface(), nil
		}
	}
	if err := json.Unmarshal(raw, target.Interface()); err != nil {
		return nil, exception.WrapCore(err, fmt.Sprintf("解析初始化字段 %s 失败", field.Name()))
	}
	value := target.Elem()
	if field.GoType().Kind() == reflect.Pointer && !value.IsNil() {
		return value.Elem().Interface(), nil
	}

	return value.Interface(), nil
}

func (record seedRecord) string(name string) (string, error) {
	raw, exists := record[name]
	if !exists {
		return "", exception.Core("初始化数据缺少 " + name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", exception.Core("初始化数据 " + name + " 无效")
	}

	return value, nil
}

func (record seedRecord) uint64(name string) (uint64, bool) {
	raw, exists := record[name]
	if !exists || string(raw) == "null" {
		return 0, false
	}
	var value uint64
	if json.Unmarshal(raw, &value) != nil || value == 0 {
		return 0, false
	}

	return value, true
}
