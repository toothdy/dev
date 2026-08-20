package security_test

import (
	"encoding/json"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/security"
)

/**
 * 验证平台租户使用 null 编解码
 * @param t 测试上下文
 * @returns null
 */
func TestTenantIdentityPlatformJSON(t *testing.T) {
	identity := security.PlatformTenant()
	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("marshal platform tenant failed: %v", err)
	}
	if string(data) != "null" {
		t.Fatalf("expected platform tenant null, got %s", data)
	}

	for _, input := range []string{"null", "0"} {
		var parsed security.TenantIdentity
		if err = json.Unmarshal([]byte(input), &parsed); err != nil {
			t.Fatalf("unmarshal platform tenant %s failed: %v", input, err)
		}
		if !parsed.IsPlatform() || parsed.IsMissing() {
			t.Fatalf("expected platform tenant for %s", input)
		}
	}
}

/**
 * 验证具体租户使用正整数编解码
 * @param t 测试上下文
 * @returns null
 */
func TestTenantIdentityTenantJSON(t *testing.T) {
	identity, err := security.NewTenantIdentity(42)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("marshal tenant identity failed: %v", err)
	}
	if string(data) != "42" {
		t.Fatalf("expected tenant id 42, got %s", data)
	}

	var parsed security.TenantIdentity
	if err = json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal tenant identity failed: %v", err)
	}
	if tenantID, ok := parsed.TenantID(); !ok || tenantID != 42 {
		t.Fatalf("expected tenant id 42, got %d, %v", tenantID, ok)
	}
}

/**
 * 验证非法租户值被拒绝
 * @param t 测试上下文
 * @returns null
 */
func TestTenantIdentityRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"-1", "1.5", `"1"`, "9223372036854775808"} {
		var identity security.TenantIdentity
		if err := json.Unmarshal([]byte(input), &identity); err == nil {
			t.Fatalf("expected invalid tenant identity rejected: %s", input)
		}
	}
	if _, err := security.NewTenantIdentity(0); err == nil {
		t.Fatal("expected zero tenant id rejected")
	}
}

/**
 * 验证未设置租户身份保持 Missing
 * @param t 测试上下文
 * @returns null
 */
func TestTenantIdentityMissing(t *testing.T) {
	var identity security.TenantIdentity
	if !identity.IsMissing() || identity.IsPlatform() {
		t.Fatal("expected zero identity to be missing")
	}
	if _, err := json.Marshal(identity); err == nil {
		t.Fatal("expected missing identity marshal rejected")
	}
}
