package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type Backend interface {
	LoadAccounts() ([]map[string]any, error)
	SaveAccounts([]map[string]any) error
	LoadAuthKeys() ([]map[string]any, error)
	SaveAuthKeys([]map[string]any) error
	HealthCheck() map[string]any
	Info() map[string]any
}

func NewBackendFromEnv(dataDir string) (Backend, error) {
	backendType := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND")))
	if backendType == "" {
		backendType = "json"
	}
	switch backendType {
	case "json":
		return NewJSONBackend(filepath.Join(dataDir, "accounts.json"), filepath.Join(dataDir, "auth_keys.json")), nil
	case "sqlite", "postgres", "postgresql", "mysql", "database":
		dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
		if dsn == "" {
			dsn = "sqlite:///" + filepath.ToSlash(filepath.Join(dataDir, "accounts.db"))
		}
		return NewDatabaseBackend(dsn)
	case "git":
		repoURL := strings.TrimSpace(os.Getenv("GIT_REPO_URL"))
		if repoURL == "" {
			return nil, fmt.Errorf("GIT_REPO_URL is required when using git storage backend")
		}
		return NewGitBackend(GitOptions{
			RepoURL:          repoURL,
			Token:            strings.TrimSpace(os.Getenv("GIT_TOKEN")),
			Branch:           envDefault("GIT_BRANCH", "main"),
			FilePath:         envDefault("GIT_FILE_PATH", "accounts.json"),
			AuthKeysFilePath: envDefault("GIT_AUTH_KEYS_FILE_PATH", "auth_keys.json"),
			CacheDir:         filepath.Join(dataDir, "git_cache"),
		}), nil
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", backendType)
	}
}

func envDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

type JSONBackend struct {
	filePath     string
	authKeysPath string
}

func NewJSONBackend(filePath, authKeysPath string) *JSONBackend {
	_ = os.MkdirAll(filepath.Dir(filePath), 0o755)
	_ = os.MkdirAll(filepath.Dir(authKeysPath), 0o755)
	return &JSONBackend{filePath: filePath, authKeysPath: authKeysPath}
}

func (b *JSONBackend) LoadAccounts() ([]map[string]any, error) {
	return loadJSONList(b.filePath), nil
}

func (b *JSONBackend) SaveAccounts(accounts []map[string]any) error {
	return saveJSONValue(b.filePath, accounts)
}

func (b *JSONBackend) LoadAuthKeys() ([]map[string]any, error) {
	raw := loadJSONValue(b.authKeysPath)
	if obj, ok := raw.(map[string]any); ok {
		raw = obj["items"]
	}
	return anyListToMaps(raw), nil
}

func (b *JSONBackend) SaveAuthKeys(keys []map[string]any) error {
	return saveJSONValue(b.authKeysPath, map[string]any{"items": keys})
}

func (b *JSONBackend) HealthCheck() map[string]any {
	if _, err := os.Stat(b.filePath); err != nil && !os.IsNotExist(err) {
		return map[string]any{"status": "unhealthy", "backend": "json", "error": err.Error()}
	}
	return map[string]any{
		"status":                "healthy",
		"backend":               "json",
		"file_exists":           exists(b.filePath),
		"file_path":             b.filePath,
		"auth_keys_file_exists": exists(b.authKeysPath),
		"auth_keys_file_path":   b.authKeysPath,
	}
}

func (b *JSONBackend) Info() map[string]any {
	return map[string]any{
		"type":                  "json",
		"description":           "本地 JSON 文件存储",
		"file_path":             b.filePath,
		"file_exists":           exists(b.filePath),
		"auth_keys_file_path":   b.authKeysPath,
		"auth_keys_file_exists": exists(b.authKeysPath),
	}
}

type DatabaseBackend struct {
	databaseURL string
	driver      string
	dsn         string
	db          *sql.DB
}

func NewDatabaseBackend(databaseURL string) (*DatabaseBackend, error) {
	driver, dsn, err := parseDatabaseURL(databaseURL)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Hour)
	db.SetMaxOpenConns(10)
	backend := &DatabaseBackend{databaseURL: databaseURL, driver: driver, dsn: dsn, db: db}
	if err := backend.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return backend, nil
}

