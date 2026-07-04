package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ai-server-agent/internal/core"
	"github.com/ai-server-agent/internal/security"
	"github.com/ai-server-agent/internal/storage"
)

// newTestStore creates an in-memory SQLite store for web tests.
func newTestStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	store, err := storage.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// newTestEngine creates a core.Engine with mock components for web tests.
func newTestEngine(t *testing.T, store *storage.SQLiteStore) *core.Engine {
	t.Helper()
	cfg := core.DefaultEngineConfig()
	cfg.Storage = store
	cfg.SafeGuard = security.NewSafeGuard(security.Config{RateLimitPerMinute: 1000000})
	return core.NewEngine(cfg)
}

// ============ TestNewServer ============

func TestNewServer(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users:        nil,
	}

	srv := NewServer(engine, store, authCfg, nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.engine != engine {
		t.Error("engine not set correctly")
	}
	if srv.store != store {
		t.Error("store not set correctly")
	}
	if srv.auth == nil {
		t.Error("auth manager not initialized")
	}

	// Verify handler can be created without panic
	handler := srv.Handler()
	if handler == nil {
		t.Error("Handler() returned nil")
	}
}

func TestNewServer_WithNilEngine(t *testing.T) {
	store := newTestStore(t)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	// Should not panic with nil engine and store
	srv := NewServer(nil, store, authCfg, nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}

// ============ Auth Tests ============

func TestNewAuthManager(t *testing.T) {
	cfg := AuthConfig{
		Enabled:      true,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, "")
	if am == nil {
		t.Fatal("NewAuthManager returned nil")
	}

	if !am.IsEnabled() {
		t.Error("expected auth to be enabled")
	}

	if !am.NeedsSetup() {
		t.Error("expected NeedsSetup to return true for default admin/admin123")
	}
}

func TestNewAuthManager_Disabled(t *testing.T) {
	cfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, "")
	if am == nil {
		t.Fatal("NewAuthManager returned nil")
	}

	if am.IsEnabled() {
		t.Error("expected auth to be disabled")
	}

	if am.NeedsSetup() {
		t.Error("NeedsSetup should return false when auth is disabled")
	}
}

func TestAuthManager_Authenticate(t *testing.T) {
	cfg := AuthConfig{
		Enabled:      true,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, "")

	// Correct password should succeed
	user, err := am.Authenticate("admin", "admin123")
	if err != nil {
		t.Fatalf("Authenticate with correct password failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", user.Username)
	}
	if user.Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", user.Role)
	}

	// Wrong password should fail
	_, err = am.Authenticate("admin", "wrongpassword")
	if err == nil {
		t.Error("expected error for wrong password")
	}

	// Non-existent user should fail
	_, err = am.Authenticate("nonexistent", "password")
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

func TestAuthManager_GenerateAndValidateToken(t *testing.T) {
	cfg := AuthConfig{
		Enabled:      true,
		JWTSecret:    "test-secret-key-for-token-testing",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, "")

	user := &User{Username: "admin", Role: "admin"}
	tokenStr, err := am.GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected non-empty token")
	}

	// Validate the token
	parsedToken, err := am.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if !parsedToken.Valid {
		t.Error("expected token to be valid")
	}

	// Extract and verify claims
	claims, err := am.ExtractClaims(parsedToken)
	if err != nil {
		t.Fatalf("ExtractClaims failed: %v", err)
	}
	if claims["username"] != "admin" {
		t.Errorf("expected username 'admin', got '%v'", claims["username"])
	}
	if claims["role"] != "admin" {
		t.Errorf("expected role 'admin', got '%v'", claims["role"])
	}
}

func TestAuthManager_SetupAdmin(t *testing.T) {
	cfg := AuthConfig{
		Enabled:      true,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, "")

	// Before setup, NeedsSetup should be true
	if !am.NeedsSetup() {
		t.Error("expected NeedsSetup to be true before SetupAdmin")
	}

	// Setup admin with a new password
	err := am.SetupAdmin("admin", "newSecurePassword123")
	if err != nil {
		t.Fatalf("SetupAdmin failed: %v", err)
	}

	// After setup, NeedsSetup should be false
	if am.NeedsSetup() {
		t.Error("expected NeedsSetup to be false after SetupAdmin")
	}

	// Verify we can authenticate with the new password
	user, err := am.Authenticate("admin", "newSecurePassword123")
	if err != nil {
		t.Fatalf("Authenticate after SetupAdmin failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", user.Username)
	}

	// Old password should no longer work
	_, err = am.Authenticate("admin", "admin123")
	if err == nil {
		t.Error("expected old password to fail after SetupAdmin")
	}
}

func TestAuthManager_SetupAdmin_PasswordTooShort(t *testing.T) {
	cfg := AuthConfig{
		Enabled:      true,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, "")

	err := am.SetupAdmin("admin", "12345")
	if err == nil {
		t.Error("expected error for short password")
	}
}

