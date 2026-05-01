# 创作台会话与提示词库实现计划

> **面向 agentic workers：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 按任务执行。步骤使用 checkbox (`- [ ]`) 语法跟踪。

**目标：** 按 B+ 方案实现创作台会话服务端持久化、图片提示词关联、账号级提示词库。

**架构：** 数据库后端新增结构化表，服务层提供会话、附件、提示词库和图片提示词元数据能力，HTTP 层暴露 `/api/image-conversations`、`/api/prompts` 和图片元数据接口。前端创作台改为通过服务端 API 读写会话，图片库展示并编辑提示词关联，新增独立提示词库页面。

**技术栈：** Go `database/sql`、`net/http`、现有 `service`/`storage` 分层、React 19、TypeScript、Vite、Tailwind CSS、shadcn 风格组件。

---

## 文件结构

- 新建 `internal/storage/image_workspace.go`：数据库结构、记录类型、CRUD 方法、数据库后端能力接口。
- 新建 `internal/storage/image_workspace_test.go`：SQLite 下的结构化存储测试。
- 新建 `internal/service/image_workspace.go`：会话、附件、提示词库服务类型、输入校验和权限规则。
- 新建 `internal/service/image_workspace_test.go`：服务层 owner 隔离、公开/私有提示词、附件校验测试。
- 新建 `internal/httpapi/image_workspace.go`：会话、附件、提示词库、图片提示词元数据 HTTP handler。
- 修改 `internal/httpapi/app.go`：初始化新服务，任务完成时记录图片提示词元数据。
- 修改 `internal/httpapi/router.go`：注册新 API 路由。
- 修改 `internal/service/image.go`：扩展图片元数据字段和手动提示词更新方法。
- 修改 `internal/service/image_test.go`：覆盖图片提示词元数据读写和权限。
- 修改 `internal/service/image_task.go`：任务 payload 和 public task 保留 `conversation_id`、`turn_id`。
- 修改 `internal/service/image_task_test.go`：覆盖任务字段持久化。
- 修改 `internal/service/permissions.go`：新增提示词库菜单和 API 权限。
- 修改 `internal/service/permissions_test.go`：覆盖默认权限和权限目录。
- 修改 `internal/config/config.go`：增加 `ConversationAttachmentsDir()`。
- 修改 `web/src/lib/api.ts`：扩展 `ManagedImage`、`CreationTask` 字段，并让任务提交支持 `conversation_id`、`turn_id`。
- 新建 `web/src/lib/image-workspace-api.ts`：会话、附件、提示词库、图片提示词元数据 API helper。
- 修改 `web/src/store/image-conversations.ts`：去掉 IndexedDB 写入路径，改为服务端 API 读写。
- 修改 `web/src/app/image/page.tsx`：接入服务端会话、附件上传、任务字段传递、提示词库入口。
- 修改 `web/src/app/image/components/image-composer.tsx`：增加我的提示词库入口按钮。
- 修改 `web/src/app/image/components/image-sidebar.tsx`：服务端会话列表交互保持现有视觉结构。
- 修改 `web/src/app/image-manager/page.tsx`：提示词搜索、详情展示、手动提示词编辑、保存到提示词库、打开会话。
- 新建 `web/src/app/prompts/page.tsx`：提示词库页面。
- 修改 `web/src/app/route-config.tsx`：注册 `/prompts`。

## 数据结构约定

### 存储层记录类型

```go
type ImageConversationRecord struct {
	ID        string
	OwnerID   string
	OwnerName string
	Title     string
	CreatedAt string
	UpdatedAt string
	DeletedAt string
}

type ImageConversationTurnRecord struct {
	ID                  string
	ConversationID      string
	OwnerID             string
	Prompt              string
	Model               string
	Mode                string
	Count               int
	Size                string
	Quality             string
	Visibility          string
	Status              string
	Error               string
	ReferenceImagesJSON string
	ResultImagesJSON    string
	CreatedAt           string
	UpdatedAt           string
}

type PromptRecord struct {
	ID                   string
	OwnerID              string
	OwnerName            string
	Title                string
	Body                 string
	TagsJSON             string
	Category             string
	Note                 string
	Model                string
	UseCase              string
	Visibility           string
	SourceConversationID string
	SourceTurnID         string
	SourceImagePath      string
	CreatedAt            string
	UpdatedAt            string
}
```

