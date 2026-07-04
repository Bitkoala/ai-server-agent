package notify

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============ mock channel ============

type mockChannel struct {
	name string
	sent []*Message
	mu   sync.Mutex
}

func (m *mockChannel) Name() string { return m.name }

func (m *mockChannel) Send(_ context.Context, msg *Message) error {
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	return nil
}

func (m *mockChannel) sentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *mockChannel) lastMessage() *Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return nil
	}
	return m.sent[len(m.sent)-1]
}

// ============ helper ============

// injectMockChannels replaces all channels in the notifier with mocks
// and returns them for inspection. This avoids any HTTP traffic.
func injectMockChannels(n *Notifier) map[string]*mockChannel {
	mocks := map[string]*mockChannel{}
	for k := range n.channels {
		mc := &mockChannel{name: k}
		mocks[k] = mc
		n.channels[k] = mc
	}
	return mocks
}

// ============ tests ============

func TestLevelConstants(t *testing.T) {
	if LevelInfo != "info" {
		t.Errorf("LevelInfo = %q, want %q", LevelInfo, "info")
	}
	if LevelWarning != "warning" {
		t.Errorf("LevelWarning = %q, want %q", LevelWarning, "warning")
	}
	if LevelError != "error" {
		t.Errorf("LevelError = %q, want %q", LevelError, "error")
	}
}

func TestNewNotifier_NoChannels(t *testing.T) {
	n := NewNotifier(NotifyConfig{})
	if len(n.channels) != 0 {
		t.Fatalf("expected 0 channels, got %d", len(n.channels))
	}
}

func TestNewNotifier_WithChannels(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook:   "https://feishu.example.com/webhook",
		DingTalkWebhook: "https://dingtalk.example.com/webhook",
		WecomWebhook:    "https://wecom.example.com/webhook",
	})
	if len(n.channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(n.channels))
	}
	for _, key := range []string{"feishu", "dingtalk", "wecom"} {
		if _, ok := n.channels[key]; !ok {
			t.Errorf("expected channel %q to be registered", key)
		}
	}
}

func TestNewNotifier_TelegramChannel(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		TelegramBotToken: "12345:token",
		TelegramChatID:   "67890",
	})
	if len(n.channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(n.channels))
	}
	if _, ok := n.channels["telegram"]; !ok {
		t.Error("expected telegram channel to be registered")
	}
}

func TestNewNotifier_TelegramChannelMissingFields(t *testing.T) {
	// Only bot token, no chat ID — should NOT register
	n := NewNotifier(NotifyConfig{
		TelegramBotToken: "12345:token",
	})
	if len(n.channels) != 0 {
		t.Fatalf("expected 0 channels, got %d", len(n.channels))
	}

	// Only chat ID, no bot token — should NOT register
	n2 := NewNotifier(NotifyConfig{
		TelegramChatID: "67890",
	})
	if len(n2.channels) != 0 {
		t.Fatalf("expected 0 channels, got %d", len(n2.channels))
	}
}

func TestNewNotifier_DefaultMinLevel(t *testing.T) {
	n := NewNotifier(NotifyConfig{})
	if n.config.MinLevel != LevelWarning {
		t.Errorf("default MinLevel = %q, want %q", n.config.MinLevel, LevelWarning)
	}
}

func TestNewNotifier_CustomMinLevel(t *testing.T) {
	n := NewNotifier(NotifyConfig{MinLevel: LevelError})
	if n.config.MinLevel != LevelError {
		t.Errorf("custom MinLevel = %q, want %q", n.config.MinLevel, LevelError)
	}
}

func TestNotifier_ShouldSend(t *testing.T) {
	tests := []struct {
		name     string
		minLevel Level
		msgLevel Level
		want     bool
	}{
		// Default: MinLevel = Warning (set inside NewNotifier when empty)
		{"info < warning (default)", LevelWarning, LevelInfo, false},
		{"warning >= warning (default)", LevelWarning, LevelWarning, true},
		{"error >= warning (default)", LevelWarning, LevelError, true},

		// MinLevel = Info (everything passes)
		{"info >= info", LevelInfo, LevelInfo, true},
		{"warning >= info", LevelInfo, LevelWarning, true},
		{"error >= info", LevelInfo, LevelError, true},

		// MinLevel = Error (only error passes)
		{"info < error", LevelError, LevelInfo, false},
		{"warning < error", LevelError, LevelWarning, false},
		{"error >= error", LevelError, LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Notifier{config: NotifyConfig{MinLevel: tt.minLevel}}
			got := n.shouldSend(tt.msgLevel)
			if got != tt.want {
				t.Errorf("shouldSend(%q) with MinLevel=%q = %v, want %v",
					tt.msgLevel, tt.minLevel, got, tt.want)
			}
		})
	}
}

