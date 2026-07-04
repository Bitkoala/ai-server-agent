package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Level 通知级别
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

// Message 通知消息
type Message struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Level   Level  `json:"level"`
	TaskID  string `json:"task_id,omitempty"`
	Time    time.Time `json:"time"`
}

// Channel 通知渠道接口
type Channel interface {
	Name() string
	Send(ctx context.Context, msg *Message) error
}

// Notifier 通知管理器
type Notifier struct {
	mu       sync.RWMutex
	channels map[string]Channel
	config   NotifyConfig
}

// NotifyConfig 通知配置
type NotifyConfig struct {
	FeishuWebhook    string `yaml:"feishu_webhook"`
	DingTalkWebhook  string `yaml:"dingtalk_webhook"`
	WecomWebhook     string `yaml:"wecom_webhook"`
	TelegramBotToken string `yaml:"telegram_bot_token"`
	TelegramChatID   string `yaml:"telegram_chat_id"`
	MinLevel         Level  `yaml:"min_level"` // 低于此级别的消息不发送
}

// NewNotifier 创建通知管理器
func NewNotifier(cfg NotifyConfig) *Notifier {
	n := &Notifier{
		channels: make(map[string]Channel),
		config:   cfg,
	}

	if cfg.FeishuWebhook != "" {
		n.channels["feishu"] = NewFeishuChannel(cfg.FeishuWebhook)
	}
	if cfg.DingTalkWebhook != "" {
		n.channels["dingtalk"] = NewDingTalkChannel(cfg.DingTalkWebhook)
	}
	if cfg.WecomWebhook != "" {
		n.channels["wecom"] = NewWecomChannel(cfg.WecomWebhook)
	}
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		n.channels["telegram"] = NewTelegramChannel(cfg.TelegramBotToken, cfg.TelegramChatID)
	}

	if n.config.MinLevel == "" {
		n.config.MinLevel = LevelWarning
	}

	return n
}

// Send 发送通知到所有渠道
func (n *Notifier) Send(ctx context.Context, msg *Message) {
	// 级别过滤
	if !n.shouldSend(msg.Level) {
		return
	}

	msg.Time = time.Now()

	n.mu.RLock()
	defer n.mu.RUnlock()

	for name, ch := range n.channels {
		go func(chName string, channel Channel) {
			if err := channel.Send(ctx, msg); err != nil {
				fmt.Printf("通知渠道 %s 发送失败: %v\n", chName, err)
			}
		}(name, ch)
	}
}

// SendInfo 发送信息通知
func (n *Notifier) SendInfo(ctx context.Context, title, content string) {
	n.Send(ctx, &Message{Title: title, Content: content, Level: LevelInfo})
}

// SendWarning 发送警告通知
func (n *Notifier) SendWarning(ctx context.Context, title, content string) {
	n.Send(ctx, &Message{Title: title, Content: content, Level: LevelWarning})
}

// SendError 发送错误通知
func (n *Notifier) SendError(ctx context.Context, title, content string) {
	n.Send(ctx, &Message{Title: title, Content: content, Level: LevelError})
}

// NotifyTaskComplete 任务完成通知
func (n *Notifier) NotifyTaskComplete(ctx context.Context, taskID, intent string, success bool, details string) {
	title := "✅ 任务执行成功"
	level := LevelInfo
	if !success {
		title = "❌ 任务执行失败"
		level = LevelError
	}
	n.Send(ctx, &Message{
		Title:   title,
		Content: fmt.Sprintf("意图: %s\n详情: %s", intent, details),
		Level:   level,
		TaskID:  taskID,
	})
}

// NotifyDangerousOp 危险操作通知
func (n *Notifier) NotifyDangerousOp(ctx context.Context, taskID, action string, params map[string]string) {
	paramsJSON, _ := json.Marshal(params)
	n.Send(ctx, &Message{
		Title:   "⚠️ 高危操作提醒",
		Content: fmt.Sprintf("操作: %s\n参数: %s\n任务ID: %s", action, string(paramsJSON), taskID),
		Level:   LevelWarning,
		TaskID:  taskID,
	})
}

func (n *Notifier) shouldSend(level Level) bool {
	levels := map[Level]int{LevelInfo: 1, LevelWarning: 2, LevelError: 3}
	return levels[level] >= levels[n.config.MinLevel]
}

