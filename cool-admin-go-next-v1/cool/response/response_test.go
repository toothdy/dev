package response_test

import (
	"encoding/json"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/response"
)

func TestOKWithDataMatchesNodeContract(t *testing.T) {
	body := response.OK(map[string]string{
		"token": "access.jwt.token",
	})

	if body.Code != exception.CodeSuccess {
		t.Fatalf("expected success code %d, got %d", exception.CodeSuccess, body.Code)
	}
	if body.Message != "success" {
		t.Fatalf("expected success message, got %q", body.Message)
	}
	if body.Data == nil {
		t.Fatal("expected data to be present")
	}

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if decoded["code"].(float64) != float64(exception.CodeSuccess) {
		t.Fatalf("expected json code %d, got %v", exception.CodeSuccess, decoded["code"])
	}
	if decoded["message"].(string) != "success" {
		t.Fatalf("expected json message success, got %v", decoded["message"])
	}
}

func TestFailDefaultsToCommonFail(t *testing.T) {
	body := response.Fail("账户或密码不正确~")

	if body.Code != exception.CodeCommFail {
		t.Fatalf("expected common fail code %d, got %d", exception.CodeCommFail, body.Code)
	}
	if body.Message != "账户或密码不正确~" {
		t.Fatalf("unexpected message: %q", body.Message)
	}
	if body.Data != nil {
		t.Fatalf("expected nil data, got %#v", body.Data)
	}
}

func TestFailSupportsCustomCode(t *testing.T) {
	body := response.Fail("参数错误", exception.CodeValidateFail)

	if body.Code != exception.CodeValidateFail {
		t.Fatalf("expected validate fail code %d, got %d", exception.CodeValidateFail, body.Code)
	}
}
