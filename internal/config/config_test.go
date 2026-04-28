package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreUpdatePersistsSettingsWithoutAuthKey(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHATGPT2API_ROOT", root)
	t.Setenv("CHATGPT2API_AUTH_KEY", "admin-secret")
	unsetEnv(t, "CHATGPT2API_BASE_URL")
	unsetEnv(t, "CHATGPT2API_PROXY")
	unsetEnv(t, "CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE")
	unsetEnv(t, "CHATGPT2API_IMAGE_RETENTION_DAYS")
	unsetEnv(t, "CHATGPT2API_AUTO_REMOVE_INVALID_ACCOUNTS")
	unsetEnv(t, "CHATGPT2API_AUTO_REMOVE_RATE_LIMITED_ACCOUNTS")
	unsetEnv(t, "CHATGPT2API_LOG_LEVELS")

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	got, err := store.Update(map[string]any{
		"auth-key":                        "new-secret",
		"base_url":                        "https://example.test/root/",
		"proxy":                           "http://127.0.0.1:8080",
		"refresh_account_interval_minute": 7,
		"image_retention_days":            14,
		"log_levels":                      []any{"debug", "error"},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, ok := got["auth-key"]; ok {
		t.Fatalf("Get() leaked auth-key: %#v", got)
	}
	if store.AuthKey() != "admin-secret" {
		t.Fatalf("AuthKey() = %q, want original external env key", store.AuthKey())
	}
	if store.BaseURL() != "https://example.test/root" {
		t.Fatalf("BaseURL() = %q", store.BaseURL())
	}

	envData, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	envText := string(envData)
	for _, want := range []string{
		"CHATGPT2API_BASE_URL=https://example.test/root/",
		"CHATGPT2API_PROXY=http://127.0.0.1:8080",
		"CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE=7",
		"CHATGPT2API_IMAGE_RETENTION_DAYS=14",
		"CHATGPT2API_LOG_LEVELS=debug,error",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf(".env missing %q in:\n%s", want, envText)
		}
	}
	if strings.Contains(envText, "CHATGPT2API_AUTH_KEY") || strings.Contains(envText, "new-secret") {
		t.Fatalf(".env should not persist auth key updates:\n%s", envText)
	}
}

func TestNewStoreDiscoversEnvFromParentDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("CHATGPT2API_AUTH_KEY=from-parent-env\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	nested := filepath.Join(root, "cmd", "chatgpt2api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})
	unsetEnv(t, "CHATGPT2API_ROOT")
	unsetEnv(t, "CHATGPT2API_AUTH_KEY")

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if store.RootDir != root {
		t.Fatalf("RootDir = %q, want %q", store.RootDir, root)
	}
	if store.AuthKey() != "from-parent-env" {
		t.Fatalf("AuthKey() = %q", store.AuthKey())
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%s): %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
