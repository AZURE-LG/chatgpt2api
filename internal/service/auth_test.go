package service

import (
	"path/filepath"
	"testing"

	"chatgpt2api/internal/storage"
)

func TestAuthServiceCreateAuthenticateDisableAndDelete(t *testing.T) {
	backend := storage.NewJSONBackend(
		filepath.Join(t.TempDir(), "accounts.json"),
		filepath.Join(t.TempDir(), "auth_keys.json"),
	)
	auth := NewAuthService(backend)

	public, raw, err := auth.CreateKey("user", "绘图用户")
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if raw == "" {
		t.Fatal("CreateKey() returned empty raw key")
	}
	if _, ok := public["key_hash"]; ok {
		t.Fatalf("public key item leaked key_hash: %#v", public)
	}
	if _, ok := public["key"]; ok {
		t.Fatalf("public key item leaked raw key: %#v", public)
	}

	identity := auth.Authenticate(raw)
	if identity == nil {
		t.Fatal("Authenticate(raw) returned nil")
	}
	if identity.Role != "user" || identity.Name != "绘图用户" {
		t.Fatalf("identity = %#v", identity)
	}

	keyID, _ := public["id"].(string)
	revealed, found := auth.RevealKey(keyID, "user")
	if !found || revealed != raw {
		t.Fatalf("RevealKey() = %q, %v; want raw, true", revealed, found)
	}

	updated := auth.UpdateKey(keyID, map[string]any{"enabled": false}, "user")
	if updated == nil {
		t.Fatal("UpdateKey() returned nil")
	}
	if auth.Authenticate(raw) != nil {
		t.Fatal("disabled key still authenticated")
	}

	if !auth.DeleteKey(keyID, "user") {
		t.Fatal("DeleteKey() = false")
	}
	if len(auth.ListKeys("user")) != 0 {
		t.Fatalf("ListKeys(user) after delete = %#v", auth.ListKeys("user"))
	}
}
