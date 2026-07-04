package core

import (
	"fmt"
	"strings"

	"github.com/ai-server-agent/internal/models"
)

// ServiceTemplate 预定义的服务编排模板
type ServiceTemplate struct {
	Name        string
	Description string
	Services    []ServiceDef
}

// ServiceDef 单个服务定义
type ServiceDef struct {
	Name    string
	Image   string
	Ports   []string
	Env     map[string]string
	Volumes []string
	DependsOn []string
}

// 预定义的 Compose 编排模板
var composeTemplates = map[string]ServiceTemplate{
	"wordpress": {
		Name:        "WordPress + MySQL",
		Description: "部署 WordPress 博客系统，包含 MySQL 数据库",
		Services: []ServiceDef{
			{Name: "wordpress", Image: "wordpress:latest", Ports: []string{"8080:80"},
				Env: map[string]string{
					"WORDPRESS_DB_HOST": "db:3306",
					"WORDPRESS_DB_USER": "wordpress",
					"WORDPRESS_DB_PASSWORD": "auto_generate",
					"WORDPRESS_DB_NAME": "wordpress",
				},
				DependsOn: []string{"db"},
			},
			{Name: "db", Image: "mysql:8.0", Ports: []string{"3306:3306"},
				Env: map[string]string{
					"MYSQL_ROOT_PASSWORD": "auto_generate",
					"MYSQL_DATABASE": "wordpress",
					"MYSQL_USER": "wordpress",
					"MYSQL_PASSWORD": "auto_generate",
				},
				Volumes: []string{"mysql_data:/var/lib/mysql"},
			},
		},
	},
	"nextjs": {
		Name:        "Next.js + PostgreSQL + Redis",
		Description: "部署 Next.js 全栈应用，包含 PostgreSQL 和 Redis",
		Services: []ServiceDef{
			{Name: "app", Image: "node:20-alpine", Ports: []string{"3000:3000"},
				Env: map[string]string{
					"DATABASE_URL": "postgresql://postgres:auto_generate@db:5432/app",
					"REDIS_URL": "redis://redis:6379",
				},
				DependsOn: []string{"db", "redis"},
			},
			{Name: "db", Image: "postgres:16-alpine", Ports: []string{"5432:5432"},
				Env: map[string]string{
					"POSTGRES_DB": "app",
					"POSTGRES_PASSWORD": "auto_generate",
				},
				Volumes: []string{"pg_data:/var/lib/postgresql/data"},
			},
			{Name: "redis", Image: "redis:7-alpine", Ports: []string{"6379:6379"},
				Volumes: []string{"redis_data:/data"},
			},
		},
	},
	"nginx-static": {
		Name:        "Nginx 静态站点",
		Description: "部署静态网站（Nginx 提供静态文件服务）",
		Services: []ServiceDef{
			{Name: "nginx", Image: "nginx:alpine", Ports: []string{"80:80", "443:443"},
				Volumes: []string{"./html:/usr/share/nginx/html", "./nginx.conf:/etc/nginx/nginx.conf"},
			},
		},
	},
	"node-app": {
		Name:        "Node.js 应用",
		Description: "部署 Node.js 应用",
		Services: []ServiceDef{
			{Name: "app", Image: "node:20-alpine", Ports: []string{"3000:3000"},
				Env: map[string]string{"NODE_ENV": "production"},
				Volumes: []string{"./app:/app"},
			},
		},
	},
	"lamp": {
		Name:        "LAMP 环境（Apache + MySQL + PHP）",
		Description: "部署 LAMP 开发环境",
		Services: []ServiceDef{
			{Name: "web", Image: "php:8.2-apache", Ports: []string{"80:80"},
				Volumes: []string{"./www:/var/www/html"},
				DependsOn: []string{"db"},
			},
			{Name: "db", Image: "mysql:8.0", Ports: []string{"3306:3306"},
				Env: map[string]string{
					"MYSQL_ROOT_PASSWORD": "auto_generate",
					"MYSQL_DATABASE": "app",
				},
				Volumes: []string{"mysql_data:/var/lib/mysql"},
			},
		},
	},
	"monitoring": {
		Name:        "监控套件（Prometheus + Grafana）",
		Description: "部署 Prometheus + Grafana 监控系统",
		Services: []ServiceDef{
			{Name: "prometheus", Image: "prom/prometheus:latest", Ports: []string{"9090:9090"},
				Volumes: []string{"./prometheus.yml:/etc/prometheus/prometheus.yml", "prometheus_data:/prometheus"},
			},
			{Name: "grafana", Image: "grafana/grafana:latest", Ports: []string{"3000:3000"},
				Env: map[string]string{"GF_SECURITY_ADMIN_PASSWORD": "auto_generate"},
				Volumes: []string{"grafana_data:/var/lib/grafana"},
				DependsOn: []string{"prometheus"},
			},
		},
	},
}

