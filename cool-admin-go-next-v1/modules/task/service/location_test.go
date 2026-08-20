package service

import (
	"testing"

	taskModule "github.com/toothdy/cool-admin-go-next/modules/task"
)

func TestNewLocationUsesTaskTimezone(t *testing.T) {
	location, err := NewLocation(taskModule.Config{Timezone: "UTC"})
	if err != nil {
		t.Fatalf("create Task location: %v", err)
	}
	if location.String() != "UTC" {
		t.Fatalf("unexpected Task location: %s", location)
	}
}

func TestNewLocationRejectsInvalidTimezone(t *testing.T) {
	_, err := NewLocation(taskModule.Config{Timezone: "Missing/Timezone"})
	if err == nil {
		t.Fatal("expected invalid Task timezone to fail")
	}
}
