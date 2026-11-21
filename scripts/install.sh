#!/bin/bash
# =========================================================
# RealmHub 一键部署管理脚本 (Server 控制面板 / Agent 受控节点)
# 项目地址: https://github.com/lgdglgc/RealmHub
# 支持系统: Debian, Ubuntu, CentOS, Rocky Linux, AlmaLinux 等
# =========================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

INSTALL_DIR="/etc/realm-hub"
REPO_RAW="https://raw.githubusercontent.com/lgdglgc/RealmHub/main"

check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}[错误] 请使用 root 权限执行此脚本 (sudo -i)${NC}"
        exit 1
    fi
}

install_deps() {
    echo -e "${BLUE}[1/4] 检查系统基础依赖 (curl, wget, iptables, tar, systemd)...${NC}"
    if command -v apt-get &> /dev/null; then
        apt-get update -qq && apt-get install -y -qq curl wget iptables tar systemd ca-certificates
    elif command -v yum &> /dev/null; then
        yum install -y curl wget iptables tar systemd ca-certificates
    elif command -v dnf &> /dev/null; then
        dnf install -y curl wget iptables tar systemd ca-certificates
    fi
}

# 部署控制面板 (Server)
install_server() {
    check_root
    install_deps

    mkdir -p "$INSTALL_DIR/data"
    echo -e "${BLUE}[2/4] 获取 RealmHub Server 核心文件...${NC}"

    if [ -f "./realm-server" ]; then
        cp ./realm-server "$INSTALL_DIR/realm-server"
    else
        echo -e "${BLUE}正在从 GitHub 下载 realm-server...${NC}"
        wget -q --show-progress -O "$INSTALL_DIR/realm-server" "$REPO_RAW/realm-server" || {
            curl -fsSL -o "$INSTALL_DIR/realm-server" "$REPO_RAW/realm-server"
        }
    fi
    chmod +x "$INSTALL_DIR/realm-server"

    # 端口参数
    if [ -z "$WEB_PORT" ]; then
        if [ -n "$1" ]; then
            WEB_PORT="$1"
        else
            read -p "请输入 Web 管理面板访问端口 (默认 3000): " input_port
            WEB_PORT=${input_port:-3000}
        fi
    fi

    echo -e "${BLUE}[3/4] 注册并启动 systemd 守护进程...${NC}"
    cat > /etc/systemd/system/realm-server.service << EOF
[Unit]
Description=RealmHub Manager Server
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/realm-server -port $WEB_PORT -data $INSTALL_DIR/data
Restart=always
RestartSec=3s
LimitNOFILE=1048576
LimitNPROC=1048576

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable realm-server
    systemctl restart realm-server

    SERVER_IP=$(curl -s ifconfig.me || curl -s icanhazip.com || echo "YOUR_SERVER_IP")
    echo
    echo -e "${GREEN}========================================================${NC}"
    echo -e "${GREEN}🎉 RealmHub 管理面板已成功启动并常驻后台！${NC}"
    echo -e "${BLUE}🌐 访问地址: ${GREEN}http://${SERVER_IP}:${WEB_PORT}${NC}"
    echo -e "${YELLOW}📝 首次访问将自动引导创建管理员账号。${NC}"
    echo -e "${BLUE}📁 数据存储目录: ${INSTALL_DIR}/data${NC}"
    echo -e "${GREEN}========================================================${NC}"
}

# 部署受控节点 (Agent)
install_agent() {
    check_root
    install_deps

    mkdir -p "$INSTALL_DIR"
    echo -e "${BLUE}[2/4] 获取 RealmHub Agent 核心程序...${NC}"

    if [ -f "./realm-agent" ]; then
        cp ./realm-agent "$INSTALL_DIR/realm-agent"
    else
        echo -e "${BLUE}正在从 GitHub 下载 realm-agent...${NC}"
        wget -q --show-progress -O "$INSTALL_DIR/realm-agent" "$REPO_RAW/realm-agent" || {
            curl -fsSL -o "$INSTALL_DIR/realm-agent" "$REPO_RAW/realm-agent"
        }
    fi
    chmod +x "$INSTALL_DIR/realm-agent"

    # 参数检查（支持命令行传入 server=... secret=... id=...）
    if [ -z "$SERVER_ADDR" ]; then
        read -p "请输入控制端 WebSocket 地址 (如 ws://1.1.1.1:3000/ws/agent): " SERVER_ADDR
    fi
    if [ -z "$NODE_SECRET" ]; then
        read -p "请输入节点 Secret 密钥: " NODE_SECRET
    fi
    if [ -z "$NODE_ID" ]; then
        read -p "请输入节点唯一 ID: " NODE_ID
    fi

    if [ -z "$SERVER_ADDR" ] || [ -z "$NODE_SECRET" ] || [ -z "$NODE_ID" ]; then
        echo -e "${RED}[错误] 缺少必要参数！无法接入管理端。${NC}"
        exit 1
    fi

    # 检查并下载官方最新 realm 转发内核
    ARCH=$(uname -m)
    case $ARCH in
      x86_64) REALM_URL="https://github.com/zhboner/realm/releases/latest/download/realm-x86_64-unknown-linux-musl.tar.gz" ;;
      aarch64|arm64) REALM_URL="https://github.com/zhboner/realm/releases/latest/download/realm-aarch64-unknown-linux-musl.tar.gz" ;;
    esac

    if [ ! -f "$INSTALL_DIR/realm" ] && [ -n "$REALM_URL" ]; then
        echo -e "${BLUE}[3/4] 下载官方 Realm 原生转发内核...${NC}"
        wget -qO /tmp/realm.tar.gz "$REALM_URL" && tar -xzf /tmp/realm.tar.gz -C "$INSTALL_DIR/" && chmod +x "$INSTALL_DIR/realm" || true
    fi

    echo -e "${BLUE}[4/4] 注册并启动 systemd 守护进程...${NC}"
    cat > /etc/systemd/system/realm-agent.service << EOF
