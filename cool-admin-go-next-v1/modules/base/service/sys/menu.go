package sys

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

// 系统菜单服务
type MenuService struct {
	*service.Base
	roleModel     entity.Definition
	roleMenuModel entity.Definition
	recycle       *recycle.Manager
}

// Node 兼容菜单导出节点(已迁移至 dto.dto.MenuExportItem,见 modules/base/dto/menu.go)
// Node 兼容菜单导入节点(已迁移至 dto.dto.MenuImportItem,见 modules/base/dto/menu.go)

type menuImportRow struct {
	ParentID   *int64  `orm:"parentId"`
	TenantID   *int64  `orm:"tenantId"`
	Name       string  `orm:"name"`
	Router     *string `orm:"router"`
	Perms      *string `orm:"perms"`
	Type       *int    `orm:"type"`
	Icon       *string `orm:"icon"`
	OrderNum   *int    `orm:"orderNum"`
	ViewPath   *string `orm:"viewPath"`
	KeepAlive  *bool   `orm:"keepAlive"`
	IsShow     *bool   `orm:"isShow"`
	CreateTime string  `orm:"createTime"`
	UpdateTime string  `orm:"updateTime"`
}

type menuMutationRow struct {
	ParentID   interface{} `orm:"parentId"`
	TenantID   interface{} `orm:"tenantId"`
	Name       interface{} `orm:"name"`
	Router     interface{} `orm:"router"`
	Perms      interface{} `orm:"perms"`
	Type       interface{} `orm:"type"`
	Icon       interface{} `orm:"icon"`
	OrderNum   interface{} `orm:"orderNum"`
	ViewPath   interface{} `orm:"viewPath"`
	KeepAlive  interface{} `orm:"keepAlive"`
	IsShow     interface{} `orm:"isShow"`
	CreateTime interface{} `orm:"createTime"`
	UpdateTime interface{} `orm:"updateTime"`
}

type menuExportNode struct {
	item dto.MenuExportItem
}

/**
 * 构建菜单局部更新数据
 * @param data 请求数据
 * @returns 更新行、数据库字段和校验错误
 */
func menuUpdateMutation(data map[string]interface{}) (menuMutationRow, []string, error) {
	row := menuMutationRow{}
	fields := make([]string, 0, 11)
	if value, ok := data["parentId"]; ok {
		row.ParentID = value
		fields = append(fields, "parentId")
	}
	if value, ok := data["name"]; ok {
		name, valid := value.(string)
		name = strings.TrimSpace(name)
		if !valid || name == "" {
			return menuMutationRow{}, nil, exception.Validate("name不能为空")
		}
		row.Name = name
		fields = append(fields, "name")
	}
	if value, ok := data["router"]; ok {
		row.Router = value
		fields = append(fields, "router")
	}
	if value, ok := data["perms"]; ok {
		row.Perms = value
		fields = append(fields, "perms")
	}
	if value, ok := data["type"]; ok {
		if value == nil {
			return menuMutationRow{}, nil, exception.Validate("type不能为空")
		}
		row.Type = value
		fields = append(fields, "type")
	}
	if value, ok := data["icon"]; ok {
		row.Icon = value
		fields = append(fields, "icon")
	}
	if value, ok := data["orderNum"]; ok {
		if value == nil {
			return menuMutationRow{}, nil, exception.Validate("orderNum不能为空")
		}
		row.OrderNum = value
		fields = append(fields, "orderNum")
	}
	if value, ok := data["viewPath"]; ok {
		row.ViewPath = value
		fields = append(fields, "viewPath")
	}
	if value, ok := data["keepAlive"]; ok {
		if value == nil {
			return menuMutationRow{}, nil, exception.Validate("keepAlive不能为空")
		}
		row.KeepAlive = value
		fields = append(fields, "keepAlive")
	}
	if value, ok := data["isShow"]; ok {
		if value == nil {
			return menuMutationRow{}, nil, exception.Validate("isShow不能为空")
		}
		row.IsShow = value
		fields = append(fields, "isShow")
	}
	return row, fields, nil
}

/**
 * 创建系统菜单服务
 * @param db 数据库实例
 * @param baseSysMenuModel 菜单模型
 * @param baseSysRoleModel 角色模型
 * @param baseSysRoleMenuModel 角色菜单关系模型
 * @param recycleManager 回收站协调器
 * @returns *MenuService
 */