### 图片元数据新增字段

```go
type imageMetadata struct {
	OwnerID        string
	OwnerName      string
	Visibility     string
	PublishedAt    string
	Prompt         string
	RevisedPrompt  string
	ManualPrompt   string
	Model          string
	Size           string
	Quality        string
	Mode           string
	ConversationID string
	TurnID         string
	TaskID         string
	PromptID       string
}
```

## Task 1：数据库结构化存储

**Files:**
- Create: `internal/storage/image_workspace.go`
- Create: `internal/storage/image_workspace_test.go`
- Modify: `internal/storage/storage.go`

- [ ] **Step 1：编写失败测试，验证表初始化和会话 CRUD**

在 `internal/storage/image_workspace_test.go` 新增：

```go
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
```

Run: `go test ./internal/storage -run TestImageWorkspaceDatabaseStoresConversationAndTurns -count=1`
Expected: FAIL，提示 `ImageConversationRecord` 或 `UpsertImageConversation` 未定义。

- [ ] **Step 2：实现表结构和存储接口**

在 `internal/storage/image_workspace.go` 新增数据库接口和方法。表结构使用 `TEXT` 存时间，兼容 SQLite/PostgreSQL/MySQL：

```go
type ImageWorkspaceStore interface {
	UpsertImageConversation(ImageConversationRecord) error
	ListImageConversations(ownerID string) ([]ImageConversationRecord, error)
	GetImageConversation(ownerID, id string) (*ImageConversationRecord, error)
	SoftDeleteImageConversation(ownerID, id, deletedAt string) error
	UpsertImageConversationTurn(ImageConversationTurnRecord) error
	ListImageConversationTurns(ownerID, conversationID string) ([]ImageConversationTurnRecord, error)
	UpsertPrompt(PromptRecord) error
	ListPrompts(ownerID, scope, query string) ([]PromptRecord, error)
	GetPrompt(ownerID, id string, includePublic bool) (*PromptRecord, error)
	DeletePrompt(ownerID, id string, isAdmin bool) error
}

func ImageWorkspaceStoreFromBackend(backend Backend) (ImageWorkspaceStore, bool) {
	store, ok := backend.(*DatabaseBackend)
	return store, ok
}
```

在 `DatabaseBackend.init()` 的 schema 中追加三张表和索引：

