package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-server-agent/internal/llm"
	"github.com/ai-server-agent/internal/models"
	"github.com/ai-server-agent/internal/security"
	"github.com/ai-server-agent/internal/storage"
)

// ============ Tracking Mock Driver ============

// trackingMockDriver wraps the existing mockDriver with execution tracking.
type trackingMockDriver struct {
	*mockDriver
	mu              sync.Mutex
	executedActions []string
	executedParams  []map[string]string
}

func newTrackingMockDriver(name string) *trackingMockDriver {
	md := &mockDriver{name: name}
	tmd := &trackingMockDriver{
		mockDriver:      md,
		executedActions: make([]string, 0),
		executedParams:  make([]map[string]string, 0),
	}

	// Override the execute function to track calls
	md.executeFunc = func(ctx context.Context, action string, params map[string]string) (string, error) {
		tmd.mu.Lock()
		tmd.executedActions = append(tmd.executedActions, action)
		tmd.executedParams = append(tmd.executedParams, params)
		tmd.mu.Unlock()
		return "ok", nil
	}

	return tmd
}

func (t *trackingMockDriver) ExecutedActions() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]string, len(t.executedActions))
	copy(result, t.executedActions)
	return result
}

func (t *trackingMockDriver) ExecutedParams() []map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]map[string]string, len(t.executedParams))
	copy(result, t.executedParams)
	return result
}

// ============ Mock LLM Client ============

// mockIntentParser implements the IntentParser interface for testing.
type mockIntentParser struct {
	parseIntentFunc func(ctx context.Context, userInput string) (string, []models.TaskStep, error)
}

func (m *mockIntentParser) ParseIntent(ctx context.Context, userInput string) (string, []models.TaskStep, error) {
	if m.parseIntentFunc != nil {
		return m.parseIntentFunc(ctx, userInput)
	}
	return "unknown", nil, errors.New("mock: not implemented")
}

// ============ Helper Functions ============

// setupIntegrationEngine creates a test Engine with mock components.
func setupIntegrationEngine(t *testing.T, drv *trackingMockDriver, llmParser IntentParser, sg *security.SafeGuard, store *storage.SQLiteStore) *Engine {
	t.Helper()

	cfg := DefaultEngineConfig()
	cfg.Driver = drv
	cfg.Storage = store
	cfg.SafeGuard = sg
	cfg.LLMClient = llmParser
	cfg.StepTimeout = 5 * time.Second
	cfg.ConfirmationTimeout = 5 * time.Minute

	return NewEngine(cfg)
}

// newTestStore creates an in-memory SQLite store for testing.
func newIntegrationStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// ============ Test 1: TemplateMatchToExecution ============

func TestIntegration_TemplateMatchToExecution(t *testing.T) {
	store := newIntegrationStore(t)
	sg := newTestSafeGuard()
	drv := newTrackingMockDriver("mock")

	llmClient := llm.NewClient(llm.Config{Provider: "local"})
	engine := setupIntegrationEngine(t, drv, llmClient, sg, store)

	ctx := context.Background()
	task, err := engine.Execute(ctx, "查看CPU使用率")
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	// Verify task was created with correct intent and steps
	if task.Status != "auto_confirmed" {
		t.Errorf("期望状态 auto_confirmed, 实际 %s", task.Status)
	}
	if task.Intent != "查看 CPU 使用率" {
		t.Errorf("期望意图 '查看 CPU 使用率', 实际 '%s'", task.Intent)
	}
	if len(task.Steps) != 1 {
		t.Fatalf("期望 1 个步骤, 实际 %d", len(task.Steps))
	}
	if task.Steps[0].Action != "monitor.cpu" {
		t.Errorf("期望 action 'monitor.cpu', 实际 '%s'", task.Steps[0].Action)
	}

	// Run the auto-confirmed task
	completedTask, err := engine.ConfirmAndRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("ConfirmAndRun 失败: %v", err)
	}

	// Verify execution completed
	if completedTask.Status != "done" {
		t.Errorf("期望状态 done, 实际 %s", completedTask.Status)
	}

	actions := drv.ExecutedActions()
	if len(actions) != 1 {
		t.Errorf("期望执行 1 个动作, 实际 %d", len(actions))
	}
	if len(actions) > 0 && actions[0] != "monitor.cpu" {
		t.Errorf("期望执行 'monitor.cpu', 实际 '%s'", actions[0])
	}

	// Verify step result was captured
	if completedTask.Steps[0].Status != "done" {
		t.Errorf("期望步骤状态 done, 实际 %s", completedTask.Steps[0].Status)
	}
}

// ============ Test 2: ConfirmRequiredFlow ============

