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

// OpenAIProvider OpenAI 兼容的 LLM 提供商
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewOpenAI 创建 OpenAI 兼容提供商
func NewOpenAI(apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Chat 标准对话
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*Response, error) {
	reqBody := map[string]interface{}{
		"model":       p.model,
		"messages":    toOpenAIMessages(messages),
		"temperature": 0.3,
		"max_tokens":  2000,
	}

	resp, err := p.doRequest(ctx, "/chat/completions", reqBody)
	if err != nil {
		return nil, err
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析 OpenAI 响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI 返回空响应")
	}

	return &Response{
		Content:      result.Choices[0].Message.Content,
		TokensUsed:   result.Usage.TotalTokens,
		Model:        p.model,
		FinishReason: result.Choices[0].FinishReason,
	}, nil
}

// ChatStream 流式对话
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	reqBody := map[string]interface{}{
		"model":       p.model,
		"messages":    toOpenAIMessages(messages),
		"temperature": 0.3,
		"max_tokens":  2000,
		"stream":      true,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 OpenAI 失败: %w", err)
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
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}

			var streamResp struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				continue
			}

			if len(streamResp.Choices) > 0 {
				ch <- StreamChunk{
					Content: streamResp.Choices[0].Delta.Content,
					Done:    streamResp.Choices[0].FinishReason == "stop",
				}
			}
		}
	}()

	return ch, nil
}

// ParseIntent 意图解析
func (p *OpenAIProvider) ParseIntent(ctx context.Context, userInput string, availableActions []ActionDef) (*IntentResult, error) {
	actionsJSON, _ := json.MarshalIndent(availableActions, "", "  ")

	systemPrompt := fmt.Sprintf(`你是一个 Linux 服务器管理助手。请分析用户的指令，输出结构化任务计划。

## 可用操作
%s

## 规则
1. 简单查询类操作直接执行
2. 危险操作（删除、重启、停止）标记 need_confirm=true
3. 不确定的参数用 "auto_detect" 并标记 need_confirm=true
4. 按依赖关系排序步骤
5. 输出必须是合法 JSON，不要包含 markdown 代码块

## 输出格式
{
  "intent": "意图简述",
  "explanation": "你将如何执行这个任务",
  "confidence": 0.95,
  "steps": [
    {
      "id": "step_1",
      "action": "操作名",
      "params": {"key": "value"},
      "status": "pending",
      "need_confirm": false
    }
  ]
}`, string(actionsJSON))

	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userInput},
	}

	resp, err := p.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM 意图解析失败: %w", err)
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
		return nil, fmt.Errorf("解析 LLM 输出失败: %w\n原始输出: %s", err, resp.Content)
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
			ID:     s.ID,
			Action: s.Action,
			Params: s.Params,
			Status: status,
		})
	}

	return result, nil
}

// doRequest 发送 HTTP 请求
func (p *OpenAIProvider) doRequest(ctx context.Context, path string, body interface{}) ([]byte, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 OpenAI 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("OpenAI API 错误 (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func toOpenAIMessages(messages []Message) []map[string]string {
	result := make([]map[string]string, len(messages))
	for i, m := range messages {
		result[i] = map[string]string{"role": m.Role, "content": m.Content}
	}
	return result
}

func extractJSON(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			lines = lines[1 : len(lines)-1]
			content = strings.Join(lines, "\n")
		}
	}
	return content
}
