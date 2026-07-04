package drivers

// BaseStep 基础操作实现 —— 提供默认值，子类只需覆写需要的方法
type BaseStep struct {
	name        string
	description string
	isReadOnly  bool
	riskLevel   string
	rollback    string
	aliases     []string
}

// NewBaseStep 创建基础操作
func NewBaseStep(name, desc string, isReadOnly bool, riskLevel string) *BaseStep {
	return &BaseStep{
		name:        name,
		description: desc,
		isReadOnly:  isReadOnly,
		riskLevel:   riskLevel,
		aliases:     []string{},
	}
}

func (b *BaseStep) Name() string             { return b.name }
func (b *BaseStep) Description() string      { return b.description }
func (b *BaseStep) IsReadOnly() bool         { return b.isReadOnly }
func (b *BaseStep) RiskLevel() string        { return b.riskLevel }
func (b *BaseStep) RollbackAction() string   { return b.rollback }
func (b *BaseStep) Aliases() []string        { return b.aliases }
func (b *BaseStep) Validate(params map[string]string) error { return nil }

// WithRollback 设置回滚操作
func (b *BaseStep) WithRollback(action string) *BaseStep {
	b.rollback = action
	return b
}

// WithAliases 设置别名
func (b *BaseStep) WithAliases(aliases ...string) *BaseStep {
	b.aliases = aliases
	return b
}
