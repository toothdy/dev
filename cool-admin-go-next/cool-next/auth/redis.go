package auth

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gredis"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

const rotateRefreshScript = `
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
if raw ~= ARGV[1] then return 2 end
redis.call('SET', KEYS[1], ARGV[2], 'PXAT', ARGV[3])
return 1
`

const revokeUsersScript = `
local targets = {}
for index = 2, #ARGV do targets[ARGV[index]] = true end
local matches = {}
for _, key in ipairs(KEYS) do
    local raw = redis.call('GET', key)
    if raw then
        local valid, current = pcall(cjson.decode, raw)
        if not valid then return -1 end
        if current.schemaVersion ~= 1 or type(current.subject) ~= 'string' or type(current.userId) ~= 'string' then return -1 end
        if current.subject == ARGV[1] and targets[current.userId] then
            table.insert(matches, key)
        end
    end
end
for _, key in ipairs(matches) do redis.call('DEL', key) end
return #matches
`

const deleteIfUnchangedScript = `
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
if raw ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1])
return 1
`

type redisBackend interface {
	Do(context.Context, string, ...any) (*gvar.Var, error)
	Get(context.Context, string) (*gvar.Var, error)
	Set(context.Context, string, any, ...gredis.SetOption) (*gvar.Var, error)
	Eval(context.Context, string, int64, []string, []any) (*gvar.Var, error)
	Del(context.Context, ...string) (int64, error)
	Scan(context.Context, uint64, ...gredis.ScanOption) (uint64, []string, error)
}

// Redis Session Store
type RedisStore struct {
	client redisBackend
	prefix string
	now    func() time.Time
}

// 创建并探测指定连接组的 Redis Session Store
func NewRedis(ctx context.Context, group, prefix string) (*RedisStore, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, exception.Core("Redis Session Group 不能为空")
	}
	if _, exists := gredis.GetConfig(group); !exists {
		return nil, exception.Core("Redis Session Group 不存在: " + group)
	}
	client := gredis.Instance(group)
	if client == nil {
		return nil, exception.Core("创建 Redis Session 连接失败: " + group)
	}
	return newRedisStore(ctx, client, prefix, time.Now)
}

// 创建并探测 Redis Session Store
func newRedisStore(ctx context.Context, client redisBackend, prefix string, now func() time.Time) (*RedisStore, error) {
	if client == nil {
		return nil, exception.Core("Redis Session Client 不能为空")
	}
	if now == nil {
		return nil, exception.Core("Redis Session 时钟不能为空")
	}
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}
	result, err := client.Do(ctx, "PING")
	if err != nil {
		return nil, exception.WrapCore(err, "Redis Session PING 失败")
	}
	if result == nil || !strings.EqualFold(result.String(), "PONG") {
		return nil, exception.Core("Redis Session PING 响应无效")
	}

	return &RedisStore{client: client, prefix: prefix, now: now}, nil
}

// 读取有效 Session
func (store *RedisStore) Get(ctx context.Context, sessionID string) (Snapshot, bool, error) {
	if err := contextError(ctx); err != nil {
		return Snapshot{}, false, err
	}
	if !validIdentifier(sessionID) {
		return Snapshot{}, false, exception.Core("Session ID 无效")
	}

	key := store.prefix + sessionID
	result, err := store.client.Get(ctx, key)
	if err != nil {
		return Snapshot{}, false, exception.WrapCore(err, "读取 Redis Session 失败")
	}
	if result == nil || result.IsNil() {
		return Snapshot{}, false, nil
	}
	content := result.Bytes()
	value, err := decode(content, key, store.prefix, store.now())
	if errors.Is(err, errExpired) {
		if _, err = store.client.Eval(ctx, deleteIfUnchangedScript, 1, []string{key}, []any{content}); err != nil {
			return Snapshot{}, false, exception.WrapCore(err, "清理过期 Redis Session 失败")
		}
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}

	return value, true, nil
}

// 保存有效 Session
func (store *RedisStore) Save(ctx context.Context, value Snapshot) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	content, err := encode(value, store.now())
	if err != nil {
		return err
	}
	expiresAt := value.ExpiresAt.UnixMilli()
	_, err = store.client.Set(ctx, store.prefix+value.SessionID, content, gredis.SetOption{
		TTLOption: gredis.TTLOption{PXAT: &expiresAt},
	})
	if err != nil {
		return exception.WrapCore(err, "保存 Redis Session 失败")
	}

	return nil
}

