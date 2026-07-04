package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OnePanelDriver 1Panel API 驱动（使用 StepRegistry 策略模式）
type OnePanelDriver struct {
	baseURL  string
	apiKey   string
	client   *http.Client
	registry *StepRegistry
}

// NewOnePanelDriver 创建 1Panel 驱动并注册所有操作
func NewOnePanelDriver(baseURL, apiKey string) *OnePanelDriver {
	d := &OnePanelDriver{
		baseURL:  baseURL,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 30 * time.Second},
		registry: NewStepRegistry(),
	}

	// execFn 是通用执行函数，所有 step 共用
	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		endpoint, method, body := d.mapActionToAPI(action, params)
		return d.doAPIRequest(ctx, method, endpoint, body)
	}

	// 注册所有操作
	// 容器
	d.registry.Register(NewContainerListStep(execFn))
	d.registry.Register(NewContainerStartStep(execFn))
	d.registry.Register(NewContainerStopStep(execFn))
	d.registry.Register(NewContainerRestartStep(execFn))
	d.registry.Register(NewContainerLogsStep(execFn))
	// 监控
	d.registry.Register(NewMonitorCPUStep(execFn))
	d.registry.Register(NewMonitorMemoryStep(execFn))
	d.registry.Register(NewMonitorDiskStep(execFn))
	d.registry.Register(NewMonitorNetworkStep(execFn))
	d.registry.Register(NewSystemInfoStep(execFn))
	d.registry.Register(NewHealthCheckStep(execFn))
	// 应用
	d.registry.Register(NewAppInstallStep(execFn))
	d.registry.Register(NewAppUninstallStep(execFn))
	d.registry.Register(NewAppListStep(execFn))
	// SSL
	d.registry.Register(NewSSLApplyStep(execFn))
	d.registry.Register(NewSSLRenewStep(execFn))
	d.registry.Register(NewSSLStatusStep(execFn))
	// Nginx
	d.registry.Register(NewNginxReloadStep(execFn))
	d.registry.Register(NewNginxStatusStep(execFn))
	// 数据库
	d.registry.Register(NewDatabaseCreateStep(execFn))
	d.registry.Register(NewDatabaseBackupStep(execFn))
	d.registry.Register(NewDatabaseListStep(execFn))
	d.registry.Register(NewDatabaseDeleteStep(execFn))
	d.registry.Register(NewDatabaseRestoreStep(execFn))
	d.registry.Register(NewDatabaseSlowQueryStep(execFn))
	d.registry.Register(NewDatabaseConnectionStep(execFn))
	d.registry.Register(NewDatabaseOptimizeStep(execFn))
	// 网站
	d.registry.Register(NewWebsiteCreateStep(execFn))
	d.registry.Register(NewWebsiteDeleteStep(execFn))
	d.registry.Register(NewWebsiteListStep(execFn))
	d.registry.Register(NewWebsiteProxyStep(execFn))
	d.registry.Register(NewWebsiteDomainStep(execFn))
	d.registry.Register(NewWebsitePHPConfigStep(execFn))
	// 文件
	d.registry.Register(NewFileListStep(execFn))
	d.registry.Register(NewFileReadStep(execFn))
	d.registry.Register(NewFileUploadStep(execFn))
	d.registry.Register(NewFileDeleteStep(execFn))

	return d
}

func (d *OnePanelDriver) Name() string { return "1Panel" }

// Execute 通过注册表执行操作
func (d *OnePanelDriver) Execute(ctx context.Context, action string, params map[string]string) (string, error) {
	return d.registry.Execute(ctx, action, params)
}

// RollbackAction 查找回滚操作
func (d *OnePanelDriver) RollbackAction(action string) string {
	return d.registry.RollbackAction(action)
}

// Registry 返回注册表（供 engine 或其他模块查询）
func (d *OnePanelDriver) Registry() *StepRegistry {
	return d.registry
}

// AvailableActions 返回所有注册的操作名
func (d *OnePanelDriver) AvailableActions() []string {
	return d.registry.Names()
}

// HealthCheck 健康检查
func (d *OnePanelDriver) HealthCheck(ctx context.Context) error {
	_, err := d.Execute(ctx, "health", nil)
	return err
}

// doAPIRequest 执行 HTTP 请求
func (d *OnePanelDriver) doAPIRequest(ctx context.Context, method, endpoint string, body map[string]interface{}) (string, error) {
	url := d.baseURL + endpoint

	var reqBody io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		req.Header.Set("1Panel-Token", d.apiKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("1Panel API 错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

// mapActionToAPI 将 action 映射到 1Panel API 端点
func (d *OnePanelDriver) mapActionToAPI(action string, params map[string]string) (endpoint, method string, body map[string]interface{}) {
	routes := map[string]struct {
		endpoint string
		method   string
	}{
		"app.install":      {"/api/v1/apps/install", "POST"},
		"app.uninstall":    {"/api/v1/apps/uninstall", "POST"},
		"app.list":         {"/api/v1/apps", "GET"},
		"app.status":       {"/api/v1/apps/status", "GET"},
		"container.list":   {"/api/v1/containers", "GET"},
		"container.start":  {"/api/v1/containers/start", "POST"},
		"container.stop":   {"/api/v1/containers/stop", "POST"},
		"container.restart": {"/api/v1/containers/restart", "POST"},
		"container.logs":   {"/api/v1/containers/logs", "GET"},
		"monitor.cpu":      {"/api/v1/monitor/cpu", "GET"},
		"monitor.memory":   {"/api/v1/monitor/memory", "GET"},
		"monitor.disk":     {"/api/v1/monitor/disk", "GET"},
		"monitor.network":  {"/api/v1/monitor/network", "GET"},
		"system.info":      {"/api/v1/system/info", "GET"},
		"ssl.apply":        {"/api/v1/websites/ssl", "POST"},
		"ssl.renew":        {"/api/v1/websites/ssl/renew", "POST"},
		"ssl.status":       {"/api/v1/websites/ssl", "GET"},
		"nginx.reload":     {"/api/v1/nginx/reload", "POST"},
		"nginx.status":     {"/api/v1/nginx/status", "GET"},
		"nginx.config":     {"/api/v1/nginx/config", "GET"},
		"database.create":  {"/api/v1/databases", "POST"},
		"database.delete":  {"/api/v1/databases/delete", "POST"},
		"database.backup":  {"/api/v1/databases/backup", "POST"},
		"database.list":    {"/api/v1/databases", "GET"},
		"database.restore": {"/api/v1/databases/restore", "POST"},
		"database.slow_query": {"/api/v1/databases/slow-query", "GET"},
		"database.connections": {"/api/v1/databases/connections", "GET"},
		"database.optimize": {"/api/v1/databases/optimize", "POST"},
		"website.create":   {"/api/v1/websites", "POST"},
		"website.delete":   {"/api/v1/websites/delete", "POST"},
		"website.list":     {"/api/v1/websites", "GET"},
		"website.proxy":    {"/api/v1/websites/proxy", "POST"},
		"website.domain":   {"/api/v1/websites/domain", "POST"},
		"website.php":      {"/api/v1/websites/php", "POST"},
		"file.list":        {"/api/v1/files", "GET"},
		"file.read":        {"/api/v1/files/read", "GET"},
		"file.upload":      {"/api/v1/files/upload", "POST"},
		"file.delete":      {"/api/v1/files/delete", "POST"},
		"health":           {"/api/v1/health", "GET"},
	}

	if route, ok := routes[action]; ok {
		return route.endpoint, route.method, nil
	}
	return "/api/v1/" + action, "POST", nil
}
