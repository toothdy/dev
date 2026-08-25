package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path"
	"strconv"

	"github.com/gogf/gf/v2/net/ghttp"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
)

// 裁剪时读取请求体的上限，超过则原样交回 Binder 按自身上限报错
const normalizeBodyLimit = 1 << 20

// 任务写入请求体裁剪
type BodyNormalizer struct {
	writable map[string]bool
}

// 任务请求体裁剪器
func NewBodyNormalizer() (*BodyNormalizer, error) {
	descriptor, err := coreentity.Compile[entity.Info, uint64](entity.InfoSchema())
	if err != nil {
		return nil, exception.WrapCore(err, "编译任务实体 Descriptor 失败")
	}
	writable := make(map[string]bool, len(descriptor.Fields()))
	for _, field := range descriptor.Fields() {
		writable[field.JSONName()] = !field.SystemMaintained()
	}

	return &BodyNormalizer{writable: writable}, nil
}

// 前端任务表单回传整条记录并附带 _every 这类视图字段，Node 侧由 TypeORM 静默忽略；
// Go Binder 拒绝未知字段与系统维护字段，因此在绑定前按实体可写字段裁剪写入请求体
func (normalizer *BodyNormalizer) Trim(ctx context.Context) error {
	if normalizer == nil {
		return exception.Core("任务请求体裁剪器未初始化")
	}
	request := ghttp.RequestFromCtx(ctx)
	if request == nil || request.Body == nil {
		return nil
	}
	switch path.Base(request.URL.Path) {
	case "add", "update":
	default:
		return nil
	}
	content, err := io.ReadAll(io.LimitReader(request.Body, normalizeBodyLimit+1))
	request.Body = io.NopCloser(bytes.NewReader(content))
	if err != nil || len(content) > normalizeBodyLimit {
		return nil
	}
	trimmed, changed := normalizer.trimBody(content)
	if !changed {
		return nil
	}
	request.Body = io.NopCloser(bytes.NewReader(trimmed))
	request.ContentLength = int64(len(trimmed))
	request.Header.Set("Content-Length", strconv.Itoa(len(trimmed)))

	return nil
}

func (normalizer *BodyNormalizer) trimBody(content []byte) ([]byte, bool) {
	switch trimmed := bytes.TrimSpace(content); {
	case len(trimmed) == 0:
		return nil, false
	case trimmed[0] == '[':
		var items []map[string]json.RawMessage
		if json.Unmarshal(trimmed, &items) != nil {
			return nil, false
		}
		changed := false
		for _, item := range items {
			changed = normalizer.trimObject(item) || changed
		}

		return marshalTrimmed(items, changed)
	default:
		var item map[string]json.RawMessage
		if json.Unmarshal(trimmed, &item) != nil {
			return nil, false
		}
		changed := normalizer.trimObject(item)

		return marshalTrimmed(item, changed)
	}
}

func (normalizer *BodyNormalizer) trimObject(item map[string]json.RawMessage) bool {
	changed := false
	for name := range item {
		if !normalizer.writable[name] {
			delete(item, name)
			changed = true
		}
	}

	return changed
}

func marshalTrimmed(value any, changed bool) ([]byte, bool) {
	if !changed {
		return nil, false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}

	return encoded, true
}
