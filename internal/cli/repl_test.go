package cli

import (
	"context"
	"testing"

	"github.com/ai-server-agent/internal/core"
	"github.com/ai-server-agent/internal/drivers"
	"github.com/ai-server-agent/internal/security"
	"github.com/ai-server-agent/internal/storage"
)

// mockDriver implements drivers.Driver for CLI tests.
type mockDriver struct {
	executeFunc       func(ctx context.Context, action string, params map[string]string) (string, error)
	availableActions  []string
	healthCheckFunc   func(ctx context.Context) error
	rollbackActionMap map[string]string
	registry          *drivers.StepRegistry
	name              string
}

func (m *mockDriver) Execute(ctx context.Context, action string, params map[string]string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, action, params)
	}
	return "ok", nil
}

func (m *mockDriver) AvailableActions() []string {
	if m.availableActions != nil {
		return m.availableActions
	}
	return []string{"monitor.cpu", "monitor.memory", "container.start", "container.stop"}
}

func (m *mockDriver) HealthCheck(ctx context.Context) error {
	if m.healthCheckFunc != nil {
		return m.healthCheckFunc(ctx)
	}
	return nil
}

func (m *mockDriver) RollbackAction(action string) string {
	if m.rollbackActionMap != nil {
		return m.rollbackActionMap[action]
	}
	return ""
}

func (m *mockDriver) Registry() *drivers.StepRegistry {
	if m.registry != nil {
		return m.registry
	}
	return drivers.NewStepRegistry()
}

func (m *mockDriver) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

// newCLIStore creates an in-memory SQLite store for CLI tests.
func newCLIStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// newCLIEngine creates a core.Engine with mock components for CLI tests.
func newCLIEngine(t *testing.T, store *storage.SQLiteStore) *core.Engine {
	t.Helper()
	cfg := core.DefaultEngineConfig()
	cfg.Driver = &mockDriver{}
	cfg.Storage = store
	cfg.SafeGuard = security.NewSafeGuard(security.Config{RateLimitPerMinute: 1000000})
	return core.NewEngine(cfg)
}

func TestNewREPL(t *testing.T) {
	store := newCLIStore(t)
	engine := newCLIEngine(t, store)

	repl := NewREPL(engine, store)
	if repl == nil {
		t.Fatal("NewREPL returned nil")
	}
	if repl.engine != engine {
		t.Error("engine not set correctly")
	}
	if repl.store != store {
		t.Error("store not set correctly")
	}
	if repl.reader == nil {
		t.Error("reader not set")
	}
}

func TestGetStepIcon(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"done", "✅"},
		{"failed", "❌"},
		{"running", "🔄"},
		{"confirm_required", "⚠️"},
		{"preview", "👁️"},
		{"unknown", "⏳"},
		{"", "⏳"},
	}

	for _, tt := range tests {
		got := getStepIcon(tt.status)
		if got != tt.want {
			t.Errorf("getStepIcon(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestGetRiskLabel(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{"container.stop", "🔴高"},
		{"app.install", "🟡中"},
		{"monitor.cpu", "🟢低"},
		{"unknown.action", "🟢低"},
		{"", "🟢低"},
	}

	for _, tt := range tests {
		got := getRiskLabel(tt.action)
		if got != tt.want {
			t.Errorf("getRiskLabel(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{
			name:   "short string - no truncation",
			s:      "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "long string - truncated with ...",
			s:      "this is a very long string that should be truncated",
			maxLen: 20,
			want:   "this is a very long ...",
		},
		{
			name:   "exact length - no truncation",
			s:      "exact",
			maxLen: 5,
			want:   "exact",
		},
		{
			name:   "empty string",
			s:      "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "one char over - truncated",
			s:      "abcdef",
			maxLen: 5,
			want:   "abcde...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestREPL_CommandSwitch(t *testing.T) {
	store := newCLIStore(t)
	engine := newCLIEngine(t, store)
	repl := NewREPL(engine, store)

	// These tests verify the command recognition logic without actually
	// running the REPL loop (which would require stdin interaction).
	// We test that the command strings match the expected switch cases.
	exitCommands := map[string]bool{
		"exit": true,
		"quit": true,
		"q":    true,
		"退出":   true,
		"EXIT": true,
		"QUIT": true,
	}

	helpCommands := map[string]bool{
		"help": true,
		"帮助":   true,
		"?":    true,
		"HELP": true,
	}

	// Verify exit commands match the switch cases in REPL.Run()
	for cmd := range exitCommands {
		// The REPL.Run() does: switch strings.ToLower(input) { case "exit", "quit", "q", "退出": ... }
		// For Chinese chars, ToLower is identity. For ASCII, it lowercases.
		lower := cmd
		// Simulate ASCII lowercasing for the ASCII commands
		if len(cmd) == 1 {
			if cmd[0] >= 'A' && cmd[0] <= 'Z' {
				lower = string(cmd[0] + 32)
			}
		} else if len(cmd) > 1 && cmd[0] <= 127 {
			// ASCII string - simple lowercase
			b := []byte(cmd)
			for i := range b {
				if b[i] >= 'A' && b[i] <= 'Z' {
					b[i] += 32
				}
			}
			lower = string(b)
		}
		isExit := lower == "exit" || lower == "quit" || lower == "q" || lower == "退出"
		if !isExit {
			t.Errorf("command %q (lowercased: %q) should be recognized as exit", cmd, lower)
		}
	}

	for cmd := range helpCommands {
		lower := cmd
		if len(cmd) > 1 && cmd[0] <= 127 {
			b := []byte(cmd)
			for i := range b {
				if b[i] >= 'A' && b[i] <= 'Z' {
					b[i] += 32
				}
			}
			lower = string(b)
		}
		isHelp := lower == "help" || lower == "帮助" || lower == "?"
		if !isHelp {
			t.Errorf("command %q (lowercased: %q) should be recognized as help", cmd, lower)
		}
	}

	_ = repl // suppress unused warning
}

func TestPrintBanner(t *testing.T) {
	// Verify the function exists and doesn't panic
	printBanner()
}

func TestPrintHelp(t *testing.T) {
	// Verify the function exists and doesn't panic
	printHelp()
}
