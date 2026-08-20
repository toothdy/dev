package crud

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

// 查询 SQL
type selectQuery struct {
	SQL  string
	Args []interface{}
}

// 写入 SQL
type writeQuery struct {
	SQL  string
	Args []interface{}
}

/**
 * 构建新增 SQL
 * @param resource 资源定义
 * @param input 输入数据
 * @param scope 已解析的租户作用域
 * @returns 写入 SQL
 */
func buildInsertQuery(resource Resource, input map[string]interface{}, scope tenant.Scope) (writeQuery, error) {
	if resource.Tenant.IsAware() {
		input = cloneMap(input)
		delete(input, resource.Tenant.JSONField())
	}
	values, err := mapWritableValues(resource, input, false)
	if err != nil {
		return writeQuery{}, err
	}
	if len(values) == 0 {
		return writeQuery{}, exception.Validate("新增数据不能为空")
	}
	tenantValue, shouldWriteTenant, err := tenant.InsertValue(scope, resource.Tenant)
	if err != nil {
		return writeQuery{}, err
	}
	if shouldWriteTenant {
		values[resource.Tenant.Column()] = tenantValue
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, fieldName := range []string{"createTime", "updateTime"} {
		if field, ok := resource.FieldsByJSON[fieldName]; ok {
			values[field.ColumnName] = now
		}
	}

	columns := sortedKeys(values)
	quotedColumns := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	args := make([]interface{}, 0, len(columns))
	for _, columnName := range columns {
		quotedColumns = append(quotedColumns, quoteIdentifier(columnName))
		placeholders = append(placeholders, "?")
		args = append(args, values[columnName])
	}

	return writeQuery{
		SQL: fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			quoteIdentifier(resource.Spec.Model.TableName),
			strings.Join(quotedColumns, ", "),
			strings.Join(placeholders, ", "),
		),
		Args: args,
	}, nil
}

/**
 * 构建更新 SQL
 * @param resource 资源定义
 * @param input 输入数据
 * @param scope 已解析的租户作用域
 * @returns 写入 SQL 和主键值
 */
func buildUpdateQuery(resource Resource, input map[string]interface{}, scope tenant.Scope) (writeQuery, interface{}, error) {
	idValue, ok := input[resource.PrimaryField.JSONName]
	if !ok || isEmptyValue(idValue) {
		return writeQuery{}, nil, exception.Validate("更新数据缺少主键")
	}
	values, err := mapWritableValues(resource, input, true)
	if err != nil {
		return writeQuery{}, nil, err
	}
	if len(values) == 0 {
		return writeQuery{}, nil, exception.Validate("更新数据不能为空")
	}

	columns := sortedKeys(values)
	sets := make([]string, 0, len(columns)+1)
	args := make([]interface{}, 0, len(columns)+1)
	for _, columnName := range columns {
		sets = append(sets, fmt.Sprintf("%s = ?", quoteIdentifier(columnName)))
		args = append(args, values[columnName])
	}
	if updateTimeField, ok := resource.FieldsByJSON["updateTime"]; ok {
		sets = append(sets, fmt.Sprintf("%s = CURRENT_TIMESTAMP", quoteIdentifier(updateTimeField.ColumnName)))
	}
	args = append(args, idValue)
	tenantCondition, err := tenant.PredicateForScope(scope, resource.Tenant, "")
	if err != nil {
		return writeQuery{}, nil, err
	}
	whereSQL := fmt.Sprintf("%s = ?", quoteIdentifier(resource.PrimaryField.ColumnName))
	if tenantCondition.SQL != "" {
		whereSQL += " AND " + tenantCondition.SQL
		args = append(args, tenantCondition.Args...)
	}

	return writeQuery{
		SQL: fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s",
			quoteIdentifier(resource.Spec.Model.TableName),
			strings.Join(sets, ", "),
			whereSQL,
		),
		Args: args,
	}, idValue, nil
}

/**
 * 构建删除 SQL
 * @param resource 资源定义
 * @param ids ID 列表
 * @param scope 已解析的租户作用域
 * @returns 写入 SQL
 */
