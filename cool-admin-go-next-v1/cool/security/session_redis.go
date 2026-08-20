package security

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gerror"
)

const defaultRedisSessionPrefix = "cool-admin-go-next:auth:v1"

const ensureGenerationScript = `
local current = redis.call('GET', KEYS[1])
if current then
  return current
end
if redis.call('SET', KEYS[1], ARGV[1], 'NX') then
  return ARGV[1]
end
return redis.call('GET', KEYS[1])
`

const rotateSessionScript = `
local raw = redis.call('GET', KEYS[1])
if not raw then
  return 0
end
local current = cjson.decode(raw)
if current.refreshJtiHash ~= ARGV[1] then
  return 0
end
if tonumber(current.userId) ~= tonumber(ARGV[2]) then
  return 0
end
local generation = redis.call('GET', KEYS[2])
if not generation or current.generation ~= generation then
  return 0
end
if tonumber(current.expiresAt) <= tonumber(ARGV[3]) then
  redis.call('DEL', KEYS[1])
  return 0
end
local next = cjson.decode(ARGV[4])
next.generation = generation
redis.call('SETEX', KEYS[1], ARGV[5], cjson.encode(next))
return 1
`

const revokeUsersScript = `
for index, key in ipairs(KEYS) do
  redis.call('SET', key, ARGV[index])
end
return #KEYS
`

// RedisSessionStore 使用的最小 Redis 命令接口
type RedisCommander interface {
	Do(ctx context.Context, command string, args ...any) (*gvar.Var, error)
}

// 使用 Redis 共享会话，并用用户版本实现跨实例批量撤销
type RedisSessionStore struct {
	client RedisCommander
	prefix string
}

type storedRedisSession struct {
	UserID          int64  `json:"userId"`
	AccessJTIHash   string `json:"accessJtiHash"`
	RefreshJTIHash  string `json:"refreshJtiHash"`
	PasswordVersion int64  `json:"passwordVersion"`
	ExpiresAt       int64  `json:"expiresAt"`
	Generation      string `json:"generation"`
}

// 创建 Redis 会话存储
func NewRedisSessionStore(client RedisCommander, prefix string) (*RedisSessionStore, error) {
	if client == nil {
		return nil, gerror.New("Redis client 不能为空")
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), ":")
	if prefix == "" {
		prefix = defaultRedisSessionPrefix
	}
	return &RedisSessionStore{client: client, prefix: prefix}, nil
}

func (s *RedisSessionStore) Save(ctx context.Context, session Session) error {
	generation, err := s.ensureGeneration(ctx, session.UserID)
	if err != nil {
		return err
	}
	return s.save(ctx, session, generation)
}

func (s *RedisSessionStore) ReplaceUser(ctx context.Context, userID int64, session Session) error {
	if session.UserID != userID {
		return gerror.New("session user id 与替换用户不一致")
	}
	generation, err := randomGeneration()
	if err != nil {
		return err
	}
	if _, err = s.client.Do(ctx, "SET", s.userGenerationKey(userID), generation); err != nil {
		return gerror.Wrap(err, "替换 Redis 用户会话版本失败")
	}
	return s.save(ctx, session, generation)
}

func (s *RedisSessionStore) Get(ctx context.Context, sessionID string) (Session, bool, error) {
	value, err := s.client.Do(ctx, "GET", s.sessionKey(sessionID))
	if err != nil {
		return Session{}, false, gerror.Wrap(err, "读取 Redis 会话失败")
	}
	if value == nil || value.IsNil() {
		return Session{}, false, nil
	}
	var stored storedRedisSession
	if err = json.Unmarshal(value.Bytes(), &stored); err != nil {
		return Session{}, false, gerror.Wrap(err, "解析 Redis 会话失败")
	}
	if stored.ExpiresAt <= time.Now().Unix() {
		_, _ = s.client.Do(ctx, "DEL", s.sessionKey(sessionID))
		return Session{}, false, nil
	}
	generation, err := s.client.Do(ctx, "GET", s.userGenerationKey(stored.UserID))
	if err != nil {
		return Session{}, false, gerror.Wrap(err, "读取 Redis 用户会话版本失败")
	}
	if generation == nil || generation.IsNil() || generation.String() != stored.Generation {
		return Session{}, false, nil
	}
	return sessionFromStored(sessionID, stored), true, nil
}