func TestNotifier_Send_LevelFilter(t *testing.T) {
	// Setup: notifier with a mock channel, MinLevel = Warning
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelWarning,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	// Send with LevelInfo — should be filtered, no channel call
	msg := &Message{Title: "test", Content: "content", Level: LevelInfo}
	n.Send(ctx, msg)

	// Give goroutines a moment in case they would fire (they shouldn't)
	time.Sleep(10 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 0 {
		t.Errorf("expected 0 sends for LevelInfo with MinLevel=Warning, got %d", mc.sentCount())
	}
}

func TestNotifier_Send_NotFiltered(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelWarning,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	// Send with LevelError — should pass filter and call channel
	msg := &Message{Title: "error", Content: "boom", Level: LevelError}
	n.Send(ctx, msg)

	// Wait for goroutine
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send for LevelError with MinLevel=Warning, got %d", mc.sentCount())
	}
	received := mc.lastMessage()
	if received.Title != "error" {
		t.Errorf("title = %q, want %q", received.Title, "error")
	}
	if received.Level != LevelError {
		t.Errorf("level = %q, want %q", received.Level, LevelError)
	}
}

func TestNotifier_SendInfo(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelInfo,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.SendInfo(ctx, "info title", "info content")
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send, got %d", mc.sentCount())
	}
	msg := mc.lastMessage()
	if msg.Title != "info title" {
		t.Errorf("title = %q, want %q", msg.Title, "info title")
	}
	if msg.Content != "info content" {
		t.Errorf("content = %q, want %q", msg.Content, "info content")
	}
	if msg.Level != LevelInfo {
		t.Errorf("level = %q, want %q", msg.Level, LevelInfo)
	}
}

func TestNotifier_SendWarning(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelWarning,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.SendWarning(ctx, "warn title", "warn content")
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send, got %d", mc.sentCount())
	}
	msg := mc.lastMessage()
	if msg.Level != LevelWarning {
		t.Errorf("level = %q, want %q", msg.Level, LevelWarning)
	}
}

func TestNotifier_SendError(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelError,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.SendError(ctx, "err title", "err content")
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send, got %d", mc.sentCount())
	}
	msg := mc.lastMessage()
	if msg.Level != LevelError {
		t.Errorf("level = %q, want %q", msg.Level, LevelError)
	}
}

func TestNotifier_NotifyTaskComplete_Success(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelInfo,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.NotifyTaskComplete(ctx, "task-1", "deploy", true, "deployed successfully")
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send, got %d", mc.sentCount())
	}
	msg := mc.lastMessage()
	if msg.Level != LevelInfo {
		t.Errorf("level = %q, want %q", msg.Level, LevelInfo)
	}
	if msg.TaskID != "task-1" {
		t.Errorf("taskID = %q, want %q", msg.TaskID, "task-1")
	}
	if msg.Title != "✅ 任务执行成功" {
		t.Errorf("title = %q, want %q", msg.Title, "✅ 任务执行成功")
	}
}

func TestNotifier_NotifyTaskComplete_Failure(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelInfo,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.NotifyTaskComplete(ctx, "task-2", "build", false, "build error")
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send, got %d", mc.sentCount())
	}
	msg := mc.lastMessage()
	if msg.Level != LevelError {
		t.Errorf("level = %q, want %q", msg.Level, LevelError)
	}
	if msg.TaskID != "task-2" {
		t.Errorf("taskID = %q, want %q", msg.TaskID, "task-2")
	}
	if msg.Title != "❌ 任务执行失败" {
		t.Errorf("title = %q, want %q", msg.Title, "❌ 任务执行失败")
	}
}

func TestNotifier_NotifyDangerousOp(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelWarning,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	params := map[string]string{"server": "prod-01", "action": "restart"}
	n.NotifyDangerousOp(ctx, "task-3", "restart_server", params)
	time.Sleep(20 * time.Millisecond)

	mc := mocks["feishu"]
	if mc.sentCount() != 1 {
		t.Fatalf("expected 1 send, got %d", mc.sentCount())
	}
	msg := mc.lastMessage()
	if msg.Level != LevelWarning {
		t.Errorf("level = %q, want %q", msg.Level, LevelWarning)
	}
	if msg.TaskID != "task-3" {
		t.Errorf("taskID = %q, want %q", msg.TaskID, "task-3")
	}
	if msg.Title != "⚠️ 高危操作提醒" {
		t.Errorf("title = %q, want %q", msg.Title, "⚠️ 高危操作提醒")
	}
}

func TestFeishuChannel_Name(t *testing.T) {
	ch := NewFeishuChannel("http://fake")
	if ch.Name() != "飞书" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "飞书")
	}
}

func TestDingTalkChannel_Name(t *testing.T) {
	ch := NewDingTalkChannel("http://fake")
	if ch.Name() != "钉钉" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "钉钉")
	}
}

func TestWecomChannel_Name(t *testing.T) {
	ch := NewWecomChannel("http://fake")
	if ch.Name() != "企业微信" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "企业微信")
	}
}

func TestTelegramChannel_Name(t *testing.T) {
	ch := NewTelegramChannel("token", "chat-id")
	if ch.Name() != "Telegram" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "Telegram")
	}
}

