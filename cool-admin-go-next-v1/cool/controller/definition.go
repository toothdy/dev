package controller

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// controller 分区
type Area string

const (
	AreaAdmin Area = "admin" // 后台管理接口
	AreaOpen  Area = "open"  // 后台开放接口
	AreaComm  Area = "comm"  // 后台通用接口
	AreaApp   Area = "app"   // 应用端接口
)

// 新增默认参数函数
type InsertParamFunc func(ctx context.Context) map[string]interface{}

// controller 元数据
type Definition struct {
	Module      string            // 模块名称
	Area        Area              // controller 分区
	Prefix      string            // 前缀路径
	Name        string            // controller 名称
	Description string            // 描述信息
	Model       entity.Definition // 实体定义信息
	Service     interface{}       // 服务接口
	CRUD        *CRUDDefinition   // 增删改查接口定义
	Routes      []RouteDefinition // 路由定义列表
}

// 增删改查接口配置选项
type CRUDOptions struct {
	API              []string            // API 列表
	PageQuery        QueryOptions        // 分页查询配置
	ListQuery        QueryOptions        // 列表查询配置
	PageSelect       []PageSelectOptions // 分页联表配置列表
	InsertParam      InsertParamFunc     // 新增默认参数函数
	InfoIgnoreFields []string            // 忽略的字段列表
	SortFields       []string            // 排序字段列表
	HiddenFields     []string            // 隐藏字段列表
	ReadonlyFields   []string            // 读取字段列表
	DefaultSort      string              // 默认排序字段
	DefaultOrder     string              // 默认排序顺序
}

// 增删改查接口定义
type CRUDDefinition struct {
	API              []string            // API 列表
	PageQuery        QueryOptions        // 分页查询配置
	ListQuery        QueryOptions        // 列表查询配置
	PageSelect       []PageSelectOptions // 分页联表配置列表
	InsertParam      InsertParamFunc     // 新增默认参数函数
	InfoIgnoreFields []string            // 忽略的字段列表
	SortFields       []string            // 排序字段列表
	HiddenFields     []string            // 隐藏字段列表
	ReadonlyFields   []string            // 读取字段列表
	DefaultSort      string              // 默认排序字段
	DefaultOrder     string              // 默认排序顺序
}

// 查询配置
type QueryOptions struct {
	KeyWordLikeFields []string // 关键字模糊查询字段列表
	FieldEq           []string // 等于查询字段列表
	FieldLike         []string // 模糊查询字段列表
}

// 分页联表配置
type PageSelectOptions struct {
	Alias  string            // 联表别名
	Model  entity.Definition // 关联实体
	Fields []string          // 关联字段列表
}

// DTO 请求绑定
type BindSource string

const (
	BindAuto  BindSource = "auto"  // 自动绑定请求参数
	BindQuery BindSource = "query" // 查询参数绑定
	BindForm  BindSource = "form"  // 表单参数绑定
	BindJSON  BindSource = "json"  // JSON 参数绑定
)

// 自定义路由配置
type RouteOptions struct {
	Name               string      // 路由名称
	Method             string      // 请求方法
	Path               string      // 路由路径
	Description        string      // 路由描述
	IgnoreAuth         bool        // 是否忽略认证
	Permission         string      // 权限标识
	Action             interface{} // 请求处理函数
	Bind               BindSource  // 请求绑定来源
	AllowUnknownFields bool        // 是否允许未知字段
}

// 自定义路由定义
type RouteDefinition struct {
	Name               string      // 路由名称
	Method             string      // 请求方法
	Path               string      // 路由路径
	FullPath           string      // 完整路径
	Description        string      // 路由描述
	IgnoreAuth         bool        // 是否忽略认证
	Permission         string      // 权限标识
	Action             interface{} // 请求处理函数
	Bind               BindSource  // 请求绑定来源
	AllowUnknownFields bool        // 是否允许未知字段
}
