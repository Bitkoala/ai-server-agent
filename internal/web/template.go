package web

import (
	"net/http"
)

// handleStatic 静态文件处理（嵌入的 HTML 模板）
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/login" {
		data, err := templateFS.ReadFile("templates/index.html")
		if err != nil {
			http.Error(w, "模板加载失败", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
		return
	}
	http.NotFound(w, r)
}
