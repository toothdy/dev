package security_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/security"
	"golang.org/x/crypto/bcrypt"
)

// 验证密码摘要使用成本为 12 的 bcrypt。
func TestHashPasswordUsesBcrypt(t *testing.T) {
	hashed, err := security.HashPassword("123456")
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hashed))
	if err != nil {
		t.Fatalf("read bcrypt cost failed: %v", err)
	}
	if cost != 12 {
		t.Fatalf("expected bcrypt cost 12, got %d", cost)
	}
	if !security.VerifyPassword("123456", hashed) {
		t.Fatal("expected correct password to pass")
	}
	if security.VerifyPassword("wrong", hashed) {
		t.Fatal("expected wrong password to fail")
	}
}

// 验证旧 MD5 摘要不再被接受。
func TestVerifyPasswordRejectsMD5(t *testing.T) {
	if security.VerifyPassword("123456", "e10adc3949ba59abbe56e057f20f883e") {
		t.Fatal("expected md5 hash to be rejected")
	}
}
