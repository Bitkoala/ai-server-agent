package core

import (
	"testing"
)

func TestLocalTemplateMatch_Exact(t *testing.T) {
	tests := []struct {
		input       string
		expectMatch bool
		expectIntent string
	}{
		{"查看CPU使用率", true, "查看 CPU 使用率"},
		{"查看内存", true, "查看内存使用情况"},
		{"磁盘空间不足", true, "查看磁盘使用情况"},
		{"查看系统监控", true, "系统综合监控"},
		{"部署WordPress", true, "部署 WordPress"},
		{"重启nginx", true, "重载 Nginx 配置"},
		{"查看容器日志", true, "查看容器日志"},
		{"申请SSL证书", true, "SSL 证书管理"},
		{"备份数据库", true, "备份数据库"},
		{"部署网站", true, "安装应用"},
		{"反向代理", true, "配置反向代理"},
		{"绑定域名", true, "绑定域名"},
		{"查看慢查询", true, "分析慢查询"},
		{"优化数据库", true, "优化数据库表"},
		// 不应匹配的
		{"帮我写一个Python脚本", false, ""},
	}

	for _, tt := range tests {
		intent, steps := matchLocalTemplate(tt.input)
		matched := intent != ""
		if matched != tt.expectMatch {
			t.Errorf("输入 '%s': 期望匹配=%v, 实际匹配=%v", tt.input, tt.expectMatch, matched)
		}
		if tt.expectMatch && intent != tt.expectIntent {
			t.Errorf("输入 '%s': 期望意图='%s', 实际='%s'", tt.input, tt.expectIntent, intent)
		}
		if tt.expectMatch && len(steps) == 0 {
			t.Errorf("输入 '%s': 匹配成功但步骤为空", tt.input)
		}
	}
}

func TestLocalTemplateMatch_AllTemplates(t *testing.T) {
	// 验证所有模板都有意图和至少一个步骤
	for _, tmpl := range localTemplates {
		if tmpl.Intent == "" {
			t.Errorf("模板意图为空: patterns=%v", tmpl.Patterns)
		}
		if len(tmpl.Steps) == 0 {
			t.Errorf("模板步骤为空: intent=%s", tmpl.Intent)
		}
		if len(tmpl.Patterns) == 0 {
			t.Errorf("模板匹配模式为空: intent=%s", tmpl.Intent)
		}
		for _, step := range tmpl.Steps {
			if step.Action == "" {
				t.Errorf("模板步骤 action 为空: intent=%s", tmpl.Intent)
			}
		}
	}
}

func TestLocalTemplateMatch_CaseInsensitive(t *testing.T) {
	// 验证大小写不敏感
	_, steps := matchLocalTemplate("DEPLOY WORDPRESS")
	if len(steps) == 0 {
		t.Error("大写输入应能匹配")
	}

	_, steps = matchLocalTemplate("查看 CPU 和内存")
	if len(steps) == 0 {
		t.Error("混合输入应能匹配")
	}
}

func TestLocalTemplateMatch_StepDeepCopy(t *testing.T) {
	// 验证返回的 steps 是深拷贝
	_, steps1 := matchLocalTemplate("查看CPU")
	_, steps2 := matchLocalTemplate("查看CPU")

	if len(steps1) > 0 && len(steps2) > 0 {
		steps1[0].Status = "modified"
		if steps2[0].Status == "modified" {
			t.Error("模板匹配应返回深拷贝，修改不应影响后续匹配")
		}
	}
}
