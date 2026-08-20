package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskSeedContainsStoppedNodeDemoTasks(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), "modules", "task", "db.json"))
	if err != nil {
		t.Fatalf("read Task seed failed: %v", err)
	}
	var seed struct {
		TaskInfo []struct {
			Status  int    `json:"status"`
			Service string `json:"service"`
		} `json:"task_info"`
	}
	if err = json.Unmarshal(content, &seed); err != nil {
		t.Fatalf("decode Task seed failed: %v", err)
	}
	if len(seed.TaskInfo) != 2 {
		t.Fatalf("expected two Node demo tasks, got %d", len(seed.TaskInfo))
	}
	for _, item := range seed.TaskInfo {
		if item.Status != 0 || item.Service == "" {
			t.Fatalf("Task seed must remain stopped and explicit: %#v", item)
		}
	}
}
