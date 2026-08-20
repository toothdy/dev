package exception_test

import (
	stderrors "errors"
	"net/http"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/exception"
)

func TestResolvePublicErrors(t *testing.T) {
	tests := []struct {
		err     error
		kind    exception.Kind
		status  int
		code    int
		message string
	}{
		{exception.Comm("数据不存在"), exception.KindComm, http.StatusOK, 1001, "数据不存在"},
		{exception.Validate("参数错误"), exception.KindValidate, http.StatusOK, 1002, "参数错误"},
		{exception.UnsupportedMediaType("不支持的 Content-Type"), exception.KindValidate, http.StatusUnsupportedMediaType, 1002, "不支持的 Content-Type"},
		{exception.PayloadTooLarge("请求体过大"), exception.KindValidate, http.StatusRequestEntityTooLarge, 1002, "请求体过大"},
		{exception.Unauthorized(), exception.KindUnauthorized, http.StatusUnauthorized, 1001, "登录失效~"},
		{exception.Forbidden(), exception.KindForbidden, http.StatusForbidden, 1001, "登录失效或无权限访问~"},
	}
	for _, test := range tests {
		resolved := exception.Resolve(test.err)
		if resolved.Kind != test.kind || resolved.HTTPStatus != test.status || resolved.BusinessCode != test.code || resolved.Message != test.message {
			t.Fatalf("unexpected resolved error: %#v", resolved)
		}
	}
}

func TestResolveInternalDoesNotExposeCause(t *testing.T) {
	err := exception.Internal(stderrors.New("sql: password=secret"), "query user")
	resolved := exception.Resolve(err)
	if resolved.Kind != exception.KindInternal || resolved.Message != "操作失败" {
		t.Fatalf("unexpected internal resolution: %#v", resolved)
	}
	if !strings.Contains(err.Error(), "sql: password=secret") {
		t.Fatalf("internal error lost cause: %v", err)
	}
}

func TestResolveWrappedPublicErrorDoesNotExposeCause(t *testing.T) {
	err := exception.WrapPayloadTooLarge(stderrors.New("http body reader detail"), "请求体过大")
	resolved := exception.Resolve(err)
	if resolved.HTTPStatus != http.StatusRequestEntityTooLarge || resolved.Message != "请求体过大" {
		t.Fatalf("unexpected wrapped public resolution: %#v", resolved)
	}
	if !strings.Contains(err.Error(), "http body reader detail") {
		t.Fatalf("wrapped public error lost cause: %v", err)
	}
}
