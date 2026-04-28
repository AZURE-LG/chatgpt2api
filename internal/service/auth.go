package service

import (
	"crypto/hmac"
	"sync"
	"time"

	"chatgpt2api/internal/storage"
	"chatgpt2api/internal/util"
)

type Identity struct {
	ID   string
	Name string
	Role string
}

func (i Identity) Map() map[string]any {
	return map[string]any{"id": i.ID, "name": i.Name, "role": i.Role}
}

type AuthService struct {
	mu              sync.Mutex
	storage         storage.Backend
	items           []map[string]any
	lastUsedFlushAt map[string]time.Time
}

func NewAuthService(backend storage.Backend) *AuthService {
	s := &AuthService{storage: backend, lastUsedFlushAt: map[string]time.Time{}}
	s.items = s.load()
	return s
}

func (s *AuthService) ListKeys(role string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.items))
	for _, item := range s.items {
		if role == "" || item["role"] == role {
			out = append(out, publicAuthItem(item))
		}
	}
	return out
}

func (s *AuthService) CreateKey(role, name string) (map[string]any, string, error) {
	if role != "admin" && role != "user" {
		role = "user"
	}
	name = util.Clean(name)
	if name == "" {
		if role == "admin" {
			name = "管理员密钥"
		} else {
			name = "普通用户"
		}
	}
	raw := "sk-" + util.RandomTokenURL(24)
	item := map[string]any{
		"id":           util.NewHex(12),
		"name":         name,
		"role":         role,
		"key":          raw,
		"key_hash":     util.SHA256Hex(raw),
		"enabled":      true,
		"created_at":   util.NowISO(),
		"last_used_at": nil,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
	if err := s.saveLocked(); err != nil {
		return nil, "", err
	}
	return publicAuthItem(item), raw, nil
}

func (s *AuthService) RevealKey(id, role string) (string, bool) {
	id = util.Clean(id)
	if id == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item["id"] != id || (role != "" && item["role"] != role) {
			continue
		}
		raw := util.Clean(item["key"])
		if raw == "" {
			return "", false
		}
		return raw, true
	}
	return "", false
}

func (s *AuthService) UpdateKey(id string, updates map[string]any, role string) map[string]any {
	id = util.Clean(id)
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.items {
		if item["id"] != id || (role != "" && item["role"] != role) {
			continue
		}
		next := util.CopyMap(item)
		if value, ok := updates["name"]; ok && value != nil {
			name := util.Clean(value)
			if name == "" {
				name = util.Clean(next["name"])
				if name == "" {
					name = "普通用户"
				}
			}
			next["name"] = name
		}
		if value, ok := updates["enabled"]; ok && value != nil {
			next["enabled"] = util.ToBool(value)
		}
		s.items[index] = next
		_ = s.saveLocked()
		return publicAuthItem(next)
	}
	return nil
}

func (s *AuthService) DeleteKey(id, role string) bool {
	id = util.Clean(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.items[:0]
	removed := false
	for _, item := range s.items {
		if item["id"] == id && (role == "" || item["role"] == role) {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		return false
	}
	s.items = next
	_ = s.saveLocked()
	return true
}

func (s *AuthService) Authenticate(raw string) *Identity {
	candidate := util.Clean(raw)
	if candidate == "" {
		return nil
	}
	hash := util.SHA256Hex(candidate)
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.items {
		if !util.ToBool(util.ValueOr(item["enabled"], true)) {
			continue
		}
		stored := util.Clean(item["key_hash"])
		if stored == "" || !hmac.Equal([]byte(stored), []byte(hash)) {
			continue
		}
		next := util.CopyMap(item)
		now := time.Now().UTC()
		next["last_used_at"] = now.Format(time.RFC3339Nano)
		s.items[index] = next
		id := util.Clean(next["id"])
		if last, ok := s.lastUsedFlushAt[id]; !ok || now.Sub(last) >= time.Minute {
			if s.saveLocked() == nil {
				s.lastUsedFlushAt[id] = now
			}
		}
		return &Identity{ID: util.Clean(next["id"]), Name: util.Clean(next["name"]), Role: util.Clean(next["role"])}
	}
	return nil
}

func (s *AuthService) load() []map[string]any {
	items, err := s.storage.LoadAuthKeys()
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if normalized := normalizeAuthItem(item); normalized != nil {
			out = append(out, normalized)
		}
	}
	return out
}

func (s *AuthService) saveLocked() error {
	return s.storage.SaveAuthKeys(s.items)
}

func normalizeAuthItem(raw map[string]any) map[string]any {
	role := util.Clean(raw["role"])
	if role != "admin" && role != "user" {
		return nil
	}
	key := util.Clean(raw["key"])
	if key == "" {
		return nil
	}
	hash := util.Clean(raw["key_hash"])
	if hash == "" {
		return nil
	}
	if util.SHA256Hex(key) != hash {
		return nil
	}
	id := util.Clean(raw["id"])
	if id == "" {
		id = util.NewHex(12)
	}
	name := util.Clean(raw["name"])
	if name == "" {
		if role == "admin" {
			name = "管理员密钥"
		} else {
			name = "普通用户"
		}
	}
	created := util.Clean(raw["created_at"])
	if created == "" {
		created = util.NowISO()
	}
	lastUsed := raw["last_used_at"]
	if util.Clean(lastUsed) == "" {
		lastUsed = nil
	}
	return map[string]any{
		"id":           id,
		"name":         name,
		"role":         role,
		"key":          key,
		"key_hash":     hash,
		"enabled":      util.ToBool(util.ValueOr(raw["enabled"], true)),
		"created_at":   created,
		"last_used_at": lastUsed,
	}
}

func publicAuthItem(item map[string]any) map[string]any {
	return map[string]any{
		"id":           item["id"],
		"name":         item["name"],
		"role":         item["role"],
		"enabled":      util.ToBool(util.ValueOr(item["enabled"], true)),
		"created_at":   item["created_at"],
		"last_used_at": item["last_used_at"],
	}
}
