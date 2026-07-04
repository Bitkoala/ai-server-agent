package drivers

import (
	"context"
	"fmt"
)

// ============ 监控 & 应用操作 ============

type MonitorCPUStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewMonitorCPUStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *MonitorCPUStep {
	return &MonitorCPUStep{BaseStep: NewBaseStep("monitor.cpu", "查看 CPU 使用率", true, "low").WithAliases("CPU", "cpu使用率", "处理器"), execFn: execFn}
}
func (s *MonitorCPUStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type MonitorMemoryStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewMonitorMemoryStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *MonitorMemoryStep {
	return &MonitorMemoryStep{BaseStep: NewBaseStep("monitor.memory", "查看内存使用情况", true, "low").WithAliases("内存", "memory", "RAM"), execFn: execFn}
}
func (s *MonitorMemoryStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type MonitorDiskStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewMonitorDiskStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *MonitorDiskStep {
	return &MonitorDiskStep{BaseStep: NewBaseStep("monitor.disk", "查看磁盘使用情况", true, "low").WithAliases("磁盘", "硬盘", "disk", "存储空间"), execFn: execFn}
}
func (s *MonitorDiskStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type MonitorNetworkStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewMonitorNetworkStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *MonitorNetworkStep {
	return &MonitorNetworkStep{BaseStep: NewBaseStep("monitor.network", "查看网络流量", true, "low").WithAliases("网络", "network", "流量"), execFn: execFn}
}
func (s *MonitorNetworkStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type SystemInfoStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewSystemInfoStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *SystemInfoStep {
	return &SystemInfoStep{BaseStep: NewBaseStep("system.info", "查看系统信息", true, "low").WithAliases("系统信息", "系统状态", "服务器信息", "状态"), execFn: execFn}
}
func (s *SystemInfoStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type HealthCheckStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewHealthCheckStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *HealthCheckStep {
	return &HealthCheckStep{BaseStep: NewBaseStep("health", "健康检查", true, "low").WithAliases("健康检查", "health check"), execFn: execFn}
}
func (s *HealthCheckStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

// ============ 应用 ============

type AppInstallStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewAppInstallStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *AppInstallStep {
	return &AppInstallStep{BaseStep: NewBaseStep("app.install", "安装应用", false, "medium").WithRollback("app.uninstall").WithAliases("安装", "部署", "deploy", "install"), execFn: execFn}
}
func (s *AppInstallStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *AppInstallStep) Validate(params map[string]string) error {
	if params["name"] == "" { return fmt.Errorf("缺少应用名称参数 name") }
	return nil
}

type AppUninstallStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewAppUninstallStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *AppUninstallStep {
	return &AppUninstallStep{BaseStep: NewBaseStep("app.uninstall", "卸载应用", false, "high").WithAliases("卸载", "删除应用", "uninstall"), execFn: execFn}
}
func (s *AppUninstallStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type AppListStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewAppListStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *AppListStep {
	return &AppListStep{BaseStep: NewBaseStep("app.list", "列出已安装应用", true, "low").WithAliases("应用列表", "已安装"), execFn: execFn}
}
func (s *AppListStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
