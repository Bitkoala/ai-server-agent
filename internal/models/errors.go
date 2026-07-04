package models

import "errors"

// ============ 安全相关错误 ============

// ErrDangerousCommand 危险指令被拦截
var ErrDangerousCommand = errors.New("检测到危险指令，已拦截")

// ErrRateLimitExceeded 请求频率限制
var ErrRateLimitExceeded = errors.New("请求过于频繁，请稍后再试")

// ErrOperationForbidden 操作被安全策略禁止
var ErrOperationForbidden = errors.New("操作被安全策略禁止")

// ============ 认证相关错误 ============

// ErrInvalidCredentials 无效的凭据
var ErrInvalidCredentials = errors.New("密码错误")

// ErrUserNotFound 用户不存在
var ErrUserNotFound = errors.New("用户不存在")

// ErrTokenInvalid Token 无效
var ErrTokenInvalid = errors.New("token 无效")

// ErrTokenExpired Token 已过期（可通过 errors.Is 判断）
var ErrTokenExpired = errors.New("token 已过期")

// ErrUnsupportedSignMethod 不支持的签名方法
var ErrUnsupportedSignMethod = errors.New("不支持的签名方法")

// ErrPasswordTooShort 密码太短
var ErrPasswordTooShort = errors.New("密码长度至少 6 位")

// ============ 执行相关错误 ============

// ErrStepNeedsConfirmation 步骤需要用户确认
var ErrStepNeedsConfirmation = errors.New("步骤需要确认")

// ErrCircuitBreakerOpen 熔断器已打开
var ErrCircuitBreakerOpen = errors.New("熔断器已打开，请稍后重试")

// ErrMaxRetriesExceeded 超过最大重试次数
var ErrMaxRetriesExceeded = errors.New("超过最大重试次数")

// ErrOperationCancelled 操作已取消
var ErrOperationCancelled = errors.New("操作已取消")

// ============ 通用错误 ============

// ErrNotFound 资源不存在
var ErrNotFound = errors.New("资源不存在")

// ErrAlreadyExists 资源已存在
var ErrAlreadyExists = errors.New("资源已存在")

// ErrInvalidParameter 无效参数
var ErrInvalidParameter = errors.New("无效参数")
