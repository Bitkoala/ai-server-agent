package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ai-server-agent/internal/models"
)

// ============ 执行策略 ============

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries  int           // 最大重试次数，默认 3
	BackoffBase time.Duration // 退避基数，默认 1s
	BackoffMax  time.Duration // 最大退避时间，默认 30s
}

// DefaultRetryPolicy 默认重试策略
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:  3,
		BackoffBase: 1 * time.Second,
		BackoffMax:  30 * time.Second,
	}
}

// ============ 熔断器 ============

// CircuitBreaker 简单的熔断器实现
type CircuitBreaker struct {
	mu            sync.Mutex
	failureCount  int
	successCount  int
	threshold     int           // 连续失败多少次后熔断，默认 5
	halfOpenMax   int           // 半开状态最大成功数，默认 3
	resetTimeout  time.Duration // 熔断后多久进入半开，默认 30s
	state         string        // closed / open / half-open
	lastFailTime  time.Time
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		threshold:    threshold,
		halfOpenMax:  3,
		resetTimeout: resetTimeout,
		state:        "closed",
	}
}

// Call 在熔断器保护下执行操作
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	state := cb.state

	// 熔断状态下检查是否到恢复时间
	if state == "open" {
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.state = "half-open"
			cb.successCount = 0
			state = "half-open"
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("%w (剩余 %v)", models.ErrCircuitBreakerOpen, cb.resetTimeout-time.Since(cb.lastFailTime))
		}
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastFailTime = time.Now()
		if cb.failureCount >= cb.threshold {
			cb.state = "open"
		}
		return err
	}

	// 成功
	cb.failureCount = 0
	if state == "half-open" {
		cb.successCount++
		if cb.successCount >= cb.halfOpenMax {
			cb.state = "closed"
			cb.successCount = 0
		}
	}
	return nil
}

// State 返回熔断器状态
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// ============ 重试执行器 ============

// RetryableFunc 可重试的函数签名
type RetryableFunc func(ctx context.Context) error

// IsRetryable 判断错误是否可重试
type IsRetryable func(err error) bool

// RetryWithBackoff 带指数退避的重试执行
func RetryWithBackoff(ctx context.Context, policy RetryPolicy, fn RetryableFunc, isRetryable IsRetryable) error {
	var lastErr error

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("操作已取消: %w", ctx.Err())
		default:
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// 最后一次尝试不重试
		if attempt == policy.MaxRetries {
			break
		}

		// 判断是否可重试
		if isRetryable != nil && !isRetryable(err) {
			return fmt.Errorf("不可重试的错误: %w", err)
		}

		// 指数退避
		backoff := policy.BackoffBase * time.Duration(1<<attempt)
		if backoff > policy.BackoffMax {
			backoff = policy.BackoffMax
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("重试等待期间操作被取消: %w", ctx.Err())
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("%w (已重试 %d 次): %w", models.ErrMaxRetriesExceeded, policy.MaxRetries, lastErr)
}

// ============ 超时执行器 ============

// ErrTimeout 超时错误
var ErrTimeout = errors.New("操作超时")

// ExecuteWithTimeout 带超时的执行
func ExecuteWithTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrTimeout
		}
		return ctx.Err()
	}
}

// ============ 回滚器 ============

// RollbackFunc 回滚函数
type RollbackFunc func(ctx context.Context) error

// RollbackStack 回滚栈（后进先出）
type RollbackStack struct {
	mu    sync.Mutex
	stack []RollbackFunc
}

// NewRollbackStack 创建回滚栈
func NewRollbackStack() *RollbackStack {
	return &RollbackStack{}
}

// Push 压入回滚操作
func (rs *RollbackStack) Push(fn RollbackFunc) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.stack = append(rs.stack, fn)
}

// Rollback 执行回滚（后进先出），返回所有回滚错误
func (rs *RollbackStack) Rollback(ctx context.Context) []error {
	rs.mu.Lock()
	stack := rs.stack
	rs.stack = nil
	rs.mu.Unlock()

	var errs []error
	// 后进先出
	for i := len(stack) - 1; i >= 0; i-- {
		if err := stack[i](ctx); err != nil {
			errs = append(errs, fmt.Errorf("回滚步骤 %d 失败: %w", i, err))
		}
	}
	return errs
}

// ============ 默认的可重试判断 ============

// DefaultIsRetryable 默认重试判断：网络错误、超时等可重试
func DefaultIsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrTimeout) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// 一般网络/API 错误可重试
	return true
}
