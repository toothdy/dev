package controller

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	dbtx "github.com/toothdy/cool-admin-go-next/cool-next/db/tx"
)

// HTTPRequest 是生成 Handler 使用的 GoFrame 空请求类型
type HTTPRequest struct{}

// HTTPRouteInvoke 是已绑定请求的自定义路由调用
type HTTPRouteInvoke func(context.Context) (any, error)

// HTTPDTOInvoke 是带 DTO 的自定义路由调用
type HTTPDTOInvoke[T any] func(context.Context, *T) (any, error)

// HTTPMiddleware 是生成路由可安装的业务中间件
type HTTPMiddleware interface {
	Handle(*ghttp.Request)
}

// DescriptorResolverFunc 将生成的静态 Descriptor 分支适配为 Resolver
type DescriptorResolverFunc func(any) (coreentity.Metadata, bool)

// Resolve 解析实体对应的静态 Descriptor
func (resolve DescriptorResolverFunc) Resolve(value any) (coreentity.Metadata, bool) {
	if resolve == nil {
		return nil, false
	}

	return resolve(value)
}

// HandleAdd 绑定新增输入并执行生成期选定的 Service Adapter
func HandleAdd[E any, ID comparable](
	ctx context.Context,
	binder *Binder,
	dispatcher *crud.Dispatcher,
	resolver crud.DescriptorResolver,
	definition Definition,
	mode crud.ActionMode,
	descriptor coreentity.Descriptor[E, ID],
	invoke func(context.Context, service.AddInput[E]) (service.AddResult[ID], error),
) (any, error) {
	if err := requireHTTPCRUD(ctx, binder, dispatcher, resolver, invoke); err != nil {
		return nil, err
	}
	var (
		input  service.AddInput[E]
		result service.AddResult[ID]
	)
	err := HandleCRUD(
		ctx,
		definition,
		crud.ActionAdd,
		mode,
		dispatcher,
		func(scopeCtx context.Context) (*crud.QueryRequest, error) {
			request, requestErr := httpRequest(scopeCtx)
			if requestErr != nil {
				return nil, requestErr
			}
			input, requestErr = BindAdd[E, ID](binder, request, descriptor)

			return nil, requestErr
		},
		func(scopeCtx context.Context) error {
			values := input.Many()
			if !input.IsMany() {
				values = []*service.Mutable[E]{input.One()}
			}

			return ApplyInsertParam(scopeCtx, definition, values)
		},
		func(scopeCtx context.Context, request *crud.QueryRequest) (*crud.ActionPlan, error) {
			return CompilePlan(scopeCtx, resolver, definition, crud.ActionAdd, request)
		},
		func(scopeCtx context.Context) error {
			var invokeErr error
			result, invokeErr = invoke(scopeCtx, input)

			return invokeErr
		},
	)

	return result, err
}

// HandleDelete 绑定删除输入并执行生成期选定的 Service Adapter
func HandleDelete[E any, ID comparable](
	ctx context.Context,
	binder *Binder,
	dispatcher *crud.Dispatcher,
	resolver crud.DescriptorResolver,
	definition Definition,
	mode crud.ActionMode,
	descriptor coreentity.Descriptor[E, ID],
	invoke func(context.Context, service.DeleteInput[ID]) error,
) (any, error) {
	if err := requireHTTPCRUD(ctx, binder, dispatcher, resolver, invoke); err != nil {
		return nil, err
	}
	var input service.DeleteInput[ID]
	err := HandleCRUD(
		ctx,
		definition,
		crud.ActionDelete,
		mode,
		dispatcher,
		func(scopeCtx context.Context) (*crud.QueryRequest, error) {
			request, requestErr := httpRequest(scopeCtx)
			if requestErr != nil {
				return nil, requestErr
			}
			input, requestErr = BindDelete[E, ID](binder, request, descriptor)

			return nil, requestErr
		},
		nil,
		func(scopeCtx context.Context, request *crud.QueryRequest) (*crud.ActionPlan, error) {
			return CompilePlan(scopeCtx, resolver, definition, crud.ActionDelete, request)
		},
		func(scopeCtx context.Context) error { return invoke(scopeCtx, input) },
	)

	return nil, err
}

