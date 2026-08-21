package eps

import (
	"sync/atomic"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

var publishedViews atomic.Pointer[Views]

// 由当前静态模块图编译的 EPS 视图
func PublishViews(views *Views) error {
	if views == nil {
		return exception.Core("EPS 视图不能为空")
	}
	value := *views
	publishedViews.Store(&value)

	return nil
}

// 已发布的后台 EPS 视图
func AdminView() (Document, error) {
	views := publishedViews.Load()
	if views == nil {
		return Document{}, exception.Core("EPS 视图尚未发布")
	}

	return views.Admin, nil
}

// 已发布的 App EPS 视图
func AppView() (Document, error) {
	views := publishedViews.Load()
	if views == nil {
		return Document{}, exception.Core("EPS 视图尚未发布")
	}

	return views.App, nil
}
