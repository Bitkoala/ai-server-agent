#!/bin/bash
# ============================================================
# AI Server Agent - 一键部署脚本
#
# === 用法（使用者从你的服务器拉取）===
#   curl -fsSL https://your-server.com/install.sh | bash
#   或
#   wget -qO- https://your-server.com/install.sh | bash
#
# === 高级用法 ===
#   指定端口:
#     curl -fsSL https://your-server.com/install.sh | AGENT_PORT=9090 bash
#
#   指定自定义下载地址（如果二进制放在别处）:
#     curl -fsSL https://your-server.com/install.sh | AGENT_DOWNLOAD_URL=https://cdn.example.com bash
#
#   仅下载不安装服务:
#     curl -fsSL https://your-server.com/install.sh | AGENT_NO_SERVICE=1 bash
#
# === 部署准备（在你自己的服务器上）===
#   你需要将以下文件放到服务器上供使用者下载:
#     1. install.sh          - 本脚本
#     2. ai-server-agent     - 编译好的二进制文件
#     3. config.yaml         - 默认配置文件（可选）
#
#   使用者执行 curl ... | bash 时，脚本会从同一目录下载以上文件。
# ============================================================
set -e

# ============ 配置 ============
AGENT_VERSION="${AGENT_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/opt/ai-server-agent}"
AGENT_PORT="${AGENT_PORT:-9090}"
BIN_DIR="$INSTALL_DIR/bin"
CONFIG_DIR="$INSTALL_DIR/configs"
DATA_DIR="$INSTALL_DIR/data"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step()  { echo -e "${BLUE}==>${NC} $1"; }

# ============ 检测系统 ============
detect_os() {
    case "$(uname -s)" in
        Linux)  OS="linux" ;;
        *)      log_error "不支持的操作系统: $(uname -s)"; exit 1 ;;
    esac

    case "$(uname -m)" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        armv7l)  ARCH="armv7" ;;
        *)       log_error "不支持的架构: $(uname -m)"; exit 1 ;;
    esac

    log_info "检测到系统: $OS/$ARCH"
}

# ============ 检查依赖 ============
check_deps() {
    local missing=""

    command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || missing="$missing curl|wget"
    command -v systemctl >/dev/null 2>&1 || log_warn "未检测到 systemd，将跳过服务安装"

    if [ -n "$missing" ]; then
        log_error "缺少必要依赖: $missing"
        log_info "请先安装: apt install curl 或 yum install curl"
        exit 1
    fi
}

# ============ 下载函数 ============
download() {
    local url="$1"
    local output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --connect-timeout 30 --retry 3 "$url" -o "$output"
    elif command -v wget >/dev/null 2>&1; then
        wget -q --timeout=30 --tries=3 "$url" -O "$output"
    else
        log_error "需要 curl 或 wget"
        exit 1
    fi
}

