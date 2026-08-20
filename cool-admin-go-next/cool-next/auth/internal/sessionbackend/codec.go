package sessionbackend

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessioncontract"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

const schemaVersion = 1

var errExpired = errors.New("session 已过期")

type wireSession struct {
	SchemaVersion int                  `json:"schemaVersion"`
	SessionID     string               `json:"sessionId"`
	Subject       sessioncontract.Kind `json:"subject"`
	UserID        string               `json:"userId"`
	Username      *string              `json:"username,omitempty"`
	RoleIDs       *[]string            `json:"roleIds,omitempty"`
	PasswordV     *int                 `json:"passwordV,omitempty"`
	AccessJTI     string               `json:"accessJti"`
	RefreshJTI    string               `json:"refreshJti"`
	ExpiresAt     int64                `json:"expiresAt"`
}

// 编码版本化 Session JSON
func encode(value Session, now time.Time) ([]byte, error) {
	if err := validate(value, now); err != nil {
		return nil, err
	}

	wire := wireSession{
		SchemaVersion: schemaVersion,
		SessionID:     value.sessionID,
		Subject:       value.subject,
		UserID:        strconv.FormatUint(value.userID, 10),
		AccessJTI:     value.accessJTI,
		RefreshJTI:    value.refreshJTI,
		ExpiresAt:     value.expiresAt.UnixMilli(),
	}
	if value.subject == sessioncontract.AdminKind {
		roleIDs := make([]string, len(value.roleIDs))
		for index, roleID := range value.roleIDs {
			roleIDs[index] = strconv.FormatUint(roleID, 10)
		}
		wire.Username = &value.username
		wire.RoleIDs = &roleIDs
		wire.PasswordV = &value.passwordV
	}

	content, err := json.Marshal(wire)
	if err != nil {
		return nil, exception.WrapCore(err, "编码 Session 失败")
	}

	return content, nil
}

// 解码并校验版本化 Session JSON
func decode(content []byte, expectedKey, prefix string, now time.Time) (Session, error) {
	if !utf8.Valid(content) {
		return Session{}, exception.Core("Session JSON 不是有效 UTF-8")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil {
		return Session{}, exception.WrapCore(err, "解码 Session 失败")
	}
	var wire wireSession
	if err := json.Unmarshal(content, &wire); err != nil {
		return Session{}, exception.WrapCore(err, "解码 Session 失败")
	}
	if wire.SchemaVersion != schemaVersion {
		return Session{}, exception.Core("Session schemaVersion 不受支持")
	}
	if expectedKey != "" && expectedKey != prefix+wire.SessionID {
		return Session{}, exception.Core("Redis Key 与 Session ID 不一致")
	}

	userID, err := parseUint64("userId", wire.UserID)
	if err != nil {
		return Session{}, err
	}
	value := Session{
		sessionID:  wire.SessionID,
		subject:    wire.Subject,
		userID:     userID,
		accessJTI:  wire.AccessJTI,
		refreshJTI: wire.RefreshJTI,
		expiresAt:  time.UnixMilli(wire.ExpiresAt),
	}

	switch wire.Subject {
	case sessioncontract.AdminKind:
		if wire.Username == nil || wire.RoleIDs == nil || wire.PasswordV == nil {
			return Session{}, exception.Core("管理端 Session 字段不完整")
		}
		value.username = *wire.Username
		value.passwordV = *wire.PasswordV
		value.roleIDs = make([]uint64, len(*wire.RoleIDs))
		for index, encodedRoleID := range *wire.RoleIDs {
			roleID, parseErr := parseUint64(fmt.Sprintf("roleIds[%d]", index), encodedRoleID)
			if parseErr != nil {
				return Session{}, parseErr
			}
			value.roleIDs[index] = roleID
		}
	case sessioncontract.AppKind:
		_, hasUsername := fields["username"]
		_, hasRoleIDs := fields["roleIds"]
		_, hasPasswordV := fields["passwordV"]
		if hasUsername || hasRoleIDs || hasPasswordV {
			return Session{}, exception.Core("应用端 Session 不能携带管理端字段")
		}
	default:
		return Session{}, exception.Core("Session 身份种类无效")
	}

	if !value.expiresAt.After(now) {
		return Session{}, errExpired
	}
	if err = validate(value, now); err != nil {
		return Session{}, err
	}

	return value, nil
}

// 解析十进制 uint64 字段
func parseUint64(field, value string) (uint64, error) {
	if value == "" {
		return 0, exception.Core("Session " + field + " 不能为空")
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(parsed, 10) != value {
		if err == nil {
			err = errors.New("不是规范十进制字符串")
		}
		return 0, exception.WrapCore(err, "Session "+field+" 无效")
	}

	return parsed, nil
}
