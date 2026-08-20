package controller

import (
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

func crudRouteHandler(runtime *crud.Runtime, resource crud.Resource, api string) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		var (
			data interface{}
			err  error
		)
		switch api {
		case crud.Add:
			input, inputs, batch, bindErr := bindCRUDMutation(r)
			if bindErr != nil {
				r.SetError(bindErr)
				return
			}
			if batch {
				data, err = runtime.AddMany(r.Context(), resource, inputs)
			} else {
				data, err = runtime.Add(r.Context(), resource, input)
			}
		case crud.Delete:
			input, bindErr := bindCRUDMap(r)
			if bindErr != nil {
				r.SetError(bindErr)
				return
			}
			ids, idsErr := crud.RequestIDs(input)
			if idsErr != nil {
				r.SetError(exception.Validate(idsErr.Error()))
				return
			}
			data, err = runtime.DeleteWithData(r.Context(), resource, ids, input)
		case crud.Update:
			input, inputs, batch, bindErr := bindCRUDMutation(r)
			if bindErr != nil {
				r.SetError(bindErr)
				return
			}
			if batch {
				data, err = runtime.UpdateMany(r.Context(), resource, inputs)
			} else {
				data, err = runtime.Update(r.Context(), resource, input)
			}
		case crud.Info:
			id := r.Get(resource.PrimaryField.JSONName).Val()
			if id == nil || strings.TrimSpace(r.Get(resource.PrimaryField.JSONName).String()) == "" {
				r.SetError(exception.Validate("主键不能为空"))
				return
			}
			data, err = runtime.Info(r.Context(), resource, id)
		case crud.List:
			input, bindErr := bindCRUDMap(r)
			if bindErr != nil {
				r.SetError(bindErr)
				return
			}
			data, err = runtime.List(r.Context(), resource, crud.NewQueryRequest(resource, resource.ListQuery, input))
		case crud.Page:
			input, bindErr := bindCRUDMap(r)
			if bindErr != nil {
				r.SetError(bindErr)
				return
			}
			data, err = runtime.Page(r.Context(), resource, crud.NewQueryRequest(resource, resource.PageQuery, input))
		default:
			err = exception.Internal(nil, "CRUD API 未编译")
		}
		if err != nil {
			r.SetError(err)
			return
		}
		writeJSONSuccess(r, data)
	}
}

func bindCRUDMutation(r *ghttp.Request) (map[string]interface{}, []map[string]interface{}, bool, error) {
	var payload interface{}
	if err := bindJSON(r, &payload, true); err != nil {
		return nil, nil, false, exception.Validate(err.Error())
	}
	switch typed := payload.(type) {
	case map[string]interface{}:
		return typed, nil, false, nil
	case []interface{}:
		items := make([]map[string]interface{}, len(typed))
		for index, item := range typed {
			values, ok := item.(map[string]interface{})
			if !ok {
				return nil, nil, false, exception.Validate("批量请求项必须是对象")
			}
			items[index] = values
		}
		return nil, items, true, nil
	default:
		return nil, nil, false, exception.Validate("请求 JSON 必须是对象或对象数组")
	}
}

func bindCRUDMap(r *ghttp.Request) (map[string]interface{}, error) {
	if r.Method == http.MethodGet {
		return map[string]interface{}{}, nil
	}
	input := map[string]interface{}{}
	if err := bindJSON(r, &input, true); err != nil {
		return nil, exception.Validate(err.Error())
	}
	return input, nil
}
