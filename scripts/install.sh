#!/bin/bash
# =========================================================
# RealmHub 一键部署管理脚本 (Server 管理面板 / Agent 受控节点)
# 支持 Debian, Ubuntu, CentOS, Rocky, AlmaLinux 等主流系统
# =========================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

INSTALL_DIR="/etc/realm-hub"

check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}[错误] 请使用 root 权限执行此脚本 (sudo -i)${NC}"
        exit 1
    fi
}

install_deps() {
    echo -e "${BLUE}[1/4] 检查并安装运行依赖 (curl, wget, iptables, tar)...${NC}"
    if command -v apt-get &> /dev/null; then
        apt-get update -qq && apt-get install -y -qq curl wget iptables tar systemd
    elif command -v yum &> /dev/null; then
        yum install -y curl wget iptables tar systemd
    elif command -v dnf &> /dev/null; then
        dnf install -y curl wget iptables tar systemd
    fi
}

install_server() {
    check_root
    install_deps

    mkdir -p "$INSTALL_DIR/data"
    echo -e "${BLUE}[2/4] 配置 RealmHub Server 管理端...${NC}"

    read -p "请输入 Web 管理面板端口 (默认: 3000): " WEB_PORT
    WEB_PORT=${WEB_PORT:-3000}

    # 检查本地是否有编译产物，若没有则提示
    if [ -f "./realm-server" ]; then
        cp ./realm-server "$INSTALL_DIR/realm-server"
    else
        echo -e "${YELLOW}请将编译好的 realm-server 二进制放置在当前目录，或从 Release 下载。${NC}"
    fi
    chmod +x "$INSTALL_DIR/realm-server"

    echo -e "${BLUE}[3/4] 注册并启动 systemd 服务...${NC}"
    cat > /etc/systemd/system/realm-server.service << EOF
[Unit]
Description=RealmHub Manager Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/realm-server -port $WEB_PORT -data $INSTALL_DIR/data
Restart=always
RestartSec=3s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable realm-server
    systemctl restart realm-server

    SERVER_IP=$(curl -s ifconfig.me || echo "YOUR_SERVER_IP")
    echo -e "${GREEN}========================================================${NC}"
    echo -e "${GREEN}🎉 RealmHub 管理面板已成功启动！${NC}"
    echo -e "${BLUE}访问地址: ${GREEN}http://${SERVER_IP}:${WEB_PORT}${NC}"
    echo -e "${BLUE}首次访问将自动引导创建管理员账号。${NC}"
    echo -e "${GREEN}========================================================${NC}"
}

install_agent() {
    check_root
    install_deps

    mkdir -p "$INSTALL_DIR"
    echo -e "${BLUE}[2/4] 配置 RealmHub Agent 节点端...${NC}"

    read -p "请输入管理端 WebSocket 地址 (如 ws://1.1.1.1:3000/ws/agent): " SERVER_ADDR
    read -p "请输入节点 Secret 密钥: " NODE_SECRET
    read -p "请输入节点 ID: " NODE_ID

    if [ -z "$SERVER_ADDR" ] || [ -z "$NODE_SECRET" ] || [ -z "$NODE_ID" ]; then
        echo -e "${RED}[错误] 参数不能为空！${NC}"
        exit 1
    fi

    if [ -f "./realm-agent" ]; then
        cp ./realm-agent "$INSTALL_DIR/realm-agent"
    fi
    chmod +x "$INSTALL_DIR/realm-agent"

    # 下载官方最新 realm 内核
    ARCH=$(uname -m)
    case $ARCH in
      x86_64) REALM_URL="https://github.com/zhboner/realm/releases/latest/download/realm-x86_64-unknown-linux-musl.tar.gz" ;;
      aarch64|arm64) REALM_URL="https://github.com/zhboner/realm/releases/latest/download/realm-aarch64-unknown-linux-musl.tar.gz" ;;
    esac

    if [ ! -f "$INSTALL_DIR/realm" ] && [ -n "$REALM_URL" ]; then
        echo -e "${BLUE}[3/4] 下载官方 Realm 转发内核...${NC}"
        wget -qO /tmp/realm.tar.gz "$REALM_URL" && tar -xzf /tmp/realm.tar.gz -C "$INSTALL_DIR/" && chmod +x "$INSTALL_DIR/realm" || true
    fi

    echo -e "${BLUE}[4/4] 注册并启动 systemd 服务...${NC}"
    cat > /etc/systemd/system/realm-agent.service << EOF
[Unit]
Description=RealmHub Agent Node
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/realm-agent --server "$SERVER_ADDR" --secret "$NODE_SECRET" --id "$NODE_ID" --dir "$INSTALL_DIR"
Restart=always
RestartSec=3s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable realm-agent
    systemctl restart realm-agent

    echo -e "${GREEN}========================================================${NC}"
    echo -e "${GREEN}✅ RealmHub Agent 已成功启动并接入管理端！${NC}"
    echo -e "${GREEN}========================================================${NC}"
}

echo -e "${GREEN}==============================================${NC}"
echo -e "${GREEN}        RealmHub 统一管理与部署脚本            ${NC}"
echo -e "${GREEN}==============================================${NC}"
echo " 1) 安装 / 更新 Server 管理端"
echo " 2) 安装 / 更新 Agent 受控节点"
echo " 3) 卸载 Server"
echo " 4) 卸载 Agent"
echo " 0) 退出"
read -p "请输入选项 [0-4]: " OPTION

case $OPTION in
    1) install_server ;;
    2) install_agent ;;
    3)
        systemctl stop realm-server 2>/dev/null || true
        systemctl disable realm-server 2>/dev/null || true
        rm -f /etc/systemd/system/realm-server.service
        echo -e "${GREEN}Server 服务已卸载。${NC}"
        ;;
    4)
        systemctl stop realm-agent 2>/dev/null || true
        systemctl disable realm-agent 2>/dev/null || true
        rm -f /etc/systemd/system/realm-agent.service
        echo -e "${GREEN}Agent 服务已卸载。${NC}"
        ;;
    *) exit 0 ;;
esac

