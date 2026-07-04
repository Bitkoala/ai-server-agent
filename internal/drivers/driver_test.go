package drivers

import (
	"context"
	"testing"
	"time"
)

// mockDriver implements the Driver interface for testing.
type mockDriver struct {
	name        string
	actions     []string
	executeFunc func(ctx context.Context, action string, params map[string]string) (string, error)
	healthFunc  func(ctx context.Context) error
}

func (m *mockDriver) Name() string                         { return m.name }
func (m *mockDriver) AvailableActions() []string            { return m.actions }
func (m *mockDriver) RollbackAction(action string) string   { return "" }
func (m *mockDriver) Registry() *StepRegistry               { return nil }
func (m *mockDriver) Execute(ctx context.Context, action string, params map[string]string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, action, params)
	}
	return "ok", nil
}
func (m *mockDriver) HealthCheck(ctx context.Context) error {
	if m.healthFunc != nil {
		return m.healthFunc(ctx)
	}
	return nil
}

// ========== OnePanelDriver Tests ==========

func TestOnePanelDriver_Creation(t *testing.T) {
	d := NewOnePanelDriver("http://localhost:9999", "test-key")
	if d == nil {
		t.Fatal("NewOnePanelDriver 应返回非 nil 实例")
	}
	if d.baseURL != "http://localhost:9999" {
		t.Errorf("baseURL 应为 http://localhost:9999, 实际 %s", d.baseURL)
	}
	if d.apiKey != "test-key" {
		t.Errorf("apiKey 应为 test-key, 实际 %s", d.apiKey)
	}
	if d.registry == nil {
		t.Error("registry 不应为 nil")
	}
}

func TestOnePanelDriver_Name(t *testing.T) {
	d := NewOnePanelDriver("http://localhost:9999", "test-key")
	if name := d.Name(); name != "1Panel" {
		t.Errorf("Name() 应返回 1Panel, 实际 %s", name)
	}
}

func TestOnePanelDriver_AvailableActions(t *testing.T) {
	d := NewOnePanelDriver("http://localhost:9999", "test-key")
	actions := d.AvailableActions()
	if len(actions) < 30 {
		t.Errorf("AvailableActions 应返回至少 30 个操作, 实际 %d", len(actions))
	}
}

func TestOnePanelDriver_MapActionToAPI(t *testing.T) {
	d := NewOnePanelDriver("http://localhost:9999", "test-key")

	tests := []struct {
		action         string
		expectedMethod string
		expectedPrefix string
	}{
		{"container.list", "GET", "/api/v1/containers"},
		{"container.start", "POST", "/api/v1/containers/start"},
		{"monitor.cpu", "GET", "/api/v1/monitor/cpu"},
		{"database.backup", "POST", "/api/v1/databases/backup"},
		{"nginx.reload", "POST", "/api/v1/nginx/reload"},
		{"website.create", "POST", "/api/v1/websites"},
		{"file.list", "GET", "/api/v1/files"},
		{"app.install", "POST", "/api/v1/apps/install"},
		{"ssl.apply", "POST", "/api/v1/websites/ssl"},
	}

	for _, tc := range tests {
		endpoint, method, _ := d.mapActionToAPI(tc.action, nil)
		if method != tc.expectedMethod {
			t.Errorf("%s: method 应为 %s, 实际 %s", tc.action, tc.expectedMethod, method)
		}
		if endpoint != tc.expectedPrefix {
			t.Errorf("%s: endpoint 应为 %s, 实际 %s", tc.action, tc.expectedPrefix, endpoint)
		}
	}
}

func TestOnePanelDriver_MapActionToAPI_Unknown(t *testing.T) {
	d := NewOnePanelDriver("http://localhost:9999", "test-key")

	endpoint, method, _ := d.mapActionToAPI("unknown.action", nil)
	if method != "POST" {
		t.Errorf("未知 action 的 method 应为 POST, 实际 %s", method)
	}
	if endpoint != "/api/v1/unknown.action" {
		t.Errorf("未知 action 的 endpoint 应为 /api/v1/unknown.action, 实际 %s", endpoint)
	}
}

func TestOnePanelDriver_Execute_ContextCancelled(t *testing.T) {
	d := NewOnePanelDriver("http://localhost:9999", "test-key")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// container.list 是只读操作，不需要 params 验证
	_, err := d.Execute(ctx, "container.list", nil)
	if err == nil {
		t.Error("已取消的 context 应返回错误")
	}
}

// ========== SSHDriver Tests ==========

