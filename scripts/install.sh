#!/bin/bash
# AI Server Agent 安装脚本
set -e

INSTALL_DIR="/opt/ai-server-agent"
BIN_DIR="$INSTALL_DIR/bin"
CONFIG_DIR="$INSTALL_DIR/configs"
DATA_DIR="$INSTALL_DIR/data"

echo "=== AI Server Agent 安装脚本 ==="

# 创建目录
mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$DATA_DIR"

# 复制二进制
cp bin/agent "$BIN_DIR/agent"
chmod +x "$BIN_DIR/agent"

# 复制配置（如果不存在）
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    cp configs/config.yaml "$CONFIG_DIR/config.yaml"
    echo "已创建默认配置: $CONFIG_DIR/config.yaml"
    echo "请编辑配置文件填入 1Panel API 密钥和 LLM API 密钥"
fi

# 安装 systemd service
cp scripts/ai-server-agent.service /etc/systemd/system/ai-server-agent.service
systemctl daemon-reload

echo "安装完成！"
echo ""
echo "启动服务:"
echo "  systemctl enable ai-server-agent"
echo "  systemctl start ai-server-agent"
echo ""
echo "查看状态:"
echo "  systemctl status ai-server-agent"
echo ""
echo "查看日志:"
echo "  journalctl -u ai-server-agent -f"
echo ""
echo "交互模式:"
echo "  $BIN_DIR/agent"
