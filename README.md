# AI Server Agent

> 基于 1Panel 的智能服务器管理助理 —— 用自然语言管理你的 Linux 服务器。

## 架构

```
用户 (自然语言) → Agent (意图解析 + 任务编排) → 1Panel API (执行)
```

## 项目结构

```
cmd/agent/          # 主入口
internal/
  core/             # 核心引擎：意图解析、任务编排
  drivers/          # 能力驱动：1Panel、Docker、System
  storage/          # SQLite 持久化
  security/         # 安全：权限校验、操作确认
  llm/              # LLM 交互层
  web/              # Web API + Chat 界面
configs/            # 配置文件
scripts/            # 部署脚本
```

## 快速开始

```bash
go build -o bin/agent ./cmd/agent
./bin/agent
```
