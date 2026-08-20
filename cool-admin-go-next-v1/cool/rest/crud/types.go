package crud

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

const (
	// 新增接口
	Add = "add"
	// 删除接口
	Delete = "delete"
	// 更新接口
	Update = "update"
	// 详情接口
	Info = "info"
	// 列表接口
	List = "list"
	// 分页接口
	Page = "page"
)

var Methods = map[string]string{
	Add:    http.MethodPost,
	Delete: http.MethodPost,
	Update: http.MethodPost,
	Info:   http.MethodGet,
	List:   http.MethodPost,
	Page:   http.MethodPost,
}

const (
	defaultPage = 1
	defaultSize = 15
	// 同步分页的服务端硬上限
	MaxPageSize = 200
	// 同步导出的服务端硬上限
	MaxExportSize = 10000
	// 无分页列表的服务端硬上限
	MaxListSize = 1000
	// 批量写入的服务端硬上限
	MaxBatchSize = 500
)

// CRUD 查询字段配置
type QuerySpec struct {
	KeywordFields []string
	EqualFields   []string
	LikeFields    []string
}

// CRUD 资源配置
type ResourceSpec struct {
	Name             string
	Prefix           string
	Model            entity.Definition
	Service          interface{}
	InsertParam      func(ctx context.Context) map[string]interface{}
	API              []string
	ListQuery        QuerySpec
	PageQuery        QuerySpec
	KeywordFields    []string
	EqualFields      []string
	LikeFields       []string
	SortFields       []string
	HiddenFields     []string
	ReadonlyFields   []string
	InfoIgnoreFields []string
	DefaultSort      string
	DefaultOrder     string
}

// 列表和分页查询请求
type QueryRequest struct {
	Page           int
	Size           int
	Keyword        string
	Sort           string
	Order          string
	IsExport       bool
	MaxExportLimit int
	FieldEq        map[string]interface{}
	FieldLike      map[string]interface{}
	Raw            map[string]interface{}
}

// 校验后的排序字段
type SortTerm struct {
	Column string
	Order  string
}

// 分页响应数据
type PageResult struct {
	List       []map[string]interface{} `json:"list"`
	Pagination Pagination               `json:"pagination"`
}

// 分页信息
type Pagination struct {
	Page  int `json:"page"`
	Size  int `json:"size"`
	Total int `json:"total"`
}

// 判断资源是否启用接口
func (s ResourceSpec) HasAPI(api string) bool {
	for _, item := range s.API {
		if item == api {
			return true
		}
	}
	return false
}

// 返回 CRUD 接口对应的 HTTP 方法
func RouteMethod(api string) (string, bool) {
	method, ok := Methods[api]
	return method, ok
}

// 返回 CRUD 接口完整路由键
func RouteKey(prefix string, api string) (string, bool) {
	method, ok := RouteMethod(api)
	if !ok {
		return "", false
	}
	return method + ":" + prefix + "/" + api, true
}