func TestIntegration_ConfirmRequiredFlow(t *testing.T) {
	store := newIntegrationStore(t)
	sg := newTestSafeGuard()
	drv := newTrackingMockDriver("mock")

	llmClient := llm.NewClient(llm.Config{Provider: "local"})
	engine := setupIntegrationEngine(t, drv, llmClient, sg, store)

	ctx := context.Background()

	// "重启容器" matches template with confirm_required status
	task, err := engine.Execute(ctx, "重启容器")
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	// Task should NOT be auto_confirmed because restart is high risk
	if task.Status == "auto_confirmed" {
		t.Error("高风险操作不应自动确认")
	}

	// Verify step has confirm_required status
	if len(task.Steps) != 1 {
		t.Fatalf("期望 1 个步骤, 实际 %d", len(task.Steps))
	}
	if task.Steps[0].Status != "confirm_required" {
		t.Errorf("期望步骤状态 confirm_required, 实际 %s", task.Steps[0].Status)
	}

	// ConfirmAndRun should detect the confirm_required step and stop
	completedTask, err := engine.ConfirmAndRun(ctx, task.ID)
	if err != nil {
		if !errors.Is(err, models.ErrStepNeedsConfirmation) {
			t.Errorf("期望 ErrStepNeedsConfirmation, 实际 %v", err)
		}
		if completedTask.Status != "awaiting_confirmation" {
			t.Errorf("期望状态 awaiting_confirmation, 实际 %s", completedTask.Status)
		}
	} else {
		t.Fatal("ConfirmAndRun 应返回 ErrStepNeedsConfirmation")
	}

	// Verify no actions were executed yet (all steps are confirm_required)
	actions := drv.ExecutedActions()
	if len(actions) != 0 {
		t.Errorf("确认前不应执行任何动作, 实际执行了 %d: %v", len(actions), actions)
	}

	// Now simulate user confirmation: set step to pending
	reloadedTask, err := store.GetTask(task.ID)
	if err != nil {
		t.Fatalf("获取任务失败: %v", err)
	}
	reloadedTask.Steps[0].Status = "pending"
	reloadedTask.Status = "pending"
	store.UpdateTask(reloadedTask)

	// ConfirmAndRun again -> should execute
	finalTask, err := engine.ConfirmAndRun(ctx, reloadedTask.ID)
	if err != nil {
		t.Fatalf("ConfirmAndRun 失败: %v", err)
	}

	if finalTask.Status != "done" {
		t.Errorf("期望状态 done, 实际 %s", finalTask.Status)
	}

	actions = drv.ExecutedActions()
	if len(actions) != 1 {
		t.Errorf("期望执行 1 个动作, 实际 %d", len(actions))
	}
	if len(actions) > 0 && actions[0] != "container.restart" {
		t.Errorf("期望执行 'container.restart', 实际 '%s'", actions[0])
	}
}

// ============ Test 3: ForbiddenAction ============

func TestIntegration_ForbiddenAction(t *testing.T) {
	store := newIntegrationStore(t)
	sg := newTestSafeGuard()
	drv := newTrackingMockDriver("mock")
	llmClient := llm.NewClient(llm.Config{Provider: "local"})
	engine := setupIntegrationEngine(t, drv, llmClient, sg, store)

	ctx := context.Background()

	// "rm -rf /" should be blocked by security check at input level
	_, err := engine.Execute(ctx, "rm -rf /")
	if err == nil {
		t.Fatal("期望 Execute 返回错误（安全拦截）")
	}
	if !strings.Contains(err.Error(), "安全校验失败") {
		t.Errorf("期望错误包含 '安全校验失败', 实际: %v", err)
	}

	// Also test forbidden action via the risk assessment system
	// Use a mock parser that returns a forbidden action
	forbiddenParser := &mockIntentParser{
		parseIntentFunc: func(ctx context.Context, userInput string) (string, []models.TaskStep, error) {
			return "危险操作", []models.TaskStep{
				{ID: "s1", Action: "system.factory_reset", Status: "pending"},
			}, nil
		},
	}
	forbiddenEngine := setupIntegrationEngine(t, newTrackingMockDriver("mock2"), forbiddenParser, sg, store)

	_, err = forbiddenEngine.Execute(ctx, "重置系统")
	if err == nil {
		t.Fatal("期望 forbidden 操作被拦截")
	}
	if !errors.Is(err, models.ErrOperationForbidden) {
		t.Errorf("期望 ErrOperationForbidden, 实际 %v", err)
	}
}

// ============ Test 4: LLMFallback ============

