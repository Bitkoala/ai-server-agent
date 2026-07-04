package core

import (
	"context"
	"fmt"
	"time"

	"github.com/ai-server-agent/internal/drivers"
	"github.com/ai-server-agent/internal/models"
	"github.com/ai-server-agent/internal/notify"
	"github.com/ai-server-agent/internal/security"
	"github.com/ai-server-agent/internal/storage"
)

// EngineConfig 引擎配置
type EngineConfig struct {
	Driver      drivers.Driver
	Storage     *storage.SQLiteStore
	SafeGuard   *security.SafeGuard
	LLMClient   IntentParser
	AuditLogger *storage.AuditLogger
	Notifier    *notify.Notifier
	RetryPolicy RetryPolicy
	StepTimeout         time.Duration
	ConfirmationTimeout time.Duration // 二次确认超时（超时自动取消）
}

// DefaultEngineConfig 默认引擎配置
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		RetryPolicy:         DefaultRetryPolicy(),
		StepTimeout:         60 * time.Second,
		ConfirmationTimeout: 5 * time.Minute, // 5 分钟未确认自动取消
	}
}

// IntentParser LLM 意图解析接口（避免循环依赖）
type IntentParser interface {
	ParseIntent(ctx context.Context, userInput string) (string, []models.TaskStep, error)
}

// Engine 核心引擎：意图解析 + 任务编排 + 执行调度
type Engine struct {
	driver              drivers.Driver
	store               *storage.SQLiteStore
	safeGuard           *security.SafeGuard
	llm                 IntentParser
	auditLogger         *storage.AuditLogger
	notifier            *notify.Notifier
	retryPolicy         RetryPolicy
	stepTimeout         time.Duration
	confirmationTimeout time.Duration
	cb                  *CircuitBreaker
}

// NewEngine 创建引擎实例
func NewEngine(cfg EngineConfig) *Engine {
	if cfg.StepTimeout <= 0 {
		cfg.StepTimeout = 60 * time.Second
	}
	if cfg.RetryPolicy.MaxRetries == 0 {
		cfg.RetryPolicy = DefaultRetryPolicy()
	}
	if cfg.ConfirmationTimeout <= 0 {
		cfg.ConfirmationTimeout = 5 * time.Minute
	}
	return &Engine{
		driver:              cfg.Driver,
		store:               cfg.Storage,
		safeGuard:           cfg.SafeGuard,
		llm:                 cfg.LLMClient,
		auditLogger:         cfg.AuditLogger,
		notifier:            cfg.Notifier,
		retryPolicy:         cfg.RetryPolicy,
		stepTimeout:         cfg.StepTimeout,
		confirmationTimeout: cfg.ConfirmationTimeout,
		cb:                  NewCircuitBreaker(5, 30*time.Second),
	}
}

