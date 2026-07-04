package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ai-server-agent/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled        bool   `yaml:"enabled"`
	JWTSecret      string `yaml:"jwt_secret"`
	JWTSigningAlg  string `yaml:"jwt_signing_alg"`  // HS256 (默认) / RS256
	JWTPrivateKey  string `yaml:"jwt_private_key"`  // RS256 私钥路径
	JWTPublicKey   string `yaml:"jwt_public_key"`   // RS256 公钥路径
	TokenExpiry    int    `yaml:"token_expiry_hours"` // 默认 24 小时
	Users          []User `yaml:"users"`
}

// User 用户定义
type User struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"` // bcrypt hash，配置文件中可以是明文
	Role     string `yaml:"role"`     // admin / viewer
}

// AuthManager 认证管理器
type AuthManager struct {
	config     AuthConfig
	configPath string   // 配置文件路径，用于回写
	setupDone  bool     // 是否已完成初始化设置
}

// NewAuthManager 创建认证管理器
func NewAuthManager(cfg AuthConfig, configPath string) *AuthManager {
	// 如果没有设置 secret，生成一个随机的
	if cfg.JWTSecret == "" {
		secret := make([]byte, 32)
		rand.Read(secret)
		cfg.JWTSecret = hex.EncodeToString(secret)
	}

	if cfg.TokenExpiry <= 0 {
		cfg.TokenExpiry = 24
	}

	// 对明文密码进行 bcrypt 哈希处理
	for i, u := range cfg.Users {
		if !strings.HasPrefix(u.Password, "$2a$") && !strings.HasPrefix(u.Password, "$2b$") {
			hashed, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
			if err == nil {
				cfg.Users[i].Password = string(hashed)
			}
		}
	}

	// 判断是否已完成初始化：用户列表非空且密码不是默认的 admin123
	setupDone := false
	if len(cfg.Users) > 0 {
		// 检查是否还存在默认密码（admin/admin123）
		defaultPwd := false
		for _, u := range cfg.Users {
			if u.Username == "admin" {
				err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("admin123"))
				if err == nil {
					defaultPwd = true // 还在用默认密码
				}
			}
		}
		setupDone = !defaultPwd && len(cfg.Users) > 0
	}

	return &AuthManager{
		config:     cfg,
		configPath: configPath,
		setupDone:  setupDone,
	}
}

// IsEnabled 是否启用认证
func (a *AuthManager) IsEnabled() bool {
	return a.config.Enabled
}

// NeedsSetup 是否需要初始化设置（首次部署引导）
func (a *AuthManager) NeedsSetup() bool {
	return a.config.Enabled && !a.setupDone
}

// SetupAdmin 初始化管理员密码
func (a *AuthManager) SetupAdmin(username, password string) error {
	if username == "" {
		username = "admin"
	}
	if len(password) < 6 {
		return models.ErrPasswordTooShort
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %w", err)
	}

	a.config.Users = []User{
		{Username: username, Password: string(hashed), Role: "admin"},
	}
	a.setupDone = true

	// 回写到配置文件
	if a.configPath != "" {
		return a.saveConfig()
	}
	return nil
}

