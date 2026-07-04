package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ai-server-agent/internal/drivers"
	"github.com/ai-server-agent/internal/security"
	"github.com/ai-server-agent/internal/storage"
)

// mockDriver implements drivers.Driver for testing.
type mockDriver struct {
	executeFunc       func(ctx context.Context, action string, params map[string]string) (string, error)
	availableActions  []string
	healthCheckFunc   func(ctx context.Context) error
	rollbackActionMap map[string]string
	registry          *drivers.StepRegistry
	name              string
}

func (m *mockDriver) Execute(ctx context.Context, action string, params map[string]string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, action, params)
	}
	return "ok", nil
}

func (m *mockDriver) AvailableActions() []string {
	if m.availableActions != nil {
		return m.availableActions
	}
	return []string{"monitor.cpu", "monitor.memory", "container.start", "container.stop"}
}

func (m *mockDriver) HealthCheck(ctx context.Context) error {
	if m.healthCheckFunc != nil {
		return m.healthCheckFunc(ctx)
	}
	return nil
}

func (m *mockDriver) RollbackAction(action string) string {
	if m.rollbackActionMap != nil {
		return m.rollbackActionMap[action]
	}
	return ""
}

func (m *mockDriver) Registry() *drivers.StepRegistry {
	if m.registry != nil {
		return m.registry
	}
	return drivers.NewStepRegistry()
}

func (m *mockDriver) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

// newTestSafeGuard creates a SafeGuard with rate limiting disabled for tests.
func newTestSafeGuard() *security.SafeGuard {
	return security.NewSafeGuard(security.Config{RateLimitPerMinute: 1000000})
}

func TestEngineConfigDefaults(t *testing.T) {
	cfg := DefaultEngineConfig()
	if cfg.StepTimeout != 60*time.Second {
		t.Errorf("expected StepTimeout 60s, got %v", cfg.StepTimeout)
	}
	if cfg.RetryPolicy.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.RetryPolicy.MaxRetries)
	}
}

func TestNewEngine(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	engine := NewEngine(EngineConfig{
		Storage:   store,
		SafeGuard: newTestSafeGuard(),
		Driver:    &mockDriver{},
	})
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestExecute_InvalidInput(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	engine := NewEngine(EngineConfig{
		Storage:   store,
		SafeGuard: newTestSafeGuard(),
		Driver:    &mockDriver{},
	})

	tests := []string{
		"rm -rf / --no-preserve-root",
		"DROP TABLE users; --",
		"truncate table logs",
	}
	for _, input := range tests {
		_, err := engine.Execute(context.Background(), input)
		if err == nil {
			t.Errorf("expected error for malicious input: %s", input)
		}
		if !strings.Contains(err.Error(), "安全校验失败") {
			t.Errorf("expected 安全校验失败 error, got: %v", err)
		}
	}
}

func TestExecute_TemplateMatch(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	engine := NewEngine(EngineConfig{
		Storage:   store,
		SafeGuard: newTestSafeGuard(),
		Driver:    &mockDriver{},
	})

	task, err := engine.Execute(context.Background(), "查看CPU使用率")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Intent != "查看 CPU 使用率" {
		t.Errorf("expected intent '查看 CPU 使用率', got '%s'", task.Intent)
	}
	if len(task.Steps) == 0 || task.Steps[0].Action != "monitor.cpu" {
		t.Errorf("expected step monitor.cpu, got %v", task.Steps)
	}
}

func TestExecute_RiskAssessment(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	engine := NewEngine(EngineConfig{
		Storage:   store,
		SafeGuard: newTestSafeGuard(),
		Driver:    &mockDriver{},
	})

	// "停止容器" template has confirm_required step
	task, err := engine.Execute(context.Background(), "停止容器")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hasConfirmRequired := false
	for _, step := range task.Steps {
		if step.Status == "confirm_required" {
			hasConfirmRequired = true
		}
	}
	if !hasConfirmRequired {
		t.Errorf("expected at least one confirm_required step for high-risk operation")
	}
}

func TestExecute_ForbiddenAction(t *testing.T) {
	sg := newTestSafeGuard()

	tests := []struct {
		action string
		risk   security.RiskLevel
	}{
		{"system.factory_reset", security.RiskForbidden},
		{"disk.format", security.RiskForbidden},
	}
	for _, tt := range tests {
		risk := sg.AssessRisk(tt.action)
		if risk != tt.risk {
			t.Errorf("action %s: expected risk %d, got %d", tt.action, tt.risk, risk)
		}
	}
}

func TestConfirmAndRun_TaskNotFound(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	engine := NewEngine(EngineConfig{
		Storage:   store,
		SafeGuard: newTestSafeGuard(),
		Driver:    &mockDriver{},
	})

	_, err = engine.ConfirmAndRun(context.Background(), "nonexistent_task")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestConfirmAndRun_AlreadyDone(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	engine := NewEngine(EngineConfig{
		Storage:   store,
		SafeGuard: newTestSafeGuard(),
		Driver:    &mockDriver{},
	})

	task, err := engine.Execute(context.Background(), "查看CPU使用率")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task.Status = "done"
	store.UpdateTask(task)

	result, err := engine.ConfirmAndRun(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "done" {
		t.Errorf("expected status done, got %s", result.Status)
	}
}

func TestGenerateTaskID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateTaskID()
		if ids[id] {
			t.Errorf("duplicate task ID generated: %s", id)
		}
		ids[id] = true
		time.Sleep(time.Microsecond) // ensure different nanosecond
	}
}
