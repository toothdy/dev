package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
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
)

// 系统参数服务
type ParamService struct {
	*service.Base
	recycle *recycle.Manager
}

type paramMutationRow struct {
	KeyName    interface{} `orm:"keyName"`
	Name       interface{} `orm:"name"`
	Data       interface{} `orm:"data"`
	DataType   interface{} `orm:"dataType"`
	Remark     interface{} `orm:"remark"`
	TenantID   interface{} `orm:"tenantId"`
	CreateTime interface{} `orm:"createTime"`
	UpdateTime interface{} `orm:"updateTime"`
}

var paramMutationFields = []string{"id", "keyName", "name", "data", "dataType", "remark", "tenantId"}

/**
 * 创建系统参数服务
 * @param db 数据库实例
 * @param baseSysParamModel 参数模型
 * @param recycleManager 回收站协调器
 * @returns *ParamService
 */
func NewParamService(db gdb.DB, baseSysParamModel entity.Definition, recycleManager *recycle.Manager) *ParamService {
	return &ParamService{
		Base: service.NewBase(db, baseSysParamModel),
		recycle:     recycleManager,
	}
}

// 根据参数键返回转换后的参数数据
func (s *ParamService) DataByKey(ctx context.Context, key string) (interface{}, error) {
	if s == nil || s.Base == nil || s.DB == nil {
		return nil, exception.Internal(nil, "参数服务不可用")
	}
	query, err := s.publicParamModel(ctx)
	if err != nil {
		return nil, err
	}
	record, err := query.Fields("data", "dataType").Where("keyName", key).One()
	if err != nil {
		return nil, gerror.Wrap(err, "查询参数失败")
	}
	if record.IsEmpty() {
		return nil, nil
	}
	data := record["data"].String()
	switch record["dataType"].Int() {
	case 0:
		var value interface{}
		if json.Unmarshal([]byte(data), &value) == nil {
			return value, nil
		}
		return data, nil
	case 2:
		if data == "" {
			return []string{}, nil
		}
		return strings.Split(data, ","), nil
	default:
		return data, nil
	}
}

// 新增参数并校验键名唯一
func (s *ParamService) Add(ctx context.Context, request crud.AddRequest) (interface{}, error) {
	applyTenantMutation(ctx, request.Data)
	if _, ok := request.Data["dataType"]; !ok {
		request.Data["dataType"] = 0
	}
	if err := validateParamMutation(request.Data); err != nil {
		return nil, err
	}
	key, err := requiredParamString(request.Data, "keyName")
	if err != nil {
		return nil, err
	}
	if _, err = requiredParamString(request.Data, "name"); err != nil {
		return nil, err
	}
	if err = s.ensureParamKeyUnique(ctx, s.DB, key, 0); err != nil {
		return nil, err
	}
	row, err := paramRowFromData(request.Data, 0)
	if err != nil {
		return nil, err
	}
	now := mutationTimestamp()
	row.CreateTime = now
	row.UpdateTime = now
	dbModel, err := tenant.ScopedModel(ctx, s.DB, s.Model, "")
	if err != nil {
		return nil, err
	}
	id, err := dbModel.Data(row).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "新增参数失败")
	}
	return map[string]interface{}{"id": id}, nil
}

