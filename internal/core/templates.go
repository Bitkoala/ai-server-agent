package core

import (
	"strings"

	"github.com/ai-server-agent/internal/models"
)

// CommandTemplate 本地命令模板
type CommandTemplate struct {
	Patterns []string // 匹配关键词（任一命中即匹配）
	Intent   string   // 意图描述
	Steps    []models.TaskStep
}

// 本地命令模板表 —— 覆盖 80% 高频运维操作
var localTemplates = []CommandTemplate{
	// ============ 监控查询类 (低风险) ============
	{
		Patterns: []string{"cpu", "CPU", "处理器", "cpu使用"},
		Intent:   "查看 CPU 使用率",
		Steps:    []models.TaskStep{{ID: "s1", Action: "monitor.cpu", Status: "pending"}},
	},
	{
		Patterns: []string{"内存", "memory", "RAM", "运存"},
		Intent:   "查看内存使用情况",
		Steps:    []models.TaskStep{{ID: "s1", Action: "monitor.memory", Status: "pending"}},
	},
	{
		Patterns: []string{"磁盘", "硬盘", "存储", "空间", "disk", "容量"},
		Intent:   "查看磁盘使用情况",
		Steps:    []models.TaskStep{{ID: "s1", Action: "monitor.disk", Status: "pending"}},
	},
	{
		Patterns: []string{"网络", "流量", "network", "带宽"},
		Intent:   "查看网络流量",
		Steps:    []models.TaskStep{{ID: "s1", Action: "monitor.network", Status: "pending"}},
	},
	{
		Patterns: []string{"系统信息", "系统状态", "服务器信息", "系统概览"},
		Intent:   "查看系统信息",
		Steps:    []models.TaskStep{{ID: "s1", Action: "system.info", Status: "pending"}},
	},
	{
		Patterns: []string{"健康检查", "health"},
		Intent:   "健康检查",
		Steps:    []models.TaskStep{{ID: "s1", Action: "health", Status: "pending"}},
	},

	// ============ 监控组合类 ============
	{
		Patterns: []string{"查看监控", "系统监控", "运行状态", "服务器状态", "资源使用", "资源占用"},
		Intent:   "系统综合监控",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "monitor.cpu", Status: "pending"},
			{ID: "s2", Action: "monitor.memory", Status: "pending"},
			{ID: "s3", Action: "monitor.disk", Status: "pending"},
		},
	},

	// ============ 容器操作 ============
	{
		Patterns: []string{"容器日志", "查看日志", "logs", "log"},
		Intent:   "查看容器日志",
		Steps:    []models.TaskStep{{ID: "s1", Action: "container.logs", Params: map[string]string{"lines": "100"}, Status: "pending"}},
	},
	{
		Patterns: []string{"容器列表", "列出容器", "查看容器", "docker ps", "运行中的容器"},
		Intent:   "列出所有容器",
		Steps:    []models.TaskStep{{ID: "s1", Action: "container.list", Status: "pending"}},
	},
	{
		Patterns: []string{"启动容器", "start"},
		Intent:   "启动容器",
		Steps:    []models.TaskStep{{ID: "s1", Action: "container.start", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}},
	},
	{
		Patterns: []string{"停止容器", "stop"},
		Intent:   "停止容器",
		Steps:    []models.TaskStep{{ID: "s1", Action: "container.stop", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}},
	},
	{
		Patterns: []string{"重启容器", "restart", "重新启动"},
		Intent:   "重启容器",
		Steps:    []models.TaskStep{{ID: "s1", Action: "container.restart", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}},
	},

	// ============ 应用部署 ============
	{
		Patterns: []string{"部署wordpress", "安装wordpress", "部署 WordPress", "安装 WordPress"},
		Intent:   "部署 WordPress",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "app.install", Params: map[string]string{"name": "wordpress"}, Status: "preview"},
			{ID: "s2", Action: "nginx.reload", Status: "pending"},
		},
	},
	{
		Patterns: []string{"部署nginx", "安装nginx", "部署 Nginx", "安装 Nginx"},
		Intent:   "安装 Nginx",
		Steps:    []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "nginx"}, Status: "preview"}},
	},
	{
		Patterns: []string{"部署mysql", "安装mysql", "部署 MySQL", "安装 MySQL", "数据库安装"},
		Intent:   "安装 MySQL",
		Steps:    []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "mysql"}, Status: "preview"}},
	},
	{
		Patterns: []string{"部署node", "安装node", "部署 Node", "nodejs"},
		Intent:   "安装 Node.js",
		Steps:    []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "nodejs"}, Status: "preview"}},
	},
	{
		Patterns: []string{"部署", "安装", "deploy", "install"},
		Intent:   "安装应用",
		Steps:    []models.TaskStep{{ID: "s1", Action: "app.install", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}},
	},
	{
		Patterns: []string{"应用列表", "已安装", "已部署"},
		Intent:   "列出已安装应用",
		Steps:    []models.TaskStep{{ID: "s1", Action: "app.list", Status: "pending"}},
	},
	{
		Patterns: []string{"卸载", "删除应用", "uninstall"},
		Intent:   "卸载应用",
		Steps:    []models.TaskStep{{ID: "s1", Action: "app.uninstall", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}},
	},

	// ============ SSL ============
	{
		Patterns: []string{"ssl", "SSL", "证书", "https", "HTTPS", "申请证书", "配置证书", "lets encrypt"},
		Intent:   "SSL 证书管理",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "ssl.status", Status: "pending"},
			{ID: "s2", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
		},
	},
	{
		Patterns: []string{"续期证书", "证书过期", "renew"},
		Intent:   "续期 SSL 证书",
		Steps:    []models.TaskStep{{ID: "s1", Action: "ssl.renew", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"}},
	},

	// ============ Nginx ============
	{
		Patterns: []string{"重载nginx", "nginx重载", "reload nginx", "nginx reload", "重载配置", "重启nginx", "nginx重启"},
		Intent:   "重载 Nginx 配置",
		Steps:    []models.TaskStep{{ID: "s1", Action: "nginx.reload", Status: "preview"}},
	},
	{
		Patterns: []string{"nginx状态", "nginx status", "nginx运行"},
		Intent:   "查看 Nginx 状态",
		Steps:    []models.TaskStep{{ID: "s1", Action: "nginx.status", Status: "pending"}},
	},

	// ============ 数据库 ============
	{
		Patterns: []string{"数据库列表", "查看数据库", "列出数据库"},
		Intent:   "列出所有数据库",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.list", Status: "pending"}},
	},
	{
		Patterns: []string{"创建数据库", "新建数据库"},
		Intent:   "创建数据库",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.create", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}},
	},
	{
		Patterns: []string{"备份数据库", "数据库备份", "backup database"},
		Intent:   "备份数据库",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.backup", Params: map[string]string{"name": "auto_detect"}, Status: "preview"}},
	},
	{
		Patterns: []string{"删除数据库", "drop database"},
		Intent:   "删除数据库",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.delete", Params: map[string]string{"name": "auto_detect"}, Status: "confirm_required"}},
	},
	{
		Patterns: []string{"恢复数据库", "数据库恢复", "restore database"},
		Intent:   "恢复数据库",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.restore", Params: map[string]string{"db_name": "auto_detect"}, Status: "confirm_required"}},
	},
	{
		Patterns: []string{"慢查询", "慢SQL", "slow query", "慢日志"},
		Intent:   "分析慢查询",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.slow_query", Status: "pending"}},
	},
	{
		Patterns: []string{"数据库连接", "连接数", "数据库连接数"},
		Intent:   "查看数据库连接数",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.connections", Status: "pending"}},
	},
	{
		Patterns: []string{"优化数据库", "优化表", "optimize table"},
		Intent:   "优化数据库表",
		Steps:    []models.TaskStep{{ID: "s1", Action: "database.optimize", Status: "preview"}},
	},

	// ============ Docker Compose 编排 ============
	{
		Patterns: []string{"部署wordpress", "搭建博客", "wordpress", "部署博客"},
		Intent:   "部署 WordPress + MySQL（Docker Compose 编排）",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "compose.generate", Params: map[string]string{"template": "wordpress"}, Status: "preview"},
			{ID: "s2", Action: "container.start", Params: map[string]string{"name": "wordpress"}, Status: "preview"},
			{ID: "s3", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
		},
	},
	{
		Patterns: []string{"部署nextjs", "next.js", "nextjs应用", "全栈应用"},
		Intent:   "部署 Next.js + PostgreSQL + Redis",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "compose.generate", Params: map[string]string{"template": "nextjs"}, Status: "preview"},
			{ID: "s2", Action: "container.start", Params: map[string]string{"name": "app"}, Status: "preview"},
		},
	},
	{
		Patterns: []string{"部署监控", "监控系统", "prometheus", "grafana"},
		Intent:   "部署 Prometheus + Grafana 监控系统",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "compose.generate", Params: map[string]string{"template": "monitoring"}, Status: "preview"},
			{ID: "s2", Action: "container.start", Params: map[string]string{"name": "prometheus"}, Status: "preview"},
		},
	},
	{
		Patterns: []string{"部署网站", "搭建网站", "建站", "上线网站"},
		Intent:   "完整建站流程",
		Steps: []models.TaskStep{
			{ID: "s1", Action: "app.install", Params: map[string]string{"name": "auto_detect"}, Status: "preview"},
			{ID: "s2", Action: "nginx.reload", Status: "pending"},
			{ID: "s3", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
		},
	},

	// ============ 网站管理 ============
	{
		Patterns: []string{"网站列表", "列出网站", "查看网站"},
		Intent:   "列出所有网站",
		Steps:    []models.TaskStep{{ID: "s1", Action: "website.list", Status: "pending"}},
	},
	{
		Patterns: []string{"反向代理", "配置代理", "proxy", "反代"},
		Intent:   "配置反向代理",
		Steps:    []models.TaskStep{{ID: "s1", Action: "website.proxy", Params: map[string]string{"domain": "auto_detect", "target": "auto_detect"}, Status: "preview"}},
	},
	{
		Patterns: []string{"绑定域名", "添加域名", "域名绑定"},
		Intent:   "绑定域名",
		Steps:    []models.TaskStep{{ID: "s1", Action: "website.domain", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"}},
	},
	{
		Patterns: []string{"文件列表", "查看文件", "文件管理", "ls"},
		Intent:   "查看文件列表",
		Steps:    []models.TaskStep{{ID: "s1", Action: "file.list", Status: "pending"}},
	},
	{
		Patterns: []string{"读取文件", "查看内容", "cat", "文件内容"},
		Intent:   "读取文件内容",
		Steps:    []models.TaskStep{{ID: "s1", Action: "file.read", Params: map[string]string{"path": "auto_detect"}, Status: "pending"}},
	},
	{
		Patterns: []string{"上传文件", "upload"},
		Intent:   "上传文件",
		Steps:    []models.TaskStep{{ID: "s1", Action: "file.upload", Params: map[string]string{"path": "auto_detect"}, Status: "preview"}},
	},
}

// matchLocalTemplate 本地模板匹配 —— 覆盖 80% 高频操作，无需调 LLM
func matchLocalTemplate(input string) (string, []models.TaskStep) {
	inputLower := strings.ToLower(input)

	for _, t := range localTemplates {
		for _, pattern := range t.Patterns {
			if strings.Contains(inputLower, strings.ToLower(pattern)) || strings.Contains(input, pattern) {
				// 深拷贝 steps
				steps := make([]models.TaskStep, len(t.Steps))
				for i, s := range t.Steps {
					steps[i] = s
					if steps[i].Params == nil {
						steps[i].Params = make(map[string]string)
					}
				}
				return t.Intent, steps
			}
		}
	}
	return "", nil
}
