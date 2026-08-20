package crud

import "context"

// 新增请求
type AddRequest struct {
	Data map[string]interface{}
}

// 删除请求
type DeleteRequest struct {
	IDs  []interface{}
	Data map[string]interface{}
}

// 更新请求
type UpdateRequest struct {
	Data map[string]interface{}
}

// 详情请求
type InfoRequest struct {
	ID interface{}
}

// 新增重写接口
type AddHandler interface {
	Add(ctx context.Context, request AddRequest) (interface{}, error)
}

// 删除重写接口
type DeleteHandler interface {
	Delete(ctx context.Context, request DeleteRequest) (interface{}, error)
}

// 更新重写接口
type UpdateHandler interface {
	Update(ctx context.Context, request UpdateRequest) (interface{}, error)
}

// 详情重写接口
type InfoHandler interface {
	Info(ctx context.Context, request InfoRequest) (interface{}, error)
}

// 列表重写接口
type ListHandler interface {
	List(ctx context.Context, request QueryRequest) (interface{}, error)
}

// 分页重写接口
type PageHandler interface {
	Page(ctx context.Context, request QueryRequest) (interface{}, error)
}