func NewMenuService(
	db gdb.DB,
	baseSysMenuModel entity.Definition,
	baseSysRoleModel entity.Definition,
	baseSysRoleMenuModel entity.Definition,
	recycleManager *recycle.Manager,
) *MenuService {
	return &MenuService{
		Base:   service.NewBase(db, baseSysMenuModel),
		recycle:       recycleManager,
		roleModel:     baseSysRoleModel,
		roleMenuModel: baseSysRoleMenuModel,
	}
}

// 新增菜单，并应用 Node 实体的默认字段值
func (s *MenuService) Add(ctx context.Context, request crud.AddRequest) (interface{}, error) {
	applyTenantMutation(ctx, request.Data)
	for field := range request.Data {
		switch field {
		case "parentId", "tenantId", "name", "router", "perms", "type", "icon", "orderNum", "viewPath", "keepAlive", "isShow":
		default:
			return nil, exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	if strings.TrimSpace(fmt.Sprint(request.Data["name"])) == "" || fmt.Sprint(request.Data["name"]) == "<nil>" {
		return nil, exception.Validate("name不能为空")
	}
	if _, ok := request.Data["type"]; !ok {
		request.Data["type"] = 0
	}
	if _, ok := request.Data["orderNum"]; !ok {
		request.Data["orderNum"] = 0
	}
	if _, ok := request.Data["keepAlive"]; !ok {
		request.Data["keepAlive"] = true
	}
	if _, ok := request.Data["isShow"]; !ok {
		request.Data["isShow"] = true
	}
	row := menuMutationRow{
		ParentID: request.Data["parentId"], TenantID: request.Data["tenantId"], Name: request.Data["name"],
		Router: request.Data["router"], Perms: request.Data["perms"], Type: request.Data["type"], Icon: request.Data["icon"],
		OrderNum: request.Data["orderNum"], ViewPath: request.Data["viewPath"], KeepAlive: request.Data["keepAlive"], IsShow: request.Data["isShow"],
	}
	now := mutationTimestamp()
	row.CreateTime = now
	row.UpdateTime = now
	parentID := int64Value(row.ParentID)
	if parentID <= 0 {
		dbModel, err := tenant.ScopedModel(ctx, s.DB, s.Model, "")
		if err != nil {
			return nil, err
		}
		id, err := dbModel.Data(row).InsertAndGetId()
		if err != nil {
			return nil, gerror.Wrap(err, "新增菜单失败")
		}
		return map[string]interface{}{"id": id}, nil
	}
	if _, err := tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return nil, err
	}
	var id int64
	err := s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if parentErr := s.ensureMenuParent(ctx, tx, parentID, 0); parentErr != nil {
			return parentErr
		}
		dbModel, modelErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if modelErr != nil {
			return modelErr
		}
		id, modelErr = dbModel.Data(row).InsertAndGetId()
		if modelErr != nil {
			return gerror.Wrap(modelErr, "新增菜单失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": id}, nil
}

// 只修改当前租户的菜单
func (s *MenuService) Update(ctx context.Context, request crud.UpdateRequest) (interface{}, error) {
	delete(request.Data, "tenantId")
	delete(request.Data, "createTime")
	delete(request.Data, "updateTime")
	for field := range request.Data {
		switch field {
		case "id", "parentId", "name", "router", "perms", "type", "icon", "orderNum", "viewPath", "keepAlive", "isShow":
		default:
			return nil, exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	id := int64Value(request.Data["id"])
	if id <= 0 {
		return nil, exception.Validate("id参数错误")
	}
	row, fields, err := menuUpdateMutation(request.Data)
	if err != nil {
		return nil, err
	}
	if row.ParentID != nil && int64Value(row.ParentID) == id {
		return nil, exception.Validate("上级菜单不能是自身")
	}
	row.UpdateTime = mutationTimestamp()
	fields = append(fields, "updateTime")
	if _, err := tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return nil, err
	}
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		query, queryErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		current, queryErr := query.Fields("id").Where("id", id).LockUpdate().One()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询菜单失败")
		}
		if current.IsEmpty() {
			return exception.Comm("菜单不存在")
		}
		if parentID := int64Value(row.ParentID); parentID > 0 {
			if parentErr := s.ensureMenuParent(ctx, tx, parentID, id); parentErr != nil {
				return parentErr
			}
		}
		query, queryErr = tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		if _, queryErr = query.Fields(fields).Where("id", id).Data(row).Update(); queryErr != nil {
			return gerror.Wrap(queryErr, "修改菜单失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

/**
 * 导出选中菜单树
 * @param ctx 请求上下文
 * @param ids 菜单 ID
 * @returns 菜单树
 */
func (s *MenuService) Export(ctx context.Context, ids []int64) ([]dto.MenuExportItem, error) {
	if s == nil || s.Base == nil || s.DB == nil {
		return nil, exception.Internal(nil, "菜单服务不可用")
	}
	if len(ids) == 0 {
		return []dto.MenuExportItem{}, nil
	}

	condition, err := s.menuTenantCondition(ctx, s.Model, "")
	if err != nil {
		return nil, err
	}
	where := "id IN(?)"
	args := []interface{}{ids}
	if condition.SQL != "" {
		where += " AND " + condition.SQL
		args = append(args, condition.Args...)
	}
	rows, err := s.DB.GetAll(ctx, "SELECT id, parentId AS parentId, tenantId AS tenantId, name, router, perms, type, icon, orderNum AS orderNum, viewPath AS viewPath, keepAlive AS keepAlive, isShow AS isShow FROM base_sys_menu WHERE "+where+" ORDER BY orderNum ASC, id ASC", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询导出菜单失败")
	}

	nodes := make(map[int64]menuExportNode, len(rows))
	children := make(map[int64][]int64)
	roots := make([]int64, 0)
	for _, row := range rows {
		id := row["id"].Int64()
		parentID := optionalInt64(row["parentId"].Val())
		nodes[id] = menuExportNode{item: dto.MenuExportItem{
			TenantID:   optionalInt64(row["tenantId"].Val()),
			Name:       row["name"].String(),
			Router:     optionalString(row["router"].Val()),
			Perms:      optionalString(row["perms"].Val()),
			Type:       row["type"].Int(),
			Icon:       optionalString(row["icon"].Val()),
			OrderNum:   row["orderNum"].Int(),
			ViewPath:   optionalString(row["viewPath"].Val()),
			KeepAlive:  row["keepAlive"].Bool(),
			IsShow:     row["isShow"].Bool(),
			ChildMenus: []dto.MenuExportItem{},
		}}
		if parentID == nil {
			roots = append(roots, id)
			continue
		}
		children[*parentID] = append(children[*parentID], id)
	}

	result := make([]dto.MenuExportItem, 0, len(roots))
	for _, id := range roots {
		item, buildErr := buildMenuExportItem(id, nodes, children, map[int64]bool{})
		if buildErr != nil {
			return nil, buildErr
		}
		result = append(result, item)
	}
	return result, nil
}

/**
 * 导入菜单树
 * @param ctx 请求上下文
 * @param menus 菜单树
 * @returns 导入错误
 */
func (s *MenuService) Import(ctx context.Context, menus []dto.MenuImportItem) error {
	if s == nil || s.Base == nil || s.DB == nil {
		return exception.Internal(nil, "菜单服务不可用")
	}
	if len(menus) == 0 {
		return exception.Validate("导入菜单不能为空")
	}
	if _, err := tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return err
	}
	var tenantID *int64
	if value, ok := contextTenantID(ctx); ok {
		tenantID = &value
	}
	menus = applyMenuTenant(menus, tenantID)
	count := 0
	for _, menu := range menus {
		if err := validateMenuImportItem(menu, 1, &count); err != nil {
			return err
		}
	}

	return s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, menu := range menus {
			if err := s.importMenuItem(ctx, tx, menu, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

func applyMenuTenant(menus []dto.MenuImportItem, tenantID *int64) []dto.MenuImportItem {
	result := make([]dto.MenuImportItem, len(menus))
	for index, item := range menus {
		item.TenantID = tenantID
		item.ChildMenus = applyMenuTenant(item.ChildMenus, tenantID)
		result[index] = item
	}
	return result
}

func (s *MenuService) importMenuItem(ctx context.Context, tx gdb.TX, item dto.MenuImportItem, parentID *int64) error {
	row := menuImportRow{
		ParentID:  parentID,
		TenantID:  item.TenantID,
		Name:      strings.TrimSpace(item.Name),
		Router:    item.Router,
		Perms:     item.Perms,
		Type:      item.Type,
		Icon:      item.Icon,
		OrderNum:  item.OrderNum,
		ViewPath:  item.ViewPath,
		KeepAlive: item.KeepAlive,
		IsShow:    item.IsShow,
	}
	row.CreateTime = mutationTimestamp()
	row.UpdateTime = row.CreateTime
	dbModel, err := tenant.ScopedModel(ctx, tx, s.Model, "")
	if err != nil {
		return err
	}
	insertedID, err := dbModel.Data(row).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "导入菜单失败")
	}
	for _, child := range item.ChildMenus {
		if err := s.importMenuItem(ctx, tx, child, &insertedID); err != nil {
			return err
		}
	}
	return nil
}

func validateMenuImportItem(item dto.MenuImportItem, depth int, count *int) error {
	if strings.TrimSpace(item.Name) == "" {
		return exception.Validate("菜单名称不能为空")
	}
	if depth > 100 {
		return exception.Validate("菜单层级过深")
	}
	*count = *count + 1
	if *count > 10000 {
		return exception.Validate("导入菜单数量过多")
	}
	if item.Type != nil && (*item.Type < 0 || *item.Type > 2) {
		return exception.Validate("菜单类型错误")
	}
	for _, child := range item.ChildMenus {
		if err := validateMenuImportItem(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func buildMenuExportItem(id int64, nodes map[int64]menuExportNode, children map[int64][]int64, visiting map[int64]bool) (dto.MenuExportItem, error) {
	if visiting[id] {
		return dto.MenuExportItem{}, exception.Comm("菜单层级存在循环")
	}
	node, ok := nodes[id]
	if !ok {
		return dto.MenuExportItem{}, exception.Comm("菜单数据不存在")
	}
	visiting[id] = true
	item := node.item
	for _, childID := range children[id] {
		child, err := buildMenuExportItem(childID, nodes, children, visiting)
		if err != nil {
			return dto.MenuExportItem{}, err
		}
		item.ChildMenus = append(item.ChildMenus, child)
	}
	delete(visiting, id)
	return item, nil
}

func optionalString(value interface{}) *string {
	if value == nil {
		return nil
	}
	result := fmt.Sprint(value)
	return &result
}

func optionalInt64(value interface{}) *int64 {
	if value == nil {
		return nil
	}
	var result int64
	if _, err := fmt.Sscan(fmt.Sprint(value), &result); err != nil {
		return nil
	}
	return &result
}

// Go 版菜单代码文件创建请求
type MenuCreateRequest struct {
	Module     string `json:"module"`
	Entity     string `json:"entity"`
	Controller string `json:"controller"`
	Service    string `json:"service"`
	FileName   string `json:"fileName"`
}

var (
	safeModuleName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	safeModulePath = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(/[A-Za-z][A-Za-z0-9_-]*)*$`)
	safeFileName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	definitionExpr = regexp.MustCompile(`NewDefinition\(\s*"[^"]+"\s*,\s*"([^"]+)"\s*,\s*"([^"]+)"\s*\)`)
	fieldExpr      = regexp.MustCompile(`NewField\(\s*"([^"]+)"\s*,\s*"[^"]+"\s*,\s*"([^"]+)"\s*\)([^\n]*)`)
	sizeExpr       = regexp.MustCompile(`\.Size\((\d+)\)`)
	commentExpr    = regexp.MustCompile(`\.Comment\("([^"]*)"\)`)
	entityExpr     = regexp.MustCompile(`@Entity\(\s*['"]([^'"]+)['"]\s*\)`)
	classExpr      = regexp.MustCompile(`class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsFieldExpr    = regexp.MustCompile(`(?s)@(Column|PrimaryGeneratedColumn)\((.*?)\)\s*(?:public\s+)?([A-Za-z_][A-Za-z0-9_]*)\??\s*:\s*([^;\n]+)`)
	optionLength   = regexp.MustCompile(`length\s*:\s*['"]?([^,'"}\s]+)`)
	optionComment  = regexp.MustCompile(`comment\s*:\s*['"]([^'"]*)['"]`)
	optionNullable = regexp.MustCompile(`nullable\s*:\s*true`)
	optionType     = regexp.MustCompile(`type\s*:\s*['"]([^'"]+)['"]`)
	adminPathExpr  = regexp.MustCompile(`Admin\(\s*"([^"]+)"\s*\)`)
	importFileExpr = regexp.MustCompile(`from\s+['"][^'"]*/([A-Za-z0-9_-]+)['"]`)
)

// 返回当前用户可见菜单并补充父菜单名称
func (s *MenuService) List(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	menuCondition, err := s.menuTenantCondition(ctx, s.Model, "a")
	if err != nil {
		return nil, err
	}
	parentCondition, err := s.menuTenantCondition(ctx, s.Model, "p")
	if err != nil {
		return nil, err
	}
	selectSQL := "SELECT DISTINCT a.id, a.parentId AS parentId, a.name, a.router, a.perms, a.type, a.icon, a.orderNum AS orderNum, a.viewPath AS viewPath, a.keepAlive AS keepAlive, a.isShow AS isShow, a.createTime AS createTime, a.updateTime AS updateTime, a.tenantId AS tenantId, p.name AS parentName FROM base_sys_menu a LEFT JOIN base_sys_menu p ON a.parentId = p.id"
	args := []interface{}{}
	if parentCondition.SQL != "" {
		selectSQL += " AND " + parentCondition.SQL
		args = append(args, parentCondition.Args...)
	}
	currentUser, err := requireAuthorizationCaller(ctx)
	if err != nil {
		return nil, err
	}
	isAdmin, err := isPlatformAdministrator(ctx, s.DB, currentUser.UserId)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if len(currentUser.RoleIds) == 0 {
			return []map[string]interface{}{}, nil
		}
		roleCondition, conditionErr := s.menuTenantCondition(ctx, s.roleModel, "r")
		if conditionErr != nil {
			return nil, conditionErr
		}
		selectSQL += " INNER JOIN base_sys_role_menu rm ON rm.menuId = a.id AND rm.roleId IN (?)"
		args = append(args, roleIDsToInterfaces(currentUser.RoleIds))
		selectSQL += " INNER JOIN base_sys_role r ON r.id = rm.roleId"
		if roleCondition.SQL != "" {
			selectSQL += " AND " + roleCondition.SQL
			args = append(args, roleCondition.Args...)
		}
	}
	where := " WHERE 1 = 1"
	if menuCondition.SQL != "" {
		where += " AND " + menuCondition.SQL
		args = append(args, menuCondition.Args...)
	}
	rows, err := s.DB.GetAll(ctx, selectSQL+where+" ORDER BY a.orderNum ASC", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询菜单列表失败")
	}
	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		result = append(result, normalizeMenuResponse(row.Map()))
	}
	return result, nil
}

// 返回 Node 实体字段类型兼容的菜单详情
func (s *MenuService) Info(ctx context.Context, request crud.InfoRequest) (interface{}, error) {
	condition, err := s.menuTenantCondition(ctx, s.Model, "")
	if err != nil {
		return nil, err
	}
	where := "id = ?"
	args := []interface{}{request.ID}
	if condition.SQL != "" {
		where += " AND " + condition.SQL
		args = append(args, condition.Args...)
	}
	row, err := s.DB.GetOne(ctx, "SELECT "+menuSelectColumns+" FROM base_sys_menu WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询菜单详情失败")
	}
	if len(row) == 0 {
		return nil, nil
	}
	return normalizeMenuResponse(row.Map()), nil
}

// 返回 Node 实体字段类型兼容的菜单分页数据
func (s *MenuService) Page(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	request = crud.NormalizePageRequest(request)
	condition, err := s.menuTenantCondition(ctx, s.Model, "")
	if err != nil {
		return nil, err
	}
	where := "1 = 1"
	args := []interface{}{}
	if condition.SQL != "" {
		where += " AND " + condition.SQL
		args = append(args, condition.Args...)
	}
	total, err := s.DB.GetCount(ctx, "SELECT COUNT(*) FROM base_sys_menu WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询菜单总数失败")
	}
	orderBy, err := pageOrderBy(request, map[string]string{
		"id": "id", "orderNum": "orderNum", "createTime": "createTime", "updateTime": "updateTime",
	}, "id", "DESC")
	if err != nil {
		return nil, err
	}
	limitSQL, limitArgs := pageLimit(request)
	listArgs := append(append([]interface{}{}, args...), limitArgs...)
	rows, err := s.DB.GetAll(ctx, "SELECT "+menuSelectColumns+" FROM base_sys_menu WHERE "+where+orderBy+limitSQL, listArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询菜单分页失败")
	}
	list := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		list = append(list, row.Map())
	}
	return crud.PageResult{
		List:       list,
		Pagination: crud.Pagination{Page: request.Page, Size: request.Size, Total: total},
	}, nil
}

const menuSelectColumns = "id, parentId AS parentId, name, router, perms, type, icon, orderNum AS orderNum, viewPath AS viewPath, keepAlive AS keepAlive, isShow AS isShow, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId"

// 递归删除菜单及角色菜单关系
func (s *MenuService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	ids, err := parseMenuIDs(request.IDs)
	if err != nil || len(ids) == 0 {
		return nil, exception.Validate("删除ID不能为空")
	}
	if _, err = tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return nil, err
	}
	requestIDs := menuInterfaceValues(ids)
	err = runManagedDelete(ctx, s.DB, s.recycle, s.Model, requestIDs, request, func(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope) error {
		targetQuery, queryErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		targets, queryErr := targetQuery.Fields("id").WhereIn("id", ids).OrderAsc("id").LockUpdate().All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询菜单失败")
		}
		if len(targets) != len(ids) {
			return exception.Comm("菜单不存在")
		}
		allIDs := append([]int64{}, ids...)
		seen := map[int64]struct{}{}
		itemKeys := map[int64]string{}
		branchKeys := map[int64]string{}
		for _, id := range allIDs {
			seen[id] = struct{}{}
			branchKeys[id] = strconv.FormatInt(id, 10)
			if scope != nil && scope.IsArchiving() {
				rootKey, exists := scope.RootKey(id)
				if !exists {
					return gerror.Newf("菜单 %d 缺少回收站根归档项", id)
				}
				itemKeys[id] = rootKey
			}
		}
		frontier := append([]int64{}, ids...)
		for len(frontier) > 0 {
			childQuery, childErr := tenant.ScopedModel(ctx, tx, s.Model, "")
			if childErr != nil {
				return childErr
			}
			rows, queryErr := childQuery.WhereIn("parentId", menuInterfaceValues(frontier)).OrderAsc("parentId").OrderAsc("id").LockUpdate().All()
			if queryErr != nil {
				return gerror.Wrap(queryErr, "查询子菜单失败")
			}
			frontier = frontier[:0]
			for _, row := range rows {
				id := row["id"].Int64()
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				parentID := row["parentId"].Int64()
				branchKeys[id] = branchKeys[parentID]
				if scope != nil && scope.IsArchiving() {
					parentKey, exists := itemKeys[parentID]
					if !exists {
						return gerror.Newf("菜单 %d 缺少父归档项", id)
					}
					itemKey, addErr := scope.AddRecord(s.Model, row.Map(), recycle.ItemOptions{
						BranchKey: branchKeys[id], ParentKey: parentKey, RestoreOrder: 10,
					})
					if addErr != nil {
						return addErr
					}
					itemKeys[id] = itemKey
				}
				allIDs = append(allIDs, id)
				frontier = append(frontier, id)
			}
		}
		values := menuInterfaceValues(allIDs)
		relationQuery := tx.Model(s.roleMenuModel.TableName).Ctx(ctx).WhereIn("menuId", values)
		if scope != nil && scope.IsArchiving() {
			rows, queryErr := relationQuery.Clone().OrderAsc("menuId").OrderAsc("roleId").LockUpdate().All()
			if queryErr != nil {
				return gerror.Wrap(queryErr, "锁定菜单权限失败")
			}
			for _, row := range rows {
				menuID := row["menuId"].Int64()
				parentKey, exists := itemKeys[menuID]
				if !exists {
					return gerror.Newf("菜单 %d 缺少回收站归档项", menuID)
				}
				if _, addErr := scope.AddRecord(s.roleMenuModel, row.Map(), recycle.ItemOptions{
					BranchKey: branchKeys[menuID], ParentKey: parentKey, RestoreOrder: 20,
				}); addErr != nil {
					return addErr
				}
			}
		}
		relationResult, deleteErr := relationQuery.Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除菜单权限失败")
		}
		if markErr := markManagedDeleted(scope, relationResult, "读取菜单权限删除数量失败"); markErr != nil {
			return markErr
		}
		deleteModel, deleteErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if deleteErr != nil {
			return deleteErr
		}
		deleteResult, deleteErr := deleteModel.WhereIn("id", values).Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除菜单失败")
		}
		return markManagedDeleted(scope, deleteResult, "读取菜单删除数量失败")
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// 校验上级菜单属于当前作用域
func (s *MenuService) ensureMenuParent(ctx context.Context, provider tenant.ModelProvider, parentID int64, selfID int64) error {
	if parentID <= 0 {
		return nil
	}
	if parentID == selfID {
		return exception.Validate("上级菜单不能是自身")
	}
	dbModel, err := tenant.ScopedModel(ctx, provider, s.Model, "")
	if err != nil {
		return err
	}
	parent, err := dbModel.Fields("id").Where("id", parentID).LockUpdate().One()
	if err != nil {
		return gerror.Wrap(err, "查询上级菜单失败")
	}
	if parent.IsEmpty() {
		return exception.Comm("上级菜单不存在")
	}
	return nil
}

// 返回菜单原始 SQL 的租户条件
func (s *MenuService) menuTenantCondition(ctx context.Context, definition entity.Definition, alias string) (tenant.Condition, error) {
	metadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		return tenant.Condition{}, err
	}
	return tenant.Predicate(ctx, metadata, alias)
}

// 解析 Go 模型源码，也兼容 Node TypeScript 实体源码的静态解析
func (s *MenuService) Parse(request dto.MenuParseReq) (dto.MenuParseRes, error) {
	modulePath := strings.Trim(request.Module, "/")
	if !safeModulePath.MatchString(modulePath) {
		return dto.MenuParseRes{}, exception.Validate("模块参数错误")
	}
	columns, className, tableName := parseGoModel(request.Entity)
	if className == "" || tableName == "" {
		columns, className, tableName = parseTypeScriptEntity(request.Entity)
	}
	if className == "" || tableName == "" {
		return dto.MenuParseRes{}, exception.Validate("代码结构不正确，请检查")
	}
	fileName := tableFileName(tableName)
	if request.Controller != "" {
		if controllerFile := parseControllerFile(request.Controller); controllerFile != "" {
			fileName = controllerFile
		}
		if !safeFileName.MatchString(strings.ReplaceAll(fileName, "-", "_")) {
			return dto.MenuParseRes{}, exception.Validate("代码结构不正确，请检查")
		}
		return dto.MenuParseRes{Columns: columns, Path: "/admin/" + modulePath + "/" + fileName}, nil
	}
	if !safeFileName.MatchString(fileName) {
		return dto.MenuParseRes{}, exception.Validate("代码结构不正确，请检查")
	}
	return dto.MenuParseRes{Columns: columns, ClassName: className, TableName: tableName, FileName: fileName, Path: "/admin/" + modulePath + "/" + fileName}, nil
}

// 在当前 Go 项目的 modules 目录下创建模型、Controller 和 Service 文件
func (s *MenuService) Create(request MenuCreateRequest) error {
	if !safeModuleName.MatchString(request.Module) || !safeFileName.MatchString(request.FileName) {
		return exception.Validate("模块或文件名参数错误")
	}
	files := []struct {
		folder  string
		content string
	}{
		{folder: "model", content: request.Entity},
		{folder: filepath.Join("controller", "admin"), content: request.Controller},
		{folder: "service", content: request.Service},
	}
	for _, file := range files {
		if strings.TrimSpace(file.content) == "" {
			continue
		}
		if _, err := parser.ParseFile(token.NewFileSet(), request.FileName+".go", file.content, parser.AllErrors); err != nil {
			return exception.Validate("Go代码格式错误")
		}
	}
	root, err := findGoProjectRoot()
	if err != nil {
		return err
	}
	modulesRoot, err := filepath.EvalSymlinks(filepath.Join(root, "modules"))
	if err != nil {
		return gerror.Wrap(err, "读取模块目录失败")
	}
	moduleRoot := filepath.Join(modulesRoot, request.Module)
	if err = os.MkdirAll(moduleRoot, 0o755); err != nil {
		return gerror.Wrap(err, "创建模块目录失败")
	}
	for _, file := range files {
		if strings.TrimSpace(file.content) == "" {
			continue
		}
		directory := filepath.Join(moduleRoot, file.folder)
		if err = os.MkdirAll(directory, 0o755); err != nil {
			return gerror.Wrap(err, "创建代码目录失败")
		}
		resolvedDirectory, resolveErr := filepath.EvalSymlinks(directory)
		if resolveErr != nil {
			return gerror.Wrap(resolveErr, "读取代码目录失败")
		}
		if !pathWithin(modulesRoot, resolvedDirectory) {
			return exception.Validate("非法文件路径")
		}
		path := filepath.Join(resolvedDirectory, request.FileName+".go")
		if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return exception.Validate("非法文件路径")
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return gerror.Wrap(statErr, "读取代码文件失败")
		}
		if err = os.WriteFile(path, []byte(file.content), 0o644); err != nil {
			return gerror.Wrap(err, "创建代码文件失败")
		}
	}
	return nil
}

func pathWithin(root string, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func parseGoModel(source string) ([]dto.MenuParseColumn, string, string) {
	definition := definitionExpr.FindStringSubmatch(source)
	if len(definition) != 3 {
		return nil, "", ""
	}
	columns := make([]dto.MenuParseColumn, 0)
	commonTail := []dto.MenuParseColumn{}
	if strings.Contains(source, "BaseFields()") {
		columns = append(columns,
			dto.MenuParseColumn{PropertyName: "id", Type: "bigint"},
			dto.MenuParseColumn{PropertyName: "tenantId", Type: "bigint", Nullable: true},
		)
		commonTail = append(commonTail,
			dto.MenuParseColumn{PropertyName: "createTime", Type: "varchar"},
			dto.MenuParseColumn{PropertyName: "updateTime", Type: "varchar"},
		)
	}
	for _, match := range fieldExpr.FindAllStringSubmatch(source, -1) {
		column := dto.MenuParseColumn{PropertyName: match[1], Type: match[2], Nullable: strings.Contains(match[3], ".Nullable()")}
		if size := sizeExpr.FindStringSubmatch(match[3]); len(size) == 2 {
			column.Length = size[1]
		}
		if comment := commentExpr.FindStringSubmatch(match[3]); len(comment) == 2 {
			column.Comment = comment[1]
		}
		columns = append(columns, column)
	}
	return append(columns, commonTail...), definition[1], definition[2]
}

func parseTypeScriptEntity(source string) ([]dto.MenuParseColumn, string, string) {
	entity := entityExpr.FindStringSubmatch(source)
	class := classExpr.FindStringSubmatch(source)
	if len(entity) != 2 || len(class) != 2 {
		return nil, "", ""
	}
	columns := make([]dto.MenuParseColumn, 0)
	for _, match := range tsFieldExpr.FindAllStringSubmatch(source, -1) {
		columnType := strings.TrimSpace(match[4])
		if option := optionType.FindStringSubmatch(match[2]); len(option) == 2 {
			columnType = option[1]
		}
		column := dto.MenuParseColumn{PropertyName: match[3], Type: strings.ToLower(strings.TrimSuffix(columnType, "[]")), Nullable: optionNullable.MatchString(match[2]) || strings.Contains(match[0], match[3]+"?")}
		if length := optionLength.FindStringSubmatch(match[2]); len(length) == 2 {
			column.Length = length[1]
		}
		if comment := optionComment.FindStringSubmatch(match[2]); len(comment) == 2 {
			column.Comment = comment[1]
		}
		columns = append(columns, column)
	}
	return columns, class[1], entity[1]
}

func parseControllerFile(source string) string {
	if match := adminPathExpr.FindStringSubmatch(source); len(match) == 2 {
		parts := strings.Split(strings.Trim(match[1], "/"), "/")
		return parts[len(parts)-1]
	}
	if match := importFileExpr.FindStringSubmatch(source); len(match) == 2 {
		return match[1]
	}
	if parsed, err := parser.ParseFile(token.NewFileSet(), "controller.go", source, 0); err == nil {
		for _, declaration := range parsed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && strings.HasSuffix(function.Name.Name, "Controller") {
				return strings.ToLower(strings.TrimSuffix(function.Name.Name, "Controller"))
			}
		}
	}
	return ""
}

func tableFileName(tableName string) string {
	parts := strings.Split(tableName, "_")
	return parts[len(parts)-1]
}

func findGoProjectRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", gerror.Wrap(err, "读取项目目录失败")
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", exception.Internal(nil, "未找到Go项目根目录")
		}
		directory = parent
	}
}

func menuInterfaceValues(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for index, id := range ids {
		result[index] = id
	}
	return result
}

func parseMenuIDs(value interface{}) ([]int64, error) {
	values, ok := value.([]interface{})
	if !ok {
		switch typed := value.(type) {
		case []int64:
			return append([]int64{}, typed...), nil
		case []int:
			values = make([]interface{}, len(typed))
			for index, item := range typed {
				values[index] = item
			}
		default:
			values = []interface{}{typed}
		}
	}
	result := make([]int64, 0, len(values))
	seen := map[int64]struct{}{}
	for _, value := range values {
		id, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil || id <= 0 {
			return nil, exception.Validate("删除ID不能为空")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}
