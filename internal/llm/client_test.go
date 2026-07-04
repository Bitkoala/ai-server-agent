package llm

import (
	"context"
	"testing"

	"github.com/ai-server-agent/internal/models"
)

// ========== TestNewClient ==========

func TestNewClient(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		wantProvider string
	}{
		{name: "openai", provider: "openai", wantProvider: "openai"},
		{name: "ollama", provider: "ollama", wantProvider: "ollama"},
		{name: "empty defaults to openai", provider: "", wantProvider: "openai"},
		{name: "unknown defaults to openai", provider: "unknown", wantProvider: "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{Provider: tt.provider, APIKey: "test-key"})
			if c.Provider().Name() != tt.wantProvider {
				t.Errorf("NewClient(%q).Provider().Name() = %q, want %q",
					tt.provider, c.Provider().Name(), tt.wantProvider)
			}
		})
	}
}

// ========== TestClientProvider ==========

func TestClientProvider(t *testing.T) {
	c := NewClient(Config{Provider: "openai", APIKey: "key"})
	p := c.Provider()
	if p == nil {
		t.Fatal("Provider() returned nil")
	}
	if p.Name() != "openai" {
		t.Errorf("Provider().Name() = %q, want %q", p.Name(), "openai")
	}

	_, ok := p.(*OpenAIProvider)
	if !ok {
		t.Errorf("Provider() returned %T, want *OpenAIProvider", p)
	}
}

// ========== TestClassifyIntent ==========

