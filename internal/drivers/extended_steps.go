package drivers

import (
	"context"
	"fmt"
)

// ============ 数据库完整操作 ============

// DatabaseRestoreStep 恢复数据库
type DatabaseRestoreStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseRestoreStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseRestoreStep {
	return &DatabaseRestoreStep{BaseStep: NewBaseStep("database.restore", "恢复数据库备份", false, "high").WithAliases("恢复数据库", "数据库恢复", "restore"), execFn: execFn}
}
func (s *DatabaseRestoreStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *DatabaseRestoreStep) Validate(params map[string]string) error {
	if params["db_name"] == "" { return fmt.Errorf("缺少数据库名称") }
	if params["backup_file"] == "" { return fmt.Errorf("缺少备份文件路径") }
	return nil
}

// DatabaseSlowQueryStep 慢查询分析
type DatabaseSlowQueryStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseSlowQueryStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseSlowQueryStep {
	return &DatabaseSlowQueryStep{BaseStep: NewBaseStep("database.slow_query", "分析慢查询日志", true, "low").WithAliases("慢查询", "慢SQL", "slow query"), execFn: execFn}
}
func (s *DatabaseSlowQueryStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// DatabaseConnectionStep 查看连接数
type DatabaseConnectionStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseConnectionStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseConnectionStep {
	return &DatabaseConnectionStep{BaseStep: NewBaseStep("database.connections", "查看数据库连接数", true, "low").WithAliases("数据库连接", "连接数", "connections"), execFn: execFn}
}
func (s *DatabaseConnectionStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// DatabaseOptimizeStep 优化表
type DatabaseOptimizeStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseOptimizeStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseOptimizeStep {
	return &DatabaseOptimizeStep{BaseStep: NewBaseStep("database.optimize", "优化数据库表", false, "medium").WithAliases("优化数据库", "optimize", "表优化"), execFn: execFn}
}
func (s *DatabaseOptimizeStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// ============ 网站管理完整操作 ============

// WebsiteCreateStep 创建网站
type WebsiteCreateStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewWebsiteCreateStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *WebsiteCreateStep {
	return &WebsiteCreateStep{BaseStep: NewBaseStep("website.create", "创建网站", false, "medium").WithRollback("website.delete").WithAliases("创建网站", "新建站点", "建站"), execFn: execFn}
}
func (s *WebsiteCreateStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *WebsiteCreateStep) Validate(params map[string]string) error {
	if params["domain"] == "" { return fmt.Errorf("缺少域名参数") }
	return nil
}

// WebsiteDeleteStep 删除网站
type WebsiteDeleteStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewWebsiteDeleteStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *WebsiteDeleteStep {
	return &WebsiteDeleteStep{BaseStep: NewBaseStep("website.delete", "删除网站", false, "high").WithAliases("删除网站", "删除站点"), execFn: execFn}
}
func (s *WebsiteDeleteStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// WebsiteListStep 列出网站
type WebsiteListStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewWebsiteListStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *WebsiteListStep {
	return &WebsiteListStep{BaseStep: NewBaseStep("website.list", "列出所有网站", true, "low").WithAliases("网站列表", "站点列表"), execFn: execFn}
}
func (s *WebsiteListStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// WebsiteProxyStep 配置反向代理
type WebsiteProxyStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewWebsiteProxyStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *WebsiteProxyStep {
	return &WebsiteProxyStep{BaseStep: NewBaseStep("website.proxy", "配置反向代理", false, "medium").WithAliases("反向代理", "代理", "proxy"), execFn: execFn}
}
func (s *WebsiteProxyStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *WebsiteProxyStep) Validate(params map[string]string) error {
	if params["domain"] == "" { return fmt.Errorf("缺少域名") }
	if params["target"] == "" { return fmt.Errorf("缺少代理目标地址") }
	return nil
}

// WebsiteDomainStep 绑定域名
type WebsiteDomainStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewWebsiteDomainStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *WebsiteDomainStep {
	return &WebsiteDomainStep{BaseStep: NewBaseStep("website.domain", "绑定域名", false, "medium").WithAliases("绑定域名", "添加域名"), execFn: execFn}
}
func (s *WebsiteDomainStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// WebsitePHPConfigStep PHP 配置
type WebsitePHPConfigStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewWebsitePHPConfigStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *WebsitePHPConfigStep {
	return &WebsitePHPConfigStep{BaseStep: NewBaseStep("website.php", "配置 PHP 版本", false, "medium").WithAliases("PHP配置", "PHP版本"), execFn: execFn}
}
func (s *WebsitePHPConfigStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// ============ 文件管理操作 ============

// FileListStep 列出文件
type FileListStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewFileListStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *FileListStep {
	return &FileListStep{BaseStep: NewBaseStep("file.list", "列出文件", true, "low").WithAliases("文件列表", "查看文件", "ls"), execFn: execFn}
}
func (s *FileListStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// FileReadStep 读取文件
type FileReadStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewFileReadStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *FileReadStep {
	return &FileReadStep{BaseStep: NewBaseStep("file.read", "读取文件内容", true, "low").WithAliases("读取文件", "查看内容", "cat"), execFn: execFn}
}
func (s *FileReadStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// FileUploadStep 上传文件
type FileUploadStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewFileUploadStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *FileUploadStep {
	return &FileUploadStep{BaseStep: NewBaseStep("file.upload", "上传文件", false, "medium").WithAliases("上传文件", "upload"), execFn: execFn}
}
func (s *FileUploadStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// FileDeleteStep 删除文件
type FileDeleteStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewFileDeleteStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *FileDeleteStep {
	return &FileDeleteStep{BaseStep: NewBaseStep("file.delete", "删除文件", false, "high").WithAliases("删除文件", "rm"), execFn: execFn}
}
func (s *FileDeleteStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