func (b *DatabaseBackend) init() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, access_token TEXT UNIQUE NOT NULL, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS auth_keys (id INTEGER PRIMARY KEY AUTOINCREMENT, key_id TEXT UNIQUE NOT NULL, data TEXT NOT NULL)`,
	}
	if b.driver == "postgres" {
		schema = []string{
			`CREATE TABLE IF NOT EXISTS accounts (id SERIAL PRIMARY KEY, access_token TEXT UNIQUE NOT NULL, data TEXT NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS auth_keys (id SERIAL PRIMARY KEY, key_id TEXT UNIQUE NOT NULL, data TEXT NOT NULL)`,
		}
	}
	for _, stmt := range schema {
		if _, err := b.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (b *DatabaseBackend) LoadAccounts() ([]map[string]any, error) {
	return b.loadRows("accounts")
}

func (b *DatabaseBackend) SaveAccounts(accounts []map[string]any) error {
	return b.saveRows("accounts", "access_token", accounts)
}

func (b *DatabaseBackend) LoadAuthKeys() ([]map[string]any, error) {
	return b.loadRows("auth_keys")
}

func (b *DatabaseBackend) SaveAuthKeys(keys []map[string]any) error {
	return b.saveRows("auth_keys", "key_id", keys)
}

func (b *DatabaseBackend) HealthCheck() map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.db.PingContext(ctx); err != nil {
		return map[string]any{"status": "unhealthy", "backend": "database", "error": err.Error()}
	}
	accountCount := b.count("accounts")
	authKeyCount := b.count("auth_keys")
	return map[string]any{"status": "healthy", "backend": "database", "database_url": maskPassword(b.databaseURL), "account_count": accountCount, "auth_key_count": authKeyCount}
}

func (b *DatabaseBackend) Info() map[string]any {
	dbType := "unknown"
	switch b.driver {
	case "sqlite":
		dbType = "sqlite"
	case "postgres":
		dbType = "postgresql"
	case "mysql":
		dbType = "mysql"
	}
	return map[string]any{"type": "database", "db_type": dbType, "description": "数据库存储 (" + dbType + ")", "database_url": maskPassword(b.databaseURL)}
}

func (b *DatabaseBackend) loadRows(table string) ([]map[string]any, error) {
	rows, err := b.db.Query("SELECT data FROM " + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			continue
		}
		var item map[string]any
		if json.Unmarshal([]byte(text), &item) == nil && item != nil {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func (b *DatabaseBackend) saveRows(table, keyColumn string, items []map[string]any) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.Exec("DELETE FROM " + table); err != nil {
		return err
	}
	sourceKey := "access_token"
	if table == "auth_keys" {
		sourceKey = "id"
	}
	stmtText := "INSERT INTO " + table + " (" + keyColumn + ", data) VALUES (?, ?)"
	if b.driver == "postgres" {
		stmtText = "INSERT INTO " + table + " (" + keyColumn + ", data) VALUES ($1, $2)"
	}
	stmt, err := tx.Prepare(stmtText)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		key := strings.TrimSpace(fmt.Sprint(item[sourceKey]))
		if key == "" {
			continue
		}
		data, err := json.Marshal(item)
		if err != nil {
			continue
		}
		if _, err := stmt.Exec(key, string(data)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (b *DatabaseBackend) count(table string) int {
	var count int
	_ = b.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	return count
}

type GitOptions struct {
	RepoURL          string
	Token            string
	Branch           string
	FilePath         string
	AuthKeysFilePath string
	CacheDir         string
}

type GitBackend struct {
	options GitOptions
}

func NewGitBackend(options GitOptions) *GitBackend {
	if options.Branch == "" {
		options.Branch = "main"
	}
	if options.FilePath == "" {
		options.FilePath = "accounts.json"
	}
	if options.AuthKeysFilePath == "" {
		options.AuthKeysFilePath = "auth_keys.json"
	}
	_ = os.MkdirAll(options.CacheDir, 0o755)
	return &GitBackend{options: options}
}

func (b *GitBackend) LoadAccounts() ([]map[string]any, error) {
	data, err := b.loadValue(b.options.FilePath)
	if err != nil {
		return nil, err
	}
	return anyListToMaps(data), nil
}

func (b *GitBackend) SaveAccounts(accounts []map[string]any) error {
	return b.saveValue(b.options.FilePath, accounts, "Update accounts data")
}

func (b *GitBackend) LoadAuthKeys() ([]map[string]any, error) {
	data, err := b.loadValue(b.options.AuthKeysFilePath)
	if err != nil {
		return nil, err
	}
	if obj, ok := data.(map[string]any); ok {
		data = obj["items"]
	}
	return anyListToMaps(data), nil
}

func (b *GitBackend) SaveAuthKeys(keys []map[string]any) error {
	return b.saveValue(b.options.AuthKeysFilePath, map[string]any{"items": keys}, "Update auth keys data")
}

func (b *GitBackend) HealthCheck() map[string]any {
	repo, err := b.cloneOrPull()
	if err != nil {
		return map[string]any{"status": "unhealthy", "backend": "git", "error": err.Error()}
	}
	commit, _ := gitOutput(repo, "rev-parse", "--short=8", "HEAD")
	return map[string]any{"status": "healthy", "backend": "git", "repo_url": maskToken(b.options.RepoURL), "branch": b.options.Branch, "file_path": b.options.FilePath, "auth_keys_file_path": b.options.AuthKeysFilePath, "last_commit": strings.TrimSpace(commit)}
}

func (b *GitBackend) Info() map[string]any {
	return map[string]any{"type": "git", "description": "Git 私有仓库存储", "repo_url": maskToken(b.options.RepoURL), "branch": b.options.Branch, "file_path": b.options.FilePath, "auth_keys_file_path": b.options.AuthKeysFilePath}
}

func (b *GitBackend) loadValue(filePath string) (any, error) {
	repo, err := b.cloneOrPull()
	if err != nil {
		return nil, err
	}
	full := filepath.Join(repo, filepath.FromSlash(filePath))
	data, err := os.ReadFile(full)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *GitBackend) saveValue(filePath string, value any, message string) error {
	repo, err := b.cloneOrPull()
	if err != nil {
		return err
	}
	full := filepath.Join(repo, filepath.FromSlash(filePath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := saveJSONValue(full, value); err != nil {
		return err
	}
	if _, err := gitOutput(repo, "add", filePath); err != nil {
		return err
	}
	status, err := gitOutput(repo, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if _, err := gitOutput(repo, "commit", "-m", message); err != nil {
		return err
	}
	_, err = gitOutput(repo, "push", "origin", b.options.Branch)
	return err
}

func (b *GitBackend) cloneOrPull() (string, error) {
	repoPath := filepath.Join(b.options.CacheDir, "repo")
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		if _, err := gitOutput(repoPath, "pull", "origin", b.options.Branch); err == nil {
			return repoPath, nil
		}
		_ = os.RemoveAll(repoPath)
	}
	authURL := buildAuthURL(b.options.RepoURL, b.options.Token)
	if _, err := gitOutput("", "clone", "--branch", b.options.Branch, authURL, repoPath); err != nil {
		return "", err
	}
	return repoPath, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func loadJSONList(path string) []map[string]any {
	return anyListToMaps(loadJSONValue(path))
}

func loadJSONValue(path string) any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil
	}
	return out
}

func saveJSONValue(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func anyListToMaps(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseDatabaseURL(databaseURL string) (driver, dsn string, err error) {
	lower := strings.ToLower(databaseURL)
	switch {
	case strings.HasPrefix(lower, "sqlite:///"):
		return "sqlite", strings.TrimPrefix(databaseURL, "sqlite:///"), nil
	case strings.HasPrefix(lower, "sqlite://"):
		return "sqlite", strings.TrimPrefix(databaseURL, "sqlite://"), nil
	case strings.HasPrefix(lower, "postgresql://"), strings.HasPrefix(lower, "postgres://"):
		return "postgres", databaseURL, nil
	case strings.HasPrefix(lower, "mysql://"):
		u, parseErr := url.Parse(databaseURL)
		if parseErr != nil {
			return "", "", parseErr
		}
		pass, _ := u.User.Password()
		user := u.User.Username()
		db := strings.TrimPrefix(u.Path, "/")
		return "mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", user, pass, u.Host, db), nil
	default:
		if strings.Contains(lower, "postgres") {
			return "postgres", databaseURL, nil
		}
		return "sqlite", databaseURL, nil
	}
}

func maskPassword(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	username := u.User.Username()
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(username, "****")
	}
	return u.String()
}

func maskToken(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = url.User("****")
	return u.String()
}

func buildAuthURL(repoURL, token string) string {
	if token == "" {
		return repoURL
	}
	if strings.HasPrefix(repoURL, "https://") {
		return strings.Replace(repoURL, "https://", "https://"+url.QueryEscape(token)+"@", 1)
	}
	if strings.HasPrefix(repoURL, "git@") {
		converted := strings.Replace(repoURL, "git@", "https://", 1)
		converted = strings.Replace(converted, ".com:", ".com/", 1)
		return strings.Replace(converted, "https://", "https://"+url.QueryEscape(token)+"@", 1)
	}
	return repoURL
}
