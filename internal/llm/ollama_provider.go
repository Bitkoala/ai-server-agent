package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ai-server-agent/internal/models"
)

// OllamaProvider Ollama 本地模型提供商
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllama 创建 Ollama 提供商
func NewOllama(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "qwen2.5:7b"
	}
	return &OllamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Chat 标准对话（Ollama /api/chat）
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (*Response, error) {
	reqBody := map[string]interface{}{
		"model":    p.model,
		"messages": toOllamaMessages(messages),
		"stream":   false,
		"options": map[string]interface{}{
			"temperature": 0.3,
		},
	}

	resp, err := p.doRequest(ctx, "/api/chat", reqBody)
	if err != nil {
		return nil, err
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		DoneReason string `json:"done_reason"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析 Ollama 响应失败: %w", err)
	}

	return &Response{
		Content:      result.Message.Content,
		Model:        p.model,
		FinishReason: result.DoneReason,
	}, nil
}

// ChatStream 流式对话（Ollama /api/chat stream）
func (p *OllamaProvider) ChatStream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	reqBody := map[string]interface{}{
		"model":    p.model,
		"messages": toOllamaMessages(messages),
		"stream":   true,
		"options": map[string]interface{}{
			"temperature": 0.3,
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Ollama 失败: %w", err)
	}

	ch := make(chan StreamChunk, 10)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				ch <- StreamChunk{Error: ctx.Err(), Done: true}
				return
			default:
			}

			line := scanner.Text()
			var streamResp struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}

			if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
				continue
			}

			ch <- StreamChunk{
				Content: streamResp.Message.Content,
				Done:    streamResp.Done,
			}

			if streamResp.Done {
				return
			}
		}
	}()

	return ch, nil
}

// ParseIntent 意图解析（使用 Ollama 本地模型）
func (p *OllamaProvider) ParseIntent(ctx context.Context, userInput string, availableActions []ActionDef) (*IntentResult, error) {
	actionsJSON, _ := json.MarshalIndent(availableActions, "", "  ")

	systemPrompt := fmt.Sprintf(`你是一个 Linux 服务器管理助手。请分析用户的指令，输出结构化任务计划。

## 可用操作
%s

## 规则
1. 简单查询类操作直接执行
2. 危险操作（删除、重启、停止）标记 need_confirm=true
3. 不确定的参数用 "auto_detect" 并标记 need_confirm=true
4. 按依赖关系排序步骤
5. 只输出 JSON，不要包含其他文本

## 输出格式
{
  "intent": "意图简述",
  "explanation": "执行方式",
  "confidence": 0.95,
  "steps": [{"id": "step_1", "action": "操作名", "params": {}, "status": "pending", "need_confirm": false}]
}`, string(actionsJSON))

	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userInput},
	}

	resp, err := p.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("Ollama 意图解析失败: %w", err)
	}

	content := extractJSON(resp.Content)

	var raw struct {
		Intent      string  `json:"intent"`
		Explanation string  `json:"explanation"`
		Confidence  float64 `json:"confidence"`
		Steps       []struct {
			ID          string            `json:"id"`
			Action      string            `json:"action"`
			Params      map[string]string `json:"params"`
			Status      string            `json:"status"`
			NeedConfirm bool              `json:"need_confirm"`
		} `json:"steps"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("解析 Ollama 输出失败: %w\n原始输出: %s", err, resp.Content)
	}

	result := &IntentResult{
		Intent:      raw.Intent,
		Explanation: raw.Explanation,
		Confidence:  raw.Confidence,
	}

	for _, s := range raw.Steps {
		status := s.Status
		if status == "" {
			status = "pending"
		}
		if s.NeedConfirm {
			status = "confirm_required"
		}
		result.Steps = append(result.Steps, models.TaskStep{
			ID: s.ID, Action: s.Action, Params: s.Params, Status: status,
		})
	}

	return result, nil
}

// doRequest 发送请求到 Ollama
func (p *OllamaProvider) doRequest(ctx context.Context, path string, body interface{}) ([]byte, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Ollama 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Ollama API 错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func toOllamaMessages(messages []Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		result[i] = map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
	}
	return result
}
