package storage

import (
	"database/sql"
	"errors"
	"strings"
)

type ImageWorkspaceStore interface {
	UpsertImageConversation(ImageConversationRecord) error
	ListImageConversations(ownerID string) ([]ImageConversationRecord, error)
	GetImageConversation(ownerID, id string) (*ImageConversationRecord, error)
	SoftDeleteImageConversation(ownerID, id, deletedAt string) error
	UpsertImageConversationTurn(ImageConversationTurnRecord) error
	ListImageConversationTurns(ownerID, conversationID string) ([]ImageConversationTurnRecord, error)
	UpsertConversationAttachment(ConversationAttachmentRecord) error
	ListConversationAttachments(ownerID, conversationID string) ([]ConversationAttachmentRecord, error)
	GetConversationAttachment(ownerID, id string) (*ConversationAttachmentRecord, error)
	DeleteConversationAttachment(ownerID, id string) error
	UpsertPrompt(PromptRecord) error
	ListPrompts(ownerID string, query PromptListQuery) ([]PromptRecord, error)
	GetPrompt(ownerID, id string, includePublic bool) (*PromptRecord, error)
	DeletePrompt(ownerID, id string, isAdmin bool) error
}

type PromptListQuery struct {
	Scope    string
	Query    string
	Category string
	Tag      string
}

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

