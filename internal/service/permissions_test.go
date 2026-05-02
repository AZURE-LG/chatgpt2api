package service

import "testing"

func TestNormalizeAPIPermissionsMigratesCreationTaskPermissions(t *testing.T) {
	permissions := NormalizeAPIPermissions([]string{
		APIPermissionKey("GET", "/api/image-tasks"),
		"POST /api/image-tasks",
	})

	if !HasAPIPermission(PermissionSet{APIPermissions: permissions}, "GET", "/api/creation-tasks") {
		t.Fatalf("migrated permissions missing creation task read: %#v", permissions)
	}
	if !HasAPIPermission(PermissionSet{APIPermissions: permissions}, "POST", "/api/creation-tasks/chat-completions") {
		t.Fatalf("migrated permissions missing creation task submit subtree: %#v", permissions)
	}
	if HasAPIPermission(PermissionSet{APIPermissions: permissions}, "GET", "/api/image-tasks") {
		t.Fatalf("old image task route should not be authorized: %#v", permissions)
	}
}

func TestAccountPoolPermissionsAreExplicit(t *testing.T) {
	readOnly := PermissionSet{APIPermissions: []string{APIPermissionKey("GET", "/api/accounts")}}
	if !HasAPIPermission(readOnly, "GET", "/api/accounts") {
		t.Fatalf("read-only account permission missing account list")
	}
	if HasAPIPermission(readOnly, "GET", "/api/accounts/tokens") {
		t.Fatalf("account list permission should not allow token export")
	}
	if HasAPIPermission(readOnly, "POST", "/api/accounts/refresh") {
		t.Fatalf("account list permission should not allow refresh")
	}

	operators := PermissionSet{APIPermissions: NormalizeAPIPermissions([]string{
		APIPermissionKey("GET", "/api/accounts/tokens"),
		APIPermissionKey("POST", "/api/accounts"),
		APIPermissionKey("POST", "/api/accounts/refresh"),
		APIPermissionKey("POST", "/api/accounts/update"),
		APIPermissionKey("DELETE", "/api/accounts"),
	})}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{"GET", "/api/accounts/tokens"},
		{"POST", "/api/accounts"},
		{"POST", "/api/accounts/refresh"},
		{"POST", "/api/accounts/update"},
		{"DELETE", "/api/accounts"},
	} {
		if !HasAPIPermission(operators, tc.method, tc.path) {
			t.Fatalf("missing explicit permission for %s %s in %#v", tc.method, tc.path, operators.APIPermissions)
		}
	}
}

func TestDefaultUserPermissionIncludesPromptLibrary(t *testing.T) {
	set := DefaultPermissionSetForRole(AuthRoleUser)
	if !testStringSliceContains(set.MenuPaths, "/prompts") {
		t.Fatalf("default user menu paths = %#v, want /prompts", set.MenuPaths)
	}
	if !testStringSliceContains(set.APIPermissions, APIPermissionKey("GET", "/api/prompts")) {
		t.Fatalf("default user api permissions = %#v, want GET /api/prompts", set.APIPermissions)
	}
	if !testStringSliceContains(set.APIPermissions, APIPermissionKey("POST", "/api/image-conversations")) {
		t.Fatalf("default user api permissions = %#v, want POST /api/image-conversations", set.APIPermissions)
	}
	if !HasAPIPermission(set, "POST", "/api/image-conversations/conv-1/turns") {
		t.Fatalf("default user permissions should allow image conversation subtree: %#v", set.APIPermissions)
	}
	if !HasAPIPermission(set, "PATCH", "/api/prompts/prompt-1") {
		t.Fatalf("default user permissions should allow prompt PATCH subtree: %#v", set.APIPermissions)
	}
	if !HasAPIPermission(set, "PATCH", "/api/image-conversations/conv-1/turns/turn-1") {
		t.Fatalf("default user permissions should allow image conversation PATCH subtree: %#v", set.APIPermissions)
	}
}

func testStringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
