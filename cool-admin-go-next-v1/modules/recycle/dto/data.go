package dto

// RestoreRequest 表示批量恢复请求。
type RestoreRequest struct {
	IDs []int64 `json:"ids" v:"required|length:1,500"`
}
