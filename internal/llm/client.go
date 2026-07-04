package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-server-agent/internal/models"
)

// Config LLM 客户端配置
type Config struct {
	Provider string // openai / ollama / local
	APIKey   string
	BaseURL  string
	Model    string
}

// Client LLM 统一客户端（门面模式）
type Client struct {
	config   Config
	provider Provider
}

// NewClient 创建 LLM 客户端，根据配置自动选择 Provider
func NewClient(cfg Config) *Client {
	c := &Client{config: cfg}

	switch cfg.Provider {
	case "openai", "":
		c.provider = NewOpenAI(cfg.APIKey, cfg.BaseURL, cfg.Model)
	case "ollama":
		c.provider = NewOllama(cfg.BaseURL, cfg.Model)
	default:
		c.provider = NewOpenAI(cfg.APIKey, cfg.BaseURL, cfg.Model)
	}

	return c
}

// Provider 返回底层 Provider（供 engine 直接调用）
func (c *Client) Provider() Provider {
	return c.provider
}

// ParseIntent 意图解析（保持原有接口兼容）
func (c *Client) ParseIntent(ctx context.Context, userInput string) (string, []models.TaskStep, error) {
	// 先尝试本地模板匹配
	if intent, steps := matchLocalTemplate(userInput); intent != "" {
		return intent, steps, nil
	}

	// 未命中则调 LLM
	actions := getAvailableActions()
	result, err := c.provider.ParseIntent(ctx, userInput, actions)
	if err != nil {
		// LLM 调用失败，降级到本地分类
		intent := classifyIntent(userInput)
		steps := generateSteps(intent)
		return intent, steps, nil
	}

	// 转换 LLM 返回的 steps
	steps := make([]models.TaskStep, len(result.Steps))
	for i, s := range result.Steps {
		status := "pending"
		if s.Status == "confirm_required" || s.Params["need_confirm"] == "true" {
			status = "confirm_required"
		}
		steps[i] = models.TaskStep{
			ID:     fmt.Sprintf("step_%d", i+1),
			Action: s.Action,
			Params: s.Params,
			Status: status,
		}
	}

	return result.Intent, steps, nil
}

