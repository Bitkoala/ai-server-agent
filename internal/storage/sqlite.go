package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ai-server-agent/internal/models"
	_ "modernc.org/sqlite"
)

// SQLiteStore SQLite 持久化存储
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 创建 SQLite 存储
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if dbPath == "" {
		dbPath = "data/agent.db"
	}

	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=DELETE&_foreign_keys=on&_cache_size=-2000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	return store, nil
}

// migrate 数据库迁移
func (s *SQLiteStore) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			user_input TEXT NOT NULL,
			intent TEXT,
			response TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			intent TEXT NOT NULL,
			user_input TEXT NOT NULL,
			steps_json TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			step_id TEXT,
			action TEXT NOT NULL,
			params_before TEXT,
			params_after TEXT,
			result TEXT,
			error TEXT,
			user_id TEXT,
			duration_ms INTEGER DEFAULT 0,
			chain_hash TEXT,
			prev_hash TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_task ON audit_logs(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at)`,
		// 迁移：为旧表添加新字段（SQLite 不支持 IF NOT EXISTS for columns，忽略错误）
		`ALTER TABLE audit_logs ADD COLUMN step_id TEXT DEFAULT ''`,
		`ALTER TABLE audit_logs ADD COLUMN params_before TEXT DEFAULT ''`,
		`ALTER TABLE audit_logs ADD COLUMN params_after TEXT DEFAULT ''`,
		`ALTER TABLE audit_logs ADD COLUMN duration_ms INTEGER DEFAULT 0`,
		`ALTER TABLE audit_logs ADD COLUMN chain_hash TEXT DEFAULT ''`,
		`ALTER TABLE audit_logs ADD COLUMN prev_hash TEXT DEFAULT ''`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			// SQLite ALTER TABLE ADD COLUMN 不支持 IF NOT EXISTS，忽略重复列错误
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("执行迁移失败: %w\nSQL: %s", err, q)
		}
	}
	return nil
}

// SaveTask 保存任务
func (s *SQLiteStore) SaveTask(task *models.Task) error {
	stepsJSON := mustMarshal(task.Steps)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO tasks (id, intent, user_input, steps_json, status, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		task.ID, task.Intent, task.UserInput, stepsJSON, task.Status, time.Now(),
	)
	return err
}

// GetTask 获取任务
func (s *SQLiteStore) GetTask(id string) (*models.Task, error) {
	row := s.db.QueryRow(`SELECT id, intent, user_input, steps_json, status, created_at FROM tasks WHERE id = ?`, id)

	task := &models.Task{}
	var stepsJSON string
	err := row.Scan(&task.ID, &task.Intent, &task.UserInput, &stepsJSON, &task.Status, &task.CreatedAt)
	if err != nil {
		return nil, err
	}

	mustUnmarshal(stepsJSON, &task.Steps)
	return task, nil
}

// UpdateTask 更新任务
func (s *SQLiteStore) UpdateTask(task *models.Task) error {
	return s.SaveTask(task)
}

// ListTasks 列出任务
func (s *SQLiteStore) ListTasks(limit int) ([]*models.Task, error) {
	rows, err := s.db.Query(`SELECT id, intent, user_input, steps_json, status, created_at FROM tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var stepsJSON string
		if err := rows.Scan(&task.ID, &task.Intent, &task.UserInput, &stepsJSON, &task.Status, &task.CreatedAt); err != nil {
			return nil, err
		}
		mustUnmarshal(stepsJSON, &task.Steps)
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// Close 关闭数据库
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
