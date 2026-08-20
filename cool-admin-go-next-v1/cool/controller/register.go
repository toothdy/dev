package controller

import (
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/util/route"
)

type compiledRoute struct {
	method  string
	path    string
	key     string
	module  string
	handler ghttp.HandlerFunc
}

// 完成全部校验后的不可变路由注册计划
type RoutePlan struct {
	routes []compiledRoute
}

// 在修改 server 前编译所有自定义和 CRUD 路由
func CompileRoutePlan(runtime *crud.Runtime, controllers []Definition) (*RoutePlan, error) {
	plan := &RoutePlan{}
	seen := map[string]string{}
	for _, definition := range controllers {
		controllerName := definition.Name
		if controllerName == "" {
			controllerName = definition.Prefix
		}
		for _, route := range definition.Routes {
			location := routeLocation(controllerName, route.Name, route.Method, route.FullPath)
			if strings.TrimSpace(route.Name) == "" {
				return nil, exception.Core(location + ": route name 不能为空")
			}
			key, method, path, err := canonicalRoute(route.Method, route.FullPath)
			if err != nil {
				return nil, exception.Core(fmt.Sprintf("%s: %v", location, err))
			}
			if err = validateBindOptions(method, route.Bind); err != nil {
				return nil, exception.Core(fmt.Sprintf("%s: %v", location, err))
			}
			action, err := compileAction(route.Action)
			if err != nil {
				return nil, exception.Core(fmt.Sprintf("%s: %v", location, err))
			}
			if previous, exists := seen[key]; exists {
				return nil, exception.Core(fmt.Sprintf("路由冲突 %s: %s 与 %s", key, previous, location))
			}
			seen[key] = location
			plan.routes = append(plan.routes, compiledRoute{
				method: method,
				path:   path,
				key:    key,
				module: definition.Module,
				handler: action.handler(bindOptions{
					method:             method,
					source:             route.Bind,
					allowUnknownFields: route.AllowUnknownFields,
				}),
			})
		}
		if definition.CRUD == nil || runtime == nil {
			continue
		}
		if runtime.Registry() == nil {
			return nil, exception.Core("CRUD 资源注册表不能为空")
		}
		resourceName := resourceNameFromPrefix(definition.Prefix)
		resource, ok := runtime.Registry().Resource(resourceName)
		if !ok {
			return nil, exception.Core(fmt.Sprintf("CRUD 资源不存在: %s", resourceName))
		}
		for _, api := range []string{
			crud.Add, crud.Delete, crud.Update,
			crud.Info, crud.List, crud.Page,
		} {
			if !resource.API[api] {
				continue
			}
			method, ok := crud.RouteMethod(api)
			if !ok {
				return nil, exception.Core(fmt.Sprintf("CRUD API 不受支持: %s", api))
			}
			fullPath := resource.Spec.Prefix + "/" + api
			location := routeLocation(controllerName, api, method, fullPath)
			key, canonicalMethod, canonicalPath, err := canonicalRoute(method, fullPath)
			if err != nil {
				return nil, exception.Core(fmt.Sprintf("%s: %v", location, err))
			}
			if previous, exists := seen[key]; exists {
				return nil, exception.Core(fmt.Sprintf("路由冲突 %s: %s 与 %s", key, previous, location))
			}
			seen[key] = location
			plan.routes = append(plan.routes, compiledRoute{
				method:  canonicalMethod,
				path:    canonicalPath,
				key:     key,
				module:  definition.Module,
				handler: crudRouteHandler(runtime, resource, api),
			})
		}
	}
	return plan, nil
}

// 将已验证的路由计划一次性提交给 GoFrame server
func (p *RoutePlan) Bind(server *ghttp.Server) error {
	return p.BindWithMiddlewares(server, nil)
}

// 将模块中间件附着到对应路由后提交给 GoFrame server
func (p *RoutePlan) BindWithMiddlewares(
	server *ghttp.Server,
	middlewares map[string][]ghttp.HandlerFunc,
) error {
	if server == nil {
		return exception.Core("HTTP 服务不能为空")
	}
	if p == nil {
		return exception.Core("路由计划不能为空")
	}
	groups := make(map[string]*ghttp.RouterGroup)
	for _, route := range p.routes {
		handlers := middlewares[route.module]
		if len(handlers) == 0 {
			server.BindHandler(route.key, route.handler)
			continue
		}
		group := groups[route.module]
		if group == nil {
			group = server.Group("/").Middleware(handlers...)
			groups[route.module] = group
		}
		bindGroupRoute(group, route)
	}
	return nil
}

// 通过 RouterGroup 支持的精确 HTTP 方法绑定路由
func bindGroupRoute(group *ghttp.RouterGroup, route compiledRoute) {
	switch route.method {
	case "DELETE":
		group.DELETE(route.path, route.handler)
	case "GET":
		group.GET(route.path, route.handler)
	case "HEAD":
		group.HEAD(route.path, route.handler)
	case "OPTIONS":
		group.OPTIONS(route.path, route.handler)
	case "PATCH":
		group.PATCH(route.path, route.handler)
	case "POST":
		group.POST(route.path, route.handler)
	case "PUT":
		group.PUT(route.path, route.handler)
	}
}

// 兼容现有调用，并保证先完整编译再注册
func RegisterRoutes(server *ghttp.Server, runtime *crud.Runtime, controllers []Definition) error {
	if server == nil {
		return exception.Core("HTTP 服务不能为空")
	}
	plan, err := CompileRoutePlan(runtime, controllers)
	if err != nil {
		return err
	}
	return plan.Bind(server)
}

func canonicalRoute(method string, path string) (key string, canonicalMethod string, canonicalPath string, err error) {
	key, err = route.Key(method, path)
	if err != nil {
		return "", "", "", err
	}
	canonicalMethod, canonicalPath, _ = strings.Cut(key, ":")
	return key, canonicalMethod, canonicalPath, nil
}

func routeLocation(controllerName string, routeName string, method string, path string) string {
	return fmt.Sprintf("controller=%s route=%s method=%s path=%s", controllerName, routeName, method, path)
}
