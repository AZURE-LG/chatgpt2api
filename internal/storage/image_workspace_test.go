package storage

import "testing"

func TestImageWorkspaceDatabaseStoresConversationAndTurns(t *testing.T) {
	backend, err := NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	now := "2026-05-01 18:00:00"
	conversation := ImageConversationRecord{ID: "conv-1", OwnerID: "user-1", OwnerName: "用户一", Title: "测试会话", CreatedAt: now, UpdatedAt: now}
	if err := backend.UpsertImageConversation(conversation); err != nil {
		t.Fatalf("UpsertImageConversation() error = %v", err)
	}
	turn := ImageConversationTurnRecord{ID: "turn-1", ConversationID: "conv-1", OwnerID: "user-1", Prompt: "生成一张图", Model: "gpt-image-2", Mode: "generate", Count: 1, Status: "success", CreatedAt: now, UpdatedAt: now}
	if err := backend.UpsertImageConversationTurn(turn); err != nil {
		t.Fatalf("UpsertImageConversationTurn() error = %v", err)
	}
	items, err := backend.ListImageConversations("user-1")
	if err != nil {
		t.Fatalf("ListImageConversations() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "conv-1" || items[0].Title != "测试会话" {
		t.Fatalf("ListImageConversations() = %#v", items)
	}
	turns, err := backend.ListImageConversationTurns("user-1", "conv-1")
	if err != nil {
		t.Fatalf("ListImageConversationTurns() error = %v", err)
	}
	if len(turns) != 1 || turns[0].ID != "turn-1" || turns[0].Prompt != "生成一张图" {
		t.Fatalf("ListImageConversationTurns() = %#v", turns)
	}
}

func TestImageWorkspaceDatabaseSoftDeletesConversation(t *testing.T) {
	backend, err := NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	now := "2026-05-01 18:00:00"
	deletedAt := "2026-05-01 18:05:00"
	conversation := ImageConversationRecord{ID: "conv-1", OwnerID: "user-1", OwnerName: "用户一", Title: "测试会话", CreatedAt: now, UpdatedAt: now}
	if err := backend.UpsertImageConversation(conversation); err != nil {
		t.Fatalf("UpsertImageConversation() error = %v", err)
	}
	if err := backend.SoftDeleteImageConversation("user-1", "conv-1", deletedAt); err != nil {
		t.Fatalf("SoftDeleteImageConversation() error = %v", err)
	}
	items, err := backend.ListImageConversations("user-1")
	if err != nil {
		t.Fatalf("ListImageConversations() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("ListImageConversations() = %#v", items)
	}
	item, err := backend.GetImageConversation("user-1", "conv-1")
	if err != nil {
		t.Fatalf("GetImageConversation() error = %v", err)
	}
	if item != nil {
		t.Fatalf("GetImageConversation() = %#v", item)
	}
	var storedDeletedAt string
	var storedUpdatedAt string
	if err := backend.db.QueryRow(`SELECT deleted_at, updated_at FROM image_conversations WHERE owner_id = ? AND id = ?`, "user-1", "conv-1").Scan(&storedDeletedAt, &storedUpdatedAt); err != nil {
		t.Fatalf("QueryRow(deleted conversation) error = %v", err)
	}
	if storedDeletedAt != deletedAt || storedUpdatedAt != deletedAt {
		t.Fatalf("deleted_at/updated_at = %q/%q, want %q", storedDeletedAt, storedUpdatedAt, deletedAt)
	}
}

func TestImageWorkspaceDatabaseStoresPrompts(t *testing.T) {
	backend, err := NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	now := "2026-05-01 18:00:00"
	if err := backend.UpsertPrompt(PromptRecord{ID: "prompt-1", OwnerID: "user-1", OwnerName: "用户一", Title: "公开提示词", Body: "蓝色产品海报", TagsJSON: `["产品"]`, Visibility: "public", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertPrompt(public) error = %v", err)
	}
	if err := backend.UpsertPrompt(PromptRecord{ID: "prompt-2", OwnerID: "user-1", OwnerName: "用户一", Title: "私有提示词", Body: "内部草稿", TagsJSON: `[]`, Visibility: "private", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertPrompt(private) error = %v", err)
	}
	publicItems, err := backend.ListPrompts("user-2", PromptListQuery{Scope: "public"})
	if err != nil {
		t.Fatalf("ListPrompts(public) error = %v", err)
	}
	if len(publicItems) != 1 || publicItems[0].ID != "prompt-1" {
		t.Fatalf("ListPrompts(public) = %#v", publicItems)
	}
	mineItems, err := backend.ListPrompts("user-1", PromptListQuery{Scope: "mine"})
	if err != nil {
		t.Fatalf("ListPrompts(mine) error = %v", err)
	}
	if len(mineItems) != 2 {
		t.Fatalf("ListPrompts(mine) = %#v", mineItems)
	}
}

func TestImageWorkspaceDatabaseStoresConversationAttachmentsByOwner(t *testing.T) {
	backend, err := NewDatabaseBackend("sqlite:///:memory:")
	if err != nil {
		t.Fatalf("NewDatabaseBackend() error = %v", err)
	}
	now := "2026-05-01 18:00:00"
	attachment := ConversationAttachmentRecord{
		ID:             "attachment-1",
		OwnerID:        "user-1",
		ConversationID: "conv-1",
		FileName:       "参考图.png",
		MIMEType:       "image/png",
		Size:           123,
		Path:           "user-1/conv-1/attachment-1.png",
		CreatedAt:      now,
	}
	if err := backend.UpsertConversationAttachment(attachment); err != nil {
		t.Fatalf("UpsertConversationAttachment() error = %v", err)
	}
	items, err := backend.ListConversationAttachments("user-1", "conv-1")
	if err != nil {
		t.Fatalf("ListConversationAttachments(owner) error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "attachment-1" || items[0].Path != attachment.Path {
		t.Fatalf("ListConversationAttachments(owner) = %#v", items)
	}
	foreign, err := backend.ListConversationAttachments("user-2", "conv-1")
	if err != nil {
		t.Fatalf("ListConversationAttachments(foreign) error = %v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("ListConversationAttachments(foreign) = %#v", foreign)
	}
}