// Chat 标准对话
func (c *Client) Chat(ctx context.Context, history []map[string]string, userMessage string) (string, error) {
	messages := make([]Message, 0, len(history)+2)
	messages = append(messages, Message{
		Role:    "system",
		Content: "你是 AI Server Agent，一个 Linux 服务器管理助手。请用中文回复。",
	})
	for _, h := range history {
		messages = append(messages, Message{
			Role:    h["role"],
			Content: h["content"],
		})
	}
	messages = append(messages, Message{
		Role:    "user",
		Content: userMessage,
	})

	resp, err := c.provider.Chat(ctx, messages)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// ChatStream 流式对话
func (c *Client) ChatStream(ctx context.Context, history []map[string]string, userMessage string) (<-chan StreamChunk, error) {
	messages := make([]Message, 0, len(history)+2)
	messages = append(messages, Message{
		Role:    "system",
		Content: "你是 AI Server Agent，一个 Linux 服务器管理助手。请用中文回复。",
	})
	for _, h := range history {
		messages = append(messages, Message{
			Role:    h["role"],
			Content: h["content"],
		})
	}
	messages = append(messages, Message{
		Role:    "user",
		Content: userMessage,
	})

	return c.provider.ChatStream(ctx, messages)
}

// ============ 本地模板匹配（50+ 高频命令，无需调 LLM）============

// cmdTemplate 本地命令模板
type cmdTemplate struct {
	patterns []string          // 匹配关键词（任一命中即匹配，不区分大小写）
	intent   string            // 意图描述
	steps    []models.TaskStep // 预定义步骤
}

var localTemplates = []cmdTemplate{
	// ============ 监控查询类 (低风险) ============
	{patterns: []string{"cpu", "处理器", "cpu使用"}, intent: "查看 CPU 使用率", steps: []models.TaskStep{{ID: "s1", Action: "monitor.cpu", Status: "pending"}}},
	{patterns: []string{"内存", "memory", "ram", "运存"}, intent: "查看内存使用情况", steps: []models.TaskStep{{ID: "s1", Action: "monitor.memory", Status: "pending"}}},
	{patterns: []string{"磁盘", "硬盘", "存储空间", "disk", "容量"}, intent: "查看磁盘使用情况", steps: []models.TaskStep{{ID: "s1", Action: "monitor.disk", Status: "pending"}}},
	{patterns: []string{"网络", "流量", "network", "带宽"}, intent: "查看网络流量", steps: []models.TaskStep{{ID: "s1", Action: "monitor.network", Status: "pending"}}},
	{patterns: []string{"系统信息", "系统状态", "服务器信息", "系统概览", "uname"}, intent: "查看系统信息", steps: []models.TaskStep{{ID: "s1", Action: "system.info", Status: "pending"}}},
	{patterns: []string{"健康检查", "health", "服务状态", "运行正常"}, intent: "健康检查", steps: []models.TaskStep{{ID: "s1", Action: "health", Status: "pending"}}},

	// ============ 监控组合类 ============
	{patterns: []string{"查看监控", "系统监控", "运行状态", "服务器状态", "资源使用", "资源占用", "系统负载"}, intent: "系统综合监控", steps: []models.TaskStep{
		{ID: "s1", Action: "monitor.cpu", Status: "pending"},
		{ID: "s2", Action: "monitor.memory", Status: "pending"},
		{ID: "s3", Action: "monitor.disk", Status: "pending"},
	}},

	// ============ 容器操作 ============
	{patterns: []string{"容器日志", "查看日志", "logs", "容器log"}, intent: "查看容器日志", steps: []models.TaskStep{{ID: "s1", Action: "container.logs", Params: map[string]string{"lines": "100"}, Status: "pending"}}},
	{patterns: []string{"容器列表", "列出容器", "查看容器", "docker ps", "运行中的容器", "所有容器"}, intent: "列出所有容器", steps: []models.TaskStep{{ID: "s1", Action: "container.list", Status: "pending"}}},
	{patterns: []string{"启动容器", "start", "开启容器"}, intent: "启动容器", steps: []models.TaskStep{{ID: "s1", Action: "container.start", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"停止容器", "stop", "关闭容器"}, intent: "停止容器", steps: []models.TaskStep{{ID: "s1", Action: "container.stop", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}}},
	{patterns: []string{"重启容器", "restart", "重新启动容器"}, intent: "重启容器", steps: []models.TaskStep{{ID: "s1", Action: "container.restart", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}}},

	// ============ 应用部署 (精确匹配优先) ============
	{patterns: []string{"部署wordpress", "安装wordpress", "部署 wordpress", "安装 wordpress", "搭建wordpress"}, intent: "部署 WordPress", steps: []models.TaskStep{
		{ID: "s1", Action: "app.install", Params: map[string]string{"name": "wordpress"}, Status: "preview"},
		{ID: "s2", Action: "nginx.reload", Status: "pending"},
	}},
	{patterns: []string{"部署nginx", "安装nginx", "部署 nginx", "安装 nginx"}, intent: "安装 Nginx", steps: []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "nginx"}, Status: "preview"}}},
	{patterns: []string{"部署mysql", "安装mysql", "部署 mysql", "安装 mysql", "数据库安装"}, intent: "安装 MySQL", steps: []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "mysql"}, Status: "preview"}}},
	{patterns: []string{"部署redis", "安装redis", "部署 redis", "安装 redis"}, intent: "安装 Redis", steps: []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "redis"}, Status: "preview"}}},
	{patterns: []string{"部署node", "安装node", "部署 node", "nodejs"}, intent: "安装 Node.js", steps: []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "nodejs"}, Status: "preview"}}},
	{patterns: []string{"部署", "安装", "deploy", "install"}, intent: "安装应用", steps: []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"应用列表", "已安装", "已部署", "app list"}, intent: "列出已安装应用", steps: []models.TaskStep{{ID: "s1", Action: "app.list", Status: "pending"}}},
	{patterns: []string{"卸载", "删除应用", "uninstall", "移除"}, intent: "卸载应用", steps: []models.TaskStep{{ID: "s1", Action: "app.uninstall", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}}},

	// ============ SSL / 证书 ============
	{patterns: []string{"ssl", "证书", "https", "申请证书", "配置证书", "lets encrypt", "tls"}, intent: "SSL 证书管理", steps: []models.TaskStep{
		{ID: "s1", Action: "ssl.status", Status: "pending"},
		{ID: "s2", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
	}},
	{patterns: []string{"续期证书", "证书过期", "renew", "更新证书", "续签"}, intent: "续期 SSL 证书", steps: []models.TaskStep{{ID: "s1", Action: "ssl.renew", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"}}},

	// ============ Nginx ============
	{patterns: []string{"重载nginx", "nginx重载", "reload nginx", "nginx reload", "重载配置", "重启nginx", "nginx重启"}, intent: "重载 Nginx 配置", steps: []models.TaskStep{{ID: "s1", Action: "nginx.reload", Status: "preview"}}},
	{patterns: []string{"nginx状态", "nginx status", "nginx运行"}, intent: "查看 Nginx 状态", steps: []models.TaskStep{{ID: "s1", Action: "nginx.status", Status: "pending"}}},

	// ============ 数据库 ============
	{patterns: []string{"数据库列表", "查看数据库", "列出数据库", "show databases"}, intent: "列出所有数据库", steps: []models.TaskStep{{ID: "s1", Action: "database.list", Status: "pending"}}},
	{patterns: []string{"创建数据库", "新建数据库", "create database"}, intent: "创建数据库", steps: []models.TaskStep{{ID: "s1", Action: "database.create", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"备份数据库", "数据库备份", "backup database", "备份数据"}, intent: "备份数据库", steps: []models.TaskStep{{ID: "s1", Action: "database.backup", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"删除数据库", "drop database", "移除数据库"}, intent: "删除数据库", steps: []models.TaskStep{{ID: "s1", Action: "database.delete", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}}},
	{patterns: []string{"恢复数据库", "数据库恢复", "restore database"}, intent: "恢复数据库", steps: []models.TaskStep{{ID: "s1", Action: "database.restore", Params: map[string]string{"db_name": "auto_detect"}, Status: "confirm_required"}}},
	{patterns: []string{"慢查询", "慢sql", "slow query", "慢日志", "性能分析"}, intent: "分析慢查询", steps: []models.TaskStep{{ID: "s1", Action: "database.slow_query", Status: "pending"}}},
	{patterns: []string{"数据库连接", "连接数", "数据库连接数", "活跃连接"}, intent: "查看数据库连接数", steps: []models.TaskStep{{ID: "s1", Action: "database.connections", Status: "pending"}}},
	{patterns: []string{"优化数据库", "优化表", "optimize table", "数据库优化"}, intent: "优化数据库表", steps: []models.TaskStep{{ID: "s1", Action: "database.optimize", Status: "preview"}}},

	// ============ Docker Compose 编排 ============
	{patterns: []string{"搭建博客", "wordpress博客", "部署博客"}, intent: "部署 WordPress + MySQL", steps: []models.TaskStep{
		{ID: "s1", Action: "compose.generate", Params: map[string]string{"template": "wordpress"}, Status: "preview"},
		{ID: "s2", Action: "container.start", Params: map[string]string{"name": "wordpress"}, Status: "preview"},
		{ID: "s3", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
	}},
	{patterns: []string{"部署nextjs", "next.js", "nextjs应用", "全栈应用", "next应用"}, intent: "部署 Next.js + PostgreSQL + Redis", steps: []models.TaskStep{
		{ID: "s1", Action: "compose.generate", Params: map[string]string{"template": "nextjs"}, Status: "preview"},
		{ID: "s2", Action: "container.start", Params: map[string]string{"name": "app"}, Status: "preview"},
	}},
	{patterns: []string{"部署监控", "监控系统", "prometheus", "grafana", "搭建监控"}, intent: "部署 Prometheus + Grafana 监控", steps: []models.TaskStep{
		{ID: "s1", Action: "compose.generate", Params: map[string]string{"template": "monitoring"}, Status: "preview"},
		{ID: "s2", Action: "container.start", Params: map[string]string{"name": "prometheus"}, Status: "preview"},
	}},
	{patterns: []string{"部署网站", "搭建网站", "建站", "上线网站", "新建站点"}, intent: "完整建站流程", steps: []models.TaskStep{
		{ID: "s1", Action: "app.install", Params: map[string]string{"name": "auto_detect"}, Status: "preview"},
		{ID: "s2", Action: "nginx.reload", Status: "pending"},
		{ID: "s3", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
	}},

	// ============ 网站管理 ============
	{patterns: []string{"网站列表", "列出网站", "查看网站", "站点列表"}, intent: "列出所有网站", steps: []models.TaskStep{{ID: "s1", Action: "website.list", Status: "pending"}}},
	{patterns: []string{"反向代理", "配置代理", "proxy", "反代", "代理转发"}, intent: "配置反向代理", steps: []models.TaskStep{{ID: "s1", Action: "website.proxy", Params: map[string]string{"domain": "auto_detect", "target": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"绑定域名", "添加域名", "域名绑定", "新增域名"}, intent: "绑定域名", steps: []models.TaskStep{{ID: "s1", Action: "website.domain", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"}}},

	// ============ 文件操作 ============
	{patterns: []string{"文件列表", "查看文件", "文件管理", "ls", "目录"}, intent: "查看文件列表", steps: []models.TaskStep{{ID: "s1", Action: "file.list", Status: "pending"}}},
	{patterns: []string{"读取文件", "查看内容", "cat", "文件内容", "读文件"}, intent: "读取文件内容", steps: []models.TaskStep{{ID: "s1", Action: "file.read", Params: map[string]string{"path": "auto_detect"}, Status: "pending"}}},
	{patterns: []string{"上传文件", "upload", "传文件"}, intent: "上传文件", steps: []models.TaskStep{{ID: "s1", Action: "file.upload", Params: map[string]string{"path": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"删除文件", "rm", "移除文件", "删文件"}, intent: "删除文件", steps: []models.TaskStep{{ID: "s1", Action: "file.delete", Params: map[string]string{"path": "auto_detect"}, Status: "confirm_required"}}},

	// ============ 系统操作 ============
	{patterns: []string{"重启服务器", "重启系统", "reboot", "shutdown", "关机"}, intent: "重启系统", steps: []models.TaskStep{{ID: "s1", Action: "system.restart", Status: "confirm_required"}}},
	{patterns: []string{"系统时间", "当前时间", "date", "时区"}, intent: "查看系统时间", steps: []models.TaskStep{{ID: "s1", Action: "system.time", Status: "pending"}}},
	{patterns: []string{"进程列表", "查看进程", "ps", "top", "运行进程"}, intent: "查看进程列表", steps: []models.TaskStep{{ID: "s1", Action: "system.processes", Status: "pending"}}},
	{patterns: []string{"清理缓存", "释放内存", "清理内存", "free memory"}, intent: "清理系统缓存", steps: []models.TaskStep{{ID: "s1", Action: "system.clear_cache", Status: "preview"}}},

	// ============ Docker 镜像管理 ============
	{patterns: []string{"镜像列表", "docker images", "查看镜像", "列出镜像"}, intent: "列出 Docker 镜像", steps: []models.TaskStep{{ID: "s1", Action: "container.images", Status: "pending"}}},
	{patterns: []string{"拉取镜像", "pull", "下载镜像"}, intent: "拉取 Docker 镜像", steps: []models.TaskStep{{ID: "s1", Action: "container.pull", Params: map[string]string{"image": "auto_detect"}, Status: "preview"}}},
	{patterns: []string{"清理镜像", "删除镜像", "rmi", "prune"}, intent: "清理 Docker 镜像", steps: []models.TaskStep{{ID: "s1", Action: "container.prune", Status: "confirm_required"}}},
}

// matchLocalTemplate 本地模板匹配 — 覆盖 50+ 高频操作，命中则跳过 LLM 调用
func matchLocalTemplate(input string) (string, []models.TaskStep) {
	inputLower := strings.ToLower(input)

	// 按优先级匹配：模板列表已按精确度排序（精确匹配在前，泛匹配在后）
	// 找到第一个匹配的模板即返回
	for _, t := range localTemplates {
		for _, pattern := range t.patterns {
			if strings.Contains(inputLower, strings.ToLower(pattern)) {
				// 深拷贝 steps，提取参数
				steps := make([]models.TaskStep, len(t.steps))
				for i, s := range t.steps {
					steps[i] = s
					if steps[i].Params == nil {
						steps[i].Params = make(map[string]string)
					} else {
						// 拷贝 params map
						newParams := make(map[string]string, len(s.Params))
						for k, v := range s.Params {
							newParams[k] = v
						}
						steps[i].Params = newParams
					}
				}
				// 尝试从输入中提取参数（域名、容器名、应用名等）
				extractParams(input, steps)
				return t.intent, steps
			}
		}
	}
	return "", nil
}

// extractParams 从用户输入中提取关键参数
func extractParams(input string, steps []models.TaskStep) {
	for i := range steps {
		params := steps[i].Params
		// 提取域名
		if params["domain"] == "auto_detect" || params["domain"] == "" {
			if domain := extractDomain(input); domain != "" {
				params["domain"] = domain
			}
		}
		// 提取容器名
		if params["name"] == "auto_detect" || params["container_id"] == "auto_detect" {
			if name := extractContainerName(input); name != "" {
				if params["name"] == "auto_detect" {
					params["name"] = name
				}
				if params["container_id"] == "auto_detect" {
					params["container_id"] = name
				}
			}
		}
	}
}

// extractDomain 从输入中提取域名
func extractDomain(input string) string {
	// 匹配常见域名模式：example.com, sub.example.com
	domainPatterns := []string{}
	words := strings.Fields(input)
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'（）()")
		if strings.Contains(w, ".") && !strings.Contains(w, " ") && len(w) > 4 {
			// 排除常见非域名词汇
			exclude := map[string]bool{
				"nginx.exe": true, "docker.io": true, "node.js": true,
				"next.js": true, "letsencrypt.org": true,
			}
			if !exclude[strings.ToLower(w)] {
				domainPatterns = append(domainPatterns, w)
			}
		}
	}
	if len(domainPatterns) > 0 {
		return domainPatterns[0]
	}
	return ""
}

// extractContainerName 从输入中提取容器/应用名称
func extractContainerName(input string) string {
	// 常见容器/应用名关键词后跟随的名称
	prefixes := []string{"容器", "container", "应用", "app", "服务", "service", "nginx", "mysql", "redis", "wordpress", "node"}
	inputLower := strings.ToLower(input)
	for _, prefix := range prefixes {
		idx := strings.Index(inputLower, prefix)
		if idx >= 0 {
			rest := input[idx+len(prefix):]
			rest = strings.TrimLeft(rest, " :：的")
			// 取第一个词作为名称
			if name := firstWord(rest); name != "" {
				return name
			}
		}
	}
	return ""
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	words := strings.Fields(s)
	if len(words) > 0 {
		return strings.Trim(words[0], ".,;:!?\"'（）()")
	}
	return ""
}

func classifyIntent(input string) string {
	input = strings.ToLower(input)

	switch {
	case strings.Contains(input, "部署") || strings.Contains(input, "安装") || strings.Contains(input, "deploy"):
		return "应用部署"
	case strings.Contains(input, "重启") || strings.Contains(input, "restart"):
		return "服务重启"
	case strings.Contains(input, "监控") || strings.Contains(input, "cpu") || strings.Contains(input, "内存"):
		return "系统监控"
	case strings.Contains(input, "ssl") || strings.Contains(input, "证书") || strings.Contains(input, "https"):
		return "SSL证书管理"
	case strings.Contains(input, "日志") || strings.Contains(input, "log"):
		return "日志查看"
	default:
		return "未知操作"
	}
}

func generateSteps(intent string) []models.TaskStep {
	switch intent {
	case "应用部署":
		return []models.TaskStep{
			{ID: "step_1", Action: "app.install", Params: map[string]string{"name": "auto_detect"}, Status: "pending"},
			{ID: "step_2", Action: "nginx.reload", Params: map[string]string{}, Status: "pending"},
			{ID: "step_3", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "confirm_required"},
		}
	case "服务重启":
		return []models.TaskStep{
			{ID: "step_1", Action: "container.restart", Params: map[string]string{"container_id": "auto_detect"}, Status: "confirm_required"},
		}
	case "系统监控":
		return []models.TaskStep{
			{ID: "step_1", Action: "monitor.cpu", Params: map[string]string{}, Status: "pending"},
			{ID: "step_2", Action: "monitor.memory", Params: map[string]string{}, Status: "pending"},
			{ID: "step_3", Action: "monitor.disk", Params: map[string]string{}, Status: "pending"},
		}
	default:
		return []models.TaskStep{
			{ID: "step_1", Action: "system.info", Params: map[string]string{}, Status: "pending"},
		}
	}
}

func getAvailableActions() []ActionDef {
	return []ActionDef{
		{Name: "app.install", Description: "安装应用", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "app.list", Description: "列出已安装应用", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "app.uninstall", Description: "卸载应用（危险）", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "container.list", Description: "列出所有容器", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "container.start", Description: "启动容器", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "container.stop", Description: "停止容器（危险）", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "container.restart", Description: "重启容器（危险）", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "container.logs", Description: "查看容器日志", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}, "lines": map[string]interface{}{"type": "integer"}}, "required": []string{"name"}},
		},
		{Name: "container.images", Description: "列出 Docker 镜像", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "container.pull", Description: "拉取 Docker 镜像", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"image": map[string]interface{}{"type": "string"}}, "required": []string{"image"}},
		},
		{Name: "container.prune", Description: "清理 Docker 资源（危险）", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "monitor.cpu", Description: "查看 CPU 使用率", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "monitor.memory", Description: "查看内存使用情况", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "monitor.disk", Description: "查看磁盘使用情况", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "monitor.network", Description: "查看网络流量", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "ssl.apply", Description: "申请 SSL 证书", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"domain": map[string]interface{}{"type": "string"}}, "required": []string{"domain"}},
		},
		{Name: "ssl.status", Description: "查看 SSL 证书状态", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "ssl.renew", Description: "续期 SSL 证书", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"domain": map[string]interface{}{"type": "string"}}, "required": []string{"domain"}},
		},
		{Name: "nginx.reload", Description: "重载 Nginx 配置", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "nginx.status", Description: "查看 Nginx 状态", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "database.list", Description: "列出所有数据库", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "database.create", Description: "创建数据库", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "database.backup", Description: "备份数据库", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "database.delete", Description: "删除数据库（危险）", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}}, "required": []string{"name"}},
		},
		{Name: "database.restore", Description: "恢复数据库", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"db_name": map[string]interface{}{"type": "string"}}, "required": []string{"db_name"}},
		},
		{Name: "database.slow_query", Description: "分析慢查询", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "database.connections", Description: "查看数据库连接数", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "database.optimize", Description: "优化数据库表", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "website.list", Description: "列出所有网站", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "website.proxy", Description: "配置反向代理", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"domain": map[string]interface{}{"type": "string"}, "target": map[string]interface{}{"type": "string"}}, "required": []string{"domain", "target"}},
		},
		{Name: "website.domain", Description: "绑定域名", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"domain": map[string]interface{}{"type": "string"}}, "required": []string{"domain"}},
		},
		{Name: "file.list", Description: "查看文件列表", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}}}},
		{Name: "file.read", Description: "读取文件内容", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}}, "required": []string{"path"}},
		},
		{Name: "file.upload", Description: "上传文件", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}}, "required": []string{"path"}},
		},
		{Name: "file.delete", Description: "删除文件（危险）", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}}, "required": []string{"path"}},
		},
		{Name: "system.info", Description: "查看系统信息", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "system.restart", Description: "重启系统（危险）", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "system.time", Description: "查看系统时间", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "system.processes", Description: "查看进程列表", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "system.clear_cache", Description: "清理系统缓存", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "health", Description: "健康检查", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}},
		{Name: "compose.generate", Description: "生成 Docker Compose 配置", Parameters: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"template": map[string]interface{}{"type": "string"}}, "required": []string{"template"}},
		},
	}
}
