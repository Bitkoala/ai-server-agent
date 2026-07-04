package models

import "time"

// TaskStep 任务步骤
type TaskStep struct {
	ID        string            `json:"id"`
	Action    string            `json:"action"`
	Params    map[string]string `json:"params"`
	DependsOn []string          `json:"depends_on,omitempty"`
	Status    string            `json:"status"` // pending/running/done/failed/confirm_required
	Result    string            `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// Task 任务定义
type Task struct {
	ID        string     `json:"id"`
	Intent    string     `json:"intent"`
	UserInput string     `json:"user_input"`
	Steps     []TaskStep `json:"steps"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}
