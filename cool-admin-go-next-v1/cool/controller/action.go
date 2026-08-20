package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/response"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
	resultType  = reflect.TypeOf((*Result)(nil)).Elem()
	bodyType    = reflect.TypeOf(response.Body{})
)

type compiledAction struct {
	function    reflect.Value
	requestType reflect.Type
	withContext bool
	result      bool
	errorOnly   bool
}

func compileAction(action interface{}) (*compiledAction, error) {
	if action == nil {
		return nil, exception.Core("action 不能为空")
	}
	value := reflect.ValueOf(action)
	if value.Kind() != reflect.Func || value.IsNil() {
		return nil, exception.Core("action 必须是非空函数")
	}

	typeOf := value.Type()
	compiled := &compiledAction{function: value}
	switch typeOf.NumIn() {
	case 0:
	case 1:
		if typeOf.In(0) == contextType {
			compiled.withContext = true
		} else if err := compiled.setRequestType(typeOf.In(0)); err != nil {
			return nil, err
		}
	case 2:
		if typeOf.In(0) != contextType {
			return nil, exception.Core("context.Context 必须位于请求 DTO 之前")
		}
		compiled.withContext = true
		if err := compiled.setRequestType(typeOf.In(1)); err != nil {
			return nil, err
		}
	default:
		return nil, exception.Core("action 最多接收 context.Context 和一个请求 DTO")
	}

	switch typeOf.NumOut() {
	case 1:
		output := typeOf.Out(0)
		if output == errorType {
			compiled.errorOnly = true
			return compiled, nil
		}
		if output.Implements(errorType) {
			return nil, exception.Core("action 数据返回值不能实现 error")
		}
		if output == bodyType || output == reflect.PointerTo(bodyType) {
			return nil, exception.Core("action 不能返回 response.Body")
		}
		compiled.result = output.Implements(resultType)
	case 2:
		if typeOf.Out(1) != errorType {
			return nil, exception.Core("action 的第二个返回值必须是 error")
		}
		output := typeOf.Out(0)
		if output.Implements(errorType) {
			return nil, exception.Core("action 数据返回值不能实现 error")
		}
		if output == bodyType || output == reflect.PointerTo(bodyType) {
			return nil, exception.Core("action 不能返回 response.Body")
		}
		compiled.result = output.Implements(resultType)
	default:
		return nil, exception.Core("action 必须返回数据、error 或 (数据, error)")
	}
	return compiled, nil
}

func (a *compiledAction) setRequestType(requestType reflect.Type) error {
	if requestType.Kind() != reflect.Pointer || requestType.Elem().Kind() != reflect.Struct {
		return exception.Core("action 请求 DTO 必须是 struct 指针")
	}
	a.requestType = requestType
	return nil
}

func (a *compiledAction) handler(options bindOptions) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		args := make([]reflect.Value, 0, 2)
		if a.withContext {
			args = append(args, reflect.ValueOf(r.Context()))
		}
		if a.requestType != nil {
			request := reflect.New(a.requestType.Elem())
			if err := bindRequest(r, request.Interface(), options); err != nil {
				r.SetError(err)
				return
			}
			args = append(args, request)
		}

		outputs := a.function.Call(args)
		if a.errorOnly {
			if err := reflectedError(outputs[0]); err != nil {
				r.SetError(err)
				return
			}
			writeJSONSuccess(r, nil)
			return
		}
		if len(outputs) == 2 {
			if err := reflectedError(outputs[1]); err != nil {
				r.SetError(err)
				return
			}
		}
		if a.result {
			if isNilReflectValue(outputs[0]) {
				r.SetError(exception.Internal(nil, "action 返回了空 Result"))
				return
			}
			result, ok := outputs[0].Interface().(Result)
			if !ok || isNilInterface(result) {
				r.SetError(exception.Internal(nil, "action 返回了无效 Result"))
				return
			}
			if err := result.Write(r); err != nil {
				r.SetError(err)
			}
			return
		}
		writeJSONSuccess(r, outputs[0].Interface())
	}
}

func reflectedError(value reflect.Value) error {
	if isNilReflectValue(value) {
		return nil
	}
	err, ok := value.Interface().(error)
	if !ok {
		return fmt.Errorf("invalid reflected error value: %s", value.Type())
	}
	return err
}

func isNilReflectValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func isNilInterface(value interface{}) bool {
	if value == nil {
		return true
	}
	return isNilReflectValue(reflect.ValueOf(value))
}