type ConversationAttachmentRecord struct {
	ID             string
	OwnerID        string
	ConversationID string
	TurnID         string
	FileName       string
	MIMEType       string
	Size           int64
	Path           string
	CreatedAt      string
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

func ImageWorkspaceStoreFromBackend(backend Backend) (ImageWorkspaceStore, bool) {
	store, ok := backend.(*DatabaseBackend)
	return store, ok
}

func (b *DatabaseBackend) UpsertImageConversation(item ImageConversationRecord) error {
	if b.driver == "postgres" {
		_, err := b.db.Exec(
			`INSERT INTO image_conversations (id, owner_id, owner_name, title, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET owner_name = EXCLUDED.owner_name, title = EXCLUDED.title, updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at
WHERE image_conversations.owner_id = EXCLUDED.owner_id`,
			item.ID, item.OwnerID, item.OwnerName, item.Title, item.CreatedAt, item.UpdatedAt, item.DeletedAt,
		)
		return err
	}
	if b.driver == "mysql" {
		_, err := b.db.Exec(
			`INSERT INTO image_conversations (id, owner_id, owner_name, title, created_at, updated_at, deleted_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE owner_name = IF(owner_id = VALUES(owner_id), VALUES(owner_name), owner_name), title = IF(owner_id = VALUES(owner_id), VALUES(title), title), updated_at = IF(owner_id = VALUES(owner_id), VALUES(updated_at), updated_at), deleted_at = IF(owner_id = VALUES(owner_id), VALUES(deleted_at), deleted_at)`,
			item.ID, item.OwnerID, item.OwnerName, item.Title, item.CreatedAt, item.UpdatedAt, item.DeletedAt,
		)
		return err
	}
	_, err := b.db.Exec(
		`INSERT INTO image_conversations (id, owner_id, owner_name, title, created_at, updated_at, deleted_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET owner_name = excluded.owner_name, title = excluded.title, updated_at = excluded.updated_at, deleted_at = excluded.deleted_at
WHERE image_conversations.owner_id = excluded.owner_id`,
		item.ID, item.OwnerID, item.OwnerName, item.Title, item.CreatedAt, item.UpdatedAt, item.DeletedAt,
	)
	return err
}

func (b *DatabaseBackend) ListImageConversations(ownerID string) ([]ImageConversationRecord, error) {
	rows, err := b.db.Query(
		`SELECT id, owner_id, owner_name, title, created_at, updated_at, deleted_at
FROM image_conversations
WHERE owner_id = `+b.placeholder(1)+` AND deleted_at = ''
ORDER BY updated_at DESC`,
		strings.TrimSpace(ownerID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanImageConversationRows(rows)
}

func (b *DatabaseBackend) GetImageConversation(ownerID, id string) (*ImageConversationRecord, error) {
	row := b.db.QueryRow(
		`SELECT id, owner_id, owner_name, title, created_at, updated_at, deleted_at
FROM image_conversations
WHERE owner_id = `+b.placeholder(1)+` AND id = `+b.placeholder(2)+` AND deleted_at = ''`,
		strings.TrimSpace(ownerID), strings.TrimSpace(id),
	)
	item, err := scanImageConversationRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (b *DatabaseBackend) SoftDeleteImageConversation(ownerID, id, deletedAt string) error {
	_, err := b.db.Exec(
		`UPDATE image_conversations SET deleted_at = `+b.placeholder(3)+`, updated_at = `+b.placeholder(3)+`
WHERE owner_id = `+b.placeholder(1)+` AND id = `+b.placeholder(2)+` AND deleted_at = ''`,
		strings.TrimSpace(ownerID), strings.TrimSpace(id), strings.TrimSpace(deletedAt),
	)
	return err
}

func (b *DatabaseBackend) UpsertImageConversationTurn(item ImageConversationTurnRecord) error {
	if b.driver == "postgres" {
		_, err := b.db.Exec(
			`INSERT INTO image_conversation_turns (id, conversation_id, owner_id, prompt, model, mode, count, size, quality, visibility, status, error, reference_images_json, result_images_json, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (id) DO UPDATE SET prompt = EXCLUDED.prompt, model = EXCLUDED.model, mode = EXCLUDED.mode, count = EXCLUDED.count, size = EXCLUDED.size, quality = EXCLUDED.quality, visibility = EXCLUDED.visibility, status = EXCLUDED.status, error = EXCLUDED.error, reference_images_json = EXCLUDED.reference_images_json, result_images_json = EXCLUDED.result_images_json, updated_at = EXCLUDED.updated_at
WHERE image_conversation_turns.owner_id = EXCLUDED.owner_id AND image_conversation_turns.conversation_id = EXCLUDED.conversation_id`,
			imageConversationTurnValues(item)...,
		)
		return err
	}
	if b.driver == "mysql" {
		_, err := b.db.Exec(
			`INSERT INTO image_conversation_turns (id, conversation_id, owner_id, prompt, model, mode, count, size, quality, visibility, status, error, reference_images_json, result_images_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE prompt = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(prompt), prompt), model = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(model), model), mode = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(mode), mode), count = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(count), count), size = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(size), size), quality = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(quality), quality), visibility = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(visibility), visibility), status = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(status), status), error = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(error), error), reference_images_json = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(reference_images_json), reference_images_json), result_images_json = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(result_images_json), result_images_json), updated_at = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(updated_at), updated_at)`,
			imageConversationTurnValues(item)...,
		)
		return err
	}
	_, err := b.db.Exec(
		`INSERT INTO image_conversation_turns (id, conversation_id, owner_id, prompt, model, mode, count, size, quality, visibility, status, error, reference_images_json, result_images_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET prompt = excluded.prompt, model = excluded.model, mode = excluded.mode, count = excluded.count, size = excluded.size, quality = excluded.quality, visibility = excluded.visibility, status = excluded.status, error = excluded.error, reference_images_json = excluded.reference_images_json, result_images_json = excluded.result_images_json, updated_at = excluded.updated_at
WHERE image_conversation_turns.owner_id = excluded.owner_id AND image_conversation_turns.conversation_id = excluded.conversation_id`,
		imageConversationTurnValues(item)...,
	)
	return err
}

func (b *DatabaseBackend) ListImageConversationTurns(ownerID, conversationID string) ([]ImageConversationTurnRecord, error) {
	rows, err := b.db.Query(
		`SELECT id, conversation_id, owner_id, prompt, model, mode, count, size, quality, visibility, status, error, reference_images_json, result_images_json, created_at, updated_at
FROM image_conversation_turns
WHERE owner_id = `+b.placeholder(1)+` AND conversation_id = `+b.placeholder(2)+`
ORDER BY created_at ASC`,
		strings.TrimSpace(ownerID), strings.TrimSpace(conversationID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanImageConversationTurnRows(rows)
}

func (b *DatabaseBackend) UpsertConversationAttachment(item ConversationAttachmentRecord) error {
	if b.driver == "postgres" {
		_, err := b.db.Exec(
			`INSERT INTO conversation_attachments (id, owner_id, conversation_id, turn_id, file_name, mime_type, size, path, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET turn_id = EXCLUDED.turn_id, file_name = EXCLUDED.file_name, mime_type = EXCLUDED.mime_type, size = EXCLUDED.size, path = EXCLUDED.path
WHERE conversation_attachments.owner_id = EXCLUDED.owner_id AND conversation_attachments.conversation_id = EXCLUDED.conversation_id`,
			conversationAttachmentValues(item)...,
		)
		return err
	}
	if b.driver == "mysql" {
		_, err := b.db.Exec(
			`INSERT INTO conversation_attachments (id, owner_id, conversation_id, turn_id, file_name, mime_type, size, path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE turn_id = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(turn_id), turn_id), file_name = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(file_name), file_name), mime_type = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(mime_type), mime_type), size = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(size), size), path = IF(owner_id = VALUES(owner_id) AND conversation_id = VALUES(conversation_id), VALUES(path), path)`,
			conversationAttachmentValues(item)...,
		)
		return err
	}
	_, err := b.db.Exec(
		`INSERT INTO conversation_attachments (id, owner_id, conversation_id, turn_id, file_name, mime_type, size, path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET turn_id = excluded.turn_id, file_name = excluded.file_name, mime_type = excluded.mime_type, size = excluded.size, path = excluded.path
WHERE conversation_attachments.owner_id = excluded.owner_id AND conversation_attachments.conversation_id = excluded.conversation_id`,
		conversationAttachmentValues(item)...,
	)
	return err
}

func (b *DatabaseBackend) ListConversationAttachments(ownerID, conversationID string) ([]ConversationAttachmentRecord, error) {
	rows, err := b.db.Query(
		`SELECT id, owner_id, conversation_id, turn_id, file_name, mime_type, size, path, created_at
FROM conversation_attachments
WHERE owner_id = `+b.placeholder(1)+` AND conversation_id = `+b.placeholder(2)+`
ORDER BY created_at ASC`,
		strings.TrimSpace(ownerID), strings.TrimSpace(conversationID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversationAttachmentRows(rows)
}

func (b *DatabaseBackend) GetConversationAttachment(ownerID, id string) (*ConversationAttachmentRecord, error) {
	row := b.db.QueryRow(
		`SELECT id, owner_id, conversation_id, turn_id, file_name, mime_type, size, path, created_at
FROM conversation_attachments
WHERE owner_id = `+b.placeholder(1)+` AND id = `+b.placeholder(2),
		strings.TrimSpace(ownerID), strings.TrimSpace(id),
	)
	item, err := scanConversationAttachmentRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (b *DatabaseBackend) DeleteConversationAttachment(ownerID, id string) error {
	_, err := b.db.Exec(
		`DELETE FROM conversation_attachments WHERE owner_id = `+b.placeholder(1)+` AND id = `+b.placeholder(2),
		strings.TrimSpace(ownerID), strings.TrimSpace(id),
	)
	return err
}

func (b *DatabaseBackend) UpsertPrompt(item PromptRecord) error {
	if b.driver == "postgres" {
		_, err := b.db.Exec(
			`INSERT INTO prompts (id, owner_id, owner_name, title, body, tags_json, category, note, model, use_case, visibility, source_conversation_id, source_turn_id, source_image_path, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (id) DO UPDATE SET owner_name = EXCLUDED.owner_name, title = EXCLUDED.title, body = EXCLUDED.body, tags_json = EXCLUDED.tags_json, category = EXCLUDED.category, note = EXCLUDED.note, model = EXCLUDED.model, use_case = EXCLUDED.use_case, visibility = EXCLUDED.visibility, source_conversation_id = EXCLUDED.source_conversation_id, source_turn_id = EXCLUDED.source_turn_id, source_image_path = EXCLUDED.source_image_path, updated_at = EXCLUDED.updated_at`,
			promptRecordValues(item)...,
		)
		return err
	}
	if b.driver == "mysql" {
		_, err := b.db.Exec(
			`INSERT INTO prompts (id, owner_id, owner_name, title, body, tags_json, category, note, model, use_case, visibility, source_conversation_id, source_turn_id, source_image_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE owner_name = VALUES(owner_name), title = VALUES(title), body = VALUES(body), tags_json = VALUES(tags_json), category = VALUES(category), note = VALUES(note), model = VALUES(model), use_case = VALUES(use_case), visibility = VALUES(visibility), source_conversation_id = VALUES(source_conversation_id), source_turn_id = VALUES(source_turn_id), source_image_path = VALUES(source_image_path), updated_at = VALUES(updated_at)`,
			promptRecordValues(item)...,
		)
		return err
	}
	_, err := b.db.Exec(
		`INSERT INTO prompts (id, owner_id, owner_name, title, body, tags_json, category, note, model, use_case, visibility, source_conversation_id, source_turn_id, source_image_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET owner_name = excluded.owner_name, title = excluded.title, body = excluded.body, tags_json = excluded.tags_json, category = excluded.category, note = excluded.note, model = excluded.model, use_case = excluded.use_case, visibility = excluded.visibility, source_conversation_id = excluded.source_conversation_id, source_turn_id = excluded.source_turn_id, source_image_path = excluded.source_image_path, updated_at = excluded.updated_at`,
		promptRecordValues(item)...,
	)
	return err
}

func (b *DatabaseBackend) ListPrompts(ownerID string, query PromptListQuery) ([]PromptRecord, error) {
	ownerID = strings.TrimSpace(ownerID)
	scope := strings.TrimSpace(query.Scope)
	search := strings.TrimSpace(query.Query)
	category := strings.TrimSpace(query.Category)
	tag := strings.TrimSpace(query.Tag)
	where := []string{}
	args := []any{}
	nextPlaceholder := 1
	addArg := func(value any) string {
		args = append(args, value)
		placeholder := b.placeholder(nextPlaceholder)
		nextPlaceholder++
		return placeholder
	}
	switch scope {
	case "mine":
		where = append(where, "owner_id = "+addArg(ownerID))
	case "public":
		where = append(where, "visibility = 'public'")
	case "all":
		where = append(where, "1 = 1")
	default:
		where = append(where, "(owner_id = "+addArg(ownerID)+" OR visibility = 'public')")
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		conditions := []string{}
		for _, column := range []string{"title", "body", "tags_json", "category", "note", "model", "use_case"} {
			conditions = append(conditions, "LOWER("+column+") LIKE "+addArg(like))
		}
		where = append(where, "("+strings.Join(conditions, " OR ")+")")
	}
	if category != "" {
		where = append(where, "category = "+addArg(category))
	}
	if tag != "" {
		where = append(where, "LOWER(tags_json) LIKE "+addArg("%\""+strings.ToLower(tag)+"\"%"))
	}
	sqlText := `SELECT id, owner_id, owner_name, title, body, tags_json, category, note, model, use_case, visibility, source_conversation_id, source_turn_id, source_image_path, created_at, updated_at
FROM prompts
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY updated_at DESC`
	rows, err := b.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPromptRows(rows)
}

func (b *DatabaseBackend) GetPrompt(ownerID, id string, includePublic bool) (*PromptRecord, error) {
	ownerID = strings.TrimSpace(ownerID)
	id = strings.TrimSpace(id)
	where := "id = " + b.placeholder(1) + " AND owner_id = " + b.placeholder(2)
	args := []any{id, ownerID}
	if includePublic {
		where = "id = " + b.placeholder(1) + " AND (owner_id = " + b.placeholder(2) + " OR visibility = 'public')"
	}
	row := b.db.QueryRow(
		`SELECT id, owner_id, owner_name, title, body, tags_json, category, note, model, use_case, visibility, source_conversation_id, source_turn_id, source_image_path, created_at, updated_at
FROM prompts
WHERE `+where,
		args...,
	)
	item, err := scanPromptRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (b *DatabaseBackend) DeletePrompt(ownerID, id string, isAdmin bool) error {
	ownerID = strings.TrimSpace(ownerID)
	id = strings.TrimSpace(id)
	where := "id = " + b.placeholder(1) + " AND owner_id = " + b.placeholder(2)
	if isAdmin {
		where = "id = " + b.placeholder(1) + " AND (owner_id = " + b.placeholder(2) + " OR visibility = 'public')"
	}
	_, err := b.db.Exec("DELETE FROM prompts WHERE "+where, id, ownerID)
	return err
}

func imageConversationTurnValues(item ImageConversationTurnRecord) []any {
	return []any{
		item.ID,
		item.ConversationID,
		item.OwnerID,
		item.Prompt,
		item.Model,
		item.Mode,
		item.Count,
		item.Size,
		item.Quality,
		item.Visibility,
		item.Status,
		item.Error,
		item.ReferenceImagesJSON,
		item.ResultImagesJSON,
		item.CreatedAt,
		item.UpdatedAt,
	}
}

func promptRecordValues(item PromptRecord) []any {
	return []any{
		item.ID,
		item.OwnerID,
		item.OwnerName,
		item.Title,
		item.Body,
		item.TagsJSON,
		item.Category,
		item.Note,
		item.Model,
		item.UseCase,
		item.Visibility,
		item.SourceConversationID,
		item.SourceTurnID,
		item.SourceImagePath,
		item.CreatedAt,
		item.UpdatedAt,
	}
}

func conversationAttachmentValues(item ConversationAttachmentRecord) []any {
	return []any{
		item.ID,
		item.OwnerID,
		item.ConversationID,
		item.TurnID,
		item.FileName,
		item.MIMEType,
		item.Size,
		item.Path,
		item.CreatedAt,
	}
}

func scanImageConversationRows(rows *sql.Rows) ([]ImageConversationRecord, error) {
	items := []ImageConversationRecord{}
	for rows.Next() {
		var item ImageConversationRecord
		if err := rows.Scan(&item.ID, &item.OwnerID, &item.OwnerName, &item.Title, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanImageConversationRow(row *sql.Row) (*ImageConversationRecord, error) {
	var item ImageConversationRecord
	if err := row.Scan(&item.ID, &item.OwnerID, &item.OwnerName, &item.Title, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanImageConversationTurnRows(rows *sql.Rows) ([]ImageConversationTurnRecord, error) {
	items := []ImageConversationTurnRecord{}
	for rows.Next() {
		var item ImageConversationTurnRecord
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.OwnerID, &item.Prompt, &item.Model, &item.Mode, &item.Count, &item.Size, &item.Quality, &item.Visibility, &item.Status, &item.Error, &item.ReferenceImagesJSON, &item.ResultImagesJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanConversationAttachmentRows(rows *sql.Rows) ([]ConversationAttachmentRecord, error) {
	items := []ConversationAttachmentRecord{}
	for rows.Next() {
		var item ConversationAttachmentRecord
		if err := rows.Scan(&item.ID, &item.OwnerID, &item.ConversationID, &item.TurnID, &item.FileName, &item.MIMEType, &item.Size, &item.Path, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanConversationAttachmentRow(row *sql.Row) (*ConversationAttachmentRecord, error) {
	var item ConversationAttachmentRecord
	if err := row.Scan(&item.ID, &item.OwnerID, &item.ConversationID, &item.TurnID, &item.FileName, &item.MIMEType, &item.Size, &item.Path, &item.CreatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanPromptRows(rows *sql.Rows) ([]PromptRecord, error) {
	items := []PromptRecord{}
	for rows.Next() {
		var item PromptRecord
		if err := rows.Scan(&item.ID, &item.OwnerID, &item.OwnerName, &item.Title, &item.Body, &item.TagsJSON, &item.Category, &item.Note, &item.Model, &item.UseCase, &item.Visibility, &item.SourceConversationID, &item.SourceTurnID, &item.SourceImagePath, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanPromptRow(row *sql.Row) (*PromptRecord, error) {
	var item PromptRecord
	if err := row.Scan(&item.ID, &item.OwnerID, &item.OwnerName, &item.Title, &item.Body, &item.TagsJSON, &item.Category, &item.Note, &item.Model, &item.UseCase, &item.Visibility, &item.SourceConversationID, &item.SourceTurnID, &item.SourceImagePath, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}
