package recycle

import (
	"context"
	"encoding/json"
)

type contextKey uint8

const (
	metadataContextKey contextKey = iota
	bypassContextKey
)

// RequestMetadata 表示删除请求的服务端审计信息
type RequestMetadata struct {
	UserID   int64
	TenantID *int64
	URL      string
	Method   string
	Params   json.RawMessage
}

/**
 * 写入删除请求审计信息
 * @param ctx 上下文
 * @param metadata 审计信息
 * @returns context.Context
 */
func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	metadata.Params = append(json.RawMessage(nil), metadata.Params...)
	return context.WithValue(ctx, metadataContextKey, metadata)
}

/**
 * 读取删除请求审计信息
 * @param ctx 上下文
 * @returns RequestMetadata 和是否存在
 */
func RequestMetadataFromContext(ctx context.Context) (RequestMetadata, bool) {
	if ctx == nil {
		return RequestMetadata{}, false
	}
	metadata, ok := ctx.Value(metadataContextKey).(RequestMetadata)
	metadata.Params = append(json.RawMessage(nil), metadata.Params...)
	return metadata, ok
}

/**
 * 创建显式绕过回收站的派生上下文
 * @param ctx 父上下文
 * @returns context.Context
 */
func WithBypass(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, bypassContextKey, true)
}

/**
 * 判断当前操作是否显式绕过回收站
 * @param ctx 上下文
 * @returns bool
 */
func IsBypass(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	isBypass, _ := ctx.Value(bypassContextKey).(bool)
	return isBypass
}
