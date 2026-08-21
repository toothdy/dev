package eps

import (
	"strconv"
	"strings"
)

// LegacyController 是 cool-admin-vue（@cool-vue/vite-plugin）期望的扁平 EPS Controller 契约。
// vite-plugin 通过 `lodash.values(data).flat()` 展开 /admin/base/open/eps 与 /app/base/comm/eps
// 的响应体，要求每个数组元素直接携带 prefix、api、columns 等字段，与 cool-admin-node（v1）及
// cool-admin-go-next-v1 输出的历史契约一致。
type LegacyController struct {
	Module      string            `json:"module"`
	Name        string            `json:"name"`
	Prefix      string            `json:"prefix"`
	Info        LegacyInfo        `json:"info"`
	API         []LegacyAPI       `json:"api"`
	Columns     []LegacyColumn    `json:"columns"`
	PageQueryOp LegacyPageQueryOp `json:"pageQueryOp"`
	PageColumns []LegacyColumn    `json:"pageColumns"`
}

// LegacyInfo 是 Controller 附加信息
type LegacyInfo struct {
	Type LegacyInfoType `json:"type"`
}

// LegacyInfoType 是 Controller 类型信息
type LegacyInfoType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LegacyAPI 是 cool-admin-vue 契约中的单个接口元数据
type LegacyAPI struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Summary     string                 `json:"summary"`
	DTS         map[string]interface{} `json:"dts"`
	Tag         string                 `json:"tag"`
	Prefix      string                 `json:"prefix"`
	IgnoreToken bool                   `json:"ignoreToken"`
}

// LegacyColumn 是 cool-admin-vue 契约中的单个字段元数据
type LegacyColumn struct {
	PropertyName string      `json:"propertyName"`
	Type         string      `json:"type"`
	Length       string      `json:"length"`
	Comment      string      `json:"comment"`
	Nullable     bool        `json:"nullable"`
	DefaultValue interface{} `json:"defaultValue"`
	Dict         interface{} `json:"dict"`
	Source       string      `json:"source"`
}

// LegacyPageQueryOp 是分页查询的字段匹配配置。当前 EPS 编译流程尚未透传
// FieldEq/FieldLike/KeyWordLikeFields 分类信息，故始终为空切片；
// cool-admin-vue 对此已有兜底处理，不会因此报错。
type LegacyPageQueryOp struct {
	KeyWordLikeFields []string `json:"keyWordLikeFields"`
	FieldEq           []string `json:"fieldEq"`
	FieldLike         []string `json:"fieldLike"`
}

// LegacyView 将编译文档投影为 cool-admin-vue 客户端期望的扁平契约：按模块 Key 分组的 Controller 数组
func LegacyView(document Document) map[string][]LegacyController {
	result := make(map[string][]LegacyController, len(document.Modules))
	for _, module := range document.Modules {
		if len(module.Controllers) == 0 {
			continue
		}
		items := make([]LegacyController, 0, len(module.Controllers))
		for _, controller := range module.Controllers {
			items = append(items, legacyController(module.Key, controller))
		}
		result[module.Key] = items
	}

	return result
}

func legacyController(moduleKey string, controller Controller) LegacyController {
	var (
		entityName string
		columns    = make([]LegacyColumn, 0)
	)
	if controller.Entity != nil {
		entityName = controller.Entity.Name
		columns = legacyColumns(controller.Entity.Fields)
	}
	apis := make([]LegacyAPI, 0, len(controller.API))
	for _, api := range controller.API {
		apis = append(apis, LegacyAPI{
			Method:      api.Method,
			Path:        api.Path,
			Summary:     api.Summary,
			DTS:         map[string]interface{}{},
			Prefix:      controller.Prefix,
			IgnoreToken: !api.Authenticated,
		})
	}

	return LegacyController{
		Module: moduleKey,
		Name:   entityName,
		Prefix: controller.Prefix,
		Info: LegacyInfo{Type: LegacyInfoType{
			Name:        legacyControllerTypeName(controller.Prefix),
			Description: controller.Description,
		}},
		API:     apis,
		Columns: columns,
		PageQueryOp: LegacyPageQueryOp{
			KeyWordLikeFields: []string{},
			FieldEq:           []string{},
			FieldLike:         []string{},
		},
		PageColumns: make([]LegacyColumn, 0),
	}
}

// legacyColumns 将字段列表转换为遗留 Column 列表，并将 createTime/updateTime 移到末尾，与 v1 行为一致
func legacyColumns(fields []Field) []LegacyColumn {
	columns := make([]LegacyColumn, 0, len(fields))
	trailing := make([]LegacyColumn, 0, 2)
	for _, field := range fields {
		column := legacyColumn(field)
		if field.JSONName == "createTime" || field.JSONName == "updateTime" {
			trailing = append(trailing, column)
			continue
		}
		columns = append(columns, column)
	}

	return append(columns, trailing...)
}

func legacyColumn(field Field) LegacyColumn {
	column := LegacyColumn{
		PropertyName: field.JSONName,
		Type:         legacyColumnType(field.DatabaseType),
		Comment:      field.Description,
		Nullable:     field.Nullable,
		Source:       field.Source,
	}
	if field.Size != nil {
		column.Length = strconv.FormatUint(*field.Size, 10)
	}
	if field.HasDefault {
		column.DefaultValue = field.Default
	}

	return column
}

// legacyColumnType 将 EPS 逻辑类型映射为 cool-admin-vue 前端类型推断表识别的历史类型名
// （对齐 cool-admin-node 与 cool-admin-go-next-v1 的 epsType 输出）
func legacyColumnType(databaseType string) string {
	switch databaseType {
	case "bool":
		return "boolean"
	case "int", "uint":
		return "int"
	case "float":
		return "decimal"
	case "string":
		return "varchar"
	case "bytes":
		return "text"
	case "time":
		return "datetime"
	case "json":
		return "json"
	default:
		return databaseType
	}
}

// legacyControllerTypeName 从 Controller 前缀提取类型名，与 v1 controllerTypeName 行为一致
func legacyControllerTypeName(prefix string) string {
	trimmed := strings.TrimPrefix(prefix, "/admin/")
	trimmed = strings.TrimPrefix(trimmed, "/app/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")

	return parts[len(parts)-1]
}
