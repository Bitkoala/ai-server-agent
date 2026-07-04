package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ai-server-agent/internal/core"
	"github.com/ai-server-agent/internal/models"
	"github.com/ai-server-agent/internal/storage"
)

// REPL 命令行交互界面
type REPL struct {
	engine *core.Engine
	store  *storage.SQLiteStore
	reader *bufio.Reader
}

// NewREPL 创建 REPL
func NewREPL(engine *core.Engine, store *storage.SQLiteStore) *REPL {
	return &REPL{
		engine: engine,
		store:  store,
		reader: bufio.NewReader(os.Stdin),
	}
}

// Run 启动 REPL 循环
func (r *REPL) Run() {
	printBanner()

	ctx := context.Background()

	for {
		fmt.Print("\n\033[1;36m💡 你想做什么？\033[0m ")
		input, err := r.reader.ReadString('\n')
		if err != nil {
			fmt.Println("\n再见！👋")
			return
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch strings.ToLower(input) {
		case "exit", "quit", "q", "退出":
			fmt.Println("再见！👋")
			return
		case "help", "帮助", "?":
			printHelp()
			continue
		case "history", "历史":
			r.showHistory(ctx)
			continue
		case "status", "状态":
			r.showStatus()
			continue
		}

		r.processInput(ctx, input)
	}
}

func (r *REPL) processInput(ctx context.Context, input string) {
	fmt.Print("\n⏳ 正在理解你的意图...")
	task, err := r.engine.Execute(ctx, input)
	if err != nil {
		fmt.Printf("\r❌ %v\n", err)
		return
	}

	fmt.Printf("\r✅ 理解完成：\033[1;33m%s\033[0m\n", task.Intent)

	r.printTaskPlan(task)

	switch task.Status {
	case "auto_confirmed":
		fmt.Println("\n🟢 所有步骤均为低风险操作，自动执行...")
		r.executeTask(ctx, task.ID)

	case "awaiting_confirmation", "pending":
		if r.askConfirmation(task) {
			r.executeTask(ctx, task.ID)
		} else {
			fmt.Println("⏸️ 任务已取消")
			task.Status = "cancelled"
			r.store.UpdateTask(task)
		}

	default:
		fmt.Printf("任务状态: %s\n", task.Status)
	}
}

func (r *REPL) printTaskPlan(task *models.Task) {
	fmt.Println("\n━━━━━━━━━━ 📋 任务计划 ━━━━━━━━━━")
	fmt.Printf("意图: %s\n", task.Intent)
	fmt.Println("步骤:")

	for _, step := range task.Steps {
		icon := getStepIcon(step.Status)
		risk := getRiskLabel(step.Action)
		fmt.Printf("  %s %s \033[90m[%s]\033[0m %s\n", icon, step.Action, risk, step.Status)
		if len(step.Params) > 0 {
			fmt.Printf("    └─ 参数: %v\n", step.Params)
		}
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func (r *REPL) askConfirmation(task *models.Task) bool {
	var confirmSteps []string
	for _, step := range task.Steps {
		if step.Status == "confirm_required" {
			confirmSteps = append(confirmSteps, step.Action)
		}
	}

	if len(confirmSteps) > 0 {
		fmt.Printf("\n⚠️  以下步骤需要确认: %s\n", strings.Join(confirmSteps, ", "))
	}

	fmt.Print("\n\033[1;33m是否执行？[y/N]\033[0m ")
	answer, _ := r.reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes" || answer == "是"
}

func (r *REPL) executeTask(ctx context.Context, taskID string) {
	fmt.Println("\n🚀 开始执行...")
	startTime := time.Now()

	task, err := r.engine.ConfirmAndRun(ctx, taskID)
	elapsed := time.Since(startTime)

	if err != nil {
		fmt.Printf("\n❌ 执行失败 (耗时 %v): %v\n", elapsed.Round(time.Millisecond), err)
		r.printExecutionResult(task)
		return
	}

	fmt.Printf("\n✅ 全部完成！(耗时 %v)\n", elapsed.Round(time.Millisecond))
	r.printExecutionResult(task)
}

func (r *REPL) printExecutionResult(task *models.Task) {
	fmt.Println("\n━━━━━━━━━━ 📊 执行结果 ━━━━━━━━━━")
	for _, step := range task.Steps {
		icon := getStepIcon(step.Status)
		fmt.Printf("  %s %s", icon, step.Action)
		if step.Result != "" {
			fmt.Printf(" → %s", truncate(step.Result, 80))
		}
		if step.Error != "" {
			fmt.Printf(" \033[31m错误: %s\033[0m", step.Error)
		}
		fmt.Println()
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func (r *REPL) showHistory(ctx context.Context) {
	tasks, err := r.store.ListTasks(10)
	if err != nil {
		fmt.Printf("获取历史失败: %v\n", err)
		return
	}
	if len(tasks) == 0 {
		fmt.Println("暂无历史任务")
		return
	}

	fmt.Println("\n━━━━━━━━━━ 📜 最近任务 ━━━━━━━━━━")
	for _, t := range tasks {
		statusIcon := "⏳"
		switch t.Status {
		case "done":
			statusIcon = "✅"
		case "failed":
			statusIcon = "❌"
		case "cancelled":
			statusIcon = "⏸️"
		}
		fmt.Printf("  %s [%s] %s → %s\n", statusIcon, t.CreatedAt.Format("01-02 15:04"), truncate(t.UserInput, 50), t.Intent)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func (r *REPL) showStatus() {
	fmt.Println("\n━━━━━━━━━━ 🖥️ 系统状态 ━━━━━━━━━━")
	fmt.Printf("  1Panel 连接: 检测中...\n")
	fmt.Printf("  LLM 模型: 已配置\n")
	fmt.Printf("  存储: SQLite\n")
	fmt.Printf("  安全策略: 已启用\n")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func getStepIcon(status string) string {
	switch status {
	case "done":
		return "✅"
	case "failed":
		return "❌"
	case "running":
		return "🔄"
	case "confirm_required":
		return "⚠️"
	case "preview":
		return "👁️"
	default:
		return "⏳"
	}
}

func getRiskLabel(action string) string {
	switch action {
	case "container.stop", "container.restart", "app.uninstall", "database.delete", "website.delete", "system.restart", "file.delete":
		return "🔴高"
	case "app.install", "container.start", "nginx.reload", "ssl.apply", "database.create", "website.create":
		return "🟡中"
	default:
		return "🟢低"
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func printBanner() {
	fmt.Println(`
╔══════════════════════════════════════════╗
║        🤖 AI Server Agent v0.1           ║
║   基于 1Panel 的智能服务器管理助理          ║
║                                          ║
║   输入自然语言指令，我来帮你管理服务器       ║
║   输入 'help' 查看帮助，'exit' 退出        ║
╚══════════════════════════════════════════╝`)
}

func printHelp() {
	fmt.Println(`
━━━━━━━━━━ 📖 帮助 ━━━━━━━━━━

💬 直接输入自然语言指令，例如：
   • "帮我部署一个 WordPress"
   • "查看服务器 CPU 和内存使用情况"
   • "重启 nginx 容器"
   • "为 example.com 申请 SSL 证书"
   • "查看最近的应用日志"

🟢 低风险操作 (自动执行):
   查看状态、监控、日志、列表查询

🟡 中风险操作 (显示预览):
   安装应用、启动服务、重载配置

🔴 高风险操作 (需要确认):
   停止/重启服务、删除、卸载

⌨️ 命令:
   help/帮助  - 显示此帮助
   history/历史 - 查看最近任务
   status/状态  - 查看系统状态
   exit/退出   - 退出程序

━━━━━━━━━━━━━━━━━━━━━━━━━`)
}
