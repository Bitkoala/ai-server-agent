package drivers

import (
	"context"
	"fmt"
)

// ============ 容器操作 ============

type ContainerListStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewContainerListStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *ContainerListStep {
	return &ContainerListStep{BaseStep: NewBaseStep("container.list", "列出所有容器", true, "low"), execFn: execFn}
}
func (s *ContainerListStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}

type ContainerStartStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewContainerStartStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *ContainerStartStep {
	return &ContainerStartStep{BaseStep: NewBaseStep("container.start", "启动容器", false, "medium").WithRollback("container.stop").WithAliases("启动容器", "start container"), execFn: execFn}
}
func (s *ContainerStartStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *ContainerStartStep) Validate(params map[string]string) error {
	if params["container_id"] == "" && params["name"] == "" {
		return fmt.Errorf("缺少 container_id 或 name 参数")
	}
	return nil
}

type ContainerStopStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewContainerStopStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *ContainerStopStep {
	return &ContainerStopStep{BaseStep: NewBaseStep("container.stop", "停止容器", false, "high").WithRollback("container.start").WithAliases("停止容器", "stop container"), execFn: execFn}
}
func (s *ContainerStopStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *ContainerStopStep) Validate(params map[string]string) error {
	if params["container_id"] == "" && params["name"] == "" {
		return fmt.Errorf("缺少 container_id 或 name 参数")
	}
	return nil
}

type ContainerRestartStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewContainerRestartStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *ContainerRestartStep {
	return &ContainerRestartStep{BaseStep: NewBaseStep("container.restart", "重启容器", false, "high").WithAliases("重启容器", "restart container"), execFn: execFn}
}
func (s *ContainerRestartStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
func (s *ContainerRestartStep) Validate(params map[string]string) error {
	if params["container_id"] == "" && params["name"] == "" {
		return fmt.Errorf("缺少 container_id 或 name 参数")
	}
	return nil
}

type ContainerLogsStep struct{ *BaseStep; execFn func(ctx context.Context, action string, params map[string]string) (string, error) }
func NewContainerLogsStep(execFn func(ctx context.Context, action string, params map[string]string) (string, error)) *ContainerLogsStep {
	return &ContainerLogsStep{BaseStep: NewBaseStep("container.logs", "查看容器日志", true, "low").WithAliases("查看日志", "容器日志", "logs"), execFn: execFn}
}
func (s *ContainerLogsStep) Execute(ctx context.Context, params map[string]string) (string, error) {
	return s.execFn(ctx, s.Name(), params)
}