// HandleUpdate 绑定更新输入并执行生成期选定的 Service Adapter
func HandleUpdate[E any, ID comparable](
	ctx context.Context,
	binder *Binder,
	dispatcher *crud.Dispatcher,
	resolver crud.DescriptorResolver,
	definition Definition,
	mode crud.ActionMode,
	descriptor coreentity.Descriptor[E, ID],
	invoke func(context.Context, service.UpdateInput[E, ID]) error,
) (any, error) {
	if err := requireHTTPCRUD(ctx, binder, dispatcher, resolver, invoke); err != nil {
		return nil, err
	}
	var input service.UpdateInput[E, ID]
	err := HandleCRUD(
		ctx,
		definition,
		crud.ActionUpdate,
		mode,
		dispatcher,
		func(scopeCtx context.Context) (*crud.QueryRequest, error) {
			request, requestErr := httpRequest(scopeCtx)
			if requestErr != nil {
				return nil, requestErr
			}
			input, requestErr = BindUpdate[E, ID](binder, request, descriptor)

			return nil, requestErr
		},
		nil,
		func(scopeCtx context.Context, request *crud.QueryRequest) (*crud.ActionPlan, error) {
			return CompilePlan(scopeCtx, resolver, definition, crud.ActionUpdate, request)
		},
		func(scopeCtx context.Context) error { return invoke(scopeCtx, input) },
	)

	return nil, err
}

// HandleInfo 绑定详情主键并执行生成期选定的 Service Adapter
func HandleInfo[ID comparable](
	ctx context.Context,
	binder *Binder,
	dispatcher *crud.Dispatcher,
	resolver crud.DescriptorResolver,
	definition Definition,
	mode crud.ActionMode,
	invoke func(context.Context, ID) (any, error),
) (any, error) {
	if err := requireHTTPCRUD(ctx, binder, dispatcher, resolver, invoke); err != nil {
		return nil, err
	}
	var (
		input  ID
		result any
	)
	err := HandleCRUD(
		ctx,
		definition,
		crud.ActionInfo,
		mode,
		dispatcher,
		func(scopeCtx context.Context) (*crud.QueryRequest, error) {
			request, requestErr := httpRequest(scopeCtx)
			if requestErr != nil {
				return nil, requestErr
			}
			input, requestErr = BindInfo[ID](binder, request)

			return nil, requestErr
		},
		nil,
		func(scopeCtx context.Context, request *crud.QueryRequest) (*crud.ActionPlan, error) {
			return CompilePlan(scopeCtx, resolver, definition, crud.ActionInfo, request)
		},
		func(scopeCtx context.Context) error {
			var invokeErr error
			result, invokeErr = invoke(scopeCtx, input)

			return invokeErr
		},
	)

	return result, err
}

// HandleQuery 绑定 List 或 Page 输入并执行生成期选定的 Service Adapter
func HandleQuery(
	ctx context.Context,
	binder *Binder,
	dispatcher *crud.Dispatcher,
	resolver crud.DescriptorResolver,
	definition Definition,
	action crud.Action,
	mode crud.ActionMode,
	invoke func(context.Context, service.Query) (any, error),
) (any, error) {
	if action != crud.ActionList && action != crud.ActionPage {
		return nil, exception.Core("HTTP 查询动作必须是 list 或 page")
	}
	if err := requireHTTPCRUD(ctx, binder, dispatcher, resolver, invoke); err != nil {
		return nil, err
	}
	var (
		input  service.Query
		result any
	)
	err := HandleCRUD(
		ctx,
		definition,
		action,
		mode,
		dispatcher,
		func(scopeCtx context.Context) (*crud.QueryRequest, error) {
			request, requestErr := httpRequest(scopeCtx)
			if requestErr != nil {
				return nil, requestErr
			}
			input, requestErr = BindCRUDQuery(binder, request, action)
			if requestErr != nil {
				return nil, requestErr
			}

			return input.Request(), nil
		},
		nil,
		func(scopeCtx context.Context, request *crud.QueryRequest) (*crud.ActionPlan, error) {
			return CompilePlan(scopeCtx, resolver, definition, action, request)
		},
		func(scopeCtx context.Context) error {
			var invokeErr error
			result, invokeErr = invoke(scopeCtx, input)

			return invokeErr
		},
	)

	return result, err
}

