package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"chatgpt2api/internal/service"
	"chatgpt2api/internal/util"
)

func (a *App) handleImageConversations(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.requireIdentity(w, r, "")
	if !ok {
		return
	}
	if !a.requireWorkspace(w) {
		return
	}
	parts := splitPath(r.URL.Path)
	if r.URL.Path == "/api/image-conversations" {
		switch r.Method {
		case http.MethodGet:
			items, err := a.workspace.ListConversations(identity)
			writeWorkspaceResult(w, map[string]any{"items": items}, err)
		case http.MethodPost:
			var input service.ImageConversationInput
			if err := decodeJSONBody(r, &input); err != nil {
				util.WriteError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			item, err := a.workspace.SaveConversation(identity, input)
			writeWorkspaceResult(w, map[string]any{"item": item}, err)
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

func (a *App) handleImageConversationItem(w http.ResponseWriter, r *http.Request, identity service.Identity, parts []string) {
	conversationID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, err := a.workspace.GetConversation(identity, conversationID)
			writeWorkspaceResult(w, map[string]any{"item": item}, err)
		case http.MethodPatch:
			var input service.ImageConversationInput
			if err := decodeJSONBody(r, &input); err != nil {
				util.WriteError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			input.ID = conversationID
			item, err := a.workspace.SaveConversation(identity, input)
			writeWorkspaceResult(w, map[string]any{"item": item}, err)
		case http.MethodDelete:
			writeWorkspaceResult(w, map[string]any{"ok": true}, a.workspace.DeleteConversation(identity, conversationID))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "turns" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input service.ImageTurnInput
		if err := decodeJSONBody(r, &input); err != nil {
			util.WriteError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := a.workspace.SaveTurn(identity, conversationID, input)
		writeWorkspaceResult(w, map[string]any{"item": item}, err)
		return
	}
	if len(parts) == 3 && parts[1] == "turns" {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var input service.ImageTurnInput
		if err := decodeJSONBody(r, &input); err != nil {
			util.WriteError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		input.ID = parts[2]
		item, err := a.workspace.SaveTurn(identity, conversationID, input)
		writeWorkspaceResult(w, map[string]any{"item": item}, err)
		return
	}
	if len(parts) == 2 && parts[1] == "attachments" {
		a.handleConversationAttachmentUpload(w, r, identity, conversationID)
		return
	}
	if len(parts) == 3 && parts[1] == "attachments" {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeWorkspaceResult(w, map[string]any{"ok": true}, a.workspace.DeleteAttachment(identity, parts[2]))
		return
	}
	http.NotFound(w, r)
}

func (a *App) handleConversationAttachmentUpload(w http.ResponseWriter, r *http.Request, identity service.Identity, conversationID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(maxLoginPageImageSize); err != nil {
		util.WriteError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	header := firstMultipartFile(r.MultipartForm, "file")
	if header == nil {
		util.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	file, err := header.Open()
	if err != nil {
		util.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	item, err := a.workspace.SaveAttachment(identity, conversationID, header.Filename, contentType, header.Size, file)
	writeWorkspaceResult(w, map[string]any{"item": item}, err)
}

func (a *App) handlePrompts(w http.ResponseWriter, r *http.Request) {
	identity, ok := a.requireIdentity(w, r, "")
	if !ok {
		return
	}
	if !a.requireWorkspace(w) {
		return
	}
	parts := splitPath(r.URL.Path)
	if r.URL.Path == "/api/prompts" {
		switch r.Method {
		case http.MethodGet:
			items, err := a.workspace.ListPrompts(identity, service.PromptQuery{
				Scope:    r.URL.Query().Get("scope"),
				Query:    r.URL.Query().Get("q"),
				Category: r.URL.Query().Get("category"),
				Tag:      r.URL.Query().Get("tag"),
			})
			writeWorkspaceResult(w, map[string]any{"items": items}, err)
		case http.MethodPost:
			var input service.PromptInput
			if err := decodeJSONBody(r, &input); err != nil {
				util.WriteError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			item, err := a.workspace.CreatePrompt(identity, input)
			writeWorkspaceResult(w, map[string]any{"item": item}, err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "prompts" {
		http.NotFound(w, r)
		return
	}
	promptID := parts[2]
	if len(parts) == 4 && parts[3] == "copy" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := a.workspace.CopyPrompt(identity, promptID)
		writeWorkspaceResult(w, map[string]any{"item": item}, err)
		return
	}
	if len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := a.workspace.GetPrompt(identity, promptID)
		writeWorkspaceResult(w, map[string]any{"item": item}, err)
	case http.MethodPatch:
		var input service.PromptInput
		if err := decodeJSONBody(r, &input); err != nil {
			util.WriteError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		input.ID = promptID
		item, err := a.workspace.SavePrompt(identity, input)
		writeWorkspaceResult(w, map[string]any{"item": item}, err)
	case http.MethodDelete:
		writeWorkspaceResult(w, map[string]any{"ok": true}, a.workspace.DeletePrompt(identity, promptID))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleImagePromptMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	identity, ok := a.requireIdentity(w, r, "")
	if !ok {
		return
	}
	body, err := readJSONMap(r)
	if err != nil {
		util.WriteError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	path := util.Clean(body["path"])
	if path == "" {
		util.WriteError(w, http.StatusBadRequest, "path is required")
		return
	}
	_, manualPromptSet := body["manual_prompt"]
	conversationID := util.Clean(body["conversation_id"])
	turnID := util.Clean(body["turn_id"])
	promptID := util.Clean(body["prompt_id"])
	if conversationID != "" || turnID != "" || promptID != "" {
		if !a.requireWorkspace(w) {
			return
		}
	}
	if conversationID != "" || turnID != "" {
		if err := a.workspace.ValidateConversationTurn(identity, conversationID, turnID); err != nil {
			util.WriteError(w, http.StatusNotFound, "conversation not found")
			return
		}
	}
	if promptID != "" {
		if _, err := a.workspace.GetPrompt(identity, promptID); err != nil {
			util.WriteError(w, http.StatusNotFound, "prompt not found")
			return
		}
	}
	scope := service.ImageAccessScope{OwnerID: identityScope(identity)}
	if identity.Role == service.AuthRoleAdmin {
		scope = service.ImageAccessScope{All: true}
	}
	item, err := a.images.UpdateImagePromptMetadata(service.ImagePromptMetadataUpdate{
		Path:            path,
		ManualPrompt:    util.Clean(body["manual_prompt"]),
		ManualPromptSet: manualPromptSet,
		ConversationID:  conversationID,
		TurnID:          turnID,
		TaskID:          util.Clean(body["task_id"]),
		PromptID:        promptID,
	}, scope)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		util.WriteError(w, status, err.Error())
		return
	}
	a.decorateImageItem(item, a.imageOwnerDisplayNames())
	util.WriteJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *App) handleConversationAttachment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	identity, ok := a.requireIdentity(w, r, "")
	if !ok {
		return
	}
	if !a.requireWorkspace(w) {
		return
	}
	raw := strings.TrimPrefix(r.URL.EscapedPath(), "/conversation-attachments/")
	rel, err := url.PathUnescape(raw)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if rel == "." || rel == "" {
		http.NotFound(w, r)
		return
	}
	if _, err := a.workspace.AttachmentByPath(identity, rel); err != nil {
		http.NotFound(w, r)
		return
	}
	root, err := filepath.Abs(a.config.ConversationAttachmentsDir())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	full, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || !httpPathInsideRoot(root, full) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
}

func (a *App) requireWorkspace(w http.ResponseWriter) bool {
	if a.workspace != nil {
		return true
	}
	util.WriteError(w, http.StatusBadRequest, "image workspace requires database storage backend")
	return false
}

func decodeJSONBody(r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeWorkspaceResult(w http.ResponseWriter, payload map[string]any, err error) {
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		util.WriteError(w, status, err.Error())
		return
	}
	util.WriteJSON(w, http.StatusOK, payload)
}

func httpPathInsideRoot(root, value string) bool {
	rel, err := filepath.Rel(root, value)
	if err != nil {
		return false
	}
	return rel == "." || (rel != "" && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
