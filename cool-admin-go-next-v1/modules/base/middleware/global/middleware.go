package global

import (
	"path/filepath"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
	baseEvent "github.com/toothdy/cool-admin-go-next/modules/base/event"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

const (
	translateOrder  = 100
	authorityOrder  = 200
	permissionOrder = 300
	logOrder        = 400
)

var authorityPrefixes = []string{"/admin/", "/app/"}

// TranslateDefinition 构建响应翻译全局中间件。
func TranslateDefinition(deps module.MiddlewareDeps, options module.I18nOptions) (middleware.Definition, error) {
	translator := deps.Translator
	if options.Enabled && translator == nil {
		loaded, err := NewMapTranslator(filepath.Join("resource", "locales"))
		if err != nil {
			return middleware.Definition{}, err
		}
		translator = loaded
	}
	return middleware.Definition{
		Name:  "base.translate",
		Order: translateOrder,
		Handler: NewTranslate(TranslateOptions{
			Enabled:    options.Enabled,
			Languages:  append([]string{}, options.Languages...),
			Translator: translator,
		}),
	}, nil
}

// AuthorityDefinitions 构建认证与权限全局中间件。
func AuthorityDefinitions(
	deps module.MiddlewareDeps,
	config baseModule.Config,
	options module.AuthOptions,
	permissionService *baseSysService.PermissionService,
) ([]middleware.Definition, error) {
	if !config.Middleware.Authority.Enable {
		return nil, nil
	}
	ignoreRouteKeys, err := controller.IgnoreAuthRouteKeys(deps.Controllers)
	if err != nil {
		return nil, err
	}
	permissions, err := controller.PermissionMap(deps.Controllers)
	if err != nil {
		return nil, err
	}
	return []middleware.Definition{
		{
			Name:  "base.authority",
			Order: authorityOrder,
			Handler: security.NewMiddleware(security.MiddlewareOptions{
				Manager:           deps.AuthManager,
				Sessions:          deps.SessionStore,
				IgnoreRouteKeys:   ignoreRouteKeys,
				ProtectedPrefixes: append([]string{}, authorityPrefixes...),
				SSO:               options.SSO,
			}),
		},
		{
			Name:    "base.permission",
			Order:   permissionOrder,
			Handler: controller.NewPermissionMiddleware(permissionService, permissions),
		},
	}, nil
}

// LogDefinition 构建操作日志全局中间件。
func LogDefinition(config baseModule.Config, logRuntime *baseEvent.Log) ([]middleware.Definition, error) {
	if !config.Middleware.Log.Enable {
		return nil, nil
	}
	if logRuntime == nil {
		return nil, gerror.New("操作日志提交端口不能为空")
	}
	return []middleware.Definition{{
		Name:    "base.log",
		Order:   logOrder,
		Handler: NewLog(logRuntime),
	}}, nil
}