// saveConfig 将当前用户配置回写到 YAML 配置文件
func (a *AuthManager) saveConfig() error {
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析为通用 map 以保留其他配置
	var rawCfg map[string]interface{}
	if err := yaml.Unmarshal(data, &rawCfg); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	// 更新 auth 段
	authMap := map[string]interface{}{
		"enabled":            a.config.Enabled,
		"jwt_secret":         a.config.JWTSecret,
		"token_expiry_hours": a.config.TokenExpiry,
		"users":              a.config.Users,
	}
	rawCfg["auth"] = authMap

	newData, err := yaml.Marshal(rawCfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(a.configPath, newData, 0644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	return nil
}

// getSigningMethod 获取 JWT 签名方法
func (a *AuthManager) getSigningMethod() jwt.SigningMethod {
	switch a.config.JWTSigningAlg {
	case "RS256":
		return jwt.SigningMethodRS256
	default:
		return jwt.SigningMethodHS256
	}
}

// getSigningKey 获取签名密钥
func (a *AuthManager) getSigningKey() (interface{}, error) {
	switch a.config.JWTSigningAlg {
	case "RS256":
		return a.loadRSAPrivateKey()
	default:
		return []byte(a.config.JWTSecret), nil
	}
}

// loadRSAPrivateKey 加载 RSA 私钥
func (a *AuthManager) loadRSAPrivateKey() (interface{}, error) {
	if a.config.JWTPrivateKey == "" {
		return nil, fmt.Errorf("RS256 模式需要配置 jwt_private_key 路径")
	}
	data, err := os.ReadFile(a.config.JWTPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("读取 RSA 私钥失败: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("解析 RSA 私钥失败: %w", err)
	}
	return key, nil
}

// loadRSAPublicKey 加载 RSA 公钥
func (a *AuthManager) loadRSAPublicKey() (interface{}, error) {
	if a.config.JWTPublicKey == "" {
		return nil, fmt.Errorf("RS256 模式需要配置 jwt_public_key 路径")
	}
	data, err := os.ReadFile(a.config.JWTPublicKey)
	if err != nil {
		return nil, fmt.Errorf("读取 RSA 公钥失败: %w", err)
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("解析 RSA 公钥失败: %w", err)
	}
	return key, nil
}

// Authenticate 验证用户凭据
func (a *AuthManager) Authenticate(username, password string) (*User, error) {
	for _, u := range a.config.Users {
		if u.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
				return nil, models.ErrInvalidCredentials
			}
			return &u, nil
		}
	}
	return nil, models.ErrUserNotFound
}

// GenerateToken 生成 JWT Token（支持 HS256/RS256）
func (a *AuthManager) GenerateToken(user *User) (string, error) {
	claims := jwt.MapClaims{
		"username": user.Username,
		"role":     user.Role,
		"exp":      time.Now().Add(time.Duration(a.config.TokenExpiry) * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	method := a.getSigningMethod()
	token := jwt.NewWithClaims(method, claims)

	key, err := a.getSigningKey()
	if err != nil {
		return "", fmt.Errorf("获取签名密钥失败: %w", err)
	}
	return token.SignedString(key)
}

// ValidateToken 验证 JWT Token（支持 HS256/RS256）
func (a *AuthManager) ValidateToken(tokenStr string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		expectedAlg := a.config.JWTSigningAlg
		if expectedAlg == "" {
			expectedAlg = "HS256"
		}
		if token.Header["alg"] != expectedAlg {
			return nil, fmt.Errorf("%w: 期望 %s, 实际 %v", models.ErrUnsupportedSignMethod, expectedAlg, token.Header["alg"])
		}

		switch token.Method.(type) {
		case *jwt.SigningMethodHMAC:
			return []byte(a.config.JWTSecret), nil
		case *jwt.SigningMethodRSA:
			return a.loadRSAPublicKey()
		default:
			return nil, fmt.Errorf("%w: %v", models.ErrUnsupportedSignMethod, token.Header["alg"])
		}
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, models.ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", models.ErrTokenInvalid, err)
	}
	if !token.Valid {
		return nil, models.ErrTokenInvalid
	}
	return token, nil
}

// ExtractClaims 从 token 中提取 claims
func (a *AuthManager) ExtractClaims(token *jwt.Token) (jwt.MapClaims, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("无法解析 claims")
	}
	return claims, nil
}

// AuthMiddleware 认证中间件
func (a *AuthManager) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 如果未启用认证，直接放行
		if !a.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// 这些路径不需要认证
		noAuthPaths := []string{
			"/api/auth/login", "/api/auth/status", "/api/auth/setup",
			"/api/auth/logout", "/api/auth/refresh",
			"/health", "/health/ready", "/metrics",
			"/login", "/favicon.ico",
		}
		for _, p := range noAuthPaths {
			if r.URL.Path == p {
				next.ServeHTTP(w, r)
				return
			}
		}

		// 初始化向导期间，允许访问静态首页（不含 API）
		if a.NeedsSetup() && r.URL.Path == "/" {
			next.ServeHTTP(w, r)
			return
		}

		// 从 Authorization header 获取 token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			cookie, err := r.Cookie("auth_token")
			if err == nil && cookie.Value != "" {
				authHeader = "Bearer " + cookie.Value
			}
		}

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未认证，请先登录"})
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := a.ValidateToken(tokenStr)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "认证已过期，请重新登录"})
			return
		}

		claims, err := a.ExtractClaims(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "无效的认证信息"})
			return
		}

		r.Header.Set("X-User-Name", fmt.Sprintf("%v", claims["username"]))
		r.Header.Set("X-User-Role", fmt.Sprintf("%v", claims["role"]))

		next.ServeHTTP(w, r)
	})
}

