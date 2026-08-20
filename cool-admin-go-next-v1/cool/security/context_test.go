package security_test

import (
	"context"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/security"
)

func TestContextWithUser(t *testing.T) {
	user := security.UserContext{
		UserId:          1,
		Username:        "admin",
		RoleIds:         []int64{1},
		PasswordVersion: 1,
		TenantId:        security.PlatformTenant(),
	}
	ctx := security.ContextWithUser(context.Background(), user)
	got, ok := security.UserFromContext(ctx)
	if !ok {
		t.Fatal("expected user from context")
	}
	if got.UserId != 1 || got.Username != "admin" {
		t.Fatalf("unexpected user context: %#v", got)
	}
	if !got.TenantId.IsPlatform() {
		t.Fatal("expected platform tenant context")
	}
}

func TestUserFromEmptyContext(t *testing.T) {
	if _, ok := security.UserFromContext(context.Background()); ok {
		t.Fatal("expected no user from empty context")
	}
}