```go
`CREATE TABLE IF NOT EXISTS image_conversations (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, owner_name TEXT NOT NULL, title TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, deleted_at TEXT NOT NULL)`,
`CREATE INDEX IF NOT EXISTS idx_image_conversations_owner_updated ON image_conversations (owner_id, deleted_at, updated_at)`,
`CREATE TABLE IF NOT EXISTS image_conversation_turns (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, owner_id TEXT NOT NULL, prompt TEXT NOT NULL, model TEXT NOT NULL, mode TEXT NOT NULL, count INTEGER NOT NULL, size TEXT NOT NULL, quality TEXT NOT NULL, visibility TEXT NOT NULL, status TEXT NOT NULL, error TEXT NOT NULL, reference_images_json TEXT NOT NULL, result_images_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
`CREATE INDEX IF NOT EXISTS idx_image_conversation_turns_conversation ON image_conversation_turns (owner_id, conversation_id, created_at)`,
`CREATE TABLE IF NOT EXISTS prompts (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, owner_name TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, tags_json TEXT NOT NULL, category TEXT NOT NULL, note TEXT NOT NULL, model TEXT NOT NULL, use_case TEXT NOT NULL, visibility TEXT NOT NULL, source_conversation_id TEXT NOT NULL, source_turn_id TEXT NOT NULL, source_image_path TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
`CREATE INDEX IF NOT EXISTS idx_prompts_owner_visibility_updated ON prompts (owner_id, visibility, updated_at)`,
```

- [ ] **Step 3：运行存储测试**

Run: `go test ./internal/storage -run TestImageWorkspaceDatabaseStoresConversationAndTurns -count=1`
Expected: PASS。

## Task 2：服务层权限与校验

**Files:**
- Create: `internal/service/image_workspace.go`
- Create: `internal/service/image_workspace_test.go`

- [ ] **Step 1：编写失败测试，验证 owner 隔离**

在 `internal/service/image_workspace_test.go` 新增：

```go
package service

import (
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
```

Run: `go test ./internal/service -run TestImageConversationServiceRejectsForeignConversation -count=1`
Expected: FAIL，提示 `NewImageWorkspaceService` 未定义。

- [ ] **Step 2：实现服务类型**

在 `internal/service/image_workspace.go` 新增 `ImageWorkspaceService`，用 `ownerID(identity)` 和 `Identity.Role` 做权限判断：

```go
type ImageWorkspaceService struct {
	store         storage.ImageWorkspaceStore
	attachmentDir string
}

func NewImageWorkspaceService(backend storage.Backend, attachmentDir string) (*ImageWorkspaceService, error) {
	store, ok := storage.ImageWorkspaceStoreFromBackend(backend)
	if !ok {
		return nil, errors.New("image workspace requires database storage backend")
	}
	if err := os.MkdirAll(attachmentDir, 0o755); err != nil {
		return nil, err
	}
	return &ImageWorkspaceService{store: store, attachmentDir: attachmentDir}, nil
}
```

实现 `CreateConversation`、`GetConversation`、`ListConversations`、`DeleteConversation`、`UpsertTurn`、`CreatePrompt`、`ListPrompts`、`UpdatePrompt`、`DeletePrompt`、`CopyPrompt`。所有普通用户写操作使用当前 `ownerID(identity)`，管理员只额外获得公开提示词管理能力，不获得读取所有会话能力。

- [ ] **Step 3：补充提示词公开/私有测试**

在同一测试文件新增：

```go
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
```

Run: `go test ./internal/service -run 'TestImageConversationServiceRejectsForeignConversation|TestPromptLibraryVisibilityRules' -count=1`
Expected: PASS。

## Task 3：HTTP API 与权限目录

**Files:**
- Create: `internal/httpapi/image_workspace.go`
- Modify: `internal/httpapi/app.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/service/permissions.go`
- Modify: `internal/service/permissions_test.go`
- Test: `internal/httpapi/app_test.go`

- [ ] **Step 1：新增权限目录失败测试**

在 `internal/service/permissions_test.go` 新增断言：默认用户拥有 `/prompts` 菜单和 `/api/prompts` 读写权限，权限目录包含会话 API。

```go
func TestDefaultUserPermissionIncludesPromptLibrary(t *testing.T) {
	set := DefaultPermissionSetForRole(AuthRoleUser)
	if !containsString(set.MenuPaths, "/prompts") {
		t.Fatalf("default user menu paths = %#v, want /prompts", set.MenuPaths)
	}
	if !containsString(set.APIPermissions, APIPermissionKey("GET", "/api/prompts")) {
		t.Fatalf("default user api permissions = %#v, want GET /api/prompts", set.APIPermissions)
	}
	if !containsString(set.APIPermissions, APIPermissionKey("POST", "/api/image-conversations")) {
		t.Fatalf("default user api permissions = %#v, want POST /api/image-conversations", set.APIPermissions)
	}
}
```

Run: `go test ./internal/service -run TestDefaultUserPermissionIncludesPromptLibrary -count=1`
Expected: FAIL，权限尚未加入。

- [ ] **Step 2：注册菜单和 API 权限**

在 `fullMenuPermissions` 加入：

```go
{ID: "prompts", Label: "提示词库", Path: "/prompts", Icon: "library", Order: 25},
```

在 `apiPermissionCatalog` 加入：

```go
apiPermission("GET", "/api/image-conversations", "查看创作台会话", "创作", true),
apiPermission("POST", "/api/image-conversations", "保存创作台会话", "创作", true),
apiPermission("DELETE", "/api/image-conversations", "删除创作台会话", "创作", true),
apiPermission("GET", "/api/prompts", "查看提示词库", "提示词库", true),
apiPermission("POST", "/api/prompts", "创建/修改提示词", "提示词库", true),
apiPermission("DELETE", "/api/prompts", "删除提示词", "提示词库", true),
apiPermission("PATCH", "/api/images/prompt-metadata", "编辑图片提示词", "图片库", false),
```

默认用户权限同步加入 `/prompts` 和上述 API。

- [ ] **Step 3：实现 HTTP handler**

`internal/httpapi/image_workspace.go` 负责路径分发：

```go
func (a *App) handleImageConversations(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.requireIdentity(w, r, "")
	if !ok {
		return
	}
	parts := splitPath(r.URL.Path)
	if r.URL.Path == "/api/image-conversations" {
		switch r.Method {
		case http.MethodGet:
			util.WriteJSON(w, http.StatusOK, map[string]any{"items": a.workspace.ListConversations(identity)})
		case http.MethodPost:
			body, err := readJSONMap(r)
			if err != nil {
				util.WriteError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			item, err := a.workspace.CreateOrUpdateConversation(identity, body)
			writeWorkspaceResult(w, item, err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "image-conversations" {
		a.handleImageConversationItem(w, r, identity, parts[2:])
		return
	}
	http.NotFound(w, r)
}
```

同文件实现 `handlePrompts`、`handleImagePromptMetadata`、`writeWorkspaceResult`。`handleImageConversationItem` 支持 `GET /api/image-conversations/{id}`、`PATCH /api/image-conversations/{id}`、`DELETE /api/image-conversations/{id}`、`POST /api/image-conversations/{id}/turns`、`PATCH /api/image-conversations/{id}/turns/{turn_id}`、`POST /api/image-conversations/{id}/attachments`。

- [ ] **Step 4：App 初始化和路由注册**

`App` 增加字段：

```go
workspace *service.ImageWorkspaceService
```

`NewApp()` 中初始化：

```go
workspace, err := service.NewImageWorkspaceService(storageBackend, cfg.ConversationAttachmentsDir())
if err != nil {
	cancel()
	return nil, err
}
app := &App{config: cfg, auth: auth, accounts: accounts, logs: logs, logger: logger, proxy: proxy, engine: engine, images: service.NewImageService(cfg, storageBackend), workspace: workspace, announce: service.NewAnnouncementService(cfg.DataDir, storageBackend), cpa: service.NewCPAConfig(cfg.DataDir, storageBackend), sub2: service.NewSub2APIConfig(cfg.DataDir, storageBackend), update: newUpdateService(cfg), cancel: cancel}
```

`routes()` 中加入：

```go
subtree("/api/image-conversations", a.handleImageConversations),
subtree("/api/prompts", a.handlePrompts),
exact("", "/api/images/prompt-metadata", a.handleImagePromptMetadata),
```

- [ ] **Step 5：运行 API 和权限测试**

Run: `go test ./internal/service ./internal/httpapi -run 'Prompt|ImageConversation|Permission' -count=1`
Expected: PASS。

## Task 4：图片提示词元数据和任务关联

**Files:**
- Modify: `internal/service/image.go`
- Modify: `internal/service/image_test.go`
- Modify: `internal/service/image_task.go`
- Modify: `internal/service/image_task_test.go`
- Modify: `internal/httpapi/routes.go`
- Modify: `internal/httpapi/app.go`

- [ ] **Step 1：编写失败测试，任务保留会话字段**

在 `internal/service/image_task_test.go` 新增测试，提交任务后 `publicTask` 返回 `conversation_id` 和 `turn_id`。

```go
func TestImageTaskStoresConversationFields(t *testing.T) {
	service := NewImageTaskService(filepath.Join(t.TempDir(), "tasks.json"), func(context.Context, Identity, map[string]any) (map[string]any, error) {
		return map[string]any{"data": []map[string]any{{"url": "http://example.test/images/a.png"}}}, nil
	}, nil, nil, func() int { return 7 })
	task, err := service.SubmitGeneration(context.Background(), Identity{Role: AuthRoleUser, OwnerID: "user-1"}, "task-1", "提示词", "gpt-image-2", "1024x1024", "high", "http://example.test", 1, nil, "private", "conv-1", "turn-1")
	if err != nil {
		t.Fatalf("SubmitGeneration() error = %v", err)
	}
	if task["conversation_id"] != "conv-1" || task["turn_id"] != "turn-1" {
		t.Fatalf("task = %#v, want conversation fields", task)
	}
}
```

Run: `go test ./internal/service -run TestImageTaskStoresConversationFields -count=1`
Expected: FAIL，当前签名不支持会话字段。

- [ ] **Step 2：扩展任务提交签名**

将 `SubmitGeneration`、`SubmitEdit`、`SubmitChat` 的可选参数替换为结构体，避免继续扩展可变参数：

```go
type ImageTaskSubmitOptions struct {
	Visibility     string
	ConversationID string
	TurnID         string
}
```

任务 payload 和 task map 中写入 `conversation_id`、`turn_id`。`publicTask` 保留这两个字段。

- [ ] **Step 3：写入图片提示词元数据**

扩展 `ImageService`：

```go
type ImagePromptMetadataUpdate struct {
	Path           string
	Prompt         string
	RevisedPrompt  string
	ManualPrompt   string
	Model          string
	Size           string
	Quality        string
	Mode           string
	ConversationID string
	TurnID         string
	TaskID         string
	PromptID       string
}

func (s *ImageService) UpdateImagePromptMetadata(update ImagePromptMetadataUpdate, scope ImageAccessScope) (map[string]any, error)
```

`runLoggedImageTask` 在拿到 `result["data"]` 后，按 URL 解析图片路径，写入原始 prompt、`revised_prompt`、模型、尺寸、质量、模式、`conversation_id`、`turn_id`、`task_id`。如果写入失败，记录 warning 日志，不改变图片生成任务状态。

- [ ] **Step 4：运行任务和图片测试**

Run: `go test ./internal/service -run 'ImageTaskStoresConversationFields|ImagePromptMetadata' -count=1`
Expected: PASS。

## Task 5：前端 API 层和服务端会话 store

**Files:**
- Modify: `web/src/lib/api.ts`
- Create: `web/src/lib/image-workspace-api.ts`
- Modify: `web/src/store/image-conversations.ts`

- [ ] **Step 1：定义前端类型和 API helper**

`web/src/lib/image-workspace-api.ts` 导出：

```ts
import { httpRequest } from "@/lib/request";
import type { ImageModel, ImageQuality, ImageVisibility } from "@/lib/api";

export type WorkspaceReferenceImage = {
  id: string;
  name: string;
  type: string;
  url: string;
  source?: "upload" | "conversation";
};

export type WorkspaceStoredImage = {
  id: string;
  taskId?: string;
  status?: "loading" | "success" | "error" | "cancelled" | "message";
  path?: string;
  visibility?: ImageVisibility;
  b64_json?: string;
  url?: string;
  revised_prompt?: string;
  error?: string;
  text_response?: string;
};

export type WorkspaceTurn = {
  id: string;
  prompt: string;
  model: ImageModel;
  mode: "chat" | "generate" | "image" | "edit";
  referenceImages: WorkspaceReferenceImage[];
  count: number;
  size: string;
  quality?: ImageQuality;
  visibility?: ImageVisibility;
  images: WorkspaceStoredImage[];
  createdAt: string;
  status: "queued" | "generating" | "success" | "error" | "cancelled" | "message";
  error?: string;
};

export type WorkspaceConversation = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  turns: WorkspaceTurn[];
};
```

实现 `fetchImageConversations`、`fetchImageConversation`、`saveImageConversationRemote`、`deleteImageConversationRemote`、`uploadConversationAttachment`、`fetchPrompts`、`savePrompt`、`deletePrompt`、`copyPrompt`、`updateImagePromptMetadata`。

- [ ] **Step 2：改造 store 接口**

保留 `listImageConversations`、`saveImageConversation`、`deleteImageConversation` 等导出函数名，内部调用服务端 API。`clearImageConversations` 调用批量软删除当前服务端列表。`StoredReferenceImage` 改为附件引用，不再保存 `dataUrl`：

```ts
export type StoredReferenceImage = {
  id: string;
  name: string;
  type: string;
  url: string;
  source?: StoredReferenceImageSource;
};
```

`readFileAsDataUrl` 只在上传预览阶段使用，保存会话时使用附件 `id` 和 `url`。

- [ ] **Step 3：运行前端类型检查**

Run: `cd web && npm run build`
Expected: FAIL，创作台仍按 `dataUrl` 使用参考图，下一任务修复。

## Task 6：创作台接入服务端会话和附件

**Files:**
- Modify: `web/src/app/image/page.tsx`
- Modify: `web/src/app/image/components/image-composer.tsx`
- Modify: `web/src/app/image/components/image-sidebar.tsx`

- [ ] **Step 1：上传参考图改为附件**

在 `handleReferenceImageChange` 中先调用 `uploadConversationAttachment`，将返回的 `{id, name, type, url}` 写入 `referenceImages`。提交图生图时根据附件 `url` 拉取 `File`，保持现有 `createImageEditTask` multipart 发送。

- [ ] **Step 2：任务提交传递会话字段**

`createImageGenerationTask`、`createImageEditTask`、`createChatCompletionTask` 调用增加：

```ts
{
  conversationId: activeConversation.id,
  turnId: activeTurn.id,
}
```

任务轮询成功后保存 turn 的 `images`，服务端会话成为刷新恢复来源。

- [ ] **Step 3：增加提示词库入口**

`ImageComposerProps` 增加：

```ts
onOpenPromptLibrary: () => void;
```

组件中在远程提示词市场按钮旁加一个 icon 按钮，使用 `Library` 图标和 tooltip 文案“我的提示词库”。点击后跳转 `/prompts`，不自动提交当前提示词。

- [ ] **Step 4：运行前端构建**

Run: `cd web && npm run build`
Expected: PASS。

## Task 7：图片库提示词关联 UI

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/app/image-manager/page.tsx`

- [ ] **Step 1：扩展 `ManagedImage` 类型**

在 `ManagedImage` 增加：

```ts
prompt?: string;
revised_prompt?: string;
manual_prompt?: string;
model?: string;
quality?: string;
mode?: string;
conversation_id?: string;
turn_id?: string;
task_id?: string;
prompt_id?: string;
```

- [ ] **Step 2：搜索提示词字段**

`matchesManagedImageKeyword` 搜索数组加入：

```ts
item.prompt,
item.revised_prompt,
item.manual_prompt,
item.model,
item.conversation_id,
item.turn_id,
item.task_id,
```

- [ ] **Step 3：详情操作**

在图片详情 popover 中展示原始提示词、改写提示词、手动提示词。新增按钮：复制原始提示词、复制改写提示词、编辑手动提示词、保存到提示词库、打开对应会话。打开会话时写入 `ACTIVE_IMAGE_CONVERSATION_STORAGE_KEY` 并导航到 `/image`，同时派发 `IMAGE_ACTIVE_CONVERSATION_REQUEST_EVENT`。

- [ ] **Step 4：运行前端构建**

Run: `cd web && npm run build`
Expected: PASS。

## Task 8：提示词库页面

**Files:**
- Create: `web/src/app/prompts/page.tsx`
- Modify: `web/src/app/route-config.tsx`

- [ ] **Step 1：新增页面路由**

`route-config.tsx` 导入并注册：

```tsx
import PromptsPage from "@/app/prompts/page";

{ path: "/prompts", element: <PromptsPage />, requiredPath: "/prompts" },
```

- [ ] **Step 2：实现提示词库页面**

页面布局使用 `PageHeader`、搜索框、公开/私有筛选、分类和标签筛选、列表、编辑对话框。按钮使用 lucide 图标：`Plus` 新建、`Copy` 复制公开提示词、`Pencil` 编辑、`Trash2` 删除、`Send` 应用到创作台。

提交表单字段：

```ts
type PromptFormState = {
  title: string;
  body: string;
  tags: string;
  category: string;
  note: string;
  model: string;
  use_case: string;
  visibility: "private" | "public";
};
```

“应用到创作台”把正文写入 `sessionStorage.setItem("chatgpt2api:pending_prompt", body)` 后导航 `/image`，创作台读取后填入输入框并立即清除该 key。

- [ ] **Step 3：运行前端 lint 和 build**

Run: `cd web && npm run lint`
Expected: PASS。

Run: `cd web && npm run build`
Expected: PASS。

## Task 9：全量验证和提交

**Files:**
- Modify: all files above

- [ ] **Step 1：后端全量测试**

Run: `go test ./...`
Expected: PASS。

- [ ] **Step 2：前端验证**

Run: `cd web && npm run lint`
Expected: PASS。

Run: `cd web && npm run build`
Expected: PASS。

- [ ] **Step 3：手动验证**

本地启动后验证：

```powershell
go build -ldflags "-X chatgpt2api/internal/version.Version=dev" -o chatgpt2api.exe ./cmd/chatgpt2api
$env:CHATGPT2API_ADMIN_PASSWORD='change_me_please'
.\chatgpt2api.exe
```

另一个终端：

```powershell
cd web
npm run dev
```

浏览器验证：

- `/image` 新建会话，刷新后仍恢复。
- `/image` 上传参考图，刷新后仍显示附件预览。
- `/image` 提交文生图、图生图、文本对话，任务记录包含 `conversation_id` 和 `turn_id`。
- `/image-manager` 能看到原始提示词、改写提示词、会话入口，能编辑手动提示词。
- `/prompts` 能新建私有提示词、公开提示词，普通用户能复制公开提示词到自己的库。
- 普通用户不能读取其他用户会话，不能编辑其他用户私有提示词。

- [ ] **Step 4：提交**

Run:

```powershell
git status --short
git add internal web docs
git commit -m "feat: persist image workspace conversations"
```

Expected: commit 创建成功，工作区只剩用户无关改动或为空。

## 自查结果

- 规格覆盖：会话持久化、图片提示词关联、提示词库、公开/私有、`conversation_id`、参考图附件、旧 IndexedDB 不迁移均有任务覆盖。
- 类型一致性：后端记录字段使用 snake_case 对外，Go 结构体使用 PascalCase；前端 API 层负责驼峰/蛇形字段映射。
- 路由一致性：创作任务仍使用 `/api/creation-tasks` 下的 `image-generations`、`image-edits`、`chat-completions`，没有新增 image 命名任务别名。
- 验证覆盖：后端存储、服务、HTTP、任务、图片元数据和前端 build/lint 均列入计划。

## 执行选项

计划保存后有两个执行方式：

1. Subagent-Driven（推荐）：每个任务派发一个新 worker，任务之间做审查和集成。
2. Inline Execution：在当前会话按任务顺序执行，阶段性检查。
