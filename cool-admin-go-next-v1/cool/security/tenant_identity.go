package security

import (
	"bytes"
	"encoding/json"

	"github.com/gogf/gf/v2/errors/gerror"
)

type tenantIdentityKind uint8

const (
	tenantIdentityMissing tenantIdentityKind = iota
	tenantIdentityPlatform
	tenantIdentityTenant
)

// TenantIdentity 表示 JWT 和用户上下文中的租户身份
type TenantIdentity struct {
	kind     tenantIdentityKind
	tenantID int64
}

/**
 * 创建平台租户身份
 * @returns 平台租户身份
 */
func PlatformTenant() TenantIdentity {
	return TenantIdentity{kind: tenantIdentityPlatform}
}

/**
 * 创建具体租户身份
 * @param tenantID 租户 ID
 * @returns 租户身份和校验错误
 */
func NewTenantIdentity(tenantID int64) (TenantIdentity, error) {
	if tenantID <= 0 {
		return TenantIdentity{}, gerror.New("租户 ID 必须大于 0")
	}
	return TenantIdentity{
		kind:     tenantIdentityTenant,
		tenantID: tenantID,
	}, nil
}

/**
 * 判断租户身份是否缺失
 * @returns 是否缺失
 */
func (i TenantIdentity) IsMissing() bool {
	return i.kind == tenantIdentityMissing
}

/**
 * 判断是否为平台租户身份
 * @returns 是否为平台身份
 */
func (i TenantIdentity) IsPlatform() bool {
	return i.kind == tenantIdentityPlatform
}

/**
 * 获取具体租户 ID
 * @returns 租户 ID 和是否存在
 */
func (i TenantIdentity) TenantID() (int64, bool) {
	if i.kind != tenantIdentityTenant {
		return 0, false
	}
	return i.tenantID, true
}

/**
 * 编码租户身份
 * @returns JSON 数据和编码错误
 */
func (i TenantIdentity) MarshalJSON() ([]byte, error) {
	switch i.kind {
	case tenantIdentityPlatform:
		return []byte("null"), nil
	case tenantIdentityTenant:
		return json.Marshal(i.tenantID)
	default:
		return nil, gerror.New("租户身份缺失")
	}
}

/**
 * 解码租户身份
 * @param data JSON 数据
 * @returns 解码错误
 */
func (i *TenantIdentity) UnmarshalJSON(data []byte) error {
	if i == nil {
		return gerror.New("租户身份接收器为空")
	}
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*i = PlatformTenant()
		return nil
	}

	var tenantID int64
	if err := json.Unmarshal(data, &tenantID); err != nil {
		return gerror.Wrap(err, "租户 ID 格式错误")
	}
	if tenantID == 0 {
		*i = PlatformTenant()
		return nil
	}
	identity, err := NewTenantIdentity(tenantID)
	if err != nil {
		return err
	}
	*i = identity
	return nil
}
