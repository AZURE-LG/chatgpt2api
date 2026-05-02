package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"chatgpt2api/internal/storage"
	"chatgpt2api/internal/util"
)

const (
	PromptVisibilityPrivate = ImageVisibilityPrivate
	PromptVisibilityPublic  = ImageVisibilityPublic

	maxConversationAttachmentSize = 10 << 20
)

type ImageWorkspaceService struct {
	store         storage.ImageWorkspaceStore
	attachmentDir string
}

type ImageConversationInput struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Turns     []ImageTurnInput `json:"turns"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

type ImageTurnInput struct {
	ID              string                       `json:"id"`
	Prompt          string                       `json:"prompt"`
	Model           string                       `json:"model"`
	Mode            string                       `json:"mode"`
	ReferenceImages []ConversationReferenceImage `json:"reference_images"`
	Images          []ConversationResultImage    `json:"images"`
	Count           int                          `json:"count"`
	Size            string                       `json:"size"`
	Quality         string                       `json:"quality"`
	Visibility      string                       `json:"visibility"`
	Status          string                       `json:"status"`
	Error           string                       `json:"error"`
	CreatedAt       string                       `json:"created_at"`
	UpdatedAt       string                       `json:"updated_at"`
}

type ImageConversationItem struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Turns     []ImageTurnItem `json:"turns,omitempty"`
}

type ImageTurnItem struct {
	ID              string                       `json:"id"`
	Prompt          string                       `json:"prompt"`
	Model           string                       `json:"model"`
	Mode            string                       `json:"mode"`
	ReferenceImages []ConversationReferenceImage `json:"reference_images"`
	Images          []ConversationResultImage    `json:"images"`
	Count           int                          `json:"count"`
	Size            string                       `json:"size"`
	Quality         string                       `json:"quality,omitempty"`
	Visibility      string                       `json:"visibility,omitempty"`
	Status          string                       `json:"status"`
	Error           string                       `json:"error,omitempty"`
	CreatedAt       string                       `json:"created_at"`
	UpdatedAt       string                       `json:"updated_at"`
}

type ConversationReferenceImage struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Source string `json:"source,omitempty"`
}

type ConversationResultImage struct {
	ID            string `json:"id"`
	TaskID        string `json:"task_id,omitempty"`
	Status        string `json:"status,omitempty"`
	Path          string `json:"path,omitempty"`
	Visibility    string `json:"visibility,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Error         string `json:"error,omitempty"`
	TextResponse  string `json:"text_response,omitempty"`
}

type PromptInput struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Body                 string   `json:"body"`
	Tags                 []string `json:"tags"`
	Category             string   `json:"category"`
	Note                 string   `json:"note"`
	Model                string   `json:"model"`
	UseCase              string   `json:"use_case"`
	Visibility           string   `json:"visibility"`
	SourceConversationID string   `json:"source_conversation_id"`
	SourceTurnID         string   `json:"source_turn_id"`
	SourceImagePath      string   `json:"source_image_path"`
}

type PromptQuery struct {
	Scope    string
	Query    string
	Category string
	Tag      string
}