func TestClassifyIntent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
	}{
		{name: "deploy", input: "部署一个 WordPress 网站", want: "应用部署"},
		{name: "install", input: "安装 Nginx", want: "应用部署"},
		{name: "english deploy", input: "deploy my app", want: "应用部署"},
		{name: "restart", input: "重启 nginx 服务", want: "服务重启"},
		{name: "english restart", input: "restart the container", want: "服务重启"},
		{name: "monitor", input: "查看系统监控", want: "系统监控"},
		{name: "cpu", input: "CPU 使用率是多少", want: "系统监控"},
		{name: "memory", input: "内存快满了", want: "系统监控"},
		{name: "ssl", input: "配置 SSL 证书", want: "SSL证书管理"},
		{name: "cert", input: "我的证书快过期了", want: "SSL证书管理"},
		{name: "https", input: "开启 HTTPS", want: "SSL证书管理"},
		{name: "log", input: "查看日志", want: "日志查看"},
		{name: "english log", input: "show me the logs", want: "日志查看"},
		{name: "unknown", input: "今天天气不错", want: "未知操作"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyIntent(tt.input)
			if got != tt.want {
				t.Errorf("classifyIntent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ========== TestGenerateSteps ==========

func TestGenerateSteps(t *testing.T) {
	tests := []struct {
		name          string
		intent        string
		wantMinSteps  int
		wantFirstAction string
	}{
		{name: "app deploy", intent: "应用部署", wantMinSteps: 3, wantFirstAction: "app.install"},
		{name: "service restart", intent: "服务重启", wantMinSteps: 1, wantFirstAction: "container.restart"},
		{name: "system monitor", intent: "系统监控", wantMinSteps: 3, wantFirstAction: "monitor.cpu"},
		{name: "unknown", intent: "未知操作", wantMinSteps: 1, wantFirstAction: "system.info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := generateSteps(tt.intent)
			if len(steps) < tt.wantMinSteps {
				t.Errorf("generateSteps(%q) returned %d steps, want at least %d",
					tt.intent, len(steps), tt.wantMinSteps)
			}
			if len(steps) > 0 && steps[0].Action != tt.wantFirstAction {
				t.Errorf("generateSteps(%q) first action = %q, want %q",
					tt.intent, steps[0].Action, tt.wantFirstAction)
			}
		})
	}

	// Test app deploy has confirm_required step
	t.Run("app deploy has confirm step", func(t *testing.T) {
		steps := generateSteps("应用部署")
		found := false
		for _, s := range steps {
			if s.Status == "confirm_required" {
				found = true
				break
			}
		}
		if !found {
			t.Error("应用部署 should have at least one confirm_required step")
		}
	})

	// Test service restart has confirm_required
	t.Run("service restart has confirm step", func(t *testing.T) {
		steps := generateSteps("服务重启")
		if len(steps) > 0 && steps[0].Status != "confirm_required" {
			t.Errorf("服务重启 step should be confirm_required, got %s", steps[0].Status)
		}
	})
}

// ========== TestGetAvailableActions ==========

func TestGetAvailableActions(t *testing.T) {
	actions := getAvailableActions()

	if len(actions) == 0 {
		t.Fatal("getAvailableActions() returned empty list")
	}

	actionNames := make(map[string]bool)
	for _, a := range actions {
		actionNames[a.Name] = true
	}

	expected := []string{
		"app.install", "app.list", "app.uninstall",
		"container.start", "container.stop", "container.restart", "container.logs",
		"container.list", "container.images", "container.pull", "container.prune",
		"ssl.apply", "ssl.status", "ssl.renew",
		"monitor.cpu", "monitor.memory", "monitor.disk", "monitor.network",
		"nginx.reload", "nginx.status",
		"system.info", "system.restart", "system.time", "system.processes", "system.clear_cache",
		"database.backup", "database.list", "database.create", "database.delete",
		"database.restore", "database.slow_query", "database.connections", "database.optimize",
		"website.list", "website.proxy", "website.domain",
		"file.list", "file.read", "file.upload", "file.delete",
		"health", "compose.generate",
	}

	for _, name := range expected {
		if !actionNames[name] {
			t.Errorf("getAvailableActions() missing action %q", name)
		}
	}

	// 验证总数（允许比 expected 多，因为可能随时新增 action）
	if len(actions) < len(expected) {
		t.Errorf("getAvailableActions() returned %d actions, expected at least %d", len(actions), len(expected))
	}
}

// ========== TestExtractJSON ==========

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
	}{
		{
			name:    "plain json",
			input:   `{"intent": "test"}`,
			want:    `{"intent": "test"}`,
		},
		{
			name:    "json code block",
			input:   "```json\n{\"intent\": \"test\"}\n```",
			want:    "{\"intent\": \"test\"}",
		},
		{
			name:    "generic code block",
			input:   "```\n{\"intent\": \"test\"}\n```",
			want:    "{\"intent\": \"test\"}",
		},
		{
			name:    "trimmed whitespace",
			input:   "  \n{\"intent\": \"test\"}\n  ",
			want:    "{\"intent\": \"test\"}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ========== TestToOpenAIMessages ==========

func TestToOpenAIMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		wantLen  int
	}{
		{
			name:     "empty messages",
			messages: []Message{},
			wantLen:  0,
		},
		{
			name: "single message",
			messages: []Message{
				{Role: "system", Content: "hello"},
			},
			wantLen: 1,
		},
		{
			name: "multiple messages",
			messages: []Message{
				{Role: "system", Content: "system prompt"},
				{Role: "user", Content: "user question"},
				{Role: "assistant", Content: "assistant reply"},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toOpenAIMessages(tt.messages)
			if len(result) != tt.wantLen {
				t.Errorf("toOpenAIMessages() returned %d messages, want %d", len(result), tt.wantLen)
			}
			for i, m := range result {
				if m["role"] != tt.messages[i].Role {
					t.Errorf("toOpenAIMessages()[%d].role = %q, want %q", i, m["role"], tt.messages[i].Role)
				}
				if m["content"] != tt.messages[i].Content {
					t.Errorf("toOpenAIMessages()[%d].content = %q, want %q", i, m["content"], tt.messages[i].Content)
				}
			}
		})
	}
}

// ========== TestToOllamaMessages ==========

func TestToOllamaMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		wantLen  int
	}{
		{
			name:     "empty messages",
			messages: []Message{},
			wantLen:  0,
		},
		{
			name: "single message",
			messages: []Message{
				{Role: "user", Content: "hello"},
			},
			wantLen: 1,
		},
		{
			name: "multiple messages",
			messages: []Message{
				{Role: "system", Content: "system prompt"},
				{Role: "user", Content: "user question"},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toOllamaMessages(tt.messages)
			if len(result) != tt.wantLen {
				t.Errorf("toOllamaMessages() returned %d messages, want %d", len(result), tt.wantLen)
			}
			for i, m := range result {
				if m["role"] != tt.messages[i].Role {
					t.Errorf("toOllamaMessages()[%d].role = %q, want %q", i, m["role"], tt.messages[i].Role)
				}
				if m["content"] != tt.messages[i].Content {
					t.Errorf("toOllamaMessages()[%d].content = %q, want %q", i, m["content"], tt.messages[i].Content)
				}
			}
		})
	}
}