// 修改参数并校验键名唯一
func (s *ParamService) Update(ctx context.Context, request crud.UpdateRequest) (interface{}, error) {
	delete(request.Data, "tenantId")
	if err := validateParamMutation(request.Data); err != nil {
		return nil, err
	}
	id, err := requiredParamID(request.Data)
	if err != nil {
		return nil, err
	}
	if _, err = tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return nil, err
	}
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		currentQuery, queryErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		current, queryErr := currentQuery.Fields("keyName", "dataType").Where("id", id).LockUpdate().One()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询参数失败")
		}
		if current.IsEmpty() {
			return exception.Comm("参数不存在")
		}
		key := current["keyName"].String()
		if value, ok := request.Data["keyName"]; ok {
			key = strings.TrimSpace(fmt.Sprint(value))
		}
		if queryErr = s.ensureParamKeyUnique(ctx, tx, key, id); queryErr != nil {
			return queryErr
		}
		values, queryErr := paramUpdateData(request.Data, current["dataType"].Int())
		if queryErr != nil {
			return queryErr
		}
		values["updateTime"] = mutationTimestamp()
		updateQuery, queryErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		if _, queryErr = updateQuery.Where("id", id).Data(values).Update(); queryErr != nil {
			return gerror.Wrap(queryErr, "修改参数失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// 按 Node 服务的详情规则解析参数数据
func (s *ParamService) Info(ctx context.Context, request crud.InfoRequest) (interface{}, error) {
	condition, err := s.paramTenantCondition(ctx, "")
	if err != nil {
		return nil, err
	}
	where := "id = ?"
	args := []interface{}{request.ID}
	if condition.SQL != "" {
		where += " AND " + condition.SQL
		args = append(args, condition.Args...)
	}
	record, err := s.DB.GetOne(ctx, "SELECT id, keyName AS keyName, name, data, dataType AS dataType, remark, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId FROM base_sys_param WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询参数详情失败")
	}
	if len(record) == 0 {
		return nil, exception.Comm("数据不存在")
	}
	info := record.Map()
	if value, ok := parseParamInfoData(record["data"].String()); ok {
		info["data"] = value
	}
	return info, nil
}

// 返回当前租户的参数分页
func (s *ParamService) Page(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	request = crud.NormalizePageRequest(request)
	condition, err := s.paramTenantCondition(ctx, "")
	if err != nil {
		return nil, err
	}
	where := "1 = 1"
	args := []interface{}{}
	if condition.SQL != "" {
		where += " AND " + condition.SQL
		args = append(args, condition.Args...)
	}
	if request.Keyword != "" {
		where += " AND (keyName LIKE ? OR name LIKE ?)"
		keyword := "%" + request.Keyword + "%"
		args = append(args, keyword, keyword)
	}
	if dataType, ok := request.Raw["dataType"]; ok && dataType != nil {
		where += " AND dataType = ?"
		args = append(args, dataType)
	}
	total, err := s.DB.GetCount(ctx, "SELECT COUNT(*) FROM base_sys_param WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询参数总数失败")
	}
	orderBy, err := pageOrderBy(request, map[string]string{
		"id": "id", "createTime": "createTime", "updateTime": "updateTime", "keyName": "keyName",
	}, "id", "DESC")
	if err != nil {
		return nil, err
	}
	limitSQL, limitArgs := pageLimit(request)
	rows, err := s.DB.GetAll(ctx, "SELECT id, keyName AS keyName, name, data, dataType AS dataType, remark, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId FROM base_sys_param WHERE "+where+orderBy+limitSQL, append(args, limitArgs...)...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询参数分页失败")
	}
	list := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		list = append(list, row.Map())
	}
	return crud.PageResult{List: list, Pagination: crud.Pagination{Page: request.Page, Size: request.Size, Total: total}}, nil
}

// 只删除当前租户的参数
func (s *ParamService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	if len(request.IDs) == 0 {
		return nil, exception.Validate("删除ID不能为空")
	}
	if _, err := tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return nil, err
	}
	err := runManagedDelete(ctx, s.DB, s.recycle, s.Model, request.IDs, request, func(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope) error {
		query, queryErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		rows, queryErr := query.Fields("id").WhereIn("id", request.IDs).OrderAsc("id").LockUpdate().All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询参数失败")
		}
		if len(rows) != len(request.IDs) {
			return exception.Comm("参数不存在")
		}
		query, queryErr = tenant.ScopedModel(ctx, tx, s.Model, "")
		if queryErr != nil {
			return queryErr
		}
		result, queryErr := query.WhereIn("id", request.IDs).Delete()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "删除参数失败")
		}
		return markManagedDeleted(scope, result, "读取参数删除数量失败")
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func parseParamInfoData(data string) (interface{}, bool) {
	var value interface{}
	nodeJSON := strings.NewReplacer("{", "[", "}", "]").Replace(data)
	if json.Unmarshal([]byte(nodeJSON), &value) != nil {
		return data, false
	}
	return value, true
}

func (s *ParamService) ensureParamKeyUnique(ctx context.Context, provider tenant.ModelProvider, key string, excludeID int64) error {
	query, err := tenant.ScopedModel(ctx, provider, s.Model, "")
	if err != nil {
		return err
	}
	query = query.Where("keyName", key)
	if excludeID > 0 {
		query = query.WhereNot("id", excludeID)
	}
	count, err := query.Count()
	if err != nil {
		return gerror.Wrap(err, "查询参数键失败")
	}
	if count > 0 {
		return exception.Comm("存在相同的keyName")
	}
	return nil
}