func buildDeleteQuery(resource Resource, ids []interface{}, scope tenant.Scope) (writeQuery, error) {
	if len(ids) == 0 {
		return writeQuery{}, exception.Validate("删除 ID 不能为空")
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		if isEmptyValue(id) {
			return writeQuery{}, exception.Validate("删除 ID 不能为空")
		}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	tenantCondition, err := tenant.PredicateForScope(scope, resource.Tenant, "")
	if err != nil {
		return writeQuery{}, err
	}
	whereSQL := fmt.Sprintf(
		"%s IN (%s)",
		quoteIdentifier(resource.PrimaryField.ColumnName),
		strings.Join(placeholders, ", "),
	)
	if tenantCondition.SQL != "" {
		whereSQL += " AND " + tenantCondition.SQL
		args = append(args, tenantCondition.Args...)
	}
	return writeQuery{
		SQL: fmt.Sprintf(
			"DELETE FROM %s WHERE %s",
			quoteIdentifier(resource.Spec.Model.TableName),
			whereSQL,
		),
		Args: args,
	}, nil
}

/**
 * 构建写入前的行锁查询
 * @param resource 资源定义
 * @param ids 已归一化的 ID 列表
 * @param scope 已解析的租户作用域
 * @returns 行锁查询
 */
func buildMutationLockQuery(resource Resource, ids []interface{}, scope tenant.Scope) (selectQuery, error) {
	if len(ids) == 0 {
		return selectQuery{}, exception.Validate("写入 ID 不能为空")
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	for _, id := range ids {
		if isEmptyValue(id) {
			return selectQuery{}, exception.Validate("写入 ID 不能为空")
		}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	tenantCondition, err := tenant.PredicateForScope(scope, resource.Tenant, "")
	if err != nil {
		return selectQuery{}, err
	}
	whereSQL := fmt.Sprintf(
		"%s IN (%s)",
		quoteIdentifier(resource.PrimaryField.ColumnName),
		strings.Join(placeholders, ", "),
	)
	if tenantCondition.SQL != "" {
		whereSQL += " AND " + tenantCondition.SQL
		args = append(args, tenantCondition.Args...)
	}
	return selectQuery{
		SQL: fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s FOR UPDATE",
			quoteIdentifier(resource.PrimaryField.ColumnName),
			quoteIdentifier(resource.Spec.Model.TableName),
			whereSQL,
			quoteIdentifier(resource.PrimaryField.ColumnName),
		),
		Args: args,
	}, nil
}

/**
 * 构建详情查询 SQL
 * @param resource 资源定义
 * @param id 主键值
 * @param scope 已解析的租户作用域
 * @returns 查询 SQL
 */
func buildInfoQuery(resource Resource, id interface{}, scope tenant.Scope) (selectQuery, error) {
	if isEmptyValue(id) {
		return selectQuery{}, exception.Validate("详情 ID 不能为空")
	}
	tenantCondition, err := tenant.PredicateForScope(scope, resource.Tenant, "")
	if err != nil {
		return selectQuery{}, err
	}
	whereSQL := fmt.Sprintf("%s = ?", quoteIdentifier(resource.PrimaryField.ColumnName))
	args := []interface{}{id}
	if tenantCondition.SQL != "" {
		whereSQL += " AND " + tenantCondition.SQL
		args = append(args, tenantCondition.Args...)
	}
	return selectQuery{
		SQL: fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s LIMIT 1",
			selectColumns(resource, true),
			quoteIdentifier(resource.Spec.Model.TableName),
			whereSQL,
		),
		Args: args,
	}, nil
}

/**
 * 构建列表查询 SQL
 * @param resource 资源定义
 * @param request 查询请求
 * @param scope 已解析的租户作用域
 * @returns 查询 SQL
 */
func buildListQuery(resource Resource, request QueryRequest, scope tenant.Scope) (selectQuery, error) {
	tenantCondition, err := tenant.PredicateForScope(scope, resource.Tenant, "")
	if err != nil {
		return selectQuery{}, err
	}
	whereSQL, args, err := buildWhereClause(resource, request, tenantCondition)
	if err != nil {
		return selectQuery{}, err
	}
	orderSQL, err := buildOrderClause(resource, request)
	if err != nil {
		return selectQuery{}, err
	}
	return selectQuery{
		SQL: fmt.Sprintf(
			"SELECT %s FROM %s%s%s LIMIT ?",
			selectColumns(resource),
			quoteIdentifier(resource.Spec.Model.TableName),
			whereSQL,
			orderSQL,
		),
		Args: append(args, MaxListSize),
	}, nil
}

/**
 * 构建分页查询 SQL
 * @param resource 资源定义
 * @param request 查询请求
 * @param scope 已解析的租户作用域
 * @returns 查询 SQL
 */
func buildPageQuery(resource Resource, request QueryRequest, scope tenant.Scope) (selectQuery, selectQuery, QueryRequest, error) {
	normalized := NormalizePageRequest(request)
	tenantCondition, err := tenant.PredicateForScope(scope, resource.Tenant, "")
	if err != nil {
		return selectQuery{}, selectQuery{}, QueryRequest{}, err
	}
	whereSQL, args, err := buildWhereClause(resource, normalized, tenantCondition)
	if err != nil {
		return selectQuery{}, selectQuery{}, QueryRequest{}, err
	}
	orderSQL, err := buildOrderClause(resource, normalized)
	if err != nil {
		return selectQuery{}, selectQuery{}, QueryRequest{}, err
	}

	countQuery := selectQuery{
		SQL:  fmt.Sprintf("SELECT COUNT(*) FROM %s%s", quoteIdentifier(resource.Spec.Model.TableName), whereSQL),
		Args: append([]interface{}{}, args...),
	}
	pageArgs := append([]interface{}{}, args...)
	limitSQL := ""
	if normalized.IsExport && normalized.MaxExportLimit > 0 {
		limitSQL = " LIMIT ?"
		pageArgs = append(pageArgs, normalized.MaxExportLimit)
	} else {
		limitSQL = " LIMIT ? OFFSET ?"
		pageArgs = append(pageArgs, normalized.Size, (normalized.Page-1)*normalized.Size)
	}
	dataQuery := selectQuery{
		SQL: fmt.Sprintf(
			"SELECT %s FROM %s%s%s%s",
			selectColumns(resource),
			quoteIdentifier(resource.Spec.Model.TableName),
			whereSQL,
			orderSQL,
			limitSQL,
		),
		Args: pageArgs,
	}
	return dataQuery, countQuery, normalized, nil
}

/**
 * 映射可写字段
 * @param resource 资源定义
 * @param input 输入数据
 * @param isUpdate 是否更新
 * @returns DB 字段值
 */
func mapWritableValues(resource Resource, input map[string]interface{}, isUpdate bool) (map[string]interface{}, error) {
	values := map[string]interface{}{}
	for jsonName, value := range input {
		field, ok := resource.FieldsByJSON[jsonName]
		if !ok {
			return nil, exception.Validate(fmt.Sprintf("未知字段: %s", jsonName))
		}
		if resource.ReadonlyFields[jsonName] {
			if isUpdate {
				continue
			}
			return nil, exception.Validate(fmt.Sprintf("字段不可写: %s", jsonName))
		}
		values[field.ColumnName] = value
	}
	return values, nil
}

/**
 * 构建查询条件
 * @param resource 资源定义
 * @param request 查询请求
 * @param tenantCondition 租户条件
 * @returns SQL 和参数
 */
func buildWhereClause(resource Resource, request QueryRequest, tenantCondition tenant.Condition) (string, []interface{}, error) {
	parts := make([]string, 0)
	args := make([]interface{}, 0)
	if tenantCondition.SQL != "" {
		parts = append(parts, tenantCondition.SQL)
		args = append(args, tenantCondition.Args...)
	}
	if request.Keyword != "" && len(resource.KeywordFields) > 0 {
		keywordParts := make([]string, 0, len(resource.KeywordFields))
		for _, field := range sortedFieldMap(resource.KeywordFields) {
			keywordParts = append(keywordParts, fmt.Sprintf("%s LIKE ?", quoteIdentifier(field.ColumnName)))
			args = append(args, "%"+request.Keyword+"%")
		}
		parts = append(parts, "("+strings.Join(keywordParts, " OR ")+")")
	}
	for _, fieldName := range sortedKeys(request.FieldEq) {
		field, ok := resource.EqualFields[fieldName]
		if !ok {
			return "", nil, exception.Validate(fmt.Sprintf("不支持等值查询字段: %s", fieldName))
		}
		value := request.FieldEq[fieldName]
		if values, ok := sliceValues(value); ok {
			if len(values) == 0 {
				parts = append(parts, "1 = 0")
				continue
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
			parts = append(parts, fmt.Sprintf("%s IN (%s)", quoteIdentifier(field.ColumnName), placeholders))
			args = append(args, values...)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s = ?", quoteIdentifier(field.ColumnName)))
		args = append(args, value)
	}
	for _, fieldName := range sortedKeys(request.FieldLike) {
		field, ok := resource.LikeFields[fieldName]
		if !ok {
			return "", nil, exception.Validate(fmt.Sprintf("不支持模糊查询字段: %s", fieldName))
		}
		parts = append(parts, fmt.Sprintf("%s LIKE ?", quoteIdentifier(field.ColumnName)))
		args = append(args, "%"+fmt.Sprint(request.FieldLike[fieldName])+"%")
	}
	if len(parts) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

/**
 * 构建排序条件
 * @param resource 资源定义
 * @param request 查询请求
 * @returns SQL
 */
func buildOrderClause(resource Resource, request QueryRequest) (string, error) {
	columns := make(map[string]string, len(resource.SortFields))
	for name, field := range resource.SortFields {
		columns[name] = field.ColumnName
	}
	defaultField := resource.Spec.DefaultSort
	if defaultField == "" {
		defaultField = resource.PrimaryField.JSONName
	}
	terms, err := ResolveSortTerms(request, columns, defaultField, resource.Spec.DefaultOrder)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		parts = append(parts, fmt.Sprintf("%s %s", quoteIdentifier(term.Column), term.Order))
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}

// 校验 Node order/sort 多字段约定并映射数据库列
func ResolveSortTerms(request QueryRequest, columns map[string]string, defaultField string, defaultOrder string) ([]SortTerm, error) {
	fieldText := strings.TrimSpace(request.Sort)
	orderText := strings.TrimSpace(request.Order)
	if fieldText == "" {
		fieldText = defaultField
	}
	if orderText == "" {
		orderText = defaultOrder
	}
	fields := splitSortValues(fieldText)
	orders := splitSortValues(orderText)
	if len(fields) == 0 {
		return nil, exception.Validate("排序字段不能为空")
	}
	if len(orders) == 0 {
		orders = []string{"DESC"}
	}
	if len(fields) != len(orders) {
		return nil, exception.Validate("排序字段与方向数量不一致")
	}
	terms := make([]SortTerm, 0, len(fields))
	for index, field := range fields {
		column, ok := columns[field]
		if !ok {
			return nil, exception.Validate(fmt.Sprintf("不支持排序字段: %s", field))
		}
		order := strings.ToUpper(orders[index])
		if order != "ASC" && order != "DESC" {
			return nil, exception.Validate(fmt.Sprintf("不支持排序方向: %s", orders[index]))
		}
		terms = append(terms, SortTerm{Column: column, Order: order})
	}
	return terms, nil
}

func splitSortValues(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func sliceValues(value interface{}) ([]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	typed := reflect.ValueOf(value)
	if typed.Kind() != reflect.Array && typed.Kind() != reflect.Slice {
		return nil, false
	}
	values := make([]interface{}, typed.Len())
	for index := 0; index < typed.Len(); index++ {
		values[index] = typed.Index(index).Interface()
	}
	return values, true
}

/**
 * 查询字段列表
 * @param resource 资源定义
 * @param infoOnly 是否仅选择详情字段
 * @returns SQL 字段列表
 */
func selectColumns(resource Resource, infoOnly ...bool) string {
	columns := make([]string, 0, len(resource.Spec.Model.FieldsValue))
	shouldIgnoreInfoFields := len(infoOnly) > 0 && infoOnly[0]
	for _, field := range resource.Spec.Model.FieldsValue {
		if resource.HiddenFields[field.JSONName] || (shouldIgnoreInfoFields && resource.InfoIgnoreFields[field.JSONName]) {
			continue
		}
		columns = append(columns, fmt.Sprintf("%s AS %s", quoteIdentifier(field.ColumnName), quoteIdentifier(field.JSONName)))
	}
	return strings.Join(columns, ", ")
}

/**
 * 规范化分页请求
 * @param request 请求参数
 * @returns 查询请求
 */
func NormalizePageRequest(request QueryRequest) QueryRequest {
	if request.Page <= 0 {
		request.Page = defaultPage
	}
	if request.Size <= 0 {
		request.Size = defaultSize
	}
	if request.Size > MaxPageSize {
		request.Size = MaxPageSize
	}
	if request.IsExport && (request.MaxExportLimit <= 0 || request.MaxExportLimit > MaxExportSize) {
		request.MaxExportLimit = MaxExportSize
	}
	return request
}

/**
 * 引用 SQL 标识符
 * @param name 标识符
 * @returns 已引用标识符
 */
func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

/**
 * 排序 map key
 * @param values map 值
 * @returns key 列表
 */
func sortedKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

/**
 * 排序字段 map
 * @param fields 字段 map
 * @returns 字段列表
 */
func sortedFieldMap(fields map[string]entity.Field) []entity.Field {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]entity.Field, 0, len(keys))
	for _, key := range keys {
		items = append(items, fields[key])
	}
	return items
}

/**
 * 是否为空值
 * @param value 原始值
 * @returns bool
 */
func isEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}