// ========== TestOpenAIProvider_Name ==========

func TestOpenAIProvider_Name(t *testing.T) {
	p := NewOpenAI("key", "", "")
	if p.Name() != "openai" {
		t.Errorf("OpenAIProvider.Name() = %q, want %q", p.Name(), "openai")
	}
}

// ========== TestOllamaProvider_Name ==========

func TestOllamaProvider_Name(t *testing.T) {
	p := NewOllama("", "")
	if p.Name() != "ollama" {
		t.Errorf("OllamaProvider.Name() = %q, want %q", p.Name(), "ollama")
	}
}

// ========== TestOpenAIProvider_Defaults ==========

func TestOpenAIProvider_Defaults(t *testing.T) {
	p := NewOpenAI("", "", "")
	if p.baseURL != "https://api.openai.com/v1" {
		t.Errorf("NewOpenAI default baseURL = %q, want %q", p.baseURL, "https://api.openai.com/v1")
	}
	if p.model != "gpt-4o-mini" {
		t.Errorf("NewOpenAI default model = %q, want %q", p.model, "gpt-4o-mini")
	}
}

// ========== TestOllamaProvider_Defaults ==========

func TestOllamaProvider_Defaults(t *testing.T) {
	p := NewOllama("", "")
	if p.baseURL != "http://localhost:11434" {
		t.Errorf("NewOllama default baseURL = %q, want %q", p.baseURL, "http://localhost:11434")
	}
	if p.model != "qwen2.5:7b" {
		t.Errorf("NewOllama default model = %q, want %q", p.model, "qwen2.5:7b")
	}
}

// ========== TestConfig_Defaults ==========

func TestConfig_Defaults(t *testing.T) {
	var cfg Config

	if cfg.Provider != "" {
		t.Errorf("Config.Provider zero value = %q, want %q", cfg.Provider, "")
	}
	if cfg.APIKey != "" {
		t.Errorf("Config.APIKey zero value = %q, want %q", cfg.APIKey, "")
	}
	if cfg.BaseURL != "" {
		t.Errorf("Config.BaseURL zero value = %q, want %q", cfg.BaseURL, "")
	}
	if cfg.Model != "" {
		t.Errorf("Config.Model zero value = %q, want %q", cfg.Model, "")
	}
}

// ========== TestParseIntent_LocalFallback ==========