# ============ 主流程 ============
main() {
    echo ""
    echo "╔══════════════════════════════════════════╗"
    echo "║     🤖 AI Server Agent 一键安装         ║"
    echo "║     v1.0 - 自然语言管理服务器            ║"
    echo "╚══════════════════════════════════════════╝"
    echo ""

    detect_os
    check_deps

    # 确定下载 URL
    if [ -n "$AGENT_DOWNLOAD_URL" ]; then
        DOWNLOAD_BASE="${AGENT_DOWNLOAD_URL%/}"
        log_info "使用自定义下载地址: $DOWNLOAD_BASE"
    else
        # 默认 GitHub Releases
        GITHUB_REPO="${GITHUB_REPO:-your-org/ai-server-agent}"
        if [ "$AGENT_VERSION" = "latest" ]; then
            DOWNLOAD_BASE="https://github.com/${GITHUB_REPO}/releases/latest/download"
        else
            DOWNLOAD_BASE="https://github.com/${GITHUB_REPO}/releases/download/${AGENT_VERSION}"
        fi
        log_info "下载地址: $DOWNLOAD_BASE"
    fi

    # 创建目录
    log_step "创建安装目录..."
    mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR"

    # 下载二进制
    BIN_URL="${DOWNLOAD_BASE}/ai-server-agent-${OS}-${ARCH}"
    log_step "下载二进制文件..."
    log_info "从 $BIN_URL 下载..."

    if ! download "$BIN_URL" "$BIN_DIR/agent"; then
        # 尝试不带架构后缀的 URL
        BIN_URL="${DOWNLOAD_BASE}/ai-server-agent"
        log_info "重试: $BIN_URL"
        download "$BIN_URL" "$BIN_DIR/agent"
    fi

    chmod +x "$BIN_DIR/agent"
    log_info "二进制文件已安装: $BIN_DIR/agent"

    # 下载默认配置
    CONFIG_URL="${DOWNLOAD_BASE}/config.yaml"
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        log_step "下载默认配置..."
        if download "$CONFIG_URL" "$CONFIG_DIR/config.yaml" 2>/dev/null; then
            log_info "配置文件已下载"
        else
            log_warn "无法下载远程配置，创建默认配置..."
            cat > "$CONFIG_DIR/config.yaml" << 'YAMLEOF'
server:
  port: PORT_PLACEHOLDER
  host: "0.0.0.0"

onepanel:
  base_url: "http://localhost:9999"
  api_key: "your-1panel-api-key"
  timeout: 30

llm:
  provider: "openai"
  api_key: "your-api-key"
  base_url: "https://api.openai.com/v1"
  model: "gpt-4o-mini"

security:
  dangerous_ops_require_confirm: true
  rate_limit_per_minute: 30
  allowed_users: ["*"]

storage:
  db_path: "data/agent.db"

notify:
  feishu_webhook: ""
  dingtalk_webhook: ""
  wecom_webhook: ""
  telegram_bot_token: ""
  telegram_chat_id: ""
  min_level: "warning"

auth:
  enabled: false
  jwt_secret: ""
  token_expiry_hours: 24
  users:
    - username: "admin"
      password: "admin123"
      role: "admin"
YAMLEOF
            # 替换端口
            sed -i "s/PORT_PLACEHOLDER/${AGENT_PORT}/" "$CONFIG_DIR/config.yaml"
        fi
    fi

    # 更新配置文件中的端口
    if grep -q "PORT_PLACEHOLDER" "$CONFIG_DIR/config.yaml" 2>/dev/null; then
        sed -i "s/PORT_PLACEHOLDER/${AGENT_PORT}/" "$CONFIG_DIR/config.yaml"
    fi

    # 安装 systemd 服务
    if [ "${AGENT_NO_SERVICE}" != "1" ] && command -v systemctl >/dev/null 2>&1; then
        log_step "安装 systemd 服务..."
        cat > /etc/systemd/system/ai-server-agent.service << SERVICEEOF
[Unit]
Description=AI Server Agent - 自然语言服务器管理助手
Documentation=https://github.com/your-org/ai-server-agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=$BIN_DIR/agent -config $CONFIG_DIR/config.yaml
Restart=on-failure
RestartSec=10s
TimeoutStopSec=30s

# 安全加固
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=$DATA_DIR $CONFIG_DIR
PrivateTmp=yes
MemoryLimit=512M
CPUQuota=50%

# 日志
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICEEOF

        systemctl daemon-reload
        log_info "systemd 服务已安装"
    fi

    # 创建软链接
    ln -sf "$BIN_DIR/agent" /usr/local/bin/ai-agent 2>/dev/null || true

    # 完成
    echo ""
    echo "╔══════════════════════════════════════════╗"
    echo "║          ✅ 安装完成！                   ║"
    echo "╚══════════════════════════════════════════╝"
    echo ""
    echo "  安装目录: $INSTALL_DIR"
    echo "  配置文件: $CONFIG_DIR/config.yaml"
    echo "  数据目录: $DATA_DIR"
    echo ""
    echo -e "${YELLOW}  ⚠️  请先编辑配置文件:${NC}"
    echo "     vi $CONFIG_DIR/config.yaml"
    echo ""
    echo -e "${GREEN}  启动服务:${NC}"
    echo "     systemctl enable ai-server-agent"
    echo "     systemctl start ai-server-agent"
    echo ""
    echo -e "${GREEN}  查看状态:${NC}"
    echo "     systemctl status ai-server-agent"
    echo ""
    echo -e "${GREEN}  查看日志:${NC}"
    echo "     journalctl -u ai-server-agent -f"
    echo ""
    echo -e "${GREEN}  交互模式:${NC}"
    echo "     ai-agent"
    echo ""
    echo -e "${GREEN}  Web 界面:${NC}"
    echo "     http://\$(hostname -I 2>/dev/null | awk '{print \$1}' || echo 'YOUR_IP'):${AGENT_PORT}"
    echo ""
    echo -e "${YELLOW}  提示: 如需更改端口，编辑 ${CONFIG_DIR}/config.yaml${NC}"
    echo ""
}

main "$@"
