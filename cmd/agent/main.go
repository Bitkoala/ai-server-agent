package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ai-server-agent/internal/cli"
	"github.com/ai-server-agent/internal/core"
	"github.com/ai-server-agent/internal/drivers"
	"github.com/ai-server-agent/internal/llm"
	"github.com/ai-server-agent/internal/notify"
	"github.com/ai-server-agent/internal/security"
	"github.com/ai-server-agent/internal/storage"
	"github.com/ai-server-agent/internal/web"
	"gopkg.in/yaml.v3"
)

// Config 完整配置结构
type Config struct {
	Server struct {
		Port     int    `yaml:"port"`
		Host     string `yaml:"host"`
		TLSCert  string `yaml:"tls_cert"`
		TLSKey   string `yaml:"tls_key"`
	} `yaml:"server"`
	OnePanel struct {
		BaseURL string `yaml:"base_url"`
		APIKey  string `yaml:"api_key"`
		Timeout int    `yaml:"timeout"`
	} `yaml:"onepanel"`
	LLM struct {
		Provider string `yaml:"provider"`
		APIKey   string `yaml:"api_key"`
		BaseURL  string `yaml:"base_url"`
		Model    string `yaml:"model"`
	} `yaml:"llm"`
	Security struct {
		DangerousOpsRequireConfirm bool     `yaml:"dangerous_ops_require_confirm"`
		RateLimitPerMinute         int      `yaml:"rate_limit_per_minute"`
		AllowedUsers               []string `yaml:"allowed_users"`
	} `yaml:"security"`
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
	Notify struct {
		FeishuWebhook    string `yaml:"feishu_webhook"`
		DingTalkWebhook  string `yaml:"dingtalk_webhook"`
		WecomWebhook     string `yaml:"wecom_webhook"`
		TelegramBotToken string `yaml:"telegram_bot_token"`
		TelegramChatID   string `yaml:"telegram_chat_id"`
		MinLevel         string `yaml:"min_level"`
	} `yaml:"notify"`
	Auth struct {
		Enabled     bool       `yaml:"enabled"`
		JWTSecret   string     `yaml:"jwt_secret"`
		TokenExpiry int        `yaml:"token_expiry_hours"`
		Users       []web.User `yaml:"users"`
	} `yaml:"auth"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	cfg.applyEnvOverrides()
	if err := ensureConfigPerms(path); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法设置配置文件权限: %v\n", err)
	}
	return cfg, nil
}

// needsFirstRunSetup 检测是否需要首次配置
func needsFirstRunSetup(cfg *Config) bool {
	// 条件：1Panel API Key 还是默认值 / LLM API Key 还是默认值
	isDefault := func(s string) bool {
		s = strings.TrimSpace(s)
		return s == "" || s == "your-1panel-api-key" || s == "your-api-key" || s == "your-llm-api-key" || strings.HasPrefix(s, "your-")
	}
	return isDefault(cfg.OnePanel.APIKey) || isDefault(cfg.LLM.APIKey)
}

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	showVersion := flag.Bool("version", false, "显示版本信息")
	healthCheck := flag.Bool("health", false, "Docker 健康检查")
	skipSetup := flag.Bool("skip-setup", false, "跳过首次配置引导（直接启动）")
	flag.Parse()

	if *showVersion {
		fmt.Printf("AI Server Agent %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		os.Exit(0)
	}
	if *healthCheck {
		os.Exit(0)
	}

	// ============ 首次运行引导 ============
	if !*skipSetup {
		cfg, _ := loadConfig(*configPath)
		if cfg != nil && needsFirstRunSetup(cfg) {
			runSetupWizard(*configPath, cfg)
			// 重新加载配置
			cfg, _ = loadConfig(*configPath)
		}
	}

	// ============ 正常启动 ============
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	store, err := storage.NewSQLiteStore(cfg.Storage.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	panelDriver := drivers.NewOnePanelDriver(cfg.OnePanel.BaseURL, cfg.OnePanel.APIKey)
	safeGuard := security.NewSafeGuard(security.Config{
		DangerousOpsRequireConfirm: cfg.Security.DangerousOpsRequireConfirm,
		RateLimitPerMinute:         cfg.Security.RateLimitPerMinute,
	})

	llmClient := llm.NewClient(llm.Config{
		Provider: cfg.LLM.Provider, APIKey: cfg.LLM.APIKey,
		BaseURL: cfg.LLM.BaseURL, Model: cfg.LLM.Model,
	})

	auditLogger, err := storage.NewAuditLogger(store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化审计日志失败: %v\n", err)
		os.Exit(1)
	}

	engineCfg := core.DefaultEngineConfig()
	engineCfg.Driver = panelDriver
	engineCfg.Storage = store
	engineCfg.SafeGuard = safeGuard
	engineCfg.LLMClient = llmClient
	engineCfg.AuditLogger = auditLogger

	notifier := notify.NewNotifier(notify.NotifyConfig{
		FeishuWebhook: cfg.Notify.FeishuWebhook, DingTalkWebhook: cfg.Notify.DingTalkWebhook,
		WecomWebhook: cfg.Notify.WecomWebhook, TelegramBotToken: cfg.Notify.TelegramBotToken,
		TelegramChatID: cfg.Notify.TelegramChatID, MinLevel: notify.Level(cfg.Notify.MinLevel),
	})
	engineCfg.Notifier = notifier

	engine := core.NewEngine(engineCfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	authCfg := web.AuthConfig{
		Enabled: cfg.Auth.Enabled, JWTSecret: cfg.Auth.JWTSecret,
		TokenExpiry: cfg.Auth.TokenExpiry, Users: cfg.Auth.Users,
	}
	webSrv := web.NewServer(engine, store, authCfg, &web.ServerConfig{ConfigPath: *configPath})
	httpSrv := startHTTPServer(cfg.Server.Host, cfg.Server.Port, cfg.Server.TLSCert, cfg.Server.TLSKey, webSrv.Handler(), panelDriver, store)

	// 打印启动信息
	printStartupInfo(cfg, *configPath)

	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	go func() {
		sig := <-sigCh
		fmt.Printf("\n收到信号 %v，正在优雅关闭...\n", sig)
		if sig != syscall.SIGHUP {
			cancel()
		}
	}()

	repl := cli.NewREPL(engine, store)
	repl.Run()
	fmt.Println("REPL 已退出，HTTP 服务仍在运行。按 Ctrl+C 停止。")
	<-ctx.Done()
	fmt.Println("Agent 已停止。")
}

// ============ 终端引导向导 ============

func runSetupWizard(configPath string, cfg *Config) {
	reader := bufio.NewReader(os.Stdin)
	clearScreen()

	fmt.Print(`
╔══════════════════════════════════════════════════╗
║                                                  ║
║     🤖  欢迎使用 AI Server Agent！              ║
║     自然语言驱动的 Linux 服务器管理助手          ║
║                                                  ║
║     检测到这是首次运行，需要完成基本配置。        ║
║                                                  ║
╚══════════════════════════════════════════════════╝
`)
	fmt.Println("  请选择配置方式：")
	fmt.Println("")
	fmt.Println("  [1] 🚀 快速启动 — 跳过配置，稍后在 Web 面板中设置")
	fmt.Println("  [2] 🛠️  终端向导 — 现在就在终端里逐项配置")
	fmt.Println("  [0] 退出")
	fmt.Println("")
	fmt.Print("  请输入选项 [1/2/0]: ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		fmt.Println("\n  ✅ 快速启动模式 — Web 服务启动后，在管理面板中完成配置。")
		fmt.Print("  访问 Web 界面 → 设置 → 填入 1Panel 和 LLM 信息即可。\n\n")
	case "2":
		runTerminalWizard(reader, configPath, cfg)
	default:
		fmt.Println("\n  已取消。下次启动时会再次引导。")
		os.Exit(0)
	}
}

func runTerminalWizard(reader *bufio.Reader, configPath string, cfg *Config) {
	clearScreen()
	fmt.Print(`
╔══════════════════════════════════════════════════╗
║        🛠️  终端配置向导                         ║
║   按 Enter 使用默认值（方括号内），输入 q 跳过   ║
╚══════════════════════════════════════════════════╝
`)

	// ===== 1Panel 配置 =====
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  📦 1Panel 连接配置")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  提示：API Key 在 1Panel → 面板设置 → API 接口 中获取")

	baseURL := prompt(reader, "  1Panel 地址", "http://localhost:9999")
	apiKey := prompt(reader, "  1Panel API Key", "")
	if apiKey == "" {
		apiKey = promptRequired(reader, "  1Panel API Key（必填）")
	}
	cfg.OnePanel.BaseURL = baseURL
	cfg.OnePanel.APIKey = apiKey

	// ===== LLM 配置 =====
	fmt.Println("")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  🧠 LLM 模型配置")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	provider := prompt(reader, "  模型提供商 [openai/ollama]", "openai")
	cfg.LLM.Provider = provider

	if provider == "ollama" {
		ollamaURL := prompt(reader, "  Ollama 地址", "http://localhost:11434")
		ollamaModel := prompt(reader, "  模型名称", "qwen2.5:7b")
		cfg.LLM.BaseURL = ollamaURL
		cfg.LLM.Model = ollamaModel
	} else {
		llmKey := prompt(reader, "  OpenAI API Key", "")
		if llmKey == "" {
			llmKey = promptRequired(reader, "  OpenAI API Key（必填）")
		}
		cfg.LLM.APIKey = llmKey

		llmBase := prompt(reader, "  API 地址", "https://api.openai.com/v1")
		llmModel := prompt(reader, "  模型名称", "gpt-4o-mini")
		cfg.LLM.BaseURL = llmBase
		cfg.LLM.Model = llmModel
	}

	// ===== 认证配置 =====
	fmt.Println("")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  🔐 Web 认证配置")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	enableAuth := prompt(reader, "  启用 Web 登录认证？[Y/n]", "Y")
	if strings.ToLower(enableAuth) == "y" || enableAuth == "" {
		cfg.Auth.Enabled = true
		adminUser := prompt(reader, "  管理员用户名", "admin")
		adminPass := promptPassword(reader, "  管理员密码（至少6位）")
		cfg.Auth.Users = []web.User{{Username: adminUser, Password: adminPass, Role: "admin"}}
	} else {
		cfg.Auth.Enabled = false
	}

	// ===== 端口配置 =====
	fmt.Println("")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  🌐 Web 服务配置")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	portStr := prompt(reader, "  Web 端口", "9090")
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port > 0 {
		cfg.Server.Port = port
	}

	// ===== 保存配置 =====
	fmt.Println("")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	saveConfig(configPath, cfg)

	fmt.Println("  ✅ 配置完成！正在启动服务...")
	fmt.Println("")
}

// ============ 终端交互辅助函数 ============

func prompt(reader *bufio.Reader, label string, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "q" || input == "Q" {
		return defaultVal
	}
	if input == "" {
		return defaultVal
	}
	return input
}

func promptRequired(reader *bufio.Reader, label string) string {
	for {
		fmt.Printf("%s: ", label)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			return input
		}
		fmt.Println("  ⚠️  此项为必填，请输入。")
	}
}

func promptPassword(reader *bufio.Reader, label string) string {
	for {
		fmt.Printf("%s: ", label)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if len(input) >= 6 {
			return input
		}
		if input == "" {
			continue
		}
		fmt.Println("  ⚠️  密码至少6位，请重新输入。")
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func saveConfig(path string, cfg *Config) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ 序列化配置失败: %v\n", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ 写入配置文件失败: %v\n", err)
		return
	}
	fmt.Printf("  💾 配置已保存到: %s\n", path)
}

func printStartupInfo(cfg *Config, configPath string) {
	scheme := "http"
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if cfg.Server.Host == "0.0.0.0" {
		addr = fmt.Sprintf("localhost:%d", cfg.Server.Port)
	}

	fmt.Println("")
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║         🤖 AI Server Agent 已启动               ║")
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Printf("║  Web 界面:   %-36s ║\n", scheme+"://"+addr)
	fmt.Printf("║  健康检查:   %-36s ║\n", scheme+"://"+addr+"/health")
	fmt.Printf("║  配置文件:   %-36s ║\n", configPath)
	fmt.Println("╠══════════════════════════════════════════════════╣")

	if cfg.Auth.Enabled {
		fmt.Println("║  🔐 认证:     已启用                            ║")
	} else {
		fmt.Println("║  🔓 认证:     未启用（建议在 Web 设置中开启）    ║")
	}

	// 检查配置状态
	if needsFirstRunSetup(cfg) {
		fmt.Println("╠══════════════════════════════════════════════════╣")
		fmt.Println("║  ⚠️  尚未完成配置！                              ║")
		fmt.Printf("║  请访问 Web 界面 → 设置 → 完成 LLM 和 1Panel 配置 ║\n")
	}
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println("")
	fmt.Println("💡 在终端中输入自然语言指令，或打开 Web 界面操作。")
	fmt.Println("   输入 'help' 查看帮助，'exit' 退出 REPL（HTTP 服务继续运行）。")
	fmt.Println("")
}

// ============ HTTP 服务 ============

func startHTTPServer(host string, port int, tlsCert, tlsKey string, webHandler http.Handler, driver *drivers.OnePanelDriver, store *storage.SQLiteStore) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"ai-server-agent"}`))
	})

	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := driver.HealthCheck(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(fmt.Sprintf(`{"status":"not_ready","error":"%s"}`, err.Error())))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines\n# TYPE go_goroutines gauge\ngo_goroutines %d\n", runtime.NumGoroutine())
		fmt.Fprintf(w, "# HELP go_mem_alloc_bytes Allocated memory in bytes\n# TYPE go_mem_alloc_bytes gauge\ngo_mem_alloc_bytes %d\n", mem.Alloc)
		fmt.Fprintf(w, "# HELP go_mem_sys_bytes System memory in bytes\n# TYPE go_mem_sys_bytes gauge\ngo_mem_sys_bytes %d\n", mem.Sys)
		fmt.Fprintf(w, "# HELP go_gc_pause_ns Last GC pause in nanoseconds\n# TYPE go_gc_pause_ns gauge\ngo_gc_pause_ns %d\n", mem.PauseNs[(mem.NumGC+255)%256])

		tasks, _ := store.ListTasks(100)
		done, failed := 0, 0
		for _, t := range tasks {
			switch t.Status {
			case "done":
				done++
			case "failed":
				failed++
			}
		}
		fmt.Fprintf(w, "# HELP ai_server_agent_tasks_total Total tasks\n# TYPE ai_server_agent_tasks_total counter\nai_server_agent_tasks_total %d\n", len(tasks))
		fmt.Fprintf(w, "# HELP ai_server_agent_tasks_done Completed tasks\n# TYPE ai_server_agent_tasks_done counter\nai_server_agent_tasks_done %d\n", done)
		fmt.Fprintf(w, "# HELP ai_server_agent_tasks_failed Failed tasks\n# TYPE ai_server_agent_tasks_failed counter\nai_server_agent_tasks_failed %d\n", failed)
		fmt.Fprintf(w, "# HELP ai_server_agent_info Agent version info\n# TYPE ai_server_agent_info gauge\nai_server_agent_info{version=\"%s\",commit=\"%s\"} 1\n", Version, Commit)
	})

	mux.Handle("/", webHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		var err error
		if tlsCert != "" && tlsKey != "" {
			err = srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP 服务错误: %v\n", err)
		}
	}()

	return srv
}