// ============ 飞书 ============

type FeishuChannel struct {
	webhook string
	client  *http.Client
}

func NewFeishuChannel(webhook string) *FeishuChannel {
	return &FeishuChannel{webhook: webhook, client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *FeishuChannel) Name() string { return "飞书" }

func (c *FeishuChannel) Send(ctx context.Context, msg *Message) error {
	color := "green"
	switch msg.Level {
	case LevelWarning:
		color = "yellow"
	case LevelError:
		color = "red"
	}

	body := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title":    map[string]string{"content": msg.Title, "tag": "plain_text"},
				"template": color,
			},
			"elements": []map[string]interface{}{
				{"tag": "div", "text": map[string]string{"content": msg.Content, "tag": "lark_md"}},
				{"tag": "hr"},
				{"tag": "note", "elements": []map[string]interface{}{
					{"tag": "plain_text", "content": fmt.Sprintf("AI Server Agent · %s", msg.Time.Format("01-02 15:04"))},
				}},
			},
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.webhook, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("飞书返回 %d", resp.StatusCode)
	}
	return nil
}

// ============ 钉钉 ============

type DingTalkChannel struct {
	webhook string
	client  *http.Client
}

func NewDingTalkChannel(webhook string) *DingTalkChannel {
	return &DingTalkChannel{webhook: webhook, client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *DingTalkChannel) Name() string { return "钉钉" }

func (c *DingTalkChannel) Send(ctx context.Context, msg *Message) error {
	levelText := ""
	switch msg.Level {
	case LevelWarning:
		levelText = "【警告】"
	case LevelError:
		levelText = "【错误】"
	}

	body := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Title,
			"text": fmt.Sprintf("## %s%s\n\n%s\n\n---\n*AI Server Agent · %s*",
				levelText, msg.Title, msg.Content, msg.Time.Format("01-02 15:04")),
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.webhook, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("钉钉返回 %d", resp.StatusCode)
	}
	return nil
}

// ============ 企业微信 ============

type WecomChannel struct {
	webhook string
	client  *http.Client
}

func NewWecomChannel(webhook string) *WecomChannel {
	return &WecomChannel{webhook: webhook, client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *WecomChannel) Name() string { return "企业微信" }

func (c *WecomChannel) Send(ctx context.Context, msg *Message) error {
	levelColor := "info"
	switch msg.Level {
	case LevelWarning:
		levelColor = "warning"
	case LevelError:
		levelColor = "warning"
	}

	body := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("## %s\n> %s\n\n%s\n\n<font color=\"%s\">AI Server Agent · %s</font>",
				msg.Title, msg.Content, "", levelColor, msg.Time.Format("01-02 15:04")),
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.webhook, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("企业微信返回 %d", resp.StatusCode)
	}
	return nil
}

// ============ Telegram ============

type TelegramChannel struct {
	botToken string
	chatID   string
	client   *http.Client
}

func NewTelegramChannel(botToken, chatID string) *TelegramChannel {
	return &TelegramChannel{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *TelegramChannel) Name() string { return "Telegram" }

func (c *TelegramChannel) Send(ctx context.Context, msg *Message) error {
	emoji := "ℹ️"
	switch msg.Level {
	case LevelWarning:
		emoji = "⚠️"
	case LevelError:
		emoji = "❌"
	case LevelInfo:
		emoji = "✅"
	}

	text := fmt.Sprintf("%s *%s*\n\n%s\n\n_AI Server Agent · %s_",
		emoji, escapeTelegramMarkdown(msg.Title),
		escapeTelegramMarkdown(msg.Content),
		msg.Time.Format("2006-01-02 15:04:05"))

	body := map[string]interface{}{
		"chat_id":    c.chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	}

	jsonBody, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Telegram 返回 %d", resp.StatusCode)
	}
	return nil
}

// escapeTelegramMarkdown 转义 Telegram MarkdownV2 特殊字符
func escapeTelegramMarkdown(s string) string {
	special := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := s
	for _, ch := range special {
		replacement := "\\" + ch
		// 使用简单替换
		newResult := ""
		for _, r := range result {
			if string(r) == ch {
				newResult += replacement
			} else {
				newResult += string(r)
			}
		}
		result = newResult
	}
	return result
}
