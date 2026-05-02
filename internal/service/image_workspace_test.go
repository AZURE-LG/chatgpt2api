package service

import (
	"bytes"
	"testing"

	"chatgpt2api/internal/storage"
)

func TestImageConversationServiceRejectsForeignConversation(t *testing.T) {
	backend, err := storage.NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	service, err := NewImageWorkspaceService(backend, t.TempDir())
	if err != nil {
		t.Fatalf("NewImageWorkspaceService() error = %v", err)
	}
	owner := Identity{Role: AuthRoleUser, OwnerID: "user-1", Name: "用户一"}
	foreign := Identity{Role: AuthRoleUser, OwnerID: "user-2", Name: "用户二"}
	created, err := service.CreateConversation(owner, ImageConversationInput{Title: "我的会话"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.GetConversation(foreign, created.ID); err == nil {
		t.Fatalf("GetConversation() error = nil, want foreign access error")
	}
}

func TestImageConversationServiceRejectsForeignClientIDCollision(t *testing.T) {
	backend, err := storage.NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	service, err := NewImageWorkspaceService(backend, t.TempDir())
	if err != nil {
		t.Fatalf("NewImageWorkspaceService() error = %v", err)
	}
	owner := Identity{Role: AuthRoleUser, OwnerID: "user-1", Name: "用户一"}
	foreign := Identity{Role: AuthRoleUser, OwnerID: "user-2", Name: "用户二"}
	created, err := service.CreateConversation(owner, ImageConversationInput{Title: "原始标题"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.SaveConversation(foreign, ImageConversationInput{ID: created.ID, Title: "覆盖标题"}); err == nil {
		t.Fatalf("SaveConversation(foreign collision) error = nil")
	}
	current, err := service.GetConversation(owner, created.ID)
	if err != nil {
		t.Fatalf("GetConversation(owner) error = %v", err)
	}
	if current.Title != "原始标题" {
		t.Fatalf("foreign collision changed title to %q", current.Title)
	}
}

func TestConversationAttachmentRequiresOwnedConversation(t *testing.T) {
	backend, err := storage.NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	service, err := NewImageWorkspaceService(backend, t.TempDir())
	if err != nil {
		t.Fatalf("NewImageWorkspaceService() error = %v", err)
	}
	owner := Identity{Role: AuthRoleUser, OwnerID: "user-1", Name: "用户一"}
	foreign := Identity{Role: AuthRoleUser, OwnerID: "user-2", Name: "用户二"}
	conversation, err := service.CreateConversation(owner, ImageConversationInput{Title: "我的会话"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.SaveAttachment(foreign, conversation.ID, "foreign.png", "image/png", 3, bytes.NewReader([]byte("png"))); err == nil {
		t.Fatalf("SaveAttachment(foreign) error = nil")
	}
	attachment, err := service.SaveAttachment(owner, conversation.ID, "owner.png", "image/png", 3, bytes.NewReader([]byte("png")))
	if err != nil {
		t.Fatalf("SaveAttachment(owner) error = %v", err)
	}
	if _, err := service.GetAttachment(foreign, attachment.ID); err == nil {
		t.Fatalf("GetAttachment(foreign) error = nil")
	}
	if err := service.DeleteAttachment(foreign, attachment.ID); err == nil {
		t.Fatalf("DeleteAttachment(foreign) error = nil")
	}
	if _, err := service.GetAttachment(owner, attachment.ID); err != nil {
		t.Fatalf("GetAttachment(owner) error = %v", err)
	}
}

func TestPromptLibraryVisibilityRules(t *testing.T) {
	backend, err := storage.NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	service, err := NewImageWorkspaceService(backend, t.TempDir())
	if err != nil {
		t.Fatalf("NewImageWorkspaceService() error = %v", err)
	}
	owner := Identity{Role: AuthRoleUser, OwnerID: "user-1", Name: "用户一"}
	reader := Identity{Role: AuthRoleUser, OwnerID: "user-2", Name: "用户二"}
	if _, err := service.CreatePrompt(owner, PromptInput{Title: "公开提示词", Body: "蓝色产品海报", Visibility: "public"}); err != nil {
		t.Fatalf("CreatePrompt(public) error = %v", err)
	}
	if _, err := service.CreatePrompt(owner, PromptInput{Title: "私有提示词", Body: "内部草稿", Visibility: "private"}); err != nil {
		t.Fatalf("CreatePrompt(private) error = %v", err)
	}
	items, err := service.ListPrompts(reader, PromptQuery{Scope: "public"})
	if err != nil {
		t.Fatalf("ListPrompts(public) error = %v", err)
	}
	if len(items) != 1 || items[0].Title != "公开提示词" {
		t.Fatalf("ListPrompts(public) = %#v", items)
	}
}