func paramRowFromData(data map[string]interface{}, fallbackDataType int) (paramMutationRow, error) {
	dataType := fallbackDataType
	if value, ok := data["dataType"]; ok {
		parsed, parseErr := strconv.Atoi(fmt.Sprint(value))
		if parseErr != nil {
			return paramMutationRow{}, exception.Validate("dataType参数错误")
		}
		dataType = parsed
	}
	if dataType < 0 || dataType > 2 {
		return paramMutationRow{}, exception.Validate("dataType参数错误")
	}
	row := paramMutationRow{KeyName: data["keyName"], Name: data["name"], DataType: data["dataType"], Remark: data["remark"], TenantID: data["tenantId"]}
	if value, ok := data["data"]; ok {
		switch dataType {
		case 0:
			if text, isString := value.(string); isString {
				row.Data = text
			} else {
				encoded, err := json.Marshal(value)
				if err != nil {
					return row, gerror.Wrap(err, "参数数据格式错误")
				}
				row.Data = string(encoded)
			}
		case 2:
			switch values := value.(type) {
			case []interface{}:
				items := make([]string, len(values))
				for index, item := range values {
					items[index] = fmt.Sprint(item)
				}
				row.Data = strings.Join(items, ",")
			case []string:
				row.Data = strings.Join(values, ",")
			default:
				row.Data = fmt.Sprint(value)
			}
		default:
			row.Data = fmt.Sprint(value)
		}
	}
	return row, nil
}

func paramUpdateData(data map[string]interface{}, fallbackDataType int) (map[string]interface{}, error) {
	row, err := paramRowFromData(data, fallbackDataType)
	if err != nil {
		return nil, err
	}
	values := make(map[string]interface{}, len(data))
	if _, ok := data["keyName"]; ok {
		values["keyName"] = row.KeyName
	}
	if _, ok := data["name"]; ok {
		values["name"] = row.Name
	}
	if _, ok := data["data"]; ok {
		values["data"] = row.Data
	}
	if _, ok := data["dataType"]; ok {
		values["dataType"] = row.DataType
	}
	if _, ok := data["remark"]; ok {
		values["remark"] = row.Remark
	}
	return values, nil
}

func validateParamMutation(data map[string]interface{}) error {
	allowed := make(map[string]struct{}, len(paramMutationFields))
	for _, field := range paramMutationFields {
		allowed[field] = struct{}{}
	}
	for field := range data {
		if _, ok := allowed[field]; !ok {
			return exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	return nil
}

func requiredParamString(data map[string]interface{}, field string) (string, error) {
	value := strings.TrimSpace(fmt.Sprint(data[field]))
	if value == "" || value == "<nil>" {
		return "", exception.Validate(fmt.Sprintf("%s不能为空", field))
	}
	return value, nil
}

func requiredParamID(data map[string]interface{}) (int64, error) {
	value, ok := data["id"]
	if !ok {
		return 0, exception.Validate("id不能为空")
	}
	id, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, exception.Validate("id参数错误")
	}
	return id, nil
}

/**
 * 根据参数 key 生成网页内容
 * @param ctx 请求上下文
 * @param key 参数 key
 * @returns HTML 内容
 */
func (s *ParamService) HTMLByKey(ctx context.Context, key string) (string, error) {
	template := "<html><title>@title</title><body>@content</body></html>"
	if s == nil || s.Base == nil || s.DB == nil {
		return "", exception.Internal(nil, "参数服务不可用")
	}

	query, err := s.publicParamModel(ctx)
	if err != nil {
		return "", err
	}
	record, err := query.Fields("name", "data").Where("keyName", key).One()
	if err != nil {
		return "", gerror.Wrap(err, "查询参数失败")
	}
	if record.IsEmpty() {
		return strings.Replace(template, "@content", "key notfound", 1), nil
	}
	return strings.NewReplacer(
		"@title", html.EscapeString(record["name"].String()),
		"@content", html.EscapeString(record["data"].String()),
	).Replace(template), nil
}

// 返回参数资源的原始 SQL 租户条件
func (s *ParamService) paramTenantCondition(ctx context.Context, alias string) (tenant.Condition, error) {
	metadata, err := tenant.CompileMetadata(s.Model)
	if err != nil {
		return tenant.Condition{}, err
	}
	return tenant.Predicate(ctx, metadata, alias)
}

// 公开参数在缺失或平台作用域下只读取平台数据
func (s *ParamService) publicParamModel(ctx context.Context) (*gdb.Model, error) {
	switch tenant.Resolve(ctx).Kind() {
	case tenant.KindMissing, tenant.KindPlatform:
		metadata, err := tenant.CompileMetadata(s.Model)
		if err != nil {
			return nil, err
		}
		condition, err := tenant.GlobalOnlyPredicate(metadata, "")
		if err != nil {
			return nil, err
		}
		return s.DB.Model(s.Model.TableName).Ctx(ctx).Where(condition.SQL, condition.Args...), nil
	default:
		return tenant.ScopedModel(ctx, s.DB, s.Model, "")
	}
}
