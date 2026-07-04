package llm

import (
	"context"

	"github.com/ai-server-agent/internal/models"
)

// Provider LLM 提供商接口 —— 所有 LLM 实现必须满足此接口
type Provider interface {
	// Chat 发送消息并获取回复
	Chat(ctx context.Context, messages []Message) (*Response, error)

	// ChatStream 流式对话，通过 channel 返回增量内容
	ChatStream(ctx context.Context, messages []Message) (<-chan StreamChunk, error)

	// ParseIntent 解析用户意图，返回结构化任务
	ParseIntent(ctx context.Context, userInput string, availableActions []ActionDef) (*IntentResult, error)

	// Name 提供商名称
	Name() string
}

// Message 对话消息
type Message struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// Response LLM 响应
type Response struct {
	Content      string `json:"content"`
	TokensUsed   int    `json:"tokens_used"`
	Model        string `json:"model"`
	FinishReason string `json:"finish_reason"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Content string `json:"content"` // 增量文本
	Done    bool   `json:"done"`    // 是否结束
	Error   error  `json:"-"`       // 错误
}

// ActionDef 可用操作定义（传给 LLM 做 function calling）
type ActionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// IntentResult 意图解析结果
type IntentResult struct {
	Intent      string            `json:"intent"`
	Explanation string            `json:"explanation"`
	Steps       []models.TaskStep `json:"steps"`
	Confidence  float64           `json:"confidence"` // 0-1
}