func TestAuthManager_SetupAdmin_WithConfigPath(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a simple config
	configContent := "auth:\n  enabled: true\n  jwt_secret: test-secret\n  users:\n    - username: admin\n      password: admin123\n      role: admin\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg := AuthConfig{
		Enabled:      true,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
		Users: []User{
			{Username: "admin", Password: "admin123", Role: "admin"},
		},
	}

	am := NewAuthManager(cfg, configPath)

	err := am.SetupAdmin("admin", "newSecurePassword123")
	if err != nil {
		t.Fatalf("SetupAdmin with config path failed: %v", err)
	}

	if am.NeedsSetup() {
		t.Error("expected NeedsSetup to be false after SetupAdmin")
	}
}

// ============ Middleware Tests ============

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()

	writeJSON(w, http.StatusOK, map[string]string{"message": "hello"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["message"] != "hello" {
		t.Errorf("expected message 'hello', got '%s'", resp["message"])
	}
}

func TestWriteJSON_ErrorStatus(t *testing.T) {
	w := httptest.NewRecorder()

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// OPTIONS request with allowed origin
	req := httptest.NewRequest(http.MethodOptions, "/api/chat", nil)
	req.Header.Set("Origin", "http://localhost:9090")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:9090" {
		t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:9090', got '%s'", allowOrigin)
	}

	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Error("expected non-empty Access-Control-Allow-Methods")
	}

	allowHeaders := w.Header().Get("Access-Control-Allow-Headers")
	if allowHeaders == "" {
		t.Error("expected non-empty Access-Control-Allow-Headers")
	}

	allowCredentials := w.Header().Get("Access-Control-Allow-Credentials")
	if allowCredentials != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials 'true', got '%s'", allowCredentials)
	}
}

func TestCORSMiddleware_AllowedOrigin_GET(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET request with allowed origin - CORS headers should be set, request should pass through
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	req.Header.Set("Origin", "http://localhost:8080")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:8080" {
		t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:8080', got '%s'", allowOrigin)
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request with disallowed origin
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	req.Header.Set("Origin", "http://evil.example.com")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Request should still be processed (not blocked), but without CORS headers
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for disallowed origin, got '%s'", allowOrigin)
	}
}

func TestCORSMiddleware_NoOrigin(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without Origin header
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// No CORS headers should be set when no Origin
	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "" {
		t.Errorf("expected no Access-Control-Allow-Origin, got '%s'", allowOrigin)
	}
}

// ============ Handler Tests ============

func TestHandleTasks_NotFound(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	srv := NewServer(engine, store, authCfg, nil)

	// Request non-existent task
	req := httptest.NewRequest(http.MethodGet, "/api/tasks?id=nonexistent_task", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "任务不存在" {
		t.Errorf("expected error '任务不存在', got '%s'", resp["error"])
	}
}

func TestHandleTasks_List(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	srv := NewServer(engine, store, authCfg, nil)

	// Request task list (empty store)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var tasks []interface{}
	if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// Should get null or empty array for empty store
	t.Logf("tasks response: %v", tasks)
}

func TestHandleHistory(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	srv := NewServer(engine, store, authCfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestHandleAuditVerify(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	srv := NewServer(engine, store, authCfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify response has valid and tampered fields
	if _, ok := resp["valid"]; !ok {
		t.Error("expected 'valid' field in response")
	}
	if _, ok := resp["tampered"]; !ok {
		t.Error("expected 'tampered' field in response")
	}

	// valid should be a boolean, tampered should be an array
	if valid, ok := resp["valid"].(bool); ok {
		t.Logf("valid=%v", valid)
	} else {
		t.Error("expected 'valid' to be boolean")
	}
}

func TestHandleSettings_NoConfig(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	// Server without config path
	srv := NewServer(engine, store, authCfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return default settings with a note
	if note, ok := resp["note"]; ok {
		t.Logf("note: %v", note)
	}
}

func TestHandleSettings_MethodNotAllowed(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	srv := NewServer(engine, store, authCfg, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/settings", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandler_AuthStatus(t *testing.T) {
	store := newTestStore(t)
	engine := newTestEngine(t, store)

	authCfg := AuthConfig{
		Enabled:      false,
		JWTSecret:    "test-secret",
		TokenExpiry:  24,
	}

	srv := NewServer(engine, store, authCfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if enabled, ok := resp["enabled"].(bool); ok {
		if enabled {
			t.Error("expected enabled to be false")
		}
	}
	if authMode, ok := resp["auth_mode"].(string); ok {
		if authMode != "disabled" {
			t.Errorf("expected auth_mode 'disabled', got '%s'", authMode)
		}
	}
}
