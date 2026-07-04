package web

import (
	"embed"
	"net/http"
	"sync"

	"github.com/ai-server-agent/internal/core"
	"github.com/ai-server-agent/internal/storage"
)

//go:embed templates/*
var templateFS embed.FS

// Server Web API 服务
type Server struct {
	engine *core.Engine
	store  *storage.SQLiteStore
	auth   *AuthManager
	mux    *http.ServeMux
	mu     sync.Mutex
	config *ServerConfig
}

// ServerConfig 服务配置（用于设置 API）
type ServerConfig struct {
	ConfigPath string // config.yaml 路径
}

// NewServer 创建 Web API 服务
func NewServer(engine *core.Engine, store *storage.SQLiteStore, authCfg AuthConfig, srvCfg *ServerConfig) *Server {
	configPath := ""
	if srvCfg != nil {
		configPath = srvCfg.ConfigPath
	}
	s := &Server{
		engine: engine,
		store:  store,
		auth:   NewAuthManager(authCfg, configPath),
		config: srvCfg,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler 返回带认证中间件的 handler 链
func (s *Server) Handler() http.Handler {
	return s.auth.AuthMiddleware(corsMiddleware(s.mux))
}

// registerRoutes 注册所有路由
func (s *Server) registerRoutes() {
	// 认证路由
	s.auth.RegisterAuthRoutes(s.mux)

	// 业务 API
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/tasks", s.handleTasks)
	s.mux.HandleFunc("/api/tasks/confirm", s.handleConfirmTask)
	s.mux.HandleFunc("/api/history", s.handleHistory)
	s.mux.HandleFunc("/api/audit", s.handleAuditLog)
	s.mux.HandleFunc("/api/audit/verify", s.handleAuditVerify)

	// 设置 API
	s.mux.HandleFunc("/api/settings", s.handleSettings)

	// 静态文件
	s.mux.HandleFunc("/", s.handleStatic)
}
