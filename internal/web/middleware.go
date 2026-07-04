package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// CORSConfig CORS 配置
type CORSConfig struct {
	AllowedOrigins []string
}

// DefaultCORSConfig 返回默认 CORS 配置
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{AllowedOrigins: []string{}}
}

// corsMiddleware CORS 中间件，使用白名单验证 Origin
func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:9090":  true,
		"http://localhost:8080":  true,
		"http://127.0.0.1:9090":  true,
		"http://127.0.0.1:8080":  true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sendSSE 发送 Server-Sent Events 事件
func sendSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	flusher.Flush()
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
