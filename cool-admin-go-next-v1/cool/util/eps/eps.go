package eps

import (
	"strconv"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

// EPS controller 元数据
type Controller struct {
	Module      string                 `json:"module"`
	Name        string                 `json:"name"`
	Prefix      string                 `json:"prefix"`
	Info        map[string]interface{} `json:"info"`
	API         []API                  `json:"api"`
	Columns     []Column               `json:"columns"`
	PageQueryOp PageQueryOp            `json:"pageQueryOp"`
	PageColumns []Column               `json:"pageColumns"`
}

// EPS 接口元数据
type API struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Summary     string                 `json:"summary"`
	DTS         map[string]interface{} `json:"dts"`
	Tag         string                 `json:"tag"`
	Prefix      string                 `json:"prefix"`
	IgnoreToken bool                   `json:"ignoreToken"`
}

// EPS 字段元数据
type Column struct {
	PropertyName string      `json:"propertyName"`
	Type         string      `json:"type"`
	Length       string      `json:"length"`
	Comment      string      `json:"comment"`
	Nullable     bool        `json:"nullable"`
	DefaultValue interface{} `json:"defaultValue"`
	Dict         interface{} `json:"dict"`
	Source       string      `json:"source"`
}

// EPS 分页查询字段配置
type PageQueryOp struct {
	KeyWordLikeFields []string `json:"keyWordLikeFields"`
	FieldEq           []string `json:"fieldEq"`
	FieldLike         []string `json:"fieldLike"`
}

var apiSummaries = map[string]string{
	crud.Add:    "新增",
	crud.Delete: "删除",
	crud.Update: "修改",
	crud.Info:   "单个信息",
	crud.List:   "列表查询",
	crud.Page:   "分页查询",
}

// 从后台 Controller metadata 生成按模块分组的 EPS
func Generate(definitions []controller.Definition) map[string][]Controller {
	return GenerateAdmin(definitions)
}

// 生成后台 EPS，不包含应用端 Controller
func GenerateAdmin(definitions []controller.Definition) map[string][]Controller {
	return generate(definitions, func(definition controller.Definition) bool {
		return definition.Area != controller.AreaApp
	})
}

// 生成应用端 EPS
func GenerateApp(definitions []controller.Definition) map[string][]Controller {
	return generate(definitions, func(definition controller.Definition) bool {
		return definition.Area == controller.AreaApp
	})
}

func generate(definitions []controller.Definition, include func(controller.Definition) bool) map[string][]Controller {
	result := map[string][]Controller{}
	for _, definition := range definitions {
		if !include(definition) {
			continue
		}
		if definition.CRUD == nil && len(definition.Routes) == 0 {
			continue
		}
		result[definition.Module] = append(result[definition.Module], buildController(definition))
	}
	return result
}

// 将 controller metadata 映射为 EPS controller
func buildController(definition controller.Definition) Controller {
	item := Controller{
		Module:      definition.Module,
		Name:        definition.Name,
		Prefix:      definition.Prefix,
		Info:        map[string]interface{}{"type": map[string]string{"name": controllerTypeName(definition.Prefix), "description": definition.Description}},
		API:         make([]API, 0),
		Columns:     make([]Column, 0, len(definition.Model.FieldsValue)),
		PageQueryOp: emptyPageQueryOp(),
		PageColumns: make([]Column, 0),
	}

	if definition.CRUD != nil {
		item.API = make([]API, 0, len(definition.CRUD.API)+len(definition.Routes))
		item.PageQueryOp = PageQueryOp{
			KeyWordLikeFields: withAlias(definition.CRUD.PageQuery.KeyWordLikeFields),
			FieldEq:           withAlias(definition.CRUD.PageQuery.FieldEq),
			FieldLike:         withAlias(definition.CRUD.PageQuery.FieldLike),
		}
		for _, api := range definition.CRUD.API {
			method, ok := crud.RouteMethod(api)
			if !ok {
				continue
			}
			item.API = append(item.API, API{
				Method:  method,
				Path:    "/" + api,
				Summary: apiSummaries[api],
				DTS:     map[string]interface{}{},
				Prefix:  definition.Prefix,
			})
		}
		item.PageColumns = buildPageColumns(definition.CRUD.PageSelect)
	}

	for _, route := range definition.Routes {
		summary := route.Description
		if summary == "" {
			summary = route.Name
		}
		item.API = append(item.API, API{
			Method:      route.Method,
			Path:        route.Path,
			Summary:     summary,
			DTS:         map[string]interface{}{},
			Prefix:      definition.Prefix,
			IgnoreToken: route.IgnoreAuth,
		})
	}

	commonColumns := make([]Column, 0, 2)
	for _, field := range definition.Model.FieldsValue {
		if field.JSONName == "tenantId" || isInternalColumn(definition, field.JSONName) {
			continue
		}
		column := buildColumn(field)
		if field.JSONName == "createTime" || field.JSONName == "updateTime" {
			commonColumns = append(commonColumns, column)
			continue
		}
		item.Columns = append(item.Columns, column)
	}
	item.Columns = append(item.Columns, commonColumns...)
	return item
}

