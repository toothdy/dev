package middleware

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type healthChecker struct {
	err   error
	calls int
}

func (c *healthChecker) Healthy(context.Context) error {
	c.calls++
	return c.err
}

func TestTaskMiddlewareProtectsOnlySchedulingWrites(t *testing.T) {
	checker := &healthChecker{err: errors.New("backend unavailable")}
	definition := Definition(nil)
	if definition.Name != "task.health" || definition.Handler == nil {
		t.Fatalf("unexpected middleware definition: %#v", definition)
	}
	if newHandler(checker) == nil {
		t.Fatal("expected health-checking handler")
	}
	for _, path := range []string{
		"/admin/task/info/add", "/admin/task/info/update", "/admin/task/info/start",
		"/admin/task/info/stop", "/admin/task/info/once",
	} {
		if !isProtectedWrite(http.MethodPost, path) {
			t.Fatalf("expected protected Task write path: %s", path)
		}
	}
	for _, path := range []string{
		"/admin/task/info/info", "/admin/task/info/page", "/admin/task/info/log", "/admin/task/info/delete",
	} {
		if isProtectedWrite(http.MethodGet, path) || isProtectedWrite(http.MethodPost, path) {
			t.Fatalf("expected readable Task path: %s", path)
		}
	}
}
