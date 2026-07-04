package security

import (
	"strings"
	"sync"
	"time"

	"github.com/ai-server-agent/internal/models"
)

// RiskLevel 风险等级
type RiskLevel int

const (
	RiskLow    RiskLevel = 0 // 只读/查询操作，自动执行
	RiskMedium RiskLevel = 1 // 有副作用的操作，显示预览
	RiskHigh   RiskLevel = 2 // 危险操作，必须二次确认
	RiskForbidden RiskLevel = 3 // 完全禁止
)

// Config 安全配置
type Config struct {
	DangerousOpsRequireConfirm bool
	RateLimitPerMinute         int
}

// SafeGuard 安全守卫
type SafeGuard struct {
	config     Config
	mu         sync.Mutex
	requestLog []time.Time
}

// NewSafeGuard 创建安全守卫
func NewSafeGuard(cfg Config) *SafeGuard {
	return &SafeGuard{
		config:     cfg,
		requestLog: make([]time.Time, 0),
	}
}

// 风险分级映射
var riskLevels = map[string]RiskLevel{
	// 低风险：只读/查询
	"app.list":       RiskLow,
	"app.status":     RiskLow,
	"container.list": RiskLow,
	"container.logs": RiskLow,
	"file.list":      RiskLow,
	"file.read":      RiskLow,
	"monitor.cpu":    RiskLow,
	"monitor.memory": RiskLow,
	"monitor.disk":   RiskLow,
	"monitor.network": RiskLow,
	"nginx.status":   RiskLow,
	"ssl.status":     RiskLow,
	"database.list":  RiskLow,
	"website.list":   RiskLow,
	"system.info":    RiskLow,
	"health":         RiskLow,

	// 中风险：有副作用但可逆
	"app.install":     RiskMedium,
	"container.start": RiskMedium,
	"nginx.reload":    RiskMedium,
	"ssl.apply":       RiskMedium,
	"ssl.renew":       RiskMedium,
	"database.create": RiskMedium,
	"database.backup": RiskMedium,
	"website.create":  RiskMedium,
	"file.upload":     RiskMedium,
	"nginx.config":    RiskMedium,

	// 高风险：危险操作，必须确认
	"container.stop":    RiskHigh,
	"container.restart": RiskHigh,
	"app.uninstall":     RiskHigh,
	"database.delete":   RiskHigh,
	"website.delete":    RiskHigh,
	"system.restart":    RiskHigh,
	"file.delete":       RiskHigh,

	// 禁止
	"system.factory_reset": RiskForbidden,
	"disk.format":          RiskForbidden,
}

// ValidateInput 校验用户输入
func (s *SafeGuard) ValidateInput(input string) error {
	if err := s.checkRateLimit(); err != nil {
		return err
	}

	forbidden := []string{"rm -rf /", "drop table", "truncate", "shutdown -h", "mkfs"}
	for _, kw := range forbidden {
		if strings.Contains(strings.ToLower(input), strings.ToLower(kw)) {
			return models.ErrDangerousCommand
		}
	}

	return nil
}

// AssessRisk 评估操作风险等级
func (s *SafeGuard) AssessRisk(action string) RiskLevel {
	if level, ok := riskLevels[action]; ok {
		return level
	}
	// 未知操作默认高风险
	return RiskHigh
}

// IsDangerous 判断操作是否需要二次确认（高风险）
func (s *SafeGuard) IsDangerous(action string) bool {
	if !s.config.DangerousOpsRequireConfirm {
		return false
	}
	return s.AssessRisk(action) == RiskHigh
}

// IsForbidden 判断操作是否完全禁止
func (s *SafeGuard) IsForbidden(action string) bool {
	return s.AssessRisk(action) == RiskForbidden
}

// NeedsConfirmation 判断步骤是否需要用户确认（中高风险）
func (s *SafeGuard) NeedsConfirmation(action string) bool {
	level := s.AssessRisk(action)
	return level == RiskHigh || level == RiskMedium
}

// IsAutoApproved 自动执行（低风险）
func (s *SafeGuard) IsAutoApproved(action string) bool {
	return s.AssessRisk(action) == RiskLow
}

// checkRateLimit 速率限制
func (s *SafeGuard) checkRateLimit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	valid := make([]time.Time, 0)
	for _, t := range s.requestLog {
		if t.After(oneMinuteAgo) {
			valid = append(valid, t)
		}
	}
	s.requestLog = valid

	if len(s.requestLog) >= s.config.RateLimitPerMinute {
		return models.ErrRateLimitExceeded
	}

	s.requestLog = append(s.requestLog, now)
	return nil
}
