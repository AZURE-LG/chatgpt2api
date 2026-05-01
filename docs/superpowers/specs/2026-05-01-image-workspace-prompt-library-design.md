# 创作台会话、图片提示词关联与提示词库设计

## 背景

当前创作台会话主要保存在浏览器本地 IndexedDB，按登录身份分隔，但不能跨浏览器或跨设备恢复。图片库通过扫描 `data/images` 返回图片列表，并通过 `image_metadata/<path>.json` 或数据库中的同名 JSON document 记录图片归属、可见性和发布时间。提示词能力目前由静态预设和远程提示词市场组成，没有账号级的个人提示词库。

本设计目标是在不重写现有图片文件管理机制的前提下，增加服务端持久化会话、图片提示词关联和可维护提示词库。采用 B+ 方案：重要业务对象使用结构化表，图片文件和图片元数据继续沿用现有文件/元数据模型。

## 已确认范围

- 创作台会话按账号服务端持久化，用户换浏览器或设备后，登录同一账号可看到自己的会话。
- 不迁移旧浏览器本地 IndexedDB 会话；第一版只保存新版本之后创建或更新的会话。
- 图片库自动关联生成时的原始提示词、上游返回的 `revised_prompt`、模型、尺寸、质量、模式、`conversation_id`、`turn_id`、`task_id`。
- 图片库允许后期编辑手动提示词 `manual_prompt`。
- 图片记录需要能跳回对应创作台会话，最好定位到对应轮次。
- 提示词库为素材管理型，支持标题、正文、标签、分类、备注、适用模型/用途、公开/私有。
- 普通用户可创建私有或公开提示词，可修改自己的提示词；管理员可管理公开提示词。
- 参考图上传到服务端附件目录，会话只保存附件引用，不在会话 JSON 中保存 base64。

## 不纳入第一版

- 旧 IndexedDB 会话自动导入。
- 公开提示词审核流。
- 提示词变量模板、版本历史、评分、收藏、使用次数统计。
- 管理员查看所有用户会话。
- 将图片文件本身搬入数据库。
- 完整替换现有图片元数据系统。

## 总体架构

后端新增三个服务边界：

1. `ImageConversationService`
   - 负责账号级创作台会话、轮次、状态、结果图引用。
   - 底层使用结构化表，按 `owner_id` 做权限隔离。

2. `PromptLibraryService`
   - 负责提示词库 CRUD、公开/私有筛选、复制公开提示词、搜索和筛选。
   - 底层使用结构化表。

3. `ConversationAttachmentService`
   - 负责参考图附件上传、保存、删除和权限校验。
   - 文件放在 `data/conversation_attachments/`，数据库保存引用和元数据。

现有 `ImageService` 保留，扩展图片元数据字段。结果图仍保存在 `data/images/`，缩略图仍由现有缩略图逻辑管理。

前端通过 API 使用这些服务，不依赖底层存储实现。后续如果需要演进到完整结构化表方案，可以替换服务层存储实现并编写迁移脚本，尽量避免重写前端业务流程。

本功能第一版面向数据库后端实现，SQLite 和 PostgreSQL 的字段语义保持一致。JSON 文件后端不作为本功能目标；如果后续仍需要支持，需要单独设计数据迁移和存储策略。

## 数据模型

### `image_conversations`

- `id`
- `owner_id`
- `owner_name`
- `title`
- `created_at`
- `updated_at`
- `deleted_at`

会话删除采用软删除，避免误删仍被任务或结果引用的数据。

### `image_conversation_turns`

- `id`
- `conversation_id`
- `owner_id`
- `prompt`
- `model`
- `mode`
- `count`
- `size`
- `quality`
- `visibility`
- `status`
- `error`
- `reference_images_json`
- `result_images_json`
- `created_at`
- `updated_at`

`reference_images_json` 保存附件引用快照，例如附件 ID、URL、文件名、MIME、大小。`result_images_json` 保存结果图引用快照，例如图片路径、URL、可见性、任务 ID、提示词字段。两者都不保存 base64。

### `conversation_attachments`

- `id`
- `owner_id`
- `conversation_id`
- `turn_id`
- `file_name`
- `mime_type`
- `size`
- `path`
- `created_at`

真实文件路径由服务端生成，文件名仅作展示。附件读取和引用都必须校验 `owner_id`。

### `prompt_library_items`

- `id`
- `owner_id`
- `owner_name`
- `title`
- `prompt`
- `visibility`
- `category`
- `tags_json`
- `models_json`
- `usage_note`
- `source_type`
- `source_image_path`
- `source_conversation_id`
- `source_turn_id`
- `created_at`
- `updated_at`

`visibility` 取值为 `private` 或 `public`。公开提示词对所有登录用户可见；私有提示词只对创建者可见。

### 图片元数据扩展

继续使用现有 `image_metadata/<path>.json` 或数据库中的 JSON document，新增字段：

- `prompt`
- `revised_prompt`
- `manual_prompt`
- `conversation_id`
- `turn_id`
- `task_id`
- `model`
- `size`
- `quality`
- `mode`

图片展示提示词优先级为 `manual_prompt > revised_prompt > prompt`。

## API 设计

### 会话

- `GET /api/image-conversations`
- `POST /api/image-conversations`
- `GET /api/image-conversations/{id}`
- `POST /api/image-conversations/{id}`
- `DELETE /api/image-conversations/{id}`
- `POST /api/image-conversations/{id}/attachments`
- `DELETE /api/image-conversations/{id}/attachments/{attachment_id}`

