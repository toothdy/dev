package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
)

func TestBindCRUDMutationSupportsNodeBatchPayload(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(`[{"name":"read"},{"name":"write"}]`))
	request.Header.Set("Content-Type", "application/json")
	input, inputs, batch, err := bindCRUDMutation(&ghttp.Request{Request: request})
	if err != nil {
		t.Fatalf("bind batch payload failed: %v", err)
	}
	if input != nil || !batch || len(inputs) != 2 || inputs[1]["name"] != "write" {
		t.Fatalf("unexpected batch payload: %#v %#v %t", input, inputs, batch)
	}
}

func TestBindCRUDMutationRejectsNonObjectBatchItem(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(`[{"name":"read"},1]`))
	request.Header.Set("Content-Type", "application/json")
	if _, _, _, err := bindCRUDMutation(&ghttp.Request{Request: request}); err == nil {
		t.Fatal("expected non-object batch item rejection")
	}
}
