package security

import (
	"testing"
)

func TestRiskLevels(t *testing.T) {
	sg := NewSafeGuard(Config{
		DangerousOpsRequireConfirm: true,
		RateLimitPerMinute:         100,
	})

	tests := []struct {
		action   string
		expected string // low/medium/high/forbidden
	}{
		{"container.list", "low"},
		{"monitor.cpu", "low"},
		{"system.info", "low"},
		{"container.logs", "low"},
		{"nginx.status", "low"},
		{"app.install", "medium"},
		{"container.start", "medium"},
		{"ssl.apply", "medium"},
		{"database.backup", "medium"},
		{"nginx.reload", "medium"},
		{"container.stop", "high"},
		{"container.restart", "high"},
		{"app.uninstall", "high"},
		{"database.delete", "high"},
		{"website.delete", "high"},
		{"system.restart", "high"},
		{"system.factory_reset", "forbidden"},
		{"disk.format", "forbidden"},
		{"unknown.action", "high"}, // 未知操作默认高风险
	}

	for _, tt := range tests {
		risk := sg.AssessRisk(tt.action)
		riskStr := "low"
		switch risk {
		case RiskMedium:
			riskStr = "medium"
		case RiskHigh:
			riskStr = "high"
		case RiskForbidden:
			riskStr = "forbidden"
		}
		if riskStr != tt.expected {
			t.Errorf("操作 %s 风险等级应为 %s，实际为 %s", tt.action, tt.expected, riskStr)
		}
	}
}

func TestDangerousOpsNeedConfirm(t *testing.T) {
	sg := NewSafeGuard(Config{
		DangerousOpsRequireConfirm: true,
		RateLimitPerMinute:         100,
	})

	if sg.IsDangerous("container.list") {
		t.Error("container.list 不应标记为危险操作")
	}
	if !sg.IsDangerous("container.stop") {
		t.Error("container.stop 应标记为危险操作")
	}
	if sg.IsDangerous("monitor.cpu") {
		t.Error("monitor.cpu 不应标记为危险操作")
	}
}

func TestForbiddenOps(t *testing.T) {
	sg := NewSafeGuard(Config{
		DangerousOpsRequireConfirm: true,
		RateLimitPerMinute:         100,
	})

	if !sg.IsForbidden("system.factory_reset") {
		t.Error("system.factory_reset 应被禁止")
	}
	if sg.IsForbidden("container.list") {
		t.Error("container.list 不应被禁止")
	}
}

func TestAutoApproved(t *testing.T) {
	sg := NewSafeGuard(Config{
		DangerousOpsRequireConfirm: true,
		RateLimitPerMinute:         100,
	})

	autoActions := []string{"monitor.cpu", "monitor.memory", "container.list", "system.info", "health"}
	for _, a := range autoActions {
		if !sg.IsAutoApproved(a) {
			t.Errorf("%s 应自动通过", a)
		}
	}
}

func TestInputValidation(t *testing.T) {
	sg := NewSafeGuard(Config{
		DangerousOpsRequireConfirm: true,
		RateLimitPerMinute:         100,
	})

	// 危险指令应被拦截
	dangerous := []string{"rm -rf /", "drop table users", "shutdown -h now", "mkfs /dev/sda"}
	for _, d := range dangerous {
		if err := sg.ValidateInput(d); err == nil {
			t.Errorf("危险指令应被拦截: %s", d)
		}
	}

	// 正常指令应通过
	safe := []string{"查看CPU", "部署WordPress", "重启nginx"}
	for _, s := range safe {
		if err := sg.ValidateInput(s); err != nil {
			t.Errorf("正常指令不应被拦截: %s (错误: %v)", s, err)
		}
	}
}

func TestRateLimit(t *testing.T) {
	sg := NewSafeGuard(Config{
		DangerousOpsRequireConfirm: true,
		RateLimitPerMinute:         3,
	})

	// 前3次应通过
	for i := 0; i < 3; i++ {
		if err := sg.ValidateInput("test"); err != nil {
			t.Errorf("第 %d 次请求不应被限流: %v", i+1, err)
		}
	}

	// 第4次应被限流
	if err := sg.ValidateInput("test"); err == nil {
		t.Error("超过限制的请求应被拦截")
	}
}
