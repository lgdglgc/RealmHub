# 🚀 RealmHub: 开源·高性能·多节点 Realm 转发与端口段管理面板

<p align="center">
  <img src="https://img.shields.io/badge/Language-Go%201.27+-00ADD8.svg?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Core-Official%20Realm%2026.x-blue.svg?style=flat-square" alt="Realm">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Arch-Linux%20x86__64%20%7C%20ARM64-orange.svg?style=flat-square" alt="Arch">
</p>

> **完全开源可控**、基于 Go 语言构建的分布式端口转发控制系统。深度融合 **官方 Realm 原生内核** 与 **Linux 内核级 (iptables/nftables) 大端口段整段转发引擎**。
> 告别繁琐的 SSH 配置，轻松实现单端口中继与 `20000-50000` 超大端口段的统一集中管控。

---

## ⚡ 核心优势

| 特性 | 说明 |
| :--- | :--- |
| 🌐 **100% 完全开源** | 拒绝闭源黑盒，核心逻辑完全自主可控，安全无后门 |
| ⚡ **官方原生内核** | 采用官方经过验证的 **Realm (v26.x)** 转发内核，低延迟、极小内存常驻 |
| 🔥 **大端口段整段转发** | **独创双引擎调度**：支持 `20000-50000` 端口段整段映射，后端随意开服，前端永久免动 |
| 💻 **Web 统一集中管理** | 采用 Agent 模式，无需 SSH 逐台登录，通过响应式 Web 控制台实时管控全网节点 |
| 🪶 **单文件零依赖部署** | 控制端（Server）与受控端（Agent）均为纯静态单一二进制，自带嵌入式存储，无需额外安装 Docker 或 MySQL |
| 🚀 **全自动一键安装** | 预编译开箱即用，内置 systemd 守护进程与日志轮转 |

---

## 🛠️ 快速一键安装指南

### 1. 部署控制面板 (RealmHub Server)

在您的中控服务器（具有公网 IP 的 VPS）终端中执行以下一键安装命令：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/lgdglgc/RealmHub/main/scripts/install.sh) server port=3000
```
> **参数说明**：
> * `port=3000`：Web 控制面板访问端口（默认 3000，可自定义）。

---

### 2. 部署受控节点 (RealmHub Agent)

#### 方式 A：面板一键复制（最推荐）
1. 访问控制面板：`http://你的服务器IP:3000`；
2. 首次访问根据提示创建管理员账号；
3. 进入 **「节点管理」** -> 点击 **「添加节点」**；
4. 复制面板自动生成的**专属一键安装命令**，在目标转发机器上粘贴执行即可，节点将秒级自动上线！

#### 方式 B：手动执行脚本安装
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/lgdglgc/RealmHub/main/scripts/install.sh) agent server="ws://控制端IP:3000/ws/agent" secret="你的SECRET" id="你的NODE_ID"
```

---

### 3. 交互式菜单与运维管理

在任何服务器上直接运行下方命令，即可唤起交互式运维菜单：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/lgdglgc/RealmHub/main/scripts/install.sh)
```

<details>
<summary><b>🛠️ 常用管理服务命令</b></summary>

```bash
# 查看 Server 状态与日志
systemctl status realm-server
journalctl -u realm-server -f

# 查看 Agent 状态与日志
systemctl status realm-agent
journalctl -u realm-agent -f

# 重启服务
systemctl restart realm-server
systemctl restart realm-agent

# 卸载 Server
bash <(curl -fsSL https://raw.githubusercontent.com/lgdglgc/RealmHub/main/scripts/install.sh) uninstall-server

# 卸载 Agent
bash <(curl -fsSL https://raw.githubusercontent.com/lgdglgc/RealmHub/main/scripts/install.sh) uninstall-agent
```
</details>

---

## 🔥 20000-50000 端口段转发使用指南

