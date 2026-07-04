package drivers

import (
	"context"
	"fmt"
	"sync"
)

// Step 单个操作的抽象接口 —— 策略模式核心
type Step interface {
	// Name 操作名称 (如 container.start)
	Name() string

	// Description 操作描述
	Description() string

	// Execute 执行操作
	Execute(ctx context.Context, params map[string]string) (string, error)

	// Validate 参数校验
	Validate(params map[string]string) error

	// RollbackAction 对应的回滚操作名（空字符串表示无回滚）
	RollbackAction() string

	// IsReadOnly 是否只读（只读操作无需确认）
	IsReadOnly() bool

	// RiskLevel 风险等级: low / medium / high / forbidden
	RiskLevel() string

	// Aliases 自然语言别名
	Aliases() []string
}

// StepRegistry 操作注册表
type StepRegistry struct {
	mu    sync.RWMutex
	steps map[string]Step
}

// NewStepRegistry 创建注册表
func NewStepRegistry() *StepRegistry {
	return &StepRegistry{
		steps: make(map[string]Step),
	}
}

// Register 注册一个操作
func (r *StepRegistry) Register(step Step) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps[step.Name()] = step
}

// Get 获取操作
func (r *StepRegistry) Get(name string) (Step, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	step, ok := r.steps[name]
	return step, ok
}

// List 列出所有操作
func (r *StepRegistry) List() []Step {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Step, 0, len(r.steps))
	for _, s := range r.steps {
		result = append(result, s)
	}
	return result
}

// Names 返回所有注册的操作名
func (r *StepRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.steps))
	for name := range r.steps {
		names = append(names, name)
	}
	return names
}

// Execute 通过注册表执行操作
func (r *StepRegistry) Execute(ctx context.Context, action string, params map[string]string) (string, error) {
	step, ok := r.Get(action)
	if !ok {
		return "", fmt.Errorf("未知操作: %s", action)
	}
	if err := step.Validate(params); err != nil {
		return "", fmt.Errorf("参数校验失败: %w", err)
	}
	return step.Execute(ctx, params)
}

// RollbackAction 查找回滚操作
func (r *StepRegistry) RollbackAction(action string) string {
	step, ok := r.Get(action)
	if !ok {
		return ""
	}
	return step.RollbackAction()
}

// FindByAlias 通过别名查找操作名
func (r *StepRegistry) FindByAlias(alias string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, step := range r.steps {
		for _, a := range step.Aliases() {
			if a == alias {
				return name, true
			}
		}
	}
	return "", false
}
