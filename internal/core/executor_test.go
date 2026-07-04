package core

import (
	"context"
	"testing"
	"time"
)

func TestRetryPolicyDefaults(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxRetries != 3 {
		t.Errorf("默认重试次数应为3，实际 %d", p.MaxRetries)
	}
	if p.BackoffBase != 1*time.Second {
		t.Errorf("默认退避基数为1s，实际 %v", p.BackoffBase)
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	if cb.State() != "closed" {
		t.Errorf("初始状态应为 closed，实际 %s", cb.State())
	}

	// 2次失败触发熔断
	cb.Call(func() error { return &testErr{"fail"} })
	cb.Call(func() error { return &testErr{"fail"} })

	if cb.State() != "open" {
		t.Errorf("2次失败后应为 open，实际 %s", cb.State())
	}

	// 熔断状态拒绝
	err := cb.Call(func() error { return nil })
	if err == nil {
		t.Error("熔断状态应拒绝请求")
	}
}

func TestCircuitBreaker_Recovery(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Millisecond)

	cb.Call(func() error { return &testErr{"fail"} })
	cb.Call(func() error { return &testErr{"fail"} })

	// 等待进入半开
	time.Sleep(20 * time.Millisecond)

	// 半开后连续成功恢复
	for i := 0; i < 3; i++ {
		err := cb.Call(func() error { return nil })
		if err != nil {
			t.Errorf("半开状态应允许请求: %v", err)
		}
	}

	if cb.State() != "closed" {
		t.Errorf("应恢复为 closed，实际 %s", cb.State())
	}
}

func TestRollbackStack_Order(t *testing.T) {
	rs := NewRollbackStack()
	order := []string{}

	rs.Push(func(ctx context.Context) error { order = append(order, "c"); return nil })
	rs.Push(func(ctx context.Context) error { order = append(order, "b"); return nil })
	rs.Push(func(ctx context.Context) error { order = append(order, "a"); return nil })

	rs.Rollback(nil)

	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("回滚顺序应为 a→b→c (LIFO)，实际 %v", order)
	}
}

func TestDefaultIsRetryable(t *testing.T) {
	if !DefaultIsRetryable(&testErr{"network error"}) {
		t.Error("一般错误应可重试")
	}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
