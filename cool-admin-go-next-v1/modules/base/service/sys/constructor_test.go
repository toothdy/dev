package sys

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestConstructorUsesNamedAuthOptions(t *testing.T) {
	service := NewAuthService(nil, nil, nil, module.AuthOptions{SSO: true})
	if !service.SSO {
		t.Fatal("named AuthOptions SSO was not applied")
	}
}