[Unit]
Description=RealmHub Agent Node
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/realm-agent --server "$SERVER_ADDR" --secret "$NODE_SECRET" --id "$NODE_ID" --dir "$INSTALL_DIR"
Restart=always
RestartSec=3s
LimitNOFILE=1048576
LimitNPROC=1048576

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable realm-agent
    systemctl restart realm-agent

    echo
    echo -e "${GREEN}========================================================${NC}"
    echo -e "${GREEN}✅ RealmHub Agent 已成功启动并接入管理中心！${NC}"
    echo -e "${BLUE}查看日志命令: ${YELLOW}journalctl -u realm-agent -f${NC}"
    echo -e "${GREEN}========================================================${NC}"
}

# 卸载功能
uninstall_server() {
    check_root
    echo -e "${YELLOW}正在停止并移除 RealmHub Server...${NC}"
    systemctl stop realm-server 2>/dev/null || true
    systemctl disable realm-server 2>/dev/null || true
    rm -f /etc/systemd/system/realm-server.service
    systemctl daemon-reload
    echo -e "${GREEN}RealmHub Server 服务已成功卸载。数据目录保留于 ${INSTALL_DIR}/data${NC}"
}

uninstall_agent() {
    check_root
    echo -e "${YELLOW}正在停止并移除 RealmHub Agent...${NC}"
    systemctl stop realm-agent 2>/dev/null || true
    systemctl disable realm-agent 2>/dev/null || true
    rm -f /etc/systemd/system/realm-agent.service
    systemctl daemon-reload
    echo -e "${GREEN}RealmHub Agent 服务已成功卸载。${NC}"
}

# 命令行参数解析
ACTION=""
for arg in "$@"; do
    case $arg in
        server) ACTION="server" ;;
        agent) ACTION="agent" ;;
        uninstall-server) ACTION="uninstall-server" ;;
        uninstall-agent) ACTION="uninstall-agent" ;;
        port=*|web-port=*) WEB_PORT="${arg#*=}" ;;
        server=*|server-addr=*) SERVER_ADDR="${arg#*=}" ;;
        secret=*|key=*) NODE_SECRET="${arg#*=}" ;;
        id=*|node-id=*) NODE_ID="${arg#*=}" ;;
    esac
done

if [ "$ACTION" = "server" ]; then
    install_server
    exit 0
elif [ "$ACTION" = "agent" ]; then
    install_agent
    exit 0
elif [ "$ACTION" = "uninstall-server" ]; then
    uninstall_server
    exit 0
elif [ "$ACTION" = "uninstall-agent" ]; then
    uninstall_agent
    exit 0
fi

# 交互式菜单
echo
echo -e "${GREEN}======================================================${NC}"
echo -e "${GREEN}         🚀 RealmHub 统一管理与部署脚本                ${NC}"
echo -e "${GREEN}======================================================${NC}"
echo -e " ${BLUE}1)${NC} 安装 / 更新 Server 管理面板"
echo -e " ${BLUE}2)${NC} 安装 / 更新 Agent 受控节点"
echo -e " ${YELLOW}3)${NC} 卸载 Server 管理端"
echo -e " ${YELLOW}4)${NC} 卸载 Agent 节点端"
echo -e " ${RED}0)${NC} 退出"
echo -e "${GREEN}======================================================${NC}"
read -p "请输入选项 [0-4]: " OPTION

case $OPTION in
    1) install_server ;;
    2) install_agent ;;
    3) uninstall_server ;;
    4) uninstall_agent ;;
    *) exit 0 ;;
esac