// RegisterAuthRoutes 注册认证相关路由
func (a *AuthManager) RegisterAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", a.handleLogin)
	mux.HandleFunc("/api/auth/status", a.handleAuthStatus)
	mux.HandleFunc("/api/auth/logout", a.handleLogout)
	mux.HandleFunc("/api/auth/refresh", a.handleRefresh)
	mux.HandleFunc("/api/auth/setup", a.handleSetup)
}

// handleSetup 初始化管理员密码（仅 NeedsSetup 时可用）
func (a *AuthManager) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	if !a.NeedsSetup() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "初始化已完成，无法重复设置"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}

	if req.Username == "" {
		req.Username = "admin"
	}
	if len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码长度至少 6 位"})
		return
	}

	if err := a.SetupAdmin(req.Username, req.Password); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// 设置完成后自动登录
	user := &User{Username: req.Username, Role: "admin"}
	token, _ := a.GenerateToken(user)

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   a.config.TokenExpiry * 3600,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "初始化完成",
		"token":    token,
		"username": req.Username,
		"role":     "admin",
	})
}

// handleLogin 处理登录请求
func (a *AuthManager) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}

	user, err := a.Authenticate(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	token, err := a.GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "生成 token 失败"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   a.config.TokenExpiry * 3600,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":    token,
		"username": user.Username,
		"role":     user.Role,
	})
}

// handleAuthStatus 检查认证状态
func (a *AuthManager) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if !a.config.Enabled {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":     false,
			"auth_mode":   "disabled",
			"needs_setup": false,
		})
		return
	}

	// 尝试从 cookie 或 header 获取 token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		cookie, err := r.Cookie("auth_token")
		if err == nil && cookie.Value != "" {
			authHeader = "Bearer " + cookie.Value
		}
	}

	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":     true,
			"auth_mode":   "jwt",
			"logged_in":   false,
			"needs_setup": a.NeedsSetup(),
		})
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	token, err := a.ValidateToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":     true,
			"auth_mode":   "jwt",
			"logged_in":   false,
			"needs_setup": a.NeedsSetup(),
		})
		return
	}

	claims, _ := a.ExtractClaims(token)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":     true,
		"auth_mode":   "jwt",
		"logged_in":   true,
		"username":    claims["username"],
		"role":        claims["role"],
		"needs_setup": a.NeedsSetup(),
	})
}

// handleLogout 处理登出
func (a *AuthManager) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "已登出"})
}

// handleRefresh 刷新 token
func (a *AuthManager) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		cookie, err := r.Cookie("auth_token")
		if err == nil && cookie.Value != "" {
			authHeader = "Bearer " + cookie.Value
		}
	}

	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未认证"})
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	token, err := a.ValidateToken(tokenStr)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token 无效或已过期"})
		return
	}

	claims, _ := a.ExtractClaims(token)
	newToken, err := a.GenerateToken(&User{
		Username: fmt.Sprintf("%v", claims["username"]),
		Role:     fmt.Sprintf("%v", claims["role"]),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "刷新失败"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    newToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   a.config.TokenExpiry * 3600,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": newToken,
	})
}