func TestIntegration_LLMFallback(t *testing.T) {
	store := newIntegrationStore(t)
	sg := newTestSafeGuard()
	drv := newTrackingMockDriver("mock")

	// Input that doesn't match any template: "帮我把nginx的keepalive_timeout改成60秒"
	// The engine's matchLocalTemplate won't match it.
	// The LLM client's matchLocalTemplate also won't match it.
	// The LLM provider will fail (no real LLM), falling back to classifyIntent + generateSteps.
	llmClient := llm.NewClient(llm.Config{Provider: "local"})
	engine := setupIntegrationEngine(t, drv, llmClient, sg, store)

	ctx := context.Background()

	task, err := engine.Execute(ctx, "帮我把nginx的keepalive_timeout改成60秒")
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	// Verify task was created with steps from the fallback path
	if task.ID == "" {
		t.Error("任务 ID 不应为空")
	}
	if task.Status == "" {
		t.Error("任务状态不应为空")
	}
	if len(task.Steps) == 0 {
		t.Error("即使没有模板匹配，也应通过 LLM fallback 生成步骤")
	}

	t.Logf("LLM fallback 结果: intent=%s, steps=%d, status=%s", task.Intent, len(task.Steps), task.Status)

	// Verify each step has valid fields
	for i, step := range task.Steps {
		if step.ID == "" {
			t.Errorf("步骤 %d: ID 不应为空", i)
		}
		if step.Action == "" {
			t.Errorf("步骤 %d: Action 不应为空", i)
		}
	}
}

// ============ Test 5: ConfirmTimeout ============

func TestIntegration_ConfirmTimeout(t *testing.T) {
	store := newIntegrationStore(t)
	sg := newTestSafeGuard()
	drv := newTrackingMockDriver("mock")

	llmClient := llm.NewClient(llm.Config{Provider: "local"})

	// Create engine with a very short confirmation timeout
	cfg := DefaultEngineConfig()
	cfg.Driver = drv
	cfg.Storage = store
	cfg.SafeGuard = sg
	cfg.LLMClient = llmClient
	cfg.StepTimeout = 5 * time.Second
	cfg.ConfirmationTimeout = 1 * time.Millisecond

	engine := NewEngine(cfg)

	ctx := context.Background()

	// Execute a high-risk action
	task, err := engine.Execute(ctx, "重启容器")
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	// Set task status to awaiting_confirmation to simulate waiting state
	task.Status = "awaiting_confirmation"
	store.UpdateTask(task)

	// Wait past the confirmation timeout
	time.Sleep(10 * time.Millisecond)

	// Try to confirm - should fail with timeout
	_, err = engine.ConfirmAndRun(ctx, task.ID)
	if err == nil {
		t.Fatal("期望超时错误")
	}
	if !strings.Contains(err.Error(), "确认超时") {
		t.Errorf("期望错误包含 '确认超时', 实际: %v", err)
	}

	// Verify task was cancelled
	cancelledTask, err := store.GetTask(task.ID)
	if err != nil {
		t.Fatalf("获取任务失败: %v", err)
	}
	if cancelledTask.Status != "cancelled" {
		t.Errorf("期望状态 cancelled, 实际 %s", cancelledTask.Status)
	}
}

// ============ Test 6: MultiStepExecution ============

func TestIntegration_MultiStepExecution(t *testing.T) {
	store := newIntegrationStore(t)
	sg := newTestSafeGuard()
	drv := newTrackingMockDriver("mock")

	llmClient := llm.NewClient(llm.Config{Provider: "local"})
	engine := setupIntegrationEngine(t, drv, llmClient, sg, store)

	ctx := context.Background()

	// "系统监控" should match multi-step template (cpu + memory + disk)
	task, err := engine.Execute(ctx, "系统监控")
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}

	// Verify task has 3 steps
	if len(task.Steps) != 3 {
		t.Fatalf("期望 3 个步骤, 实际 %d", len(task.Steps))
	}

	expectedActions := []string{"monitor.cpu", "monitor.memory", "monitor.disk"}
	for i, step := range task.Steps {
		if step.Action != expectedActions[i] {
			t.Errorf("步骤 %d: 期望 action '%s', 实际 '%s'", i, expectedActions[i], step.Action)
		}
	}

	// All steps are low risk, so task should be auto_confirmed
	if task.Status != "auto_confirmed" {
		t.Errorf("期望状态 auto_confirmed, 实际 %s", task.Status)
	}

	// Run the task
	completedTask, err := engine.ConfirmAndRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("ConfirmAndRun 失败: %v", err)
	}

	// Verify all steps executed
	if completedTask.Status != "done" {
		t.Errorf("期望状态 done, 实际 %s", completedTask.Status)
	}

	executedActions := drv.ExecutedActions()
	if len(executedActions) != 3 {
		t.Errorf("期望执行 3 个动作, 实际 %d: %v", len(executedActions), executedActions)
	}

	for i, expected := range expectedActions {
		if i >= len(executedActions) {
			t.Errorf("缺少第 %d 个动作 '%s'", i, expected)
			continue
		}
		if executedActions[i] != expected {
			t.Errorf("第 %d 个动作: 期望 '%s', 实际 '%s'", i, expected, executedActions[i])
		}
	}

	// Verify all steps completed
	for i, step := range completedTask.Steps {
		if step.Status != "done" {
			t.Errorf("步骤 %d (%s): 期望状态 done, 实际 %s", i, step.Action, step.Status)
		}
	}
}