// HandleCRUDDTO 绑定自定义查询 DTO 并执行对应 CRUD Handler
func HandleCRUDDTO[T any](
	ctx context.Context,
	binder *Binder,
	source BindSource,
	dispatcher *crud.Dispatcher,
	resolver crud.DescriptorResolver,
	definition Definition,
	action crud.Action,
	mode crud.ActionMode,
	invoke HTTPDTOInvoke[T],
) (any, error) {
	if err := requireHTTPCRUD(ctx, binder, dispatcher, resolver, invoke); err != nil {
		return nil, err
	}
	input := new(T)
	var result any
	err := HandleCRUD(
		ctx,
		definition,
		action,
		mode,
		dispatcher,
		func(scopeCtx context.Context) (*crud.QueryRequest, error) {
			request, requestErr := httpRequest(scopeCtx)
			if requestErr != nil {
				return nil, requestErr
			}

			return nil, binder.BindDTO(request, source, input)
		},
		nil,
		func(scopeCtx context.Context, request *crud.QueryRequest) (*crud.ActionPlan, error) {
			return CompilePlan(scopeCtx, resolver, definition, action, request)
		},
		func(scopeCtx context.Context) error {
			var invokeErr error
			result, invokeErr = invoke(scopeCtx, input)

			return invokeErr
		},
	)

	return result, err
}

// HandleDTO 绑定 DTO 并按声明的事务策略调用 Handler
func HandleDTO[T any](
	ctx context.Context,
	binder *Binder,
	source BindSource,
	runner dbtx.Runner,
	policy TransactionPolicy,
	invoke HTTPDTOInvoke[T],
) (any, error) {
	if binder == nil {
		return nil, exception.Core("HTTP Binder 未初始化")
	}
	if invoke == nil {
		return nil, exception.Core("HTTP DTO Handler 不能为空")
	}
	request, err := httpRequest(ctx)
	if err != nil {
		return nil, err
	}
	input := new(T)
	if err = binder.BindDTO(request, source, input); err != nil {
		return nil, err
	}

	return handleRouteValue(ctx, runner, policy, func(scopeCtx context.Context) (any, error) {
		return invoke(scopeCtx, input)
	})
}

// HandleNoDTO 按声明的事务策略调用无 DTO Handler
func HandleNoDTO(
	ctx context.Context,
	runner dbtx.Runner,
	policy TransactionPolicy,
	invoke HTTPRouteInvoke,
) (any, error) {
	if invoke == nil {
		return nil, exception.Core("HTTP Handler 不能为空")
	}
	if _, err := httpRequest(ctx); err != nil {
		return nil, err
	}

	return handleRouteValue(ctx, runner, policy, invoke)
}

func handleRouteValue(
	ctx context.Context,
	runner dbtx.Runner,
	policy TransactionPolicy,
	invoke HTTPRouteInvoke,
) (any, error) {
	var result any
	err := HandleRoute(ctx, runner, policy, func(scopeCtx context.Context) error {
		var invokeErr error
		result, invokeErr = invoke(scopeCtx)

		return invokeErr
	})

	return result, err
}

func requireHTTPCRUD(ctx context.Context, binder *Binder, dispatcher *crud.Dispatcher, resolver crud.DescriptorResolver, invoke any) error {
	if _, err := httpRequest(ctx); err != nil {
		return err
	}
	if binder == nil {
		return exception.Core("HTTP Binder 未初始化")
	}
	if dispatcher == nil {
		return exception.Core("CRUD Dispatcher 未初始化")
	}
	if isNilValue(resolver) {
		return exception.Core("Descriptor 解析器不能为空")
	}
	if isNilValue(invoke) {
		return exception.Core("HTTP CRUD Handler 不能为空")
	}

	return nil
}

func httpRequest(ctx context.Context) (*ghttp.Request, error) {
	if ctx == nil {
		return nil, exception.Core("HTTP 请求上下文无效")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request := ghttp.RequestFromCtx(ctx)
	if request == nil {
		return nil, exception.Core("HTTP 请求上下文无效")
	}

	return request, nil
}