func isInternalColumn(definition controller.Definition, fieldName string) bool {
	if definition.CRUD == nil {
		return false
	}
	return containsString(definition.CRUD.HiddenFields, fieldName) && containsString(definition.CRUD.ReadonlyFields, fieldName)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func buildPageColumns(selects []controller.PageSelectOptions) []Column {
	columns := make([]Column, 0)
	timeColumns := make([]Column, 0, 2)
	for _, selectOption := range selects {
		selected := make(map[string]bool, len(selectOption.Fields))
		for _, fieldName := range selectOption.Fields {
			selected[fieldName] = true
		}
		for _, field := range selectOption.Model.FieldsValue {
			if field.JSONName == "tenantId" || (len(selected) > 0 && !selected[field.JSONName]) {
				continue
			}
			column := buildColumn(field)
			column.Source = selectOption.Alias + "." + field.JSONName
			if field.JSONName == "createTime" || field.JSONName == "updateTime" {
				timeColumns = append(timeColumns, column)
				continue
			}
			columns = append(columns, column)
		}
	}
	return append(columns, timeColumns...)
}

// 将模型字段映射为 EPS 字段
func buildColumn(field entity.Field) Column {
	return Column{
		PropertyName: field.JSONName,
		Type:         epsType(field.DataType),
		Length:       fieldLength(field.Length),
		Comment:      field.CommentText,
		Nullable:     field.IsNullable,
		DefaultValue: defaultValue(field),
		Dict:         fieldDict(field.Dict),
		Source:       "a." + field.JSONName,
	}
}

// 从 controller 前缀提取类型名
func controllerTypeName(prefix string) string {
	path := strings.TrimPrefix(prefix, "/admin/")
	path = strings.Trim(strings.TrimPrefix(path, "/app/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// 创建包含空切片的分页查询配置
func emptyPageQueryOp() PageQueryOp {
	return PageQueryOp{
		KeyWordLikeFields: make([]string, 0),
		FieldEq:           make([]string, 0),
		FieldLike:         make([]string, 0),
	}
}

// 为查询字段补充 a. 别名
func withAlias(fields []string) []string {
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.Contains(field, ".") {
			result = append(result, field)
			continue
		}
		result = append(result, "a."+field)
	}
	return result
}

// 将字段长度映射为 EPS 字符串
func fieldLength(length int) string {
	if length == 0 {
		return ""
	}
	return strconv.Itoa(length)
}

// 复制字段字典，避免暴露模型元数据切片
func fieldDict(dict []string) interface{} {
	if len(dict) == 0 {
		return nil
	}
	return append([]string{}, dict...)
}

// 将模型字段类型映射为 EPS 类型
func epsType(dataType string) string {
	switch strings.ToLower(dataType) {
	case "bigint", "int", "integer", "uint64", "tinyint":
		return "int"
	case "varchar", "string", "text":
		return "varchar"
	case "bool", "boolean":
		return "boolean"
	case "time", "datetime", "timestamp":
		return "datetime"
	case "json":
		return "json"
	default:
		return strings.ToLower(dataType)
	}
}

// 将模型默认值转换为 EPS JSON 标量
func defaultValue(field entity.Field) interface{} {
	if !field.HasDefault {
		return nil
	}

	dataType := strings.ToLower(field.DataType)
	switch dataType {
	case "bigint", "int", "integer", "uint64", "tinyint":
		value, err := strconv.ParseInt(field.DefaultValue, 10, 64)
		if err == nil {
			return value
		}
	case "bool", "boolean":
		value, err := strconv.ParseBool(field.DefaultValue)
		if err == nil {
			return value
		}
	}
	return field.DefaultValue
}