> **使用场景**：游戏服务器（如幻兽帕鲁、MC、CS2等联机集群）、动态端口服务、NAT VPS 或中继网关。

### 为什么选择 RealmHub 端口段模式？
* 传统用户态代理（包括原版闭源面板）每监听 1 个端口就需要建立 1 个 Socket。若整段转发 20000-50000（30,001 个端口），双协议将创建 **6 万多个 Socket**，导致系统文件描述符（FD）耗尽、内存暴增崩溃；
* **RealmHub 内核引擎**：直接在 Linux 内核层注入专属 NAT 链（`REALM_HUB_PREROUTING` / `REALM_HUB_POSTROUTING`）进行 DNAT 映射。**0 额外内存消耗，跑满网卡线速，前端配好后无需再动，后端在此区间内随意开服！**

### 配置步骤：
1. 打开控制面板，进入 **「转发规则」** -> 点击 **「新增规则」**；
2. **目标受控节点**：选择你要作为转发机的前端服务器；
3. **转发工作模式**：选择 **`🔥 内核级 iptables/nftables (大端口段 20000-50000 整段转发)`**；
4. **传输协议**：选择 `TCP + UDP`（双协议全通）；
5. **本地监听端口**：填入 `20000-50000`；
6. **目标地址**：填入后端服务器 IP 与端口段（例如 `1.2.3.4:20000-50000`）；
7. 点击 **「保存并同步」**，Agent 立即热生效！

---

## 🏗️ 双引擎架构拓扑

```
                            ┌────────────────────────────────────────┐
                            │     RealmHub Server (Web 控制中心)     │
                            │  • 单二进制内置现代化响应式暗色 UI      │
                            │  • 节点管理、一键安装命令生成          │
                            │  • 转发规则增删改查、即时下发推送      │
                            └───────────────────┬────────────────────┘
                                                │ (WebSocket / API Key 认证)
                   ┌────────────────────────────┴────────────────────────────┐
                   ▼                                                         ▼
┌──────────────────────────────────────┐  ┌──────────────────────────────────────┐
│       RealmHub Agent (节点 A)        │  │       RealmHub Agent (节点 B)        │
│                                      │  │                                      │
│  [双引擎执行器]                      │  │  [双引擎执行器]                      │
│   ├── 模式 1: Realm 引擎 ─────────┐  │  │   ├── 模式 1: Realm 引擎 ─────────┐  │
│   │   (单端口/常规高性能中继)     │  │  │   │   (单端口/常规高性能中继)     │  │
│   │                               ▼  │  │   │                               ▼  │
│   │                    [官方 Realm 内核]│  │   │                    [官方 Realm 内核]│
│   │                                  │  │   │                                  │
│   └── 模式 2: 内核引擎 ──────────┐  │  │   └── 模式 2: 内核引擎 ──────────┐  │
│       (大端口段 20000-50000)      │  │  │       (大端口段 20000-50000)      │  │
│                                   ▼  │  │                                   ▼  │
│                    [Linux iptables/DNAT]│  │                    [Linux iptables/DNAT]│
└──────────────────────────────────────┘  └──────────────────────────────────────┘
                   │                                                         │
                   └───────────────────────────┬─────────────────────────────┘
                                               ▼
                                      [ 最终目标业务服务器群 ]
```

---

## 💻 本地自编译 (开发与定制)

项目采用纯 Go 构建（无 CGO），一行命令即可跨平台交叉编译：

```bash
# 克隆仓库
git clone https://github.com/lgdglgc/RealmHub.git
cd RealmHub

# 编译 Linux amd64 生产版本
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o realm-server ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o realm-agent ./cmd/agent

# 编译 Linux arm64 生产版本 (树莓派 / 甲骨文 ARM)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o realm-server-arm64 ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o realm-agent-arm64 ./cmd/agent
```

---

## 📄 许可证

本项目采用 [MIT 许可证](LICENSE) 开源发布。
欢迎提交 Issue 和 Pull Request 一同改进！