func TestSSHDriver_Creation(t *testing.T) {
	d := NewSSHDriver(10 * time.Second)
	if d == nil {
		t.Fatal("NewSSHDriver 应返回非 nil 实例")
	}
	if d.timeout != 10*time.Second {
		t.Errorf("timeout 应为 10s, 实际 %v", d.timeout)
	}
	if d.connected {
		t.Error("新创建的 SSHDriver 不应已连接")
	}
}

func TestSSHDriver_Name(t *testing.T) {
	d := NewSSHDriver(10 * time.Second)
	if name := d.Name(); name != "ssh" {
		t.Errorf("Name() 应返回 ssh, 实际 %s", name)
	}
}

func TestSSHDriver_MapActionToCommand(t *testing.T) {
	d := NewSSHDriver(10 * time.Second)

	tests := []struct {
		action      string
		containsStr string
	}{
		{"system.info", "uname -a"},
		{"monitor.cpu", "top -bn1"},
		{"monitor.memory", "free -h"},
		{"monitor.disk", "df -h"},
		{"container.list", "docker ps"},
		{"nginx.reload", "nginx -s reload"},
		{"nginx.status", "systemctl status nginx"},
	}

	for _, tc := range tests {
		cmd := d.mapActionToCommand(tc.action, nil)
		if cmd == "" {
			t.Errorf("%s: 不应返回空命令", tc.action)
		}
		found := false
		for i := 0; i <= len(cmd)-len(tc.containsStr); i++ {
			if cmd[i:i+len(tc.containsStr)] == tc.containsStr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: 命令应包含 '%s', 实际 '%s'", tc.action, tc.containsStr, cmd)
		}
	}

	// 未知 action 的默认命令
	cmd := d.mapActionToCommand("unknown.action", nil)
	if cmd != "echo '未知操作: unknown.action'" {
		t.Errorf("未知 action 应返回 echo 命令, 实际 '%s'", cmd)
	}
}

// ========== MultiTargetRouter Tests ==========

func TestMultiTargetRouter_RegisterAndRemove(t *testing.T) {
	local := &mockDriver{name: "local", actions: []string{"system.info"}}
	router := NewMultiTargetRouter(local)

	if router.TargetCount() != 1 {
		t.Errorf("新路由器的目标数应为 1, 实际 %d", router.TargetCount())
	}

	// 注册新目标
	target := Target{
		ID:   "server-1",
		Name: "Server 1",
		Type: TargetSSH,
		Host: "192.168.1.100",
		Port: 22,
	}
	remote := &mockDriver{name: "ssh", actions: []string{"system.info"}}

	err := router.RegisterTarget(target, remote)
	if err != nil {
		t.Errorf("注册目标失败: %v", err)
	}

	if router.TargetCount() != 2 {
		t.Errorf("注册后目标数应为 2, 实际 %d", router.TargetCount())
	}

	// 重复注册应失败
	err = router.RegisterTarget(target, remote)
	if err == nil {
		t.Error("重复注册应返回错误")
	}

	// 获取已注册目标
	entry, err := router.GetTarget("server-1")
	if err != nil {
		t.Errorf("获取目标失败: %v", err)
	}
	if entry.Target.Name != "Server 1" {
		t.Errorf("目标名称应为 Server 1, 实际 %s", entry.Target.Name)
	}

	// 移除目标
	router.RemoveTarget("server-1")
	if router.TargetCount() != 1 {
		t.Errorf("移除后目标数应为 1, 实际 %d", router.TargetCount())
	}

	// 获取已移除的目标应失败
	_, err = router.GetTarget("server-1")
	if err == nil {
		t.Error("获取已移除的目标应返回错误")
	}
}

func TestMultiTargetRouter_ExecuteAll(t *testing.T) {
	local := &mockDriver{
		name:    "local",
		actions: []string{"system.info"},
		executeFunc: func(ctx context.Context, action string, params map[string]string) (string, error) {
			return "local-result", nil
		},
	}
	router := NewMultiTargetRouter(local)

	// 注册两个远程目标
	for i, id := range []string{"srv-a", "srv-b"} {
		result := id + "-result"
		remote := &mockDriver{
			name:    id,
			actions: []string{"system.info"},
			executeFunc: func(ctx context.Context, action string, params map[string]string) (string, error) {
				return result, nil
			},
		}
		router.RegisterTarget(Target{
			ID:     id,
			Name:   "Server " + string(rune('A'+i)),
			Type:   TargetSSH,
			Status: "online",
		}, remote)
	}

	results := router.ExecuteAll(context.Background(), "system.info", nil)

	// 应包含 localhost + 2 个远程
	if len(results) != 3 {
		t.Errorf("ExecuteAll 应返回 3 个结果, 实际 %d", len(results))
	}

	for _, id := range []string{"localhost", "srv-a", "srv-b"} {
		r, ok := results[id]
		if !ok {
			t.Errorf("结果中应包含 %s", id)
			continue
		}
		if r.Error != nil {
			t.Errorf("%s 不应有错误: %v", id, r.Error)
		}
	}
}

