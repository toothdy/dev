package dto

// 任务 ID 请求(once / stop 共用)
type IDReq struct {
	ID int64 `json:"id"`
}

// 启动任务请求
type StartReq struct {
	ID   int64 `json:"id"`
	Type *int  `json:"type"`
}

// 任务日志分页请求
type InfoLogRequest struct {
	ID     int64 `json:"id"`
	Status *int  `json:"status"`
	Page   int   `json:"page"`
	Size   int   `json:"size"`
}