普通用户只能操作自己的会话和附件。不存在或无权访问的资源返回 404，避免泄露资源存在性。

### 提示词库

- `GET /api/prompt-library?scope=mine|public|all&keyword=&category=&tag=`
- `POST /api/prompt-library`
- `POST /api/prompt-library/{id}`
- `DELETE /api/prompt-library/{id}`
- `POST /api/prompt-library/{id}/copy`

`scope=all` 仅管理员可用。普通用户复制公开提示词时，创建一条属于当前用户的新私有提示词。

### 图片元数据

- `PATCH /api/images/metadata`

用于编辑图片 `manual_prompt`，以及必要时修正 `conversation_id`、`turn_id`、`task_id` 等关联字段。普通用户只能编辑自己图片，管理员可编辑所有图片。

### 创作任务扩展

现有任务提交路径保持不变：

- `POST /api/creation-tasks/image-generations`
- `POST /api/creation-tasks/image-edits`
- `POST /api/creation-tasks/chat-completions`

请求体新增可选字段：

- `conversation_id`
- `turn_id`

后端校验会话和轮次归属。任务完成后，后端把提示词、生成参数、会话 ID、轮次 ID 和任务 ID 写入结果图元数据，并把结果图引用同步到对应轮次。

## 前端设计

### 创作台 `/image`

- 左侧历史会话从服务端加载。
- 新建会话时先创建服务端会话，提交轮次时创建或更新服务端轮次。
- 刷新页面后从服务端恢复会话、轮次、参考图引用、结果图引用和任务状态。
- 上传参考图时先上传为服务端附件，再把附件引用写入轮次。
- 提交任务时传递 `conversation_id` 和 `turn_id`。
- 任务提交后保留即时 UI 状态，继续轮询任务接口更新结果。
- 如果会话保存失败，保留当前输入，不清空创作台。
- 旧本地历史第一版不显示为主数据，可显示简短提示说明旧历史不会自动迁移。

### 图片库 `/image-manager`

- 搜索范围增加提示词字段：`prompt`、`revised_prompt`、`manual_prompt`。
- 图片详情展示原始提示词、改写提示词、手动提示词、模型、尺寸、质量、模式和会话入口。
- 图片操作增加：
  - 复制原始提示词
  - 复制改写提示词
  - 编辑手动提示词
  - 保存到提示词库
  - 打开对应会话
- 没有关联会话的图片不显示会话入口或显示为不可用状态。

### 提示词库 `/prompts`

新增独立页面，作为日常创作工具而不是系统设置。

页面能力：

- 搜索提示词标题、正文、备注。
- 按公开/私有、分类、标签、适用模型/用途筛选。
- 新建提示词。
- 编辑和删除自己的提示词。
- 切换自己的提示词公开/私有状态。
- 复制公开提示词到我的库。
- 应用提示词到创作台输入框，不自动提交。
- 管理员可编辑或删除公开提示词。

创作台保留远程提示词市场入口，同时新增“我的提示词库”入口。生成结果图和会话轮次都提供“保存为提示词”操作。

## 权限规则

- 会话和附件严格按 `owner_id` 隔离。
- 普通用户不能读取、引用、更新、删除其他用户的会话和附件。
- 普通用户可读取所有公开提示词和自己的私有提示词。
- 普通用户只能编辑、删除自己的提示词。
- 管理员可编辑、删除任意公开提示词。
- 普通用户只能编辑自己图片的 `manual_prompt`。
- 管理员可编辑所有图片元数据。
- 任务提交时传入的会话和轮次必须属于当前用户。

## 错误处理

- 参考图上传只允许图片 MIME。
- 单个参考图默认限制为 10MB。
- 单轮参考图默认限制为 10 张。
- 上传文件名只作展示，真实路径由服务端生成。
- 任务创建成功但会话更新失败时，任务仍保留，前端提示会话保存失败。
- 图片生成成功但图片元数据写入失败时，记录日志并返回或展示警告，不把图片生成判定为失败。
- 提示词标题和正文必填。
- 标签、分类、备注和用途做长度限制。
- 删除公开提示词需要二次确认。

## 测试计划

后端测试：

- 会话 CRUD、按 `owner_id` 隔离、软删除。
- 轮次创建、更新、结果图引用和附件引用校验。
- 附件上传类型限制、大小限制、归属校验。
- 提示词 CRUD、公开/私有筛选、公开提示词复制、管理员管理公开提示词。
- 任务提交携带 `conversation_id`、`turn_id` 时写入任务和图片元数据。
- 图片元数据手动提示词编辑权限。

前端验证：

- `cd web && npm run build`
- `cd web && npm run lint`
- 手动检查创作台刷新恢复、参考图恢复、图片库提示词展示和编辑、提示词库筛选和应用。

回归验证：

- `go test ./...`
- 保持现有 `/api/creation-tasks` 路由形态，不增加 image 命名别名，不破坏现有提交路径。

## 迁移与后续演进

第一版不迁移浏览器本地 IndexedDB 会话。后续可新增“导入本地历史”能力，把本地会话转换为服务端会话、附件和图片关联。

B+ 方案后续可以演进到完整结构化表方案。演进方向是将图片提示词关联从图片元数据 JSON 拆到专门表，并提供迁移脚本。由于第一版已保留稳定 ID，包括 `conversation_id`、`turn_id`、`task_id`、`image_path`、`prompt_id`，后续迁移可以基于这些字段建立索引和关系。