// ========== Step Validation Tests ==========

func TestAllStepRiskLevels(t *testing.T) {
	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "ok", nil
	}

	allSteps := []Step{
		NewContainerListStep(execFn),
		NewContainerStartStep(execFn),
		NewContainerStopStep(execFn),
		NewContainerRestartStep(execFn),
		NewContainerLogsStep(execFn),
		NewMonitorCPUStep(execFn),
		NewMonitorMemoryStep(execFn),
		NewMonitorDiskStep(execFn),
		NewMonitorNetworkStep(execFn),
		NewSystemInfoStep(execFn),
		NewHealthCheckStep(execFn),
		NewAppInstallStep(execFn),
		NewAppUninstallStep(execFn),
		NewAppListStep(execFn),
		NewSSLApplyStep(execFn),
		NewSSLRenewStep(execFn),
		NewSSLStatusStep(execFn),
		NewNginxReloadStep(execFn),
		NewNginxStatusStep(execFn),
		NewDatabaseCreateStep(execFn),
		NewDatabaseBackupStep(execFn),
		NewDatabaseListStep(execFn),
		NewDatabaseDeleteStep(execFn),
		NewDatabaseRestoreStep(execFn),
		NewDatabaseSlowQueryStep(execFn),
		NewDatabaseConnectionStep(execFn),
		NewDatabaseOptimizeStep(execFn),
		NewWebsiteCreateStep(execFn),
		NewWebsiteDeleteStep(execFn),
		NewWebsiteListStep(execFn),
		NewWebsiteProxyStep(execFn),
		NewWebsiteDomainStep(execFn),
		NewWebsitePHPConfigStep(execFn),
		NewFileListStep(execFn),
		NewFileReadStep(execFn),
		NewFileUploadStep(execFn),
		NewFileDeleteStep(execFn),
	}

	validLevels := map[string]bool{"low": true, "medium": true, "high": true, "forbidden": true}

	for _, step := range allSteps {
		level := step.RiskLevel()
		if level == "" {
			t.Errorf("%s: 风险等级不应为空", step.Name())
		}
		if !validLevels[level] {
			t.Errorf("%s: 风险等级 '%s' 无效, 应为 low/medium/high/forbidden", step.Name(), level)
		}
	}
}

func TestStepDescriptions(t *testing.T) {
	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "ok", nil
	}

	allSteps := []Step{
		NewContainerListStep(execFn),
		NewContainerStartStep(execFn),
		NewContainerStopStep(execFn),
		NewContainerRestartStep(execFn),
		NewContainerLogsStep(execFn),
		NewMonitorCPUStep(execFn),
		NewMonitorMemoryStep(execFn),
		NewMonitorDiskStep(execFn),
		NewMonitorNetworkStep(execFn),
		NewSystemInfoStep(execFn),
		NewHealthCheckStep(execFn),
		NewAppInstallStep(execFn),
		NewAppUninstallStep(execFn),
		NewAppListStep(execFn),
		NewSSLApplyStep(execFn),
		NewSSLRenewStep(execFn),
		NewSSLStatusStep(execFn),
		NewNginxReloadStep(execFn),
		NewNginxStatusStep(execFn),
		NewDatabaseCreateStep(execFn),
		NewDatabaseBackupStep(execFn),
		NewDatabaseListStep(execFn),
		NewDatabaseDeleteStep(execFn),
		NewDatabaseRestoreStep(execFn),
		NewDatabaseSlowQueryStep(execFn),
		NewDatabaseConnectionStep(execFn),
		NewDatabaseOptimizeStep(execFn),
		NewWebsiteCreateStep(execFn),
		NewWebsiteDeleteStep(execFn),
		NewWebsiteListStep(execFn),
		NewWebsiteProxyStep(execFn),
		NewWebsiteDomainStep(execFn),
		NewWebsitePHPConfigStep(execFn),
		NewFileListStep(execFn),
		NewFileReadStep(execFn),
		NewFileUploadStep(execFn),
		NewFileDeleteStep(execFn),
	}

	for _, step := range allSteps {
		desc := step.Description()
		if desc == "" {
			t.Errorf("%s: 描述不应为空", step.Name())
		}
	}
}
