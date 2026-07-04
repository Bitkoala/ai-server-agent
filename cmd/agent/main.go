package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
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
		Enabled     bool               `yaml:"enabled"`
		JWTSecret   string             `yaml:"jwt_secret"`
		TokenExpiry int                `yaml:"token_expiry_hours"`
		Users       []web.User         `yaml:"users"`
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

	// 环境变量覆盖（优先级高于配置文件）
	cfg.applyEnvOverrides()

	// 确保配置文件权限安全
	if err := ensureConfigPerms(path); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法设置配置文件权限: %v\n", err)
	}

	return cfg, nil
}

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	showVersion := flag.Bool("version", false, "显示版本信息")
	healthCheck := flag.Bool("health", false, "Docker 健康检查（通过检测进程存活来判断）")
	flag.Parse()

	if *showVersion {
		fmt.Printf("AI Server Agent %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		os.Exit(0)
	}

	// Docker HEALTHCHECK: 进程存活即为健康
	if *healthCheck {
		os.Exit(0)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化存储
	store, err := storage.NewSQLiteStore(cfg.Storage.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化存储失败: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// 初始化 1Panel 驱动
	panelDriver := drivers.NewOnePanelDriver(cfg.OnePanel.BaseURL, cfg.OnePanel.APIKey)

	// 初始化安全层
	safeGuard := security.NewSafeGuard(security.Config{
		DangerousOpsRequireConfirm: cfg.Security.DangerousOpsRequireConfirm,
		RateLimitPerMinute:         cfg.Security.RateLimitPerMinute,
	})

	// 初始化 LLM 客户端
	llmClient := llm.NewClient(llm.Config{
		Provider: cfg.LLM.Provider,
		APIKey:   cfg.LLM.APIKey,
		BaseURL:  cfg.LLM.BaseURL,
		Model:    cfg.LLM.Model,
	})

	// 初始化审计日志器
	auditLogger, err := storage.NewAuditLogger(store)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化审计日志失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化核心引擎
	engineCfg := core.DefaultEngineConfig()
	engineCfg.Driver = panelDriver
	engineCfg.Storage = store
	engineCfg.SafeGuard = safeGuard
	engineCfg.LLMClient = llmClient
	engineCfg.AuditLogger = auditLogger

	// 初始化通知器
	notifier := notify.NewNotifier(notify.NotifyConfig{
		FeishuWebhook:    cfg.Notify.FeishuWebhook,
		DingTalkWebhook:  cfg.Notify.DingTalkWebhook,
		WecomWebhook:     cfg.Notify.WecomWebhook,
		TelegramBotToken: cfg.Notify.TelegramBotToken,
		TelegramChatID:   cfg.Notify.TelegramChatID,
		MinLevel:         notify.Level(cfg.Notify.MinLevel),
	})
	engineCfg.Notifier = notifier

	engine := core.NewEngine(engineCfg)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 启动 HTTP 服务（健康检查 + Web API + Web 界面）
	authCfg := web.AuthConfig{
		Enabled:     cfg.Auth.Enabled,
		JWTSecret:   cfg.Auth.JWTSecret,
		TokenExpiry: cfg.Auth.TokenExpiry,
		Users:       cfg.Auth.Users,
	}
	webSrv := web.NewServer(engine, store, authCfg, &web.ServerConfig{ConfigPath: *configPath})
	httpSrv := startHTTPServer(cfg.Server.Host, cfg.Server.Port, cfg.Server.TLSCert, cfg.Server.TLSKey, webSrv.Handler(), panelDriver, store)

	// 提示初始化引导
	if cfg.Auth.Enabled {
		authManager := web.NewAuthManager(authCfg, *configPath)
		if authManager.NeedsSetup() {
			addr := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
			if cfg.Server.Host == "0.0.0.0" {
				addr = fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
			}
			fmt.Println("")
			fmt.Println("╔══════════════════════════════════════════════╗")
			fmt.Println("║  🔐 首次部署 - 需要设置管理员密码           ║")
			fmt.Println("╠══════════════════════════════════════════════╣")
			fmt.Printf("║  请打开浏览器访问: %-26s ║\n", addr)
			fmt.Println("║  按照页面引导设置管理员账号和密码           ║")
			fmt.Println("╚══════════════════════════════════════════════╝")
			fmt.Println("")
		}
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	// 监听信号
	go func() {
		sig := <-sigCh
		fmt.Printf("\n收到信号 %v，正在优雅关闭...\n", sig)
		if sig == syscall.SIGHUP {
			fmt.Println("配置重载（SIGHUP）—— 暂未实现")
			return
		}
		cancel()
	}()

	// REPL 退出后不退出整个程序（HTTP 服务继续运行）
	repl := cli.NewREPL(engine, store)
	repl.Run()

	// REPL 退出，等待信号
	fmt.Println("REPL 已退出，HTTP 服务仍在运行。按 Ctrl+C 停止。")
	<-ctx.Done()
	fmt.Println("Agent 已停止。")
}

// startHTTPServer 启动 HTTP 服务（Web API + 健康检查）
func startHTTPServer(host string, port int, tlsCert, tlsKey string, webHandler http.Handler, driver *drivers.OnePanelDriver, store *storage.SQLiteStore) *http.Server {
	mux := http.NewServeMux()

	// 健康检查端点
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

		// Runtime 指标
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		fmt.Fprintf(w, "# HELP go_goroutines Number of goroutines\n")
		fmt.Fprintf(w, "# TYPE go_goroutines gauge\n")
		fmt.Fprintf(w, "go_goroutines %d\n", runtime.NumGoroutine())
		fmt.Fprintf(w, "# HELP go_mem_alloc_bytes Allocated memory in bytes\n")
		fmt.Fprintf(w, "# TYPE go_mem_alloc_bytes gauge\n")
		fmt.Fprintf(w, "go_mem_alloc_bytes %d\n", mem.Alloc)
		fmt.Fprintf(w, "# HELP go_mem_sys_bytes System memory in bytes\n")
		fmt.Fprintf(w, "# TYPE go_mem_sys_bytes gauge\n")
		fmt.Fprintf(w, "go_mem_sys_bytes %d\n", mem.Sys)
		fmt.Fprintf(w, "# HELP go_gc_pause_ns Last GC pause in nanoseconds\n")
		fmt.Fprintf(w, "# TYPE go_gc_pause_ns gauge\n")
		fmt.Fprintf(w, "go_gc_pause_ns %d\n", mem.PauseNs[(mem.NumGC+255)%256])

		// 应用指标
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
		fmt.Fprintf(w, "# HELP ai_server_agent_tasks_total Total tasks\n")
		fmt.Fprintf(w, "# TYPE ai_server_agent_tasks_total counter\n")
		fmt.Fprintf(w, "ai_server_agent_tasks_total %d\n", len(tasks))
		fmt.Fprintf(w, "# HELP ai_server_agent_tasks_done Completed tasks\n")
		fmt.Fprintf(w, "# TYPE ai_server_agent_tasks_done counter\n")
		fmt.Fprintf(w, "ai_server_agent_tasks_done %d\n", done)
		fmt.Fprintf(w, "# HELP ai_server_agent_tasks_failed Failed tasks\n")
		fmt.Fprintf(w, "# TYPE ai_server_agent_tasks_failed counter\n")
		fmt.Fprintf(w, "ai_server_agent_tasks_failed %d\n", failed)

		// 版本信息
		fmt.Fprintf(w, "# HELP ai_server_agent_info Agent version info\n")
		fmt.Fprintf(w, "# TYPE ai_server_agent_info gauge\n")
		fmt.Fprintf(w, "ai_server_agent_info{version=\"%s\",commit=\"%s\"} 1\n", Version, Commit)
	})

	// Web API + 界面（/api/* 和 /）
	mux.Handle("/", webHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &http.Server{Addr: addr, Handler: mux}

	useTLS := tlsCert != "" && tlsKey != ""

	go func() {
		scheme := "http"
		if useTLS {
			scheme = "https"
		}
		fmt.Printf("Web 界面: %s://%s\n", scheme, addr)
		fmt.Printf("健康检查: %s://%s/health\n", scheme, addr)

		var err error
		if useTLS {
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
