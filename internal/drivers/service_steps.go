package drivers

import (
	"context"
	"fmt"
)

// ============ SSL ============

type SSLApplyStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewSSLApplyStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *SSLApplyStep {
	return &SSLApplyStep{BaseStep: NewBaseStep("ssl.apply", "申请 Let's Encrypt SSL 证书", false, "medium").WithAliases("SSL", "证书", "HTTPS", "ssl证书"), execFn: execFn}
}
func (s *SSLApplyStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *SSLApplyStep) Validate(params map[string]string) error {
	if params["domain"] == "" { return fmt.Errorf("缺少域名参数 domain") }
	return nil
}

type SSLRenewStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewSSLRenewStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *SSLRenewStep {
	return &SSLRenewStep{BaseStep: NewBaseStep("ssl.renew", "续期 SSL 证书", false, "medium").WithAliases("续期证书", "renew ssl"), execFn: execFn}
}
func (s *SSLRenewStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type SSLStatusStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewSSLStatusStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *SSLStatusStep {
	return &SSLStatusStep{BaseStep: NewBaseStep("ssl.status", "查看 SSL 证书状态", true, "low").WithAliases("证书状态", "ssl状态"), execFn: execFn}
}
func (s *SSLStatusStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// ============ Nginx ============

type NginxReloadStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewNginxReloadStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *NginxReloadStep {
	return &NginxReloadStep{BaseStep: NewBaseStep("nginx.reload", "重载 Nginx 配置", false, "medium").WithAliases("重载nginx", "nginx重载", "reload nginx"), execFn: execFn}
}
func (s *NginxReloadStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type NginxStatusStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewNginxStatusStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *NginxStatusStep {
	return &NginxStatusStep{BaseStep: NewBaseStep("nginx.status", "查看 Nginx 状态", true, "low").WithAliases("nginx状态", "nginx status"), execFn: execFn}
}
func (s *NginxStatusStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// ============ 数据库 ============

type DatabaseCreateStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseCreateStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseCreateStep {
	return &DatabaseCreateStep{BaseStep: NewBaseStep("database.create", "创建数据库", false, "medium").WithRollback("database.delete").WithAliases("创建数据库", "新建数据库"), execFn: execFn}
}
func (s *DatabaseCreateStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type DatabaseBackupStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseBackupStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseBackupStep {
	return &DatabaseBackupStep{BaseStep: NewBaseStep("database.backup", "备份数据库", false, "medium").WithAliases("备份数据库", "数据库备份", "backup"), execFn: execFn}
}
func (s *DatabaseBackupStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type DatabaseListStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseListStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseListStep {
	return &DatabaseListStep{BaseStep: NewBaseStep("database.list", "列出所有数据库", true, "low").WithAliases("数据库列表", "查看数据库"), execFn: execFn}
}
func (s *DatabaseListStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type DatabaseDeleteStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewDatabaseDeleteStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *DatabaseDeleteStep {
	return &DatabaseDeleteStep{BaseStep: NewBaseStep("database.delete", "删除数据库", false, "high").WithAliases("删除数据库"), execFn: execFn}
}
func (s *DatabaseDeleteStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