func TestParseIntent_LocalFallback(t *testing.T) {
	// Local template match returns empty, so it falls through to LLM.
	// With a real provider, LLM call would fail -> fallback to classifyIntent/generateSteps.
	// We test the classifyIntent fallback path directly through classifyIntent + generateSteps.

	tests := []struct {
		name       string
		input      string
		wantIntent string
		minSteps   int
	}{
		{name: "deploy", input: "部署 WordPress", wantIntent: "应用部署", minSteps: 3},
		{name: "restart", input: "重启服务", wantIntent: "服务重启", minSteps: 1},
		{name: "monitor", input: "CPU 使用情况", wantIntent: "系统监控", minSteps: 3},
		{name: "ssl", input: "SSL 证书过期", wantIntent: "SSL证书管理", minSteps: 1},
		{name: "log", input: "查看 nginx 日志", wantIntent: "日志查看", minSteps: 1},
		{name: "unknown", input: "你好", wantIntent: "未知操作", minSteps: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := classifyIntent(tt.input)
			if intent != tt.wantIntent {
				t.Errorf("classifyIntent(%q) = %q, want %q", tt.input, intent, tt.wantIntent)
			}

			steps := generateSteps(intent)
			if len(steps) < tt.minSteps {
				t.Errorf("generateSteps(%q) = %d steps, want at least %d", intent, len(steps), tt.minSteps)
			}
		})
	}
}

// ========== TestChat_MessageBuilding ==========

func TestChat_MessageBuilding(t *testing.T) {
	// Chat with a real provider would make HTTP calls.
	// We verify the structure: Chat takes history and userMessage, builds messages with system prompt.
	// The function is tested indirectly via Provider interface.

	// Verify that Chat is accessible and doesn't panic with nil context (before HTTP call).
	c := NewClient(Config{Provider: "openai", APIKey: "test-key"})

	ctx := context.Background()
	history := []map[string]string{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hello"},
	}
	_, err := c.Chat(ctx, history, "how are you")
	// Expect an error (HTTP request to fake API key will fail), but not a panic.
	if err == nil {
		t.Log("Chat succeeded unexpectedly (maybe a mock server is running)")
	}
}

// ========== TestChatStream_Accessible ==========

func TestChatStream_Accessible(t *testing.T) {
	c := NewClient(Config{Provider: "openai", APIKey: "test-key"})

	ctx := context.Background()
	history := []map[string]string{
		{"role": "user", "content": "hi"},
	}
	ch, err := c.ChatStream(ctx, history, "hello")
	if err != nil {
		// Expected: HTTP call to fake API will fail
		t.Logf("ChatStream returned expected error: %v", err)
		return
	}

	// If we somehow got a channel, drain it to avoid goroutine leak
	if ch != nil {
		for range ch {
		}
	}
}

// ========== TestParseIntent_CallsProvider ==========

func TestParseIntent_CallsProvider(t *testing.T) {
	c := NewClient(Config{Provider: "openai", APIKey: "test-key"})

	ctx := context.Background()
	intent, steps, err := c.ParseIntent(ctx, "部署 WordPress")

	if err != nil {
		// Expected: local template misses, LLM call fails -> fallback
		t.Logf("ParseIntent error (expected with no LLM backend): %v", err)
	}

	if intent == "" {
		t.Error("ParseIntent returned empty intent")
	}

	if len(steps) == 0 {
		t.Error("ParseIntent returned zero steps")
	}

	// Steps should have proper IDs
	for i, s := range steps {
		if s.ID == "" {
			t.Errorf("step[%d] has empty ID", i)
		}
	}
}

// ========== TestIntentResult_Structure ==========

func TestIntentResult_Structure(t *testing.T) {
	result := IntentResult{
		Intent:      "test intent",
		Explanation: "test explanation",
		Confidence:  0.95,
		Steps: []models.TaskStep{
			{ID: "step_1", Action: "test.action", Params: map[string]string{"key": "val"}, Status: "pending"},
		},
	}

	if result.Intent != "test intent" {
		t.Errorf("IntentResult.Intent = %q", result.Intent)
	}
	if result.Confidence != 0.95 {
		t.Errorf("IntentResult.Confidence = %f", result.Confidence)
	}
	if len(result.Steps) != 1 {
		t.Errorf("IntentResult.Steps length = %d, want 1", len(result.Steps))
	}
}
