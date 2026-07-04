package web

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/ai-server-agent/internal/storage"
	"gopkg.in/yaml.v3"
)

// ============ 聊天 API ============

// handleChat 处理聊天请求（SSE 流式）
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}

	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "消息不能为空"})
		return
	}

	// SSE 流式响应
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式响应", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// 1. 发送"理解中"事件
	sendSSE(w, flusher, "thinking", map[string]interface{}{"message": "正在理解你的意图..."})

	// 2. 解析意图
	task, err := s.engine.Execute(ctx, req.Message)
	if err != nil {
		sendSSE(w, flusher, "error", map[string]interface{}{"error": err.Error()})
		sendSSE(w, flusher, "done", nil)
		return
	}

	// 3. 发送任务计划
	sendSSE(w, flusher, "plan", map[string]interface{}{
		"task_id": task.ID,
		"intent":  task.Intent,
		"status":  task.Status,
		"steps":   task.Steps,
	})

	// 4. 如果需要确认，等待确认
	if task.Status == "awaiting_confirmation" || task.Status == "pending" {
		sendSSE(w, flusher, "confirm_required", map[string]interface{}{
			"task_id": task.ID,
			"message": "有步骤需要确认，请调用 /api/tasks/confirm",
		})
		sendSSE(w, flusher, "done", nil)
		return
	}

	// 5. 自动执行
	sendSSE(w, flusher, "executing", map[string]interface{}{"task_id": task.ID})

	execTask, err := s.engine.ConfirmAndRun(ctx, task.ID)
	if err != nil {
		sendSSE(w, flusher, "error", map[string]interface{}{"error": err.Error(), "task": execTask})
		sendSSE(w, flusher, "done", nil)
		return
	}

	// 6. 发送执行结果
	sendSSE(w, flusher, "result", map[string]interface{}{
		"task_id": execTask.ID,
		"status":  execTask.Status,
		"steps":   execTask.Steps,
	})

	sendSSE(w, flusher, "done", nil)
}

// handleConfirmTask 确认任务并执行
func (s *Server) handleConfirmTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TaskID  string `json:"task_id"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}

	if !req.Confirm {
		task, _ := s.store.GetTask(req.TaskID)
		if task != nil {
			task.Status = "cancelled"
			s.store.UpdateTask(task)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
		return
	}

	ctx := r.Context()
	task, err := s.engine.ConfirmAndRun(ctx, req.TaskID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
			"task":  task,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": task.Status,
		"steps":  task.Steps,
	})
}

// ============ 任务/历史 API ============

// handleTasks 查询任务
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		tasks, _ := s.store.ListTasks(20)
		writeJSON(w, http.StatusOK, tasks)
		return
	}

	task, err := s.store.GetTask(taskID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "任务不存在"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// handleHistory 查询历史
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	tasks, _ := s.store.ListTasks(20)
	writeJSON(w, http.StatusOK, tasks)
}

// ============ 审计 API ============

// handleAuditLog 查询审计日志
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	format := r.URL.Query().Get("format")

	auditLogger, _ := storage.NewAuditLogger(s.store)
	if format == "json" {
		data, err := auditLogger.ExportJSON(taskID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=audit.json")
		w.Write(data)
		return
	}

	entries, err := auditLogger.Query(taskID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleAuditVerify 验证审计链完整性
func (s *Server) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	auditLogger, _ := storage.NewAuditLogger(s.store)
	valid, tampered, err := auditLogger.Verify()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":    valid,
		"tampered": tampered,
	})
}

// ============ 设置 API ============

// handleSettings 获取/更新设置
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getSettings(w, r)
	case http.MethodPut, http.MethodPost:
		s.updateSettings(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	if s.config == nil || s.config.ConfigPath == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"llm":    map[string]string{"provider": "openai", "model": "gpt-4o-mini"},
			"notify": map[string]string{"feishu": "", "dingtalk": "", "wecom": "", "telegram_bot_token": "", "telegram_chat_id": ""},
			"security": map[string]interface{}{"require_confirm": true, "rate_limit": 30},
			"note":   "配置文件路径未设置，显示默认值",
		})
		return
	}

	data, err := os.ReadFile(s.config.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "读取配置失败: " + err.Error()})
		return
	}

	var rawCfg map[string]interface{}
	if err := yaml.Unmarshal(data, &rawCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "解析配置失败"})
		return
	}

	// 脱敏处理：隐藏 API 密钥
	maskAPIKey(rawCfg, "llm", "api_key")
	maskAPIKey(rawCfg, "onepanel", "api_key")

	writeJSON(w, http.StatusOK, rawCfg)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	if s.config == nil || s.config.ConfigPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "配置文件路径未设置，无法保存"})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}

	// 读取现有配置
	data, err := os.ReadFile(s.config.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "读取配置失败"})
		return
	}

	var currentCfg map[string]interface{}
	if err := yaml.Unmarshal(data, &currentCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "解析配置失败"})
		return
	}

	// 合并更新（深度合并）
	mergeMap(currentCfg, updates)

	// 写回
	newData, err := yaml.Marshal(currentCfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "序列化配置失败"})
		return
	}

	if err := os.WriteFile(s.config.ConfigPath, newData, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "写入配置失败: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "配置已更新，请重启服务生效"})
}

// ============ 辅助函数 ============

// mergeMap 深度合并两个 map
func mergeMap(dst, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				mergeMap(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

// maskAPIKey 脱敏处理：将 map 中指定 section 的 key 中间部分替换为 ****
func maskAPIKey(cfg map[string]interface{}, section, key string) {
	if sec, ok := cfg[section].(map[string]interface{}); ok {
		if val, ok := sec[key].(string); ok && len(val) > 8 {
			sec[key] = val[:4] + "****" + val[len(val)-4:]
		}
	}
}