func TestEscapeTelegramMarkdown(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello world"},
		{"_underscore_", "\\_underscore\\_"},
		{"*bold*", "\\*bold\\*"},
		{"[link]", "\\[link\\]"},
		{"(paren)", "\\(paren\\)"},
		{"~strike~", "\\~strike\\~"},
		{"`code`", "\\`code\\`"},
		{"> quote", "\\> quote"},
		{"# heading", "\\# heading"},
		{"+ plus", "\\+ plus"},
		{"- minus", "\\- minus"},
		{"= equals", "\\= equals"},
		{"| pipe", "\\| pipe"},
		{"{brace}", "\\{brace\\}"},
		{".dot", "\\.dot"},
		{"!bang", "\\!bang"},
		// Mixed special chars
		{"a_b*c[d]e(f)g~h`i>j#k+l-m=n|o{p}q.r!s",
			"a\\_b\\*c\\[d\\]e\\(f\\)g\\~h\\`i\\>j\\#k\\+l\\-m\\=n\\|o\\{p\\}q\\.r\\!s"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeTelegramMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("escapeTelegramMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMessage_Fields(t *testing.T) {
	now := time.Now()
	msg := &Message{
		Title:   "Test Title",
		Content: "Test Content",
		Level:   LevelError,
		TaskID:  "task-42",
		Time:    now,
	}
	if msg.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", msg.Title, "Test Title")
	}
	if msg.Content != "Test Content" {
		t.Errorf("Content = %q, want %q", msg.Content, "Test Content")
	}
	if msg.Level != LevelError {
		t.Errorf("Level = %q, want %q", msg.Level, LevelError)
	}
	if msg.TaskID != "task-42" {
		t.Errorf("TaskID = %q, want %q", msg.TaskID, "task-42")
	}
	if !msg.Time.Equal(now) {
		t.Errorf("Time = %v, want %v", msg.Time, now)
	}
}

func TestNotifyConfig_Fields(t *testing.T) {
	cfg := NotifyConfig{
		FeishuWebhook:    "https://feishu.example.com",
		DingTalkWebhook:  "https://dingtalk.example.com",
		WecomWebhook:     "https://wecom.example.com",
		TelegramBotToken: "bot123:token",
		TelegramChatID:   "chat-456",
		MinLevel:         LevelError,
	}
	if cfg.FeishuWebhook != "https://feishu.example.com" {
		t.Errorf("FeishuWebhook = %q", cfg.FeishuWebhook)
	}
	if cfg.DingTalkWebhook != "https://dingtalk.example.com" {
		t.Errorf("DingTalkWebhook = %q", cfg.DingTalkWebhook)
	}
	if cfg.WecomWebhook != "https://wecom.example.com" {
		t.Errorf("WecomWebhook = %q", cfg.WecomWebhook)
	}
	if cfg.TelegramBotToken != "bot123:token" {
		t.Errorf("TelegramBotToken = %q", cfg.TelegramBotToken)
	}
	if cfg.TelegramChatID != "chat-456" {
		t.Errorf("TelegramChatID = %q", cfg.TelegramChatID)
	}
	if cfg.MinLevel != LevelError {
		t.Errorf("MinLevel = %q, want %q", cfg.MinLevel, LevelError)
	}
}

func TestNewNotifier_AllChannels(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook:    "http://fs",
		DingTalkWebhook:  "http://dt",
		WecomWebhook:     "http://wc",
		TelegramBotToken: "tok",
		TelegramChatID:   "cid",
	})
	if len(n.channels) != 4 {
		t.Fatalf("expected 4 channels, got %d", len(n.channels))
	}
	for _, key := range []string{"feishu", "dingtalk", "wecom", "telegram"} {
		if _, ok := n.channels[key]; !ok {
			t.Errorf("expected channel %q to be registered", key)
		}
	}
}

func TestNotifier_Send_MultipleChannels(t *testing.T) {
	n := NewNotifier(NotifyConfig{
		FeishuWebhook:   "http://fs",
		DingTalkWebhook: "http://dt",
		WecomWebhook:    "http://wc",
		MinLevel:        LevelWarning,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.Send(ctx, &Message{Title: "multi", Content: "all", Level: LevelWarning})
	time.Sleep(30 * time.Millisecond)

	for _, key := range []string{"feishu", "dingtalk", "wecom"} {
		mc := mocks[key]
		if mc.sentCount() != 1 {
			t.Errorf("channel %q: expected 1 send, got %d", key, mc.sentCount())
		}
	}
}

func TestNotifier_Send_SetsTime(t *testing.T) {
	before := time.Now()

	n := NewNotifier(NotifyConfig{
		FeishuWebhook: "http://fake",
		MinLevel:      LevelWarning,
	})
	mocks := injectMockChannels(n)
	ctx := context.Background()

	n.Send(ctx, &Message{Title: "t", Content: "c", Level: LevelWarning})
	time.Sleep(20 * time.Millisecond)

	msg := mocks["feishu"].lastMessage()
	if msg == nil {
		t.Fatal("expected a message to be sent")
	}
	if msg.Time.Before(before) {
		t.Errorf("Time %v should be after %v", msg.Time, before)
	}
}
