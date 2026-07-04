package drivers

import (
	"context"
	"testing"
)

func TestStepRegistry_RegisterAndGet(t *testing.T) {
	reg := NewStepRegistry()

	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "ok", nil
	}

	reg.Register(NewContainerListStep(execFn))
	reg.Register(NewMonitorCPUStep(execFn))

	if len(reg.Names()) < 2 {
		t.Errorf("应注册至少2个操作，实际 %d", len(reg.Names()))
	}

	step, ok := reg.Get("container.list")
	if !ok {
		t.Error("应能找到 container.list")
	}
	if step.Name() != "container.list" {
		t.Errorf("名称应为 container.list，实际 %s", step.Name())
	}
	if !step.IsReadOnly() {
		t.Error("container.list 应为只读")
	}
	if step.RiskLevel() != "low" {
		t.Errorf("container.list 应为低风险，实际 %s", step.RiskLevel())
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("不应找到不存在的操作")
	}
}

func TestStepRegistry_Execute(t *testing.T) {
	reg := NewStepRegistry()

	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "result:" + action, nil
	}

	reg.Register(NewContainerListStep(execFn))
	reg.Register(NewMonitorCPUStep(execFn))

	result, err := reg.Execute(context.Background(), "container.list", nil)
	if err != nil {
		t.Errorf("执行失败: %v", err)
	}
	if result != "result:container.list" {
		t.Errorf("结果应为 'result:container.list'，实际 '%s'", result)
	}

	_, err = reg.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("不存在的操作应返回错误")
	}
}

func TestStepRegistry_Rollback(t *testing.T) {
	reg := NewStepRegistry()
	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "ok", nil
	}

	reg.Register(NewContainerStartStep(execFn))
	reg.Register(NewContainerStopStep(execFn))

	// container.start 的回滚应为 container.stop
	rollback := reg.RollbackAction("container.start")
	if rollback != "container.stop" {
		t.Errorf("container.start 回滚应为 container.stop，实际 %s", rollback)
	}

	// 无回滚的操作
	rollback = reg.RollbackAction("container.list")
	if rollback != "" {
		t.Errorf("container.list 不应有回滚，实际 %s", rollback)
	}
}

func TestStepValidation(t *testing.T) {
	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "ok", nil
	}

	// container.stop 需要 container_id
	step := NewContainerStopStep(execFn)
	if err := step.Validate(map[string]string{}); err == nil {
		t.Error("缺少 container_id 应返回错误")
	}
	if err := step.Validate(map[string]string{"container_id": "abc123"}); err != nil {
		t.Errorf("有效参数不应报错: %v", err)
	}

	// app.install 需要 name
	appStep := NewAppInstallStep(execFn)
	if err := appStep.Validate(map[string]string{}); err == nil {
		t.Error("缺少 name 应返回错误")
	}
}

func TestStepAliases(t *testing.T) {
	execFn := func(ctx context.Context, action string, params map[string]string) (string, error) {
		return "ok", nil
	}

	step := NewContainerStartStep(execFn)
	aliases := step.Aliases()

	hasAlias := false
	for _, a := range aliases {
		if a == "启动容器" {
			hasAlias = true
		}
	}
	if !hasAlias {
		t.Error("container.start 应有别名 '启动容器'")
	}
}

func TestAllStepsRegistered(t *testing.T) {
	// 创建完整驱动并验证所有操作可执行
	d := NewOnePanelDriver("http://localhost:9999", "test-key")
	actions := d.AvailableActions()

	// 至少应有 30 个操作
	if len(actions) < 30 {
		t.Errorf("操作数应 >= 30，实际 %d", len(actions))
	}

	// 验证核心类别存在
	categories := map[string]bool{
		"container.list":   false,
		"monitor.cpu":      false,
		"app.install":      false,
		"ssl.apply":        false,
		"nginx.reload":     false,
		"database.backup":  false,
		"website.create":   false,
		"file.list":        false,
		"database.restore": false,
		"website.proxy":    false,
	}

	for _, a := range actions {
		if _, ok := categories[a]; ok {
			categories[a] = true
		}
	}

	for action, found := range categories {
		if !found {
			t.Errorf("操作 %s 未注册", action)
		}
	}
}