// ComposeOrchestrator Compose 编排器
type ComposeOrchestrator struct{}

// NewComposeOrchestrator 创建编排器
func NewComposeOrchestrator() *ComposeOrchestrator {
	return &ComposeOrchestrator{}
}

// MatchTemplate 根据自然语言匹配编排模板
func (o *ComposeOrchestrator) MatchTemplate(input string) (*ServiceTemplate, float64) {
	inputLower := strings.ToLower(input)

	keywords := map[string]string{
		"wordpress": "wordpress",
		"博客":      "wordpress",
		"blog":     "wordpress",
		"nextjs":   "nextjs",
		"next.js":  "nextjs",
		"next":     "nextjs",
		"全栈":      "nextjs",
		"静态网站":   "nginx-static",
		"静态站点":   "nginx-static",
		"node":     "node-app",
		"nodejs":   "node-app",
		"node.js":  "node-app",
		"lamp":     "lamp",
		"apache":   "lamp",
		"监控":      "monitoring",
		"prometheus": "monitoring",
		"grafana":  "monitoring",
	}

	matches := make(map[string]int)
	for kw, tmpl := range keywords {
		if strings.Contains(inputLower, kw) {
			matches[tmpl]++
		}
	}

	if len(matches) == 0 {
		return nil, 0
	}

	// 找到匹配最多的模板
	var bestTmpl string
	maxCount := 0
	for tmpl, count := range matches {
		if count > maxCount {
			bestTmpl = tmpl
			maxCount = count
		}
	}

	tmpl, ok := composeTemplates[bestTmpl]
	if !ok {
		return nil, 0
	}

	confidence := float64(maxCount) / 3.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return &tmpl, confidence
}

// GenerateCompose 根据模板生成 docker-compose.yml 和部署步骤
func (o *ComposeOrchestrator) GenerateCompose(tmpl *ServiceTemplate) (string, []models.TaskStep) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n", tmpl.Description))
	sb.WriteString(fmt.Sprintf("# 由 AI Server Agent 自动生成\n\n"))
	sb.WriteString("version: '3.8'\n\n")
	sb.WriteString("services:\n")

	for _, svc := range tmpl.Services {
		sb.WriteString(fmt.Sprintf("  %s:\n", svc.Name))
		sb.WriteString(fmt.Sprintf("    image: %s\n", svc.Image))
		if len(svc.Ports) > 0 {
			sb.WriteString("    ports:\n")
			for _, p := range svc.Ports {
				sb.WriteString(fmt.Sprintf("      - \"%s\"\n", p))
			}
		}
		if len(svc.Env) > 0 {
			sb.WriteString("    environment:\n")
			for k, v := range svc.Env {
				sb.WriteString(fmt.Sprintf("      %s: \"%s\"\n", k, v))
			}
		}
		if len(svc.Volumes) > 0 {
			sb.WriteString("    volumes:\n")
			for _, v := range svc.Volumes {
				sb.WriteString(fmt.Sprintf("      - %s\n", v))
			}
		}
		if len(svc.DependsOn) > 0 {
			sb.WriteString("    depends_on:\n")
			for _, d := range svc.DependsOn {
				sb.WriteString(fmt.Sprintf("      - %s\n", d))
			}
		}
		sb.WriteString("\n")
	}

	// 添加 volumes 声明
	hasVolumes := false
	for _, svc := range tmpl.Services {
		for _, v := range svc.Volumes {
			if strings.Contains(v, ":") {
				hasVolumes = true
				break
			}
		}
	}
	if hasVolumes {
		sb.WriteString("volumes:\n")
		for _, svc := range tmpl.Services {
			for _, v := range svc.Volumes {
				if strings.Contains(v, ":") {
					name := strings.Split(v, ":")[0]
					if !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "/") {
						sb.WriteString(fmt.Sprintf("  %s:\n", name))
					}
				}
			}
		}
	}

	// 生成部署步骤
	steps := []models.TaskStep{
		{ID: "s1", Action: "file.upload", Params: map[string]string{"path": "/tmp/docker-compose.yml"}, Status: "preview"},
		{ID: "s2", Action: "container.start", Params: map[string]string{"name": tmpl.Services[0].Name}, Status: "preview"},
		{ID: "s3", Action: "nginx.reload", Status: "pending"},
		{ID: "s4", Action: "ssl.apply", Params: map[string]string{"domain": "auto_detect"}, Status: "preview"},
	}

	return sb.String(), steps
}

// ListTemplates 列出所有可用模板
func (o *ComposeOrchestrator) ListTemplates() []ServiceTemplate {
	templates := make([]ServiceTemplate, 0, len(composeTemplates))
	for _, tmpl := range composeTemplates {
		templates = append(templates, tmpl)
	}
	return templates
}
