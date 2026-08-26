package modules

import (
	"os"
	"strings"
	"testing"
)

// 生成代码不得再出现权限标识字面量或 auth.Rule 构造
func TestGeneratedCodeCarriesNoPermissionLiteral(t *testing.T) {
	source, err := os.ReadFile("modules_gen.go")
	if err != nil {
		t.Fatalf("读取生成代码失败: %v", err)
	}
	content := string(source)

	for _, forbidden := range []string{`Permission: "`, "auth.Rule{"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("生成代码仍包含 %q", forbidden)
		}
	}
	if !strings.Contains(content, "apphttp.NewContextMiddleware(authService,") {
		t.Error("生成代码未按新签名注册 HTTP 中间件")
	}
}
