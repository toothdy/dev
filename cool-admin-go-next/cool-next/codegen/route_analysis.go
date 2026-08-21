package codegen

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"net/http"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	coreroute "github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

const contextPackagePath = "context"

func controllerParameterTypes(signature *types.Signature) []types.Type {
	result := make([]types.Type, signature.Params().Len())
	for index := range signature.Params().Len() {
		result[index] = signature.Params().At(index).Type()
	}

	return result
}

func (a *analysis) analyzeControllerOptions(
	root string,
	pkg *loadedPackage,
	function *ast.FuncDecl,
	call *ast.CallExpr,
	declaration *ControllerDeclaration,
) bool {
	literal := localControllerLiteral(pkg, function, call.Args[0])
	if literal == nil || !isNamedType(pkg.packageInfo.TypesInfo.TypeOf(literal), controllerPackagePath, "RouterOptions") || !hasNamedLiteralFields(literal) {
		a.add("CG100", "Options 必须使用当前工厂内的命名字段 RouterOptions 字面量", a.position(pkg, call.Args[0].Pos()))
		return false
	}
	if expression, exists := controllerLiteralField(literal, "Sensitive"); exists {
		value, valid := controllerBoolPointer(pkg, function, expression)
		if !valid {
			a.add("CG100", "RouterOptions Sensitive 必须使用 controller.Bool 常量", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.sensitive = value
	}
	if expression, exists := controllerLiteralField(literal, "Alias"); exists {
		values, valid := controllerStringSlice(pkg, function, expression)
		if !valid || !uniqueValid(values, token.IsIdentifier) {
			a.add("CG100", "RouterOptions Alias 必须是无重复标识符常量列表", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.aliases = values
	}
	if expression, exists := controllerLiteralField(literal, "Middleware"); exists {
		values, valid := a.controllerMiddleware(root, pkg, function, expression)
		if !valid {
			a.add("CG104", "RouterOptions Middleware 必须是无重复 module.Ref 列表", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.middleware = values
	}
	if expression, exists := controllerLiteralField(literal, "Description"); exists {
		value, valid := constantControllerString(pkg, expression)
		if !valid || !validStaticText(value) {
			a.add("CG100", "RouterOptions Description 必须是合法常量字符串", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.description = value
	}
	if expression, exists := controllerLiteralField(literal, "TagName"); exists {
		value, valid := constantControllerString(pkg, expression)
		if !valid || !validStaticText(value) {
			a.add("CG100", "RouterOptions TagName 必须是合法常量字符串", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.tagName = value
	}
	if expression, exists := controllerLiteralField(literal, "IgnoreGlobalPrefix"); exists {
		value, valid := constantControllerBool(pkg, expression)
		if !valid {
			a.add("CG100", "RouterOptions IgnoreGlobalPrefix 必须是布尔常量", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.ignoreGlobalPrefix = value
	}
	if expression, exists := controllerLiteralField(literal, "DevelopmentOnly"); exists {
		value, valid := constantControllerBool(pkg, expression)
		if !valid {
			a.add("CG100", "RouterOptions DevelopmentOnly 必须是布尔常量", a.position(pkg, expression.Pos()))
			return false
		}
		declaration.developmentOnly = value
	}

	return true
}

func (a *analysis) analyzeControllerRoutes(
	root string,
	pkg *loadedPackage,
	function *ast.FuncDecl,
	signature *types.Signature,
	chain controllerChain,
	declaration ControllerDeclaration,
	curd *ast.CompositeLit,
) ([]RouteDeclaration, bool) {
	var result []RouteDeclaration
	if curd != nil {
		defaults, valid := a.analyzeDefaultRoutes(pkg, function, declaration, curd)
		if !valid {
			return nil, false
		}
		result = append(result, defaults...)
	}
	for _, call := range chain.routes {
		for _, expression := range call.Args {
			current, valid := a.analyzeCustomRoute(root, pkg, function, signature, declaration, expression)
			if !valid {
				return nil, false
			}
			result = append(result, current)
		}
	}

	return result, true
}

func (a *analysis) analyzeDefaultRoutes(
	pkg *loadedPackage,
	function *ast.FuncDecl,
	declaration ControllerDeclaration,
	literal *ast.CompositeLit,
) ([]RouteDeclaration, bool) {
	expression, exists := controllerLiteralField(literal, "API")
	if !exists {
		return nil, true
	}
	apis, valid := controllerAPIs(pkg, function, expression)
	if !valid {
		a.add("CG101", "Curd API 必须使用 API、AllAPI 或常量列表", a.position(pkg, expression.Pos()))
		return nil, false
	}
	tagName, tagAPIs, valid := controllerCurdTag(pkg, function, literal)
	if !valid {
		a.add("CG101", "Curd URLTag 必须是静态 URLTag 字面量", a.position(pkg, literal.Pos()))
		return nil, false
	}
	selectedTags := make(map[string]bool, len(tagAPIs))
	for _, value := range tagAPIs {
		selectedTags[value] = true
	}
	basePath := declaration.prefix
	if declaration.ignoreGlobalPrefix {
		basePath = removeControllerGlobalPrefix(basePath, declaration.area)
	}
	permissionPrefix, valid := controllerLiteralOptionalString(pkg, literal, "PermissionPrefix")
	if !valid || !validPermission(permissionPrefix) {
		a.add("CG101", "Curd PermissionPrefix 必须是合法权限前缀常量", a.position(pkg, literal.Pos()))
		return nil, false
	}
	if permissionPrefix == "" {
		permissionPrefix = defaultCRUDPermissionPrefix(basePath)
	}
	if permissionPrefix == "" {
		a.add("CG101", "Curd 权限前缀无法从路径推导", a.position(pkg, literal.Pos()))
		return nil, false
	}
	result := make([]RouteDeclaration, 0, len(apis))
	for _, api := range apis {
		method, bind, summary, methodName := defaultRouteValues(api)
		if method == "" {
			a.add("CG101", "Curd API 无效: "+api, a.position(pkg, expression.Pos()))
			return nil, false
		}
		handler, valid := serviceHandlerReference(declaration.serviceType, methodName)
		if !valid {
			a.add("CG102", "默认 CRUD Handler 不存在: "+methodName, declaration.position)
			return nil, false
		}
		var tags []string
		if tagName != "" && (len(tagAPIs) == 0 || selectedTags[api]) {
			tags = []string{tagName}
		}
		permission := permissionPrefix + ":" + api
		if containsString(tags, "ignoreToken") {
			permission = ""
		}
		result = append(result, RouteDeclaration{
			bind:            bind,
			developmentOnly: declaration.developmentOnly,
			handler:         handler,
			kind:            coreroute.KindCRUD,
			method:          method,
			path:            path.Join(basePath, api),
			position:        a.position(pkg, expression.Pos()),
			permission:      permission,
			summary:         summary,
			tags:            tags,
		})
	}

	return result, true
}

func defaultCRUDPermissionPrefix(routePath string) string {
	segments := strings.Split(strings.Trim(routePath, "/"), "/")
	if len(segments) > 0 && (segments[0] == "admin" || segments[0] == "app") {
		segments = segments[1:]
	}
	if len(segments) == 0 {
		return ""
	}
	for _, segment := range segments {
		if !token.IsIdentifier(segment) {
			return ""
		}
	}
	return strings.Join(segments, ":")
}

func (a *analysis) analyzeCustomRoute(
	root string,
	pkg *loadedPackage,
	function *ast.FuncDecl,
	factorySignature *types.Signature,
	controller ControllerDeclaration,
	expression ast.Expr,
) (RouteDeclaration, bool) {
	literal := localControllerLiteral(pkg, function, expression)
	if literal == nil || !isNamedType(pkg.packageInfo.TypesInfo.TypeOf(literal), controllerPackagePath, "Route") || !hasNamedLiteralFields(literal) {
		a.add("CG101", "Route 必须使用当前工厂内的命名字段 Route 字面量", a.position(pkg, expression.Pos()))
		return RouteDeclaration{}, false
	}
	methodExpression, methodExists := controllerLiteralField(literal, "Method")
	method, methodValid := constantControllerString(pkg, methodExpression)
	method = strings.ToUpper(strings.TrimSpace(method))
	if !methodExists || !methodValid || !validHTTPMethod(method) {
		a.add("CG101", "Route Method 必须是合法 HTTP Method 常量", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	pathExpression, pathExists := controllerLiteralField(literal, "Path")
	routePath, pathValid := constantControllerString(pkg, pathExpression)
	if !pathExists || !pathValid || !validCustomRoutePath(routePath) {
		a.add("CG101", "Route Path 必须是规范化绝对路径常量", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	handlerExpression, handlerExists := controllerLiteralField(literal, "Handler")
	handler, dtoType, handlerValid := controllerHandler(pkg, function, factorySignature, handlerExpression, handlerExists)
	if !handlerValid {
		a.add("CG102", "Route Handler 必须使用 Handle 包装合法 Handler", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	bind := coreroute.BindAuto
	if value, exists := controllerLiteralField(literal, "Bind"); exists {
		resolved, valid := constantControllerString(pkg, value)
		bind = coreroute.BindSource(resolved)
		if !valid || !validBindSource(bind) {
			a.add("CG101", "Route Bind 必须是合法绑定来源常量", a.position(pkg, value.Pos()))
			return RouteDeclaration{}, false
		}
	}
	if bind == coreroute.BindAuto {
		resolved, valid := inferBindSource(method, dtoType)
		if !valid {
			a.add("CG101", "Route BindAuto 无法无歧义推导", a.position(pkg, handlerExpression.Pos()))
			return RouteDeclaration{}, false
		}
		bind = resolved
	}
	middleware, valid := a.controllerRouteMiddleware(root, pkg, function, literal)
	if !valid {
		a.add("CG104", "Route Middleware 必须是无重复 module.Ref 列表", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	tags, valid := controllerRouteTags(pkg, function, literal)
	if !valid {
		a.add("CG101", "Route Tags 必须是无重复静态 URLTag 列表", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	permission, valid := controllerLiteralOptionalString(pkg, literal, "Permission")
	if !valid || !validPermission(permission) || containsString(tags, "ignoreToken") && permission != "" {
		a.add("CG101", "Route Permission 无效或与 ignoreToken 冲突", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	summary, valid := controllerLiteralOptionalString(pkg, literal, "Summary")
	if !valid || !validStaticText(summary) {
		a.add("CG101", "Route Summary 必须是合法常量字符串", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	description, valid := controllerLiteralOptionalString(pkg, literal, "Description")
	if !valid || !validStaticText(description) {
		a.add("CG101", "Route Description 必须是合法常量字符串", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	transaction, valid := controllerTransaction(pkg, function, literal)
	if !valid {
		a.add("CG101", "Route Transaction 只能使用默认事务或 NonTransactional", a.position(pkg, literal.Pos()))
		return RouteDeclaration{}, false
	}
	ignoreGlobalPrefix := controller.ignoreGlobalPrefix
	if value, exists := controllerLiteralField(literal, "IgnoreGlobalPrefix"); exists {
		resolved, valid := constantControllerBool(pkg, value)
		if !valid {
			a.add("CG101", "Route IgnoreGlobalPrefix 必须是布尔常量", a.position(pkg, value.Pos()))
			return RouteDeclaration{}, false
		}
		ignoreGlobalPrefix = resolved
	}
	developmentOnly := controller.developmentOnly
	if value, exists := controllerLiteralField(literal, "DevelopmentOnly"); exists {
		resolved, valid := constantControllerBool(pkg, value)
		if !valid {
			a.add("CG101", "Route DevelopmentOnly 必须是布尔常量", a.position(pkg, value.Pos()))
			return RouteDeclaration{}, false
		}
		developmentOnly = developmentOnly || resolved
	}
	basePath := controller.path
	if ignoreGlobalPrefix {
		basePath = removeControllerGlobalPrefix(basePath, controller.area)
	}

	return RouteDeclaration{
		bind:            bind,
		description:     description,
		developmentOnly: developmentOnly,
		handler:         handler,
		kind:            coreroute.KindCustom,
		method:          method,
		middleware:      middleware,
		path:            path.Join(basePath, routePath),
		permission:      permission,
		position:        a.position(pkg, expression.Pos()),
		summary:         summary,
		tags:            tags,
		transaction:     transaction,
	}, true
}

func controllerAPIs(pkg *loadedPackage, function *ast.FuncDecl, expression ast.Expr) ([]string, bool) {
	expression = localControllerValue(pkg, function, expression)
	if call, matches := unparenControllerExpr(expression).(*ast.CallExpr); matches {
		called := queryCalledFunction(pkg.packageInfo.TypesInfo, call.Fun)
		if isPackageFunction(called, controllerPackagePath, "AllAPI") && len(call.Args) == 0 {
			return []string{"add", "delete", "update", "info", "list", "page"}, true
		}
		if isPackageFunction(called, controllerPackagePath, "API") {
			result := make([]string, len(call.Args))
			for index, argument := range call.Args {
				value, valid := constantControllerString(pkg, argument)
				if !valid {
					return nil, false
				}
				result[index] = value
			}
			return result, uniqueValid(result, validAPI)
		}
	}
	values, valid := controllerStringSlice(pkg, function, expression)

	return values, valid && uniqueValid(values, validAPI)
}

func controllerCurdTag(pkg *loadedPackage, function *ast.FuncDecl, literal *ast.CompositeLit) (string, []string, bool) {
	expression, exists := controllerLiteralField(literal, "URLTag")
	if !exists || isNilIdentifier(expression) {
		return "", nil, true
	}
	resolved := localControllerValue(pkg, function, expression)
	if address, matches := unparenControllerExpr(resolved).(*ast.UnaryExpr); matches && address.Op == token.AND {
		resolved = unparenControllerExpr(address.X)
	}
	tag, matches := resolved.(*ast.CompositeLit)
	if !matches || !isNamedType(pkg.packageInfo.TypesInfo.TypeOf(tag), controllerPackagePath, "URLTag") || !hasNamedLiteralFields(tag) {
		return "", nil, false
	}
	nameExpression, exists := controllerLiteralField(tag, "Name")
	name, valid := constantControllerString(pkg, nameExpression)
	if !exists || !valid || !token.IsIdentifier(name) {
		return "", nil, false
	}
	urlExpression, exists := controllerLiteralField(tag, "URL")
	if !exists {
		return name, nil, true
	}
	values, valid := controllerAPIs(pkg, function, urlExpression)

	return name, values, valid
}

func defaultRouteValues(api string) (string, coreroute.BindSource, string, string) {
	switch api {
	case "add":
		return http.MethodPost, coreroute.BindJSON, "新增", "Add"
	case "delete":
		return http.MethodPost, coreroute.BindJSON, "删除", "Delete"
	case "update":
		return http.MethodPost, coreroute.BindJSON, "修改", "Update"
	case "info":
		return http.MethodGet, coreroute.BindQuery, "单个信息", "Info"
	case "list":
		return http.MethodPost, coreroute.BindJSON, "列表查询", "List"
	case "page":
		return http.MethodPost, coreroute.BindJSON, "分页查询", "Page"
	default:
		return "", "", "", ""
	}
}

func serviceHandlerReference(value types.Type, method string) (coreroute.CallableRef, bool) {
	if value == nil {
		return coreroute.CallableRef{}, false
	}
	selection := types.NewMethodSet(value).Lookup(nil, method)
	if selection == nil {
		return coreroute.CallableRef{}, false
	}
	signature, matches := selection.Obj().Type().(*types.Signature)
	if !matches || signature.Params().Len() != 2 || !isNamedType(signature.Params().At(0).Type(), contextPackagePath, "Context") ||
		(signature.Results().Len() != 1 && signature.Results().Len() != 2) {
		return coreroute.CallableRef{}, false
	}
	errorType := types.Universe.Lookup("error").Type()
	if !types.Identical(signature.Results().At(signature.Results().Len()-1).Type(), errorType) {
		return coreroute.CallableRef{}, false
	}
	pointer, matches := types.Unalias(value).(*types.Pointer)
	if !matches {
		return coreroute.CallableRef{}, false
	}
	named, matches := types.Unalias(pointer.Elem()).(*types.Named)
	if !matches || named.Obj() == nil || named.Obj().Pkg() == nil {
		return coreroute.CallableRef{}, false
	}

	reference := coreroute.CallableRef{
		Method:       method,
		PackagePath:  named.Obj().Pkg().Path(),
		ReturnsValue: signature.Results().Len() == 2,
		Symbol:       named.Obj().Name(),
		Type:         types.TypeString(value, qualifier),
	}
	request := signature.Params().At(1).Type()
	requestPointer, hasRequest := types.Unalias(request).(*types.Pointer)
	if hasRequest {
		requestNamed, namedRequest := types.Unalias(requestPointer.Elem()).(*types.Named)
		if namedRequest && requestNamed.Obj() != nil && requestNamed.Obj().Pkg() != nil {
			if _, isStructure := requestNamed.Underlying().(*types.Struct); isStructure {
				reference.HasRequest = true
				reference.RequestPackagePath = requestNamed.Obj().Pkg().Path()
				reference.RequestType = requestNamed.Obj().Name()
			}
		}
	}

	return reference, true
}

func controllerHandler(
	pkg *loadedPackage,
	function *ast.FuncDecl,
	factorySignature *types.Signature,
	expression ast.Expr,
	exists bool,
) (coreroute.CallableRef, types.Type, bool) {
	if !exists {
		return coreroute.CallableRef{}, nil, false
	}
	call, matches := unparenControllerExpr(localControllerValue(pkg, function, expression)).(*ast.CallExpr)
	if !matches || len(call.Args) != 1 || !isPackageFunction(queryCalledFunction(pkg.packageInfo.TypesInfo, call.Fun), controllerPackagePath, "Handle") {
		return coreroute.CallableRef{}, nil, false
	}
	target := unparenControllerExpr(call.Args[0])
	signature, matches := types.Unalias(pkg.packageInfo.TypesInfo.TypeOf(target)).(*types.Signature)
	if !matches {
		return coreroute.CallableRef{}, nil, false
	}
	dtoType, returnsValue, valid := validRouteHandlerSignature(signature)
	if !valid {
		return coreroute.CallableRef{}, nil, false
	}
	reference, valid := handlerCallableReference(pkg, factorySignature, target, signature)
	if !valid {
		return coreroute.CallableRef{}, nil, false
	}
	reference.HasRequest = dtoType != nil
	reference.ReturnsValue = returnsValue
	if dtoType != nil {
		pointer := types.Unalias(dtoType).(*types.Pointer)
		named := types.Unalias(pointer.Elem()).(*types.Named)
		reference.RequestPackagePath = named.Obj().Pkg().Path()
		reference.RequestType = named.Obj().Name()
	}

	return reference, dtoType, true
}

func handlerCallableReference(
	pkg *loadedPackage,
	factorySignature *types.Signature,
	expression ast.Expr,
	signature *types.Signature,
) (coreroute.CallableRef, bool) {
	switch current := expression.(type) {
	case *ast.Ident:
		object, _ := pkg.packageInfo.TypesInfo.Uses[current].(*types.Func)
		if object == nil || object.Pkg() == nil {
			return coreroute.CallableRef{}, false
		}
		return coreroute.CallableRef{
			Method:      object.Name(),
			PackagePath: object.Pkg().Path(),
			Symbol:      object.Name(),
			Type:        types.TypeString(signature, qualifier),
		}, true
	case *ast.SelectorExpr:
		selection := pkg.packageInfo.TypesInfo.Selections[current]
		if selection == nil {
			object, _ := pkg.packageInfo.TypesInfo.Uses[current.Sel].(*types.Func)
			if object == nil || object.Pkg() == nil {
				return coreroute.CallableRef{}, false
			}
			return coreroute.CallableRef{
				Method:      object.Name(),
				PackagePath: object.Pkg().Path(),
				Symbol:      object.Name(),
				Type:        types.TypeString(signature, qualifier),
			}, true
		}
		receiverName, direct := unparenControllerExpr(current.X).(*ast.Ident)
		receiver, _ := pkg.packageInfo.TypesInfo.Uses[receiverName].(*types.Var)
		if !direct || receiver == nil || !isControllerParameter(factorySignature, receiver) {
			return coreroute.CallableRef{}, false
		}
		pointer, matches := types.Unalias(receiver.Type()).(*types.Pointer)
		if !matches {
			return coreroute.CallableRef{}, false
		}
		named, matches := types.Unalias(pointer.Elem()).(*types.Named)
		if !matches || named.Obj() == nil || named.Obj().Pkg() == nil {
			return coreroute.CallableRef{}, false
		}
		return coreroute.CallableRef{
			Method:      current.Sel.Name,
			PackagePath: named.Obj().Pkg().Path(),
			Symbol:      named.Obj().Name(),
			Type:        types.TypeString(receiver.Type(), qualifier),
		}, true
	default:
		return coreroute.CallableRef{}, false
	}
}

func validRouteHandlerSignature(signature *types.Signature) (types.Type, bool, bool) {
	if signature == nil || signature.TypeParams().Len() != 0 || signature.Variadic() ||
		(signature.Params().Len() != 1 && signature.Params().Len() != 2) {
		return nil, false, false
	}
	if !isNamedType(signature.Params().At(0).Type(), contextPackagePath, "Context") {
		return nil, false, false
	}
	var dto types.Type
	if signature.Params().Len() == 2 {
		dto = signature.Params().At(1).Type()
		pointer, matches := types.Unalias(dto).(*types.Pointer)
		if !matches {
			return nil, false, false
		}
		named, matches := types.Unalias(pointer.Elem()).(*types.Named)
		if !matches || named.Obj() == nil || named.Obj().Pkg() == nil {
			return nil, false, false
		}
		if _, matches = named.Underlying().(*types.Struct); !matches {
			return nil, false, false
		}
	}
	errorType := types.Universe.Lookup("error").Type()
	if signature.Results().Len() == 1 {
		return dto, false, types.Identical(signature.Results().At(0).Type(), errorType)
	}
	if signature.Results().Len() != 2 || !types.Identical(signature.Results().At(1).Type(), errorType) {
		return nil, false, false
	}

	return dto, true, true
}

func inferBindSource(method string, dto types.Type) (coreroute.BindSource, bool) {
	if dto == nil {
		switch method {
		case http.MethodGet, http.MethodDelete, http.MethodHead:
			return coreroute.BindQuery, true
		default:
			return coreroute.BindJSON, true
		}
	}
	pointer, matches := types.Unalias(dto).(*types.Pointer)
	if !matches {
		return "", false
	}
	named, matches := types.Unalias(pointer.Elem()).(*types.Named)
	if !matches {
		return "", false
	}
	structure, matches := named.Underlying().(*types.Struct)
	if !matches {
		return "", false
	}
	sources := make(map[coreroute.BindSource]bool)
	for index := range structure.NumFields() {
		value := reflect.StructTag(structure.Tag(index)).Get("in")
		if value == "" {
			continue
		}
		source := coreroute.BindSource(value)
		if source != coreroute.BindQuery && source != coreroute.BindPath {
			return "", false
		}
		sources[source] = true
	}
	if len(sources) > 1 {
		return "", false
	}
	for source := range sources {
		return source, true
	}
	switch method {
	case http.MethodGet, http.MethodDelete, http.MethodHead:
		return coreroute.BindQuery, true
	default:
		return coreroute.BindJSON, true
	}
}

func (a *analysis) controllerRouteMiddleware(
	root string,
	pkg *loadedPackage,
	function *ast.FuncDecl,
	literal *ast.CompositeLit,
) ([]string, bool) {
	expression, exists := controllerLiteralField(literal, "Middleware")
	if !exists {
		return nil, true
	}

	return a.controllerMiddleware(root, pkg, function, expression)
}

func (a *analysis) controllerMiddleware(
	root string,
	pkg *loadedPackage,
	function *ast.FuncDecl,
	expression ast.Expr,
) ([]string, bool) {
	resolved := localControllerValue(pkg, function, expression)
	values, matches := unparenControllerExpr(resolved).(*ast.CompositeLit)
	if !matches {
		return nil, false
	}
	result := make([]string, 0, len(values.Elts))
	seen := make(map[string]bool, len(values.Elts))
	for _, element := range values.Elts {
		value, valid := a.resolveControllerMiddleware(root, pkg, unparenControllerExpr(element))
		if !valid || seen[value] {
			return nil, false
		}
		seen[value] = true
		result = append(result, value)
	}

	return result, true
}

func (a *analysis) resolveControllerMiddleware(root string, pkg *loadedPackage, expression ast.Expr) (string, bool) {
	call, matches := expression.(*ast.CallExpr)
	if !matches || len(call.Args) != 1 || !isPackageFunction(queryCalledFunction(pkg.packageInfo.TypesInfo, call.Fun), modulePackagePath, "Ref") {
		return "", false
	}
	symbol, valid := constantControllerString(pkg, call.Args[0])
	if !valid {
		return "", false
	}
	parts := strings.Split(symbol, ".")
	if len(parts) < 2 {
		return "", false
	}
	rootPackage := a.packages.byFile[filepath.Join(root, "config.go")]
	if rootPackage == nil {
		return "", false
	}
	targetPath := rootPackage.packageInfo.PkgPath + "/" + strings.Join(parts[:len(parts)-1], "/")
	target := a.packages.byPath[targetPath]
	if target == nil {
		return "", false
	}
	object, _ := target.packageInfo.Types.Scope().Lookup(parts[len(parts)-1]).(*types.Func)
	if object == nil || !validMiddlewareConstructor(object.Type()) {
		return "", false
	}
	position := a.position(target, object.Pos())
	if !a.eligible[filepath.Join(a.dir, filepath.FromSlash(position.File))] {
		return "", false
	}

	return symbol, true
}

func validMiddlewareConstructor(value types.Type) bool {
	signature, matches := types.Unalias(value).(*types.Signature)
	if !matches || signature.TypeParams().Len() != 0 || signature.Variadic() {
		return false
	}
	_, valid := constructorResult(signature)

	return valid
}

func controllerRouteTags(pkg *loadedPackage, function *ast.FuncDecl, literal *ast.CompositeLit) ([]string, bool) {
	expression, exists := controllerLiteralField(literal, "Tags")
	if !exists {
		return nil, true
	}
	resolved := localControllerValue(pkg, function, expression)
	values, matches := unparenControllerExpr(resolved).(*ast.CompositeLit)
	if !matches {
		return nil, false
	}
	result := make([]string, 0, len(values.Elts))
	seen := make(map[string]bool, len(values.Elts))
	for _, element := range values.Elts {
		tag := localControllerLiteral(pkg, function, element)
		if tag == nil || !isNamedType(pkg.packageInfo.TypesInfo.TypeOf(tag), controllerPackagePath, "URLTag") || !hasNamedLiteralFields(tag) {
			return nil, false
		}
		if url, exists := controllerLiteralField(tag, "URL"); exists {
			values, valid := controllerAPIs(pkg, function, url)
			if !valid || len(values) > 0 {
				return nil, false
			}
		}
		nameExpression, exists := controllerLiteralField(tag, "Name")
		name, valid := constantControllerString(pkg, nameExpression)
		if !exists || !valid || !token.IsIdentifier(name) || seen[name] {
			return nil, false
		}
		seen[name] = true
		result = append(result, name)
	}

	return result, true
}

func controllerTransaction(pkg *loadedPackage, function *ast.FuncDecl, literal *ast.CompositeLit) (coreroute.TransactionPolicy, bool) {
	expression, exists := controllerLiteralField(literal, "Transaction")
	if !exists {
		return coreroute.TransactionPolicy{}, true
	}
	resolved := unparenControllerExpr(localControllerValue(pkg, function, expression))
	call, matches := resolved.(*ast.CallExpr)
	if !matches || len(call.Args) != 0 || !isPackageFunction(queryCalledFunction(pkg.packageInfo.TypesInfo, call.Fun), controllerPackagePath, "NonTransactional") {
		return coreroute.TransactionPolicy{}, false
	}

	return coreroute.NonTransactional(), true
}

func controllerBoolPointer(pkg *loadedPackage, function *ast.FuncDecl, expression ast.Expr) (bool, bool) {
	call, matches := unparenControllerExpr(localControllerValue(pkg, function, expression)).(*ast.CallExpr)
	if !matches || len(call.Args) != 1 || !isPackageFunction(queryCalledFunction(pkg.packageInfo.TypesInfo, call.Fun), controllerPackagePath, "Bool") {
		return false, false
	}

	return constantControllerBool(pkg, call.Args[0])
}

func constantControllerBool(pkg *loadedPackage, expression ast.Expr) (bool, bool) {
	if expression == nil {
		return false, false
	}
	value := pkg.packageInfo.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}

	return constant.BoolVal(value), true
}

func controllerStringSlice(pkg *loadedPackage, function *ast.FuncDecl, expression ast.Expr) ([]string, bool) {
	resolved := unparenControllerExpr(localControllerValue(pkg, function, expression))
	if isNilIdentifier(resolved) {
		return nil, true
	}
	literal, matches := resolved.(*ast.CompositeLit)
	if !matches {
		return nil, false
	}
	result := make([]string, len(literal.Elts))
	for index, element := range literal.Elts {
		value, valid := constantControllerString(pkg, element)
		if !valid {
			return nil, false
		}
		result[index] = value
	}

	return result, true
}

func controllerLiteralOptionalString(pkg *loadedPackage, literal *ast.CompositeLit, field string) (string, bool) {
	expression, exists := controllerLiteralField(literal, field)
	if !exists {
		return "", true
	}

	return constantControllerString(pkg, expression)
}

func removeControllerGlobalPrefix(value string, area ControllerArea) string {
	prefix := "/" + string(area)
	if value == prefix {
		return "/"
	}

	return strings.TrimPrefix(value, prefix)
}

func validCustomRoutePath(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.ContainsAny(value, "?#") || path.Clean(value) != value {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character == 0x7f {
			return false
		}
	}

	return true
}

func validHTTPMethod(value string) bool {
	switch value {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
		return true
	default:
		return false
	}
}

func validBindSource(value coreroute.BindSource) bool {
	switch value {
	case coreroute.BindAuto, coreroute.BindJSON, coreroute.BindQuery, coreroute.BindForm, coreroute.BindPath, coreroute.BindFile:
		return true
	default:
		return false
	}
}

func validAPI(value string) bool {
	switch value {
	case "add", "delete", "update", "info", "list", "page":
		return true
	default:
		return false
	}
}

func validStaticText(value string) bool {
	if value == "" {
		return true
	}
	return strings.TrimSpace(value) != ""
}

func validPermission(value string) bool {
	if value == "" {
		return true
	}
	if strings.TrimSpace(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, ":") {
		if !token.IsIdentifier(segment) {
			return false
		}
	}

	return true
}

func uniqueValid(values []string, validate func(string) bool) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !validate(value) || seen[value] {
			return false
		}
		seen[value] = true
	}

	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func isNilIdentifier(expression ast.Expr) bool {
	identifier, matches := unparenControllerExpr(expression).(*ast.Ident)

	return matches && identifier.Name == "nil"
}

func (a *analysis) validateRouteConflicts(model *Model) {
	aliases := make(map[string]Position)
	routes := make(map[string]Position)
	for _, currentModule := range model.modules {
		moduleMiddleware := make(map[string]bool)
		for _, reference := range currentModule.references {
			if moduleMiddleware[reference.symbol] {
				a.add("CG104", "模块中间件重复注册: "+reference.symbol, reference.position)
			}
			moduleMiddleware[reference.symbol] = true
		}
		for _, controller := range currentModule.controllers {
			for _, alias := range controller.aliases {
				if _, exists := aliases[alias]; exists {
					a.add("CG103", "Controller Alias 冲突: "+alias, controller.position)
				} else {
					aliases[alias] = controller.position
				}
			}
			controllerMiddleware := make(map[string]bool, len(moduleMiddleware)+len(controller.middleware))
			for value := range moduleMiddleware {
				controllerMiddleware[value] = true
			}
			for _, value := range controller.middleware {
				if controllerMiddleware[value] {
					a.add("CG104", "Controller 中间件重复注册: "+value, controller.position)
				}
				controllerMiddleware[value] = true
			}
			for _, route := range controller.routes {
				key := route.method + " " + route.path
				if _, exists := routes[key]; exists {
					a.add("CG103", "路由冲突: "+key, route.position)
				} else {
					routes[key] = route.position
				}
				seen := make(map[string]bool, len(controllerMiddleware)+len(route.middleware))
				for value := range controllerMiddleware {
					seen[value] = true
				}
				for _, value := range route.middleware {
					if seen[value] {
						a.add("CG104", "Route 中间件重复注册: "+value, route.position)
					}
					seen[value] = true
				}
			}
		}
	}
}
