package drivers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SSHDriver SSH 远程执行驱动
type SSHDriver struct {
	target  Target
	connected bool
	timeout time.Duration
}

// NewSSHDriver 创建 SSH 驱动
func NewSSHDriver(timeout time.Duration) *SSHDriver {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &SSHDriver{timeout: timeout}
}

func (d *SSHDriver) Name() string {
	return "ssh"
}

// Connect 连接（验证 SSH 可达性）
func (d *SSHDriver) Connect(ctx context.Context, target Target) error {
	d.target = target
	// 验证 SSH 连接
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-p", fmt.Sprintf("%d", target.Port),
		fmt.Sprintf("%s@%s", target.User, target.Host),
		"echo ok",
	)
	output, err := cmd.Output()
	if err != nil {
		d.connected = false
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	if strings.TrimSpace(string(output)) == "ok" {
		d.connected = true
	}
	return nil
}

func (d *SSHDriver) Disconnect(ctx context.Context) error {
	d.connected = false
	return nil
}

func (d *SSHDriver) IsConnected() bool {
	return d.connected
}

func (d *SSHDriver) Target() Target {
	return d.target
}

// Execute 通过 SSH 执行远程命令
func (d *SSHDriver) Execute(ctx context.Context, action string, params map[string]string) (string, error) {
	if !d.connected {
		return "", fmt.Errorf("SSH 未连接到 %s", d.target.Host)
	}

	// 将 action 映射为远程命令
	remoteCmd := d.mapActionToCommand(action, params)

	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-p", fmt.Sprintf("%d", d.target.Port),
		fmt.Sprintf("%s@%s", d.target.User, d.target.Host),
		remoteCmd,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("远程执行失败: %w", err)
	}
	return string(output), nil
}

// HealthCheck SSH 连接健康检查
func (d *SSHDriver) HealthCheck(ctx context.Context) error {
	if !d.connected {
		return fmt.Errorf("未连接")
	}
	return d.Connect(ctx, d.target)
}

// AvailableActions 返回 SSH 支持的操作
func (d *SSHDriver) AvailableActions() []string {
	return []string{
		"system.info", "monitor.cpu", "monitor.memory", "monitor.disk",
		"container.list", "container.logs", "container.start", "container.stop", "container.restart",
		"nginx.reload", "nginx.status",
	}
}

// RollbackAction 回滚
func (d *SSHDriver) RollbackAction(action string) string {
	rollbacks := map[string]string{
		"container.start": "container.stop",
		"container.stop":  "container.start",
	}
	return rollbacks[action]
}

// Registry 返回空注册表（SSH 驱动不需要）
func (d *SSHDriver) Registry() *StepRegistry {
	return nil
}

// mapActionToCommand 将 action 映射为远程 shell 命令
func (d *SSHDriver) mapActionToCommand(action string, params map[string]string) string {
	commands := map[string]string{
		"system.info":      "uname -a && cat /etc/os-release 2>/dev/null | head -3",
		"monitor.cpu":      "top -bn1 | head -5",
		"monitor.memory":   "free -h",
		"monitor.disk":     "df -h",
		"container.list":   "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'",
		"container.logs":   fmt.Sprintf("docker logs --tail %s %s 2>&1", safeParam(params, "lines", "50"), safeParam(params, "container_id", "")),
		"container.start":  fmt.Sprintf("docker start %s", safeParam(params, "container_id", "")),
		"container.stop":   fmt.Sprintf("docker stop %s", safeParam(params, "container_id", "")),
		"container.restart": fmt.Sprintf("docker restart %s", safeParam(params, "container_id", "")),
		"nginx.reload":     "nginx -s reload 2>&1 || systemctl reload nginx 2>&1",
		"nginx.status":     "systemctl status nginx 2>&1 | head -5",
	}

	if cmd, ok := commands[action]; ok {
		return cmd
	}
	return fmt.Sprintf("echo '未知操作: %s'", action)
}

func safeParam(params map[string]string, key, defaultVal string) string {
	if v, ok := params[key]; ok && v != "" {
		return v
	}
	return defaultVal
}