// 原子轮换 Access 与 Refresh JTI
func (store *RedisStore) RotateRefresh(
	ctx context.Context,
	sessionID string,
	expectedRefreshJTI string,
	next Snapshot,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if !validIdentifier(sessionID) || !validIdentifier(expectedRefreshJTI) {
		return exception.Core("Session 轮换参数无效")
	}
	if next.SessionID != sessionID {
		return exception.Core("轮换后的 Session ID 不一致")
	}
	key := store.prefix + sessionID
	currentResult, err := store.client.Get(ctx, key)
	if err != nil {
		return exception.WrapCore(err, "读取 Redis Session 失败")
	}
	if currentResult == nil || currentResult.IsNil() {
		return ErrSessionNotFound
	}
	currentContent := currentResult.Bytes()
	current, err := decode(currentContent, key, store.prefix, store.now())
	if errors.Is(err, errExpired) {
		return ErrSessionNotFound
	}
	if err != nil {
		return err
	}
	if current.RefreshJTI != expectedRefreshJTI {
		return ErrRefreshReplay
	}
	if current.Subject != next.Subject || current.UserID != next.UserID {
		return exception.Core("轮换不能改变 Session 身份")
	}
	content, err := encode(next, store.now())
	if err != nil {
		return err
	}

	result, err := store.client.Eval(
		ctx,
		rotateRefreshScript,
		1,
		[]string{key},
		[]any{currentContent, content, next.ExpiresAt.UnixMilli()},
	)
	if err != nil {
		return exception.WrapCore(err, "轮换 Redis Session 失败")
	}
	switch result.Int() {
	case 0:
		return ErrSessionNotFound
	case 1:
		return nil
	case 2:
		return ErrRefreshReplay
	default:
		return exception.Core("Redis Session 轮换结果无效")
	}
}

// 撤销指定 Session
func (store *RedisStore) Revoke(ctx context.Context, sessionID string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if !validIdentifier(sessionID) {
		return exception.Core("Session ID 无效")
	}
	if _, err := store.client.Del(ctx, store.prefix+sessionID); err != nil {
		return exception.WrapCore(err, "撤销 Redis Session 失败")
	}

	return nil
}

// 按身份种类批量撤销用户 Session
func (store *RedisStore) RevokeUsers(ctx context.Context, subject Kind, userIDs []uint64) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	targets, err := revokeUserSet(subject, userIDs)
	if err != nil || len(targets) == 0 {
		return err
	}

	keys, err := store.scanKeys(ctx)
	if err != nil {
		return exception.WrapCore(err, "扫描 Redis Session 失败")
	}
	targetIDs := make([]string, 0, len(targets))
	for userID := range targets {
		targetIDs = append(targetIDs, strconv.FormatUint(userID, 10))
	}
	sort.Strings(targetIDs)
	for offset := 0; offset < len(keys); offset += 100 {
		end := min(offset+100, len(keys))
		if err := store.revokeBatch(ctx, keys[offset:end], subject, targetIDs); err != nil {
			return err
		}
	}

	return nil
}

// 扫描 Session Key
func (store *RedisStore) scanKeys(ctx context.Context) ([]string, error) {
	var (
		cursor uint64
		keys   []string
	)
	for {
		nextCursor, page, err := store.client.Scan(ctx, cursor, gredis.ScanOption{
			Match: store.prefix + "*",
			Count: 100,
			Type:  "string",
		})
		if err != nil {
			return nil, exception.WrapCore(err, "扫描 Redis Session 失败")
		}
		keys = append(keys, page...)
		cursor = nextCursor
		if cursor == 0 {
			return keys, nil
		}
	}
}

// 原子撤销一批匹配用户的 Redis Session
func (store *RedisStore) revokeBatch(
	ctx context.Context,
	keys []string,
	subject Kind,
	targetIDs []string,
) error {
	if len(keys) == 0 {
		return nil
	}
	args := make([]any, 1, len(targetIDs)+1)
	args[0] = string(subject)
	for _, targetID := range targetIDs {
		args = append(args, targetID)
	}
	result, err := store.client.Eval(ctx, revokeUsersScript, int64(len(keys)), keys, args)
	if err != nil {
		return exception.WrapCore(err, "撤销 Redis Session 失败")
	}
	if result.Int() < 0 {
		return exception.Core("Redis Session 撤销结果无效")
	}

	return nil
}

var _ redisBackend = (*gredis.Redis)(nil)

// 校验 Redis Key 前缀
func validatePrefix(prefix string) error {
	if strings.TrimSpace(prefix) == "" || strings.TrimSpace(prefix) != prefix {
		return exception.Core("Redis Session Prefix 无效")
	}
	if strings.ContainsAny(prefix, "*?[]\\") {
		return exception.Core("Redis Session Prefix 不能包含通配符")
	}

	return nil
}