type PromptItem struct {
	ID                   string   `json:"id"`
	OwnerID              string   `json:"owner_id"`
	OwnerName            string   `json:"owner_name"`
	Title                string   `json:"title"`
	Body                 string   `json:"body"`
	Tags                 []string `json:"tags"`
	Category             string   `json:"category"`
	Note                 string   `json:"note"`
	Model                string   `json:"model"`
	UseCase              string   `json:"use_case"`
	Visibility           string   `json:"visibility"`
	SourceConversationID string   `json:"source_conversation_id,omitempty"`
	SourceTurnID         string   `json:"source_turn_id,omitempty"`
	SourceImagePath      string   `json:"source_image_path,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type ConversationAttachmentItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
	Path string `json:"path"`
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

func (s *ImageWorkspaceService) CreateConversation(identity Identity, input ImageConversationInput) (*ImageConversationItem, error) {
	input.ID = ""
	return s.SaveConversation(identity, input)
}

func (s *ImageWorkspaceService) SaveConversation(identity Identity, input ImageConversationInput) (*ImageConversationItem, error) {
	owner := ownerID(identity)
	now := util.NowLocal()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = util.NewUUID()
	}
	createdAt := firstNonEmpty(strings.TrimSpace(input.CreatedAt), now)
	updatedAt := firstNonEmpty(strings.TrimSpace(input.UpdatedAt), now)
	title := strings.TrimSpace(input.Title)
	if title == "" && len(input.Turns) > 0 {
		title = buildWorkspaceConversationTitle(input.Turns[len(input.Turns)-1].Prompt)
	}
	if title == "" {
		title = "未命名会话"
	}
	if current, err := s.store.GetImageConversation(owner, id); err != nil {
		return nil, err
	} else if current != nil {
		createdAt = current.CreatedAt
	}
	record := storage.ImageConversationRecord{
		ID:        id,
		OwnerID:   owner,
		OwnerName: workspaceIdentityName(identity),
		Title:     title,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if err := s.store.UpsertImageConversation(record); err != nil {
		return nil, err
	}
	for _, turn := range input.Turns {
		if err := s.saveTurnRecord(owner, id, turn, now); err != nil {
			return nil, err
		}
	}
	return s.GetConversation(identity, id)
}

func (s *ImageWorkspaceService) ListConversations(identity Identity) ([]ImageConversationItem, error) {
	owner := ownerID(identity)
	records, err := s.store.ListImageConversations(owner)
	if err != nil {
		return nil, err
	}
	items := make([]ImageConversationItem, 0, len(records))
	for _, record := range records {
		item, err := s.conversationItem(owner, record, false)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *ImageWorkspaceService) GetConversation(identity Identity, id string) (*ImageConversationItem, error) {
	owner := ownerID(identity)
	record, err := s.store.GetImageConversation(owner, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("conversation not found")
	}
	return s.conversationItem(owner, *record, true)
}

func (s *ImageWorkspaceService) DeleteConversation(identity Identity, id string) error {
	owner := ownerID(identity)
	id = strings.TrimSpace(id)
	current, err := s.store.GetImageConversation(owner, id)
	if err != nil {
		return err
	}
	if current == nil {
		return errors.New("conversation not found")
	}
	return s.store.SoftDeleteImageConversation(owner, id, util.NowLocal())
}

func (s *ImageWorkspaceService) SaveTurn(identity Identity, conversationID string, input ImageTurnInput) (*ImageConversationItem, error) {
	owner := ownerID(identity)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("conversation_id is required")
	}
	if current, err := s.store.GetImageConversation(owner, conversationID); err != nil {
		return nil, err
	} else if current == nil {
		return nil, errors.New("conversation not found")
	}
	now := util.NowLocal()
	if err := s.saveTurnRecord(owner, conversationID, input, now); err != nil {
		return nil, err
	}
	record, err := s.store.GetImageConversation(owner, conversationID)
	if err != nil {
		return nil, err
	}
	if record != nil {
		record.UpdatedAt = now
		if title := buildWorkspaceConversationTitle(input.Prompt); title != "" {
			record.Title = title
		}
		if err := s.store.UpsertImageConversation(*record); err != nil {
			return nil, err
		}
	}
	return s.GetConversation(identity, conversationID)
}

func (s *ImageWorkspaceService) CreatePrompt(identity Identity, input PromptInput) (*PromptItem, error) {
	input.ID = ""
	return s.SavePrompt(identity, input)
}

func (s *ImageWorkspaceService) SavePrompt(identity Identity, input PromptInput) (*PromptItem, error) {
	owner := ownerID(identity)
	now := util.NowLocal()
	id := strings.TrimSpace(input.ID)
	createdAt := now
	if id == "" {
		id = util.NewUUID()
	} else {
		current, err := s.store.GetPrompt(owner, id, identity.Role == AuthRoleAdmin)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, errors.New("prompt not found")
		}
		if current.OwnerID != owner && !(identity.Role == AuthRoleAdmin && current.Visibility == PromptVisibilityPublic) {
			return nil, errors.New("prompt not found")
		}
		createdAt = current.CreatedAt
		if current.OwnerID != owner {
			owner = current.OwnerID
		}
	}
	title := strings.TrimSpace(input.Title)
	body := strings.TrimSpace(input.Body)
	if title == "" {
		return nil, errors.New("title is required")
	}
	if body == "" {
		return nil, errors.New("body is required")
	}
	visibility, err := normalizePromptVisibility(input.Visibility)
	if err != nil {
		return nil, err
	}
	tagsJSON, err := json.Marshal(normalizePromptTags(input.Tags))
	if err != nil {
		return nil, err
	}
	record := storage.PromptRecord{
		ID:                   id,
		OwnerID:              owner,
		OwnerName:            workspaceIdentityName(identity),
		Title:                title,
		Body:                 body,
		TagsJSON:             string(tagsJSON),
		Category:             strings.TrimSpace(input.Category),
		Note:                 strings.TrimSpace(input.Note),
		Model:                strings.TrimSpace(input.Model),
		UseCase:              strings.TrimSpace(input.UseCase),
		Visibility:           visibility,
		SourceConversationID: strings.TrimSpace(input.SourceConversationID),
		SourceTurnID:         strings.TrimSpace(input.SourceTurnID),
		SourceImagePath:      strings.TrimSpace(input.SourceImagePath),
		CreatedAt:            createdAt,
		UpdatedAt:            now,
	}
	if err := s.store.UpsertPrompt(record); err != nil {
		return nil, err
	}
	return promptItem(record), nil
}

func (s *ImageWorkspaceService) ListPrompts(identity Identity, query PromptQuery) ([]PromptItem, error) {
	scope := strings.TrimSpace(query.Scope)
	if scope == "" {
		scope = "visible"
	}
	if scope != "visible" && scope != "all" && scope != "mine" && scope != "public" {
		return nil, errors.New("scope must be visible, all, mine, or public")
	}
	if scope == "all" && identity.Role != AuthRoleAdmin {
		return nil, errors.New("admin permission required")
	}
	records, err := s.store.ListPrompts(ownerID(identity), storage.PromptListQuery{
		Scope:    scope,
		Query:    query.Query,
		Category: query.Category,
		Tag:      query.Tag,
	})
	if err != nil {
		return nil, err
	}
	items := make([]PromptItem, 0, len(records))
	for _, record := range records {
		items = append(items, *promptItem(record))
	}
	return items, nil
}

func (s *ImageWorkspaceService) GetPrompt(identity Identity, id string) (*PromptItem, error) {
	record, err := s.store.GetPrompt(ownerID(identity), strings.TrimSpace(id), true)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("prompt not found")
	}
	return promptItem(*record), nil
}

func (s *ImageWorkspaceService) DeletePrompt(identity Identity, id string) error {
	return s.store.DeletePrompt(ownerID(identity), strings.TrimSpace(id), identity.Role == AuthRoleAdmin)
}

func (s *ImageWorkspaceService) CopyPrompt(identity Identity, id string) (*PromptItem, error) {
	source, err := s.store.GetPrompt(ownerID(identity), strings.TrimSpace(id), true)
	if err != nil {
		return nil, err
	}
	if source == nil || (source.OwnerID != ownerID(identity) && source.Visibility != PromptVisibilityPublic) {
		return nil, errors.New("prompt not found")
	}
	input := PromptInput{
		Title:                source.Title,
		Body:                 source.Body,
		Tags:                 decodePromptTags(source.TagsJSON),
		Category:             source.Category,
		Note:                 source.Note,
		Model:                source.Model,
		UseCase:              source.UseCase,
		Visibility:           PromptVisibilityPrivate,
		SourceConversationID: source.SourceConversationID,
		SourceTurnID:         source.SourceTurnID,
		SourceImagePath:      source.SourceImagePath,
	}
	return s.CreatePrompt(identity, input)
}

func (s *ImageWorkspaceService) SaveAttachment(identity Identity, conversationID, fileName, contentType string, size int64, reader io.Reader) (*ConversationAttachmentItem, error) {
	if size > maxConversationAttachmentSize {
		return nil, fmt.Errorf("reference image is too large")
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		return nil, fmt.Errorf("reference image must be an image")
	}
	owner := ownerID(identity)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("conversation_id is required")
	}
	current, err := s.store.GetImageConversation(owner, conversationID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, errors.New("conversation not found")
	}
	id := util.NewUUID()
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" {
		ext = ".png"
	}
	rel := filepath.ToSlash(filepath.Join(owner, strings.TrimSpace(conversationID), id+ext))
	full := filepath.Join(s.attachmentDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(full)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	limited := io.LimitReader(reader, maxConversationAttachmentSize+1)
	written, err := io.Copy(file, limited)
	if err != nil {
		return nil, err
	}
	if written > maxConversationAttachmentSize {
		_ = os.Remove(full)
		return nil, fmt.Errorf("reference image is too large")
	}
	item := storage.ConversationAttachmentRecord{
		ID:             id,
		OwnerID:        owner,
		ConversationID: conversationID,
		FileName:       filepath.Base(fileName),
		MIMEType:       contentType,
		Size:           written,
		Path:           rel,
		CreatedAt:      util.NowLocal(),
	}
	if err := s.store.UpsertConversationAttachment(item); err != nil {
		_ = os.Remove(full)
		return nil, err
	}
	return conversationAttachmentItem(item), nil
}

func (s *ImageWorkspaceService) GetAttachment(identity Identity, id string) (*ConversationAttachmentItem, error) {
	record, err := s.store.GetConversationAttachment(ownerID(identity), strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("attachment not found")
	}
	return conversationAttachmentItem(*record), nil
}

func (s *ImageWorkspaceService) DeleteAttachment(identity Identity, id string) error {
	owner := ownerID(identity)
	record, err := s.store.GetConversationAttachment(owner, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if record == nil {
		return errors.New("attachment not found")
	}
	if err := s.store.DeleteConversationAttachment(owner, record.ID); err != nil {
		return err
	}
	full := filepath.Join(s.attachmentDir, filepath.FromSlash(record.Path))
	root, rootErr := filepath.Abs(s.attachmentDir)
	resolved, pathErr := filepath.Abs(full)
	if rootErr == nil && pathErr == nil && workspacePathInsideRoot(root, resolved) {
		_ = os.Remove(resolved)
	}
	return nil
}

func (s *ImageWorkspaceService) AttachmentByPath(identity Identity, rel string) (*ConversationAttachmentItem, error) {
	rel = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(rel)), "/")
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return nil, errors.New("attachment not found")
	}
	owner := parts[0]
	if identity.Role != AuthRoleAdmin && owner != ownerID(identity) {
		return nil, errors.New("attachment not found")
	}
	items, err := s.store.ListConversationAttachments(owner, parts[1])
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Path == rel {
			return conversationAttachmentItem(item), nil
		}
	}
	return nil, errors.New("attachment not found")
}

func (s *ImageWorkspaceService) ValidateConversationTurn(identity Identity, conversationID, turnID string) error {
	owner := ownerID(identity)
	conversationID = strings.TrimSpace(conversationID)
	turnID = strings.TrimSpace(turnID)
	if conversationID == "" && turnID == "" {
		return nil
	}
	if conversationID == "" {
		return errors.New("conversation not found")
	}
	conversation, err := s.store.GetImageConversation(owner, conversationID)
	if err != nil {
		return err
	}
	if conversation == nil {
		return errors.New("conversation not found")
	}
	if turnID == "" {
		return nil
	}
	turns, err := s.store.ListImageConversationTurns(owner, conversationID)
	if err != nil {
		return err
	}
	for _, turn := range turns {
		if turn.ID == turnID {
			return nil
		}
	}
	return errors.New("conversation not found")
}

func conversationAttachmentItem(record storage.ConversationAttachmentRecord) *ConversationAttachmentItem {
	return &ConversationAttachmentItem{
		ID:   record.ID,
		Name: record.FileName,
		Type: record.MIMEType,
		Size: record.Size,
		URL:  "/conversation-attachments/" + record.Path,
		Path: record.Path,
	}
}

func workspacePathInsideRoot(root, value string) bool {
	rel, err := filepath.Rel(root, value)
	if err != nil {
		return false
	}
	return rel == "." || (rel != "" && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func (s *ImageWorkspaceService) saveTurnRecord(owner, conversationID string, input ImageTurnInput, now string) error {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = util.NewUUID()
	}
	referenceImages, err := json.Marshal(input.ReferenceImages)
	if err != nil {
		return err
	}
	resultImages, err := json.Marshal(input.Images)
	if err != nil {
		return err
	}
	createdAt := firstNonEmpty(strings.TrimSpace(input.CreatedAt), now)
	updatedAt := firstNonEmpty(strings.TrimSpace(input.UpdatedAt), now)
	visibility := input.Visibility
	if visibility == "" {
		visibility = ImageVisibilityPrivate
	}
	visibility, err = NormalizeImageVisibility(visibility)
	if err != nil {
		return err
	}
	record := storage.ImageConversationTurnRecord{
		ID:                  id,
		ConversationID:      conversationID,
		OwnerID:             owner,
		Prompt:              strings.TrimSpace(input.Prompt),
		Model:               strings.TrimSpace(input.Model),
		Mode:                normalizeConversationMode(input.Mode),
		Count:               normalizedWorkspaceCount(input.Count),
		Size:                strings.TrimSpace(input.Size),
		Quality:             strings.TrimSpace(input.Quality),
		Visibility:          visibility,
		Status:              normalizeTurnStatus(input.Status),
		Error:               strings.TrimSpace(input.Error),
		ReferenceImagesJSON: string(referenceImages),
		ResultImagesJSON:    string(resultImages),
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}
	return s.store.UpsertImageConversationTurn(record)
}

func (s *ImageWorkspaceService) conversationItem(owner string, record storage.ImageConversationRecord, includeTurns bool) (*ImageConversationItem, error) {
	item := &ImageConversationItem{
		ID:        record.ID,
		Title:     record.Title,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
	if !includeTurns {
		return item, nil
	}
	turns, err := s.store.ListImageConversationTurns(owner, record.ID)
	if err != nil {
		return nil, err
	}
	item.Turns = make([]ImageTurnItem, 0, len(turns))
	for _, turn := range turns {
		item.Turns = append(item.Turns, imageTurnItem(turn))
	}
	return item, nil
}

func imageTurnItem(record storage.ImageConversationTurnRecord) ImageTurnItem {
	return ImageTurnItem{
		ID:              record.ID,
		Prompt:          record.Prompt,
		Model:           record.Model,
		Mode:            record.Mode,
		ReferenceImages: decodeReferenceImages(record.ReferenceImagesJSON),
		Images:          decodeResultImages(record.ResultImagesJSON),
		Count:           record.Count,
		Size:            record.Size,
		Quality:         record.Quality,
		Visibility:      record.Visibility,
		Status:          record.Status,
		Error:           record.Error,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func promptItem(record storage.PromptRecord) *PromptItem {
	return &PromptItem{
		ID:                   record.ID,
		OwnerID:              record.OwnerID,
		OwnerName:            record.OwnerName,
		Title:                record.Title,
		Body:                 record.Body,
		Tags:                 decodePromptTags(record.TagsJSON),
		Category:             record.Category,
		Note:                 record.Note,
		Model:                record.Model,
		UseCase:              record.UseCase,
		Visibility:           record.Visibility,
		SourceConversationID: record.SourceConversationID,
		SourceTurnID:         record.SourceTurnID,
		SourceImagePath:      record.SourceImagePath,
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
	}
}

func decodeReferenceImages(value string) []ConversationReferenceImage {
	items := []ConversationReferenceImage{}
	_ = json.Unmarshal([]byte(value), &items)
	if items == nil {
		return []ConversationReferenceImage{}
	}
	return items
}

func decodeResultImages(value string) []ConversationResultImage {
	items := []ConversationResultImage{}
	_ = json.Unmarshal([]byte(value), &items)
	if items == nil {
		return []ConversationResultImage{}
	}
	return items
}

func normalizePromptVisibility(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = PromptVisibilityPrivate
	}
	if value != PromptVisibilityPrivate && value != PromptVisibilityPublic {
		return "", errors.New("visibility must be private or public")
	}
	return value, nil
}

func normalizePromptTags(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func decodePromptTags(value string) []string {
	var tags []string
	_ = json.Unmarshal([]byte(value), &tags)
	if tags == nil {
		return []string{}
	}
	return tags
}

func workspaceIdentityName(identity Identity) string {
	if identity.Role == AuthRoleAdmin {
		return firstNonEmpty(strings.TrimSpace(identity.Name), strings.TrimSpace(identity.CredentialName), "管理员")
	}
	return firstNonEmpty(strings.TrimSpace(identity.Name), strings.TrimSpace(identity.CredentialName), strings.TrimSpace(identity.OwnerID), "用户")
}

func buildWorkspaceConversationTitle(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	if len([]rune(prompt)) <= 12 {
		return prompt
	}
	runes := []rune(prompt)
	return string(runes[:12]) + "..."
}

func normalizeConversationMode(value string) string {
	switch strings.TrimSpace(value) {
	case "chat", "generate", "image", "edit":
		return strings.TrimSpace(value)
	default:
		return "generate"
	}
}

func normalizeTurnStatus(value string) string {
	switch strings.TrimSpace(value) {
	case "queued", "generating", "success", "error", "cancelled", "message":
		return strings.TrimSpace(value)
	default:
		return "success"
	}
}

func normalizedWorkspaceCount(value int) int {
	if value < 1 {
		return 1
	}
	if value > 10 {
		return 10
	}
	return value
}
