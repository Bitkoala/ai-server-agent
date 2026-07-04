package drivers

import (
	"context"
	"fmt"
	"sync"
)

// TargetType 目标类型
type TargetType string

const (
	TargetLocal  TargetType = "local"  // 本机
	TargetSSH    TargetType = "ssh"    // SSH 远程
	TargetAgent  TargetType = "agent"  // Agent Sidecar（gRPC）
)

// Target 管理目标（服务器节点）
type Target struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     TargetType        `json:"type"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	User     string            `json:"user,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Status   string            `json:"status"` // online/offline/error
}

// RemoteDriver 远程执行驱动接口
type RemoteDriver interface {
	Driver
	// Connect 连接到远程目标
	Connect(ctx context.Context, target Target) error
	// Disconnect 断开连接
	Disconnect(ctx context.Context) error
	// IsConnected 是否已连接
	IsConnected() bool
	// Target 返回当前目标
	Target() Target
}

// MultiTargetRouter 多目标路由器
type MultiTargetRouter struct {
	mu       sync.RWMutex
	targets  map[string]*TargetEntry
	local    Driver // 本地驱动
}

// TargetEntry 目标条目
type TargetEntry struct {
	Target Target
	Driver Driver
}

// NewMultiTargetRouter 创建多目标路由器
func NewMultiTargetRouter(localDriver Driver) *MultiTargetRouter {
	return &MultiTargetRouter{
		targets: make(map[string]*TargetEntry),
		local:   localDriver,
	}
}

// RegisterTarget 注册目标
func (r *MultiTargetRouter) RegisterTarget(target Target, driver Driver) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.targets[target.ID]; exists {
		return fmt.Errorf("目标 %s 已存在", target.ID)
	}

	r.targets[target.ID] = &TargetEntry{
		Target: target,
		Driver: driver,
	}
	return nil
}

// RemoveTarget 移除目标
func (r *MultiTargetRouter) RemoveTarget(targetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.targets, targetID)
}

// GetTarget 获取目标
func (r *MultiTargetRouter) GetTarget(targetID string) (*TargetEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.targets[targetID]
	if !ok {
		return nil, fmt.Errorf("目标 %s 不存在", targetID)
	}
	return entry, nil
}

// ListTargets 列出所有目标
func (r *MultiTargetRouter) ListTargets() []Target {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targets := make([]Target, 0, len(r.targets)+1)
	// 本地总是第一个
	targets = append(targets, Target{
		ID:     "localhost",
		Name:   "本机",
		Type:   TargetLocal,
		Status: "online",
	})

	for _, entry := range r.targets {
		targets = append(targets, entry.Target)
	}
	return targets
}

// Execute 在指定目标上执行操作
func (r *MultiTargetRouter) Execute(ctx context.Context, targetID, action string, params map[string]string) (string, error) {
	// 本地执行
	if targetID == "" || targetID == "localhost" {
		return r.local.Execute(ctx, action, params)
	}

	entry, err := r.GetTarget(targetID)
	if err != nil {
		return "", err
	}

	return entry.Driver.Execute(ctx, action, params)
}

// ExecuteAll 在所有在线目标上执行操作
func (r *MultiTargetRouter) ExecuteAll(ctx context.Context, action string, params map[string]string) map[string]struct {
	Result string
	Error  error
} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]struct {
		Result string
		Error  error
	})

	// 本地
	result, err := r.local.Execute(ctx, action, params)
	results["localhost"] = struct {
		Result string
		Error  error
	}{result, err}

	// 远程
	for id, entry := range r.targets {
		if entry.Target.Status != "online" {
			continue
		}
		result, err := entry.Driver.Execute(ctx, action, params)
		results[id] = struct {
			Result string
			Error  error
		}{result, err}
	}

	return results
}

// HealthCheck 检查所有目标健康状态
func (r *MultiTargetRouter) HealthCheck(ctx context.Context) map[string]error {
	r.mu.Lock()
	defer r.mu.Unlock()

	results := make(map[string]error)

	for id, entry := range r.targets {
		err := entry.Driver.HealthCheck(ctx)
		results[id] = err
		if err != nil {
			entry.Target.Status = "error"
		} else {
			entry.Target.Status = "online"
		}
	}
	return results
}

// LocalDriver 返回本地驱动
func (r *MultiTargetRouter) LocalDriver() Driver {
	return r.local
}

// TargetCount 目标数量（含本地）
func (r *MultiTargetRouter) TargetCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.targets) + 1
}