// Execute 执行用户自然语言指令
func (e *Engine) Execute(ctx context.Context, userInput string) (*models.Task, error) {
	// 1. 安全校验
	if err := e.safeGuard.ValidateInput(userInput); err != nil {
		return nil, fmt.Errorf("安全校验失败: %w", err)
	}

	// 2. 意图解析：先用本地模板匹配，未命中则调 LLM
	intent, steps, err := e.parseIntent(ctx, userInput)
	if err != nil {
		return nil, fmt.Errorf("意图解析失败: %w", err)
	}

	// 3. 创建任务
	task := &models.Task{
		ID:        generateTaskID(),
		Intent:    intent,
		UserInput: userInput,
		Steps:     steps,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	// 4. 安全检查：风险分级标记
	hasDangerous := false
	hasMediumRisk := false
	for i, step := range task.Steps {
		risk := e.safeGuard.AssessRisk(step.Action)
		switch risk {
		case security.RiskForbidden:
			return nil, fmt.Errorf("%w: %s", models.ErrOperationForbidden, step.Action)
		case security.RiskHigh:
			task.Steps[i].Status = "confirm_required"
			hasDangerous = true
		case security.RiskMedium:
			task.Steps[i].Status = "preview"
			hasMediumRisk = true
		case security.RiskLow:
			// 低风险自动放行
		}
	}
	_ = hasMediumRisk

	// 无危险步骤则自动确认
	if !hasDangerous {
		task.Status = "auto_confirmed"
	}

	// 5. 保存任务
	if err := e.store.SaveTask(task); err != nil {
		return nil, fmt.Errorf("保存任务失败: %w", err)
	}

	return task, nil
}

// ConfirmAndRun 用户确认后执行任务（带超时/熔断/重试/回滚）
func (e *Engine) ConfirmAndRun(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := e.store.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}

	if task.Status == "done" {
		return task, nil
	}

	// 检查确认超时：如果任务已过期，自动取消
	if task.Status == "awaiting_confirmation" || task.Status == "pending" {
		if time.Since(task.CreatedAt) > e.confirmationTimeout {
			task.Status = "cancelled"
			e.store.UpdateTask(task)
			return task, fmt.Errorf("确认超时，任务已自动取消")
		}
	}

	task.Status = "running"
	e.store.UpdateTask(task)

	// 回滚栈
	rollback := NewRollbackStack()

	// 按拓扑序执行步骤
	for i := range task.Steps {
		step := &task.Steps[i]
		if step.Status == "done" {
			continue
		}

		// 跳过需要确认但未确认的步骤
		if step.Status == "confirm_required" {
			task.Status = "awaiting_confirmation"
			e.store.UpdateTask(task)
			return task, fmt.Errorf("%w: %s", models.ErrStepNeedsConfirmation, step.ID)
		}

		step.Status = "running"
		e.store.UpdateTask(task)

		// 审计：记录操作开始
		stepStartTime := time.Now()

		// 带超时 + 熔断 + 重试执行
		var stepResult string
		stepErr := e.cb.Call(func() error {
			return RetryWithBackoff(ctx, e.retryPolicy, func(ctx context.Context) error {
				return ExecuteWithTimeout(ctx, e.stepTimeout, func(ctx context.Context) error {
					result, execErr := e.driver.Execute(ctx, step.Action, step.Params)
					if execErr == nil {
						step.Result = result
						stepResult = result
					}
					return execErr
				})
			}, DefaultIsRetryable)
		})

		durationMs := time.Since(stepStartTime).Milliseconds()

		// 审计：记录操作结果
		if e.auditLogger != nil {
			e.auditLogger.LogOperation(task.ID, step.ID, step.Action, step.Params, stepResult, stepErr, durationMs)
		}

		if stepErr != nil {
			step.Status = "failed"
			step.Error = stepErr.Error()
			task.Status = "failed"
			e.store.UpdateTask(task)

			// 执行回滚
			rollbackErrs := rollback.Rollback(ctx)
			if len(rollbackErrs) > 0 {
				step.Error += fmt.Sprintf(" | 回滚错误: %v", rollbackErrs)
			}

			if e.notifier != nil {
			e.notifier.NotifyTaskComplete(ctx, task.ID, task.Intent, false, stepErr.Error())
		}

		return task, fmt.Errorf("步骤 %s (%s) 执行失败: %w", step.ID, step.Action, stepErr)
		}

		// 注册回滚（如果驱动支持）
		if rollbackAction := e.driver.RollbackAction(step.Action); rollbackAction != "" {
			rollback.Push(func(rctx context.Context) error {
				_, rerr := e.driver.Execute(rctx, rollbackAction, step.Params)
				return rerr
			})
		}

		step.Status = "done"
		e.store.UpdateTask(task)
	}

	task.Status = "done"
	e.store.UpdateTask(task)

	// 通知：任务完成
	if e.notifier != nil {
		e.notifier.NotifyTaskComplete(ctx, task.ID, task.Intent, true, fmt.Sprintf("%d 个步骤全部完成", len(task.Steps)))
	}

	return task, nil
}

// parseIntent 意图解析：本地模板 + LLM 兜底
func (e *Engine) parseIntent(ctx context.Context, input string) (string, []models.TaskStep, error) {
	// 先尝试本地模板匹配（覆盖 80% 高频操作）
	if intent, steps := matchLocalTemplate(input); intent != "" {
		return intent, steps, nil
	}

	// 未命中则调 LLM
	return e.llm.ParseIntent(ctx, input)
}

func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}
