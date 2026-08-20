package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

func TestJWTSecretBootstrapReplacesPlaceholderOnce(t *testing.T) {
	configPath := writeJWTSecretTestConfig(t, `server:
  address: ":8001"
cool:
  initDB: false
  auth:
    jwtSecret: "cool-admin-go-next-xxxxxx"
    tokenExpire: 7200
module:
  base:
    allowKeys: []
`, 0o640)
	original := readYAMLMap(t, configPath)

	secret, err := bootstrapJWTSecret(configPath, jwtSecretPlaceholder)
	if err != nil {
		t.Fatalf("bootstrap jwtSecret failed: %v", err)
	}
	parsed, err := uuid.Parse(secret)
	if err != nil || parsed.Version() != 4 {
		t.Fatalf("expected UUID v4 secret, got %q: %v", secret, err)
	}

	updated := readYAMLMap(t, configPath)
	if got := nestedString(t, updated, "cool", "auth", "jwtSecret"); got != secret {
		t.Fatalf("expected generated secret in config, got %q", got)
	}
	setNestedString(t, original, "placeholder", "cool", "auth", "jwtSecret")
	setNestedString(t, updated, "placeholder", "cool", "auth", "jwtSecret")
	if !reflect.DeepEqual(updated, original) {
		t.Fatalf("unrelated config changed:\nwant: %#v\n got: %#v", original, updated)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("expected permissions 0640, got %04o", info.Mode().Perm())
	}

	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bootstrapJWTSecret(configPath, secret)
	if err != nil {
		t.Fatalf("second bootstrap failed: %v", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if second != secret || string(after) != string(before) {
		t.Fatal("non-placeholder secret should remain unchanged")
	}
}

func TestJWTSecretBootstrapDeploymentOverrideDoesNotReadConfig(t *testing.T) {
	secret := "deployment-secret-0123456789abcdef"
	got, err := bootstrapJWTSecret(filepath.Join(t.TempDir(), "missing.yaml"), secret)
	if err != nil {
		t.Fatalf("deployment override should not read config: %v", err)
	}
	if got != secret {
		t.Fatalf("expected deployment secret unchanged, got %q", got)
	}
}

func TestJWTSecretBootstrapRejectsInvalidConfig(t *testing.T) {
	tests := map[string]string{
		"invalid yaml":       "cool: [",
		"missing node":       "cool:\n  auth:\n    tokenExpire: 7200\n",
		"disk value changed": "cool:\n  auth:\n    jwtSecret: another-secret-value\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			configPath := writeJWTSecretTestConfig(t, content, 0o600)
			_, err := bootstrapJWTSecret(configPath, jwtSecretPlaceholder)
			if err == nil || strings.Contains(err.Error(), jwtSecretPlaceholder) {
				t.Fatalf("expected sanitized config error, got %v", err)
			}
		})
	}

	_, err := bootstrapJWTSecret(filepath.Join(t.TempDir(), "missing.yaml"), jwtSecretPlaceholder)
	if err == nil || strings.Contains(err.Error(), jwtSecretPlaceholder) {
		t.Fatalf("expected sanitized missing-file error, got %v", err)
	}
}

func TestJWTSecretBootstrapDoesNotExposeGeneratedSecretOnWriteFailure(t *testing.T) {
	configPath := writeJWTSecretTestConfig(t, "cool:\n  auth:\n    jwtSecret: cool-admin-go-next-xxxxxx\n", 0o600)
	generated := "generated-secret-must-not-appear-123456"
	wantErr := errors.New("rename failed")

	_, err := bootstrapJWTSecretWithDeps(
		configPath,
		jwtSecretPlaceholder,
		func() (string, error) { return generated, nil },
		func(string, []byte, fs.FileMode) error { return wantErr },
	)
	if !errors.Is(err, wantErr) || strings.Contains(err.Error(), generated) {
		t.Fatalf("expected sanitized write error, got %v", err)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(content), generated) {
		t.Fatal("failed write changed config")
	}
}

func TestJWTSecretAtomicWritePropagatesRenameFailure(t *testing.T) {
	configPath := writeJWTSecretTestConfig(t, "old", 0o600)
	wantErr := errors.New("forced rename failure")
	err := atomicWriteJWTConfig(configPath, []byte("new"), 0o600, func(_, _ string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected rename error, got %v", err)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "old" {
		t.Fatalf("rename failure changed target: %q", content)
	}
}

func TestJWTSecretUnsafeValues(t *testing.T) {
	unsafe := []string{
		"",
		"short-secret",
		jwtSecretPlaceholder,
		"cool-admin-go-next-jwt-secret-key",
		"your-jwt-secret-change-this-before-production",
	}
	for _, secret := range unsafe {
		if !unsafeJWTSecret(secret) {
			t.Fatalf("expected unsafe jwtSecret %q to be rejected", secret)
		}
	}
	if unsafeJWTSecret("9f61720a-0e0b-42bc-a8c1-540983f70f72") {
		t.Fatal("expected generated UUID secret to be accepted")
	}
}

func writeJWTSecretTestConfig(t *testing.T, content string, mode fs.FileMode) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err = yaml.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func nestedString(t *testing.T, value map[string]any, path ...string) string {
	t.Helper()
	current := value
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("missing map node %q", key)
		}
		current = next
	}
	result, ok := current[path[len(path)-1]].(string)
	if !ok {
		t.Fatalf("missing string node %q", path[len(path)-1])
	}
	return result
}

func setNestedString(t *testing.T, value map[string]any, replacement string, path ...string) {
	t.Helper()
	current := value
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("missing map node %q", key)
		}
		current = next
	}
	current[path[len(path)-1]] = replacement
}
