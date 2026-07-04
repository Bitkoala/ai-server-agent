package drivers

import "context"

// Driver 能力驱动接口 —— 所有执行层操作的抽象
type Driver interface {
	// Execute 执行一个操作
	Execute(ctx context.Context, action string, params map[string]string) (string, error)

	// AvailableActions 返回该驱动支持的操作列表
	AvailableActions() []string

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error

	// RollbackAction 返回某操作的对应回滚操作（空字符串表示不支持回滚）
	RollbackAction(action string) string

	// Registry 返回步骤注册表（用于查询操作元数据）
	Registry() *StepRegistry

	// Name 驱动名称
	Name() string
}

// Action 定义了一个可执行的操作（保留兼容）
type Action struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []Param  `json:"params"`
	Dangerous   bool     `json:"dangerous"`
	Aliases     []string `json:"aliases"`
}

// Param 操作参数
type Param struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Choices     []string `json:"choices,omitempty"`
}
