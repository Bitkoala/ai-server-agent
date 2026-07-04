package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// AuditEntry 审计日志条目
type AuditEntry struct {
	ID           int64     `json:"id"`
	TaskID       string    `json:"task_id"`
	StepID       string    `json:"step_id"`
	Action       string    `json:"action"`
	ParamsBefore string    `json:"params_before"`
	ParamsAfter  string    `json:"params_after"`
	Result       string    `json:"result"`
	Error        string    `json:"error,omitempty"`
	UserID       string    `json:"user_id"`
	DurationMs   int64     `json:"duration_ms"`
	ChainHash    string    `json:"chain_hash"`
	PrevHash     string    `json:"prev_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuditLogger 审计日志器（append-only + 链式哈希防篡改）
type AuditLogger struct {
	store    *SQLiteStore
	lastHash string
}

// NewAuditLogger 创建审计日志器
func NewAuditLogger(store *SQLiteStore) (*AuditLogger, error) {
	al := &AuditLogger{store: store}
	last, _ := al.getLastHash()
	al.lastHash = last
	return al, nil
}

// Log 记录审计日志（链式哈希）
func (al *AuditLogger) Log(entry *AuditEntry) error {
	entry.PrevHash = al.lastHash
	raw := fmt.Sprintf("%s|%s|%s|%s|%d",
		entry.PrevHash, entry.TaskID, entry.Action, entry.Result, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(raw))
	entry.ChainHash = hex.EncodeToString(hash[:])

	_, err := al.store.db.Exec(
		`INSERT INTO audit_logs 
		 (task_id, step_id, action, params_before, params_after, result, error, user_id, duration_ms, chain_hash, prev_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.TaskID, entry.StepID, entry.Action,
		entry.ParamsBefore, entry.ParamsAfter,
		entry.Result, entry.Error, entry.UserID,
		entry.DurationMs, entry.ChainHash, entry.PrevHash,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("写入审计日志失败: %w", err)
	}
	al.lastHash = entry.ChainHash
	return nil
}

// LogOperation 记录操作执行（自动脱敏敏感参数）
func (al *AuditLogger) LogOperation(taskID, stepID, action string, params map[string]string, result string, opErr error, durationMs int64) {
	entry := &AuditEntry{
		TaskID:       taskID,
		StepID:       stepID,
		Action:       action,
		ParamsBefore: mustMarshal(maskSensitiveParams(params)),
		Result:       truncateStr(result, 500),
		DurationMs:   durationMs,
	}
	if opErr != nil {
		entry.Error = opErr.Error()
	}
	al.Log(entry)
}

// LogDiff 记录变更 diff（自动脱敏敏感参数）
func (al *AuditLogger) LogDiff(taskID, stepID, action string, before, after map[string]string, result string, opErr error, durationMs int64) {
	entry := &AuditEntry{
		TaskID:       taskID,
		StepID:       stepID,
		Action:       action,
		ParamsBefore: mustMarshal(maskSensitiveParams(before)),
		ParamsAfter:  mustMarshal(maskSensitiveParams(after)),
		Result:       truncateStr(result, 500),
		DurationMs:   durationMs,
	}
	if opErr != nil {
		entry.Error = opErr.Error()
	}
	al.Log(entry)
}

// Verify 验证审计链完整性
func (al *AuditLogger) Verify() (bool, []string, error) {
	rows, err := al.store.db.Query(
		`SELECT id, task_id, action, result, prev_hash, chain_hash FROM audit_logs ORDER BY id ASC`)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()

	var tampered []string
	prevHash := ""
	for rows.Next() {
		var id int64
		var taskID, action, result, prevH, chainH string
		if err := rows.Scan(&id, &taskID, &action, &result, &prevH, &chainH); err != nil {
			continue
		}
		if prevH != prevHash {
			tampered = append(tampered, fmt.Sprintf("日志 #%d 前置哈希不匹配: 期望 %s, 实际 %s", id, prevHash, prevH))
		}
		prevHash = chainH
	}
	return len(tampered) == 0, tampered, nil
}

// Query 查询审计日志
func (al *AuditLogger) Query(taskID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if taskID != "" {
		rows, err = al.store.db.Query(
			`SELECT id, task_id, step_id, action, params_before, params_after, result, error, user_id, duration_ms, chain_hash, prev_hash, created_at 
			 FROM audit_logs WHERE task_id = ? ORDER BY id DESC LIMIT ?`, taskID, limit)
	} else {
		rows, err = al.store.db.Query(
			`SELECT id, task_id, step_id, action, params_before, params_after, result, error, user_id, duration_ms, chain_hash, prev_hash, created_at 
			 FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.StepID, &e.Action,
			&e.ParamsBefore, &e.ParamsAfter, &e.Result, &e.Error,
			&e.UserID, &e.DurationMs, &e.ChainHash, &e.PrevHash, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ExportJSON 导出审计日志
func (al *AuditLogger) ExportJSON(taskID string) ([]byte, error) {
	entries, err := al.Query(taskID, 1000)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(entries, "", "  ")
}

func (al *AuditLogger) getLastHash() (string, error) {
	var hash string
	err := al.store.db.QueryRow(`SELECT chain_hash FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&hash)
	if err != nil {
		return "", nil
	}
	return hash, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

// maskSensitiveParams 对敏感参数进行脱敏处理
// 将密码、密钥、token 等字段的值替换为 "***REDACTED***"
func maskSensitiveParams(params map[string]string) map[string]string {
	if params == nil {
		return nil
	}
	sensitiveKeys := map[string]bool{
		"password":    true,
		"passwd":      true,
		"api_key":     true,
		"apikey":      true,
		"secret":      true,
		"token":       true,
		"jwt_secret":  true,
		"private_key": true,
		"access_key":  true,
		"secret_key":  true,
		"auth_token":  true,
		"credential":  true,
	}
	masked := make(map[string]string, len(params))
	for k, v := range params {
		if sensitiveKeys[k] {
			masked[k] = "***REDACTED***"
		} else {
			masked[k] = v
		}
	}
	return masked
}
