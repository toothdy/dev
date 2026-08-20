package route_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/util/route"
)

func TestKeyNormalizesMethodAndPath(t *testing.T) {
	key, err := route.Key("post", "/admin//base/user/")
	if err != nil || key != "POST:/admin/base/user" {
		t.Fatalf("unexpected route key %q: %v", key, err)
	}
}

func TestNormalizePathRejectsUnsafeMetadata(t *testing.T) {
	for _, value := range []string{
		"relative",
		"/admin/../open",
		"/path?x=1",
		"/path#fragment",
		"/users/:id",
		"/files/*path",
		"/users/{id}",
	} {
		if _, err := route.NormalizePath(value); err == nil {
			t.Fatalf("expected unsafe path rejected: %q", value)
		}
	}
}
