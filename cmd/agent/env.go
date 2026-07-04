package main

import (
	"os"
	"strings"
)

// envOverrides 定义环境变量覆盖映射
// 格式: 环境变量名 → "配置路径" (用 . 分隔层级)
// 配置路径: onepanel.api_key, llm.api_key, llm.provider 等
var envOverrides = map[string]string{
	// LLM
	"AI_SERVER_LLM_PROVIDER": "llm.provider",
	"AI_SERVER_LLM_KEY":      "llm.api_key",
	"AI_SERVER_LLM_URL":      "llm.base_url",
	"AI_SERVER_LLM_MODEL":    "llm.model",

	// 1Panel
	"AI_SERVER_ONEPANEL_URL": "onepanel.base_url",
	"AI_SERVER_ONEPANEL_KEY": "onepanel.api_key",

	// 通知渠道
	"AI_SERVER_NOTIFY_FEISHU":    "notify.feishu_webhook",
	"AI_SERVER_NOTIFY_DINGTALK":  "notify.dingtalk_webhook",
	"AI_SERVER_NOTIFY_WECOM":     "notify.wecom_webhook",
	"AI_SERVER_NOTIFY_TG_TOKEN":  "notify.telegram_bot_token",
	"AI_SERVER_NOTIFY_TG_CHAT":   "notify.telegram_chat_id",

	// JWT
	"AI_SERVER_JWT_SECRET": "auth.jwt_secret",
}

// applyEnvOverrides 用环境变量覆盖配置中的敏感字段
// 环境变量优先级高于配置文件
func (c *Config) applyEnvOverrides() {
	for envKey, configPath := range envOverrides {
		val := os.Getenv(envKey)
		if val == "" {
			continue
		}

		parts := strings.Split(configPath, ".")
		switch parts[0] {
		case "llm":
			switch parts[1] {
			case "provider":
				c.LLM.Provider = val
			case "api_key":
				c.LLM.APIKey = val
			case "base_url":
				c.LLM.BaseURL = val
			case "model":
				c.LLM.Model = val
			}
		case "onepanel":
			switch parts[1] {
			case "base_url":
				c.OnePanel.BaseURL = val
			case "api_key":
				c.OnePanel.APIKey = val
			}
		case "notify":
			switch parts[1] {
			case "feishu_webhook":
				c.Notify.FeishuWebhook = val
			case "dingtalk_webhook":
				c.Notify.DingTalkWebhook = val
			case "wecom_webhook":
				c.Notify.WecomWebhook = val
			case "telegram_bot_token":
				c.Notify.TelegramBotToken = val
			case "telegram_chat_id":
				c.Notify.TelegramChatID = val
			}
		case "auth":
			switch parts[1] {
			case "jwt_secret":
				c.Auth.JWTSecret = val
			}
		}
	}
}

// ensureConfigPerms 确保配置文件权限为 0600
func ensureConfigPerms(path string) error {
	return os.Chmod(path, 0600)
}