func (s *RedisSessionStore) Rotate(ctx context.Context, sessionID string, oldRefreshJTIHash string, next Session) (bool, error) {
	if next.ID != sessionID {
		return false, nil
	}
	ttl, err := sessionTTL(next.RefreshTokenExpires)
	if err != nil {
		return false, err
	}
	stored := storedFromSession(next, "")
	payload, err := json.Marshal(stored)
	if err != nil {
		return false, gerror.Wrap(err, "编码 Redis 会话失败")
	}
	result, err := s.client.Do(
		ctx,
		"EVAL",
		rotateSessionScript,
		2,
		s.sessionKey(sessionID),
		s.userGenerationKey(next.UserID),
		oldRefreshJTIHash,
		next.UserID,
		time.Now().Unix(),
		string(payload),
		ttl,
	)
	if err != nil {
		return false, gerror.Wrap(err, "轮换 Redis 会话失败")
	}
	return result.Int() == 1, nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, sessionID string) error {
	if _, err := s.client.Do(ctx, "DEL", s.sessionKey(sessionID)); err != nil {
		return gerror.Wrap(err, "删除 Redis 会话失败")
	}
	return nil
}

func (s *RedisSessionStore) DeleteUser(ctx context.Context, userID int64) error {
	return s.DeleteUsers(ctx, []int64{userID})
}

func (s *RedisSessionStore) DeleteUsers(ctx context.Context, userIDs []int64) error {
	normalized := normalizeUserIDs(userIDs)
	if len(normalized) == 0 {
		return nil
	}
	arguments := make([]any, 0, 2+len(normalized)*2)
	arguments = append(arguments, revokeUsersScript, len(normalized))
	for _, userID := range normalized {
		arguments = append(arguments, s.userGenerationKey(userID))
	}
	for range normalized {
		generation, err := randomGeneration()
		if err != nil {
			return err
		}
		arguments = append(arguments, generation)
	}
	if _, err := s.client.Do(ctx, "EVAL", arguments...); err != nil {
		return gerror.Wrap(err, "批量撤销 Redis 用户会话失败")
	}
	return nil
}

func (s *RedisSessionStore) save(ctx context.Context, session Session, generation string) error {
	if session.ID == "" || session.UserID <= 0 || generation == "" {
		return gerror.New("会话参数不完整")
	}
	ttl, err := sessionTTL(session.RefreshTokenExpires)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(storedFromSession(session, generation))
	if err != nil {
		return gerror.Wrap(err, "编码 Redis 会话失败")
	}
	if _, err = s.client.Do(ctx, "SETEX", s.sessionKey(session.ID), ttl, string(payload)); err != nil {
		return gerror.Wrap(err, "保存 Redis 会话失败")
	}
	return nil
}

func (s *RedisSessionStore) ensureGeneration(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", gerror.New("用户 ID 无效")
	}
	candidate, err := randomGeneration()
	if err != nil {
		return "", err
	}
	result, err := s.client.Do(ctx, "EVAL", ensureGenerationScript, 1, s.userGenerationKey(userID), candidate)
	if err != nil {
		return "", gerror.Wrap(err, "初始化 Redis 用户会话版本失败")
	}
	if result == nil || result.IsNil() || result.String() == "" {
		return "", gerror.New("Redis 用户会话版本为空")
	}
	return result.String(), nil
}

func (s *RedisSessionStore) sessionKey(sessionID string) string {
	return fmt.Sprintf("%s:session:%s", s.prefix, sessionID)
}

func (s *RedisSessionStore) userGenerationKey(userID int64) string {
	return fmt.Sprintf("%s:user:%d:generation", s.prefix, userID)
}

func storedFromSession(session Session, generation string) storedRedisSession {
	return storedRedisSession{
		UserID:          session.UserID,
		AccessJTIHash:   session.AccessJTIHash,
		RefreshJTIHash:  session.RefreshJTIHash,
		PasswordVersion: session.PasswordVersion,
		ExpiresAt:       session.RefreshTokenExpires.Unix(),
		Generation:      generation,
	}
}

func sessionFromStored(sessionID string, stored storedRedisSession) Session {
	return Session{
		ID:                  sessionID,
		UserID:              stored.UserID,
		AccessJTIHash:       stored.AccessJTIHash,
		RefreshJTIHash:      stored.RefreshJTIHash,
		PasswordVersion:     stored.PasswordVersion,
		RefreshTokenExpires: time.Unix(stored.ExpiresAt, 0),
	}
}

func sessionTTL(expiresAt time.Time) (int64, error) {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return 0, gerror.New("会话已过期")
	}
	ttl := int64(remaining / time.Second)
	if remaining%time.Second != 0 {
		ttl++
	}
	return ttl, nil
}

func randomGeneration() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", gerror.Wrap(err, "生成会话版本失败")
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
