package controller

import (
	"fmt"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/util/route"
)

/**
 * 获取忽略认证路径
 * @param controllers controller 元数据
 * @returns []string
 */
func IgnoreAuthPaths(controllers []Definition) []string {
	paths := make([]string, 0)
	for _, definition := range controllers {
		for _, r := range definition.Routes {
			if r.IgnoreAuth {
				paths = append(paths, r.FullPath)
			}
		}
	}
	return paths
}

// 按 method 和 path 生成认证白名单
func IgnoreAuthRouteKeys(controllers []Definition) (map[string]struct{}, error) {
	keys := map[string]struct{}{}
	for _, definition := range controllers {
		for _, item := range definition.Routes {
			if !item.IgnoreAuth {
				continue
			}
			key, err := route.Key(item.Method, item.FullPath)
			if err != nil {
				return nil, err
			}
			keys[key] = struct{}{}
		}
	}
	return keys, nil
}

/**
 * 构建权限映射
 * @param controllers controller 元数据
 * @returns 权限映射
 */
func PermissionMap(controllers []Definition) (map[string]string, error) {
	permissions := map[string]string{}
	for _, definition := range controllers {
		if definition.CRUD != nil {
			for _, api := range definition.CRUD.API {
				routeKey, ok := crud.RouteKey(definition.Prefix, api)
				if !ok {
					return nil, fmt.Errorf("unsupported CRUD api: %s", api)
				}
				permissions[routeKey] = permissionCode(definition.Prefix, api)
			}
		}
		for _, r := range definition.Routes {
			if r.IgnoreAuth || r.Permission == "" {
				continue
			}
			key, err := route.Key(r.Method, r.FullPath)
			if err != nil {
				return nil, err
			}
			permissions[key] = r.Permission
		}
	}
	return permissions, nil
}

/**
 * 生成 CRUD 资源配置
 * @param controllers controller 元数据
 * @returns []crud.ResourceSpec
 */
func CRUDResourceSpecs(controllers []Definition) ([]crud.ResourceSpec, error) {
	specs := make([]crud.ResourceSpec, 0)
	for _, definition := range controllers {
		if definition.CRUD == nil {
			continue
		}
		resourceName := resourceNameFromPrefix(definition.Prefix)
		specs = append(specs, crud.ResourceSpec{
			Name:        resourceName,
			Prefix:      definition.Prefix,
			Model:       definition.Model,
			Service:     definition.Service,
			InsertParam: definition.CRUD.InsertParam,
			API:        cloneStrings(definition.CRUD.API),
			ListQuery: crud.QuerySpec{
				KeywordFields: cloneStrings(definition.CRUD.ListQuery.KeyWordLikeFields),
				EqualFields:   cloneStrings(definition.CRUD.ListQuery.FieldEq),
				LikeFields:    cloneStrings(definition.CRUD.ListQuery.FieldLike),
			},
			PageQuery: crud.QuerySpec{
				KeywordFields: cloneStrings(definition.CRUD.PageQuery.KeyWordLikeFields),
				EqualFields:   cloneStrings(definition.CRUD.PageQuery.FieldEq),
				LikeFields:    cloneStrings(definition.CRUD.PageQuery.FieldLike),
			},
			KeywordFields:    cloneStrings(definition.CRUD.PageQuery.KeyWordLikeFields),
			EqualFields:      cloneStrings(definition.CRUD.PageQuery.FieldEq),
			LikeFields:       cloneStrings(definition.CRUD.PageQuery.FieldLike),
			SortFields:       cloneStrings(definition.CRUD.SortFields),
			HiddenFields:     cloneStrings(definition.CRUD.HiddenFields),
			ReadonlyFields:   cloneStrings(definition.CRUD.ReadonlyFields),
			InfoIgnoreFields: cloneStrings(definition.CRUD.InfoIgnoreFields),
			DefaultSort:      definition.CRUD.DefaultSort,
			DefaultOrder:     definition.CRUD.DefaultOrder,
		})
	}
	return specs, nil
}

/**
 * 生成权限码
 * @param prefix 路由前缀
 * @param api CRUD API
 * @returns string
 */
func permissionCode(prefix string, api string) string {
	resource := strings.Trim(strings.TrimPrefix(prefix, "/admin/"), "/")
	return strings.ReplaceAll(resource, "/", ":") + ":" + api
}

/**
 * 从前缀提取资源名
 * @param prefix 路由前缀
 * @returns string
 */
func resourceNameFromPrefix(prefix string) string {
	return strings.Trim(strings.TrimPrefix(prefix, "/admin/"), "/")
}
