package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"

	"realm-hub/pkg/models"
	"realm-hub/pkg/protocol"
	"realm-hub/pkg/store"
)

//go:embed web/*
var embeddedWeb embed.FS

var (
	jwtSecret = []byte(generateRandomString(32))
	upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

type Hub struct {
	mu      sync.RWMutex
	clients map[string]*AgentConn // nodeID -> connection
	store   *store.Store
}

type AgentConn struct {
	NodeID string
	Conn   *websocket.Conn
	mu     sync.Mutex
}

func (c *AgentConn) Send(envelope *protocol.Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(envelope)
}

func NewHub(s *store.Store) *Hub {
	return &Hub{
		clients: make(map[string]*AgentConn),
		store:   s,
	}
}

func (h *Hub) Register(nodeID string, conn *websocket.Conn) *AgentConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 若已有旧连接，关闭
	if old, exists := h.clients[nodeID]; exists {
		_ = old.Conn.Close()
	}

	ac := &AgentConn{NodeID: nodeID, Conn: conn}
	h.clients[nodeID] = ac

	// 更新节点状态
	if node, ok := h.store.GetNode(nodeID); ok {
		node.Online = true
		node.LastSeen = time.Now()
		_ = h.store.SaveNode(*node)
	}

	return ac
}

func (h *Hub) Unregister(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, nodeID)
	if node, ok := h.store.GetNode(nodeID); ok {
		node.Online = false
		node.LastSeen = time.Now()
		_ = h.store.SaveNode(*node)
	}
}

// SyncRulesToNode 向特定节点同步其所有活跃规则
func (h *Hub) SyncRulesToNode(nodeID string) {
	h.mu.RLock()
	client, exists := h.clients[nodeID]
	h.mu.RUnlock()

	if !exists {
		return
	}

	rules := h.store.GetRulesByNode(nodeID)
	env, err := protocol.NewEnvelope(protocol.TypeSyncRules, protocol.SyncRulesPayload{Rules: rules})
	if err == nil {
		_ = client.Send(env)
	}
}

func main() {
	port := flag.Int("port", 3000, "Web 管理界面及 API 端口")
	dataDir := flag.String("data", "./data", "数据持久化存储目录")
	flag.Parse()

	dbStore, err := store.NewStore(*dataDir)
	if err != nil {
		log.Fatalf("初始化存储失败: %v", err)
	}

	hub := NewHub(dbStore)

	// API 路由
	mux := http.NewServeMux()

	// 认证接口
	mux.HandleFunc("/api/auth/me", handleAuthMe(dbStore))
	mux.HandleFunc("/api/auth/login", handleAuthLogin(dbStore))

	// 节点管理
	mux.HandleFunc("/api/nodes", authMiddleware(handleNodes(dbStore, hub)))
	mux.HandleFunc("/api/nodes/", authMiddleware(handleNodeByID(dbStore, hub)))

	// 规则管理
	mux.HandleFunc("/api/rules", authMiddleware(handleRules(dbStore, hub)))
	mux.HandleFunc("/api/rules/", authMiddleware(handleRuleByID(dbStore, hub)))

	// WebSocket 节点通信端点
	mux.HandleFunc("/ws/agent", handleAgentWS(hub, dbStore))

	// 一键安装脚本路由
	mux.HandleFunc("/install/agent.sh", handleAgentInstallScript())

	// 静态文件服务
	webFS, _ := fs.Sub(embeddedWeb, "web")
	fileServer := http.FileServer(http.FS(webFS))
	mux.Handle("/", fileServer)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("[Server] RealmHub 管理面板已启动: http://0.0.0.0:%d", *port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// handleAgentWS 处理受控 Agent 节点的 WebSocket 连接
func handleAgentWS(hub *Hub, s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := r.URL.Query().Get("secret")
		nodeID := r.URL.Query().Get("id")

		if secret == "" || nodeID == "" {
			http.Error(w, "缺少认证参数", http.StatusUnauthorized)
			return
		}

		node, ok := s.GetNode(nodeID)
		if !ok || node.Secret != secret {
			http.Error(w, "节点凭证错误", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[WS] 升级 WebSocket 失败: %v", err)
			return
		}

		// 记录远端 IP
		node.IP = strings.Split(r.RemoteAddr, ":")[0]
		_ = s.SaveNode(*node)

		hub.Register(nodeID, conn)
		log.Printf("[WS] 节点已上线: %s (%s)", node.Name, nodeID)

		// 立即下发该节点的所有最新规则
		hub.SyncRulesToNode(nodeID)

		defer func() {
			hub.Unregister(nodeID)
			_ = conn.Close()
			log.Printf("[WS] 节点已断开: %s (%s)", node.Name, nodeID)
		}()

		for {
			var env protocol.Envelope
			if err := conn.ReadJSON(&env); err != nil {
				break
			}

			switch env.Type {
			case protocol.TypeHeartbeat:
				var hb protocol.HeartbeatPayload
				if err := json.Unmarshal(env.Payload, &hb); err == nil {
					if n, ok := s.GetNode(nodeID); ok {
						n.LastSeen = time.Now()
						n.Online = true
						n.CPUUsage = hb.CPUUsage
						n.MemUsage = hb.MemUsage
						n.MemTotal = hb.MemTotal
						n.MemUsed = hb.MemUsed
						n.UptimeSec = hb.UptimeSec
						_ = s.SaveNode(*n)
					}
				}
			}
		}
	}
}

// handleAuthMe 检查当前登录状态
func handleAuthMe(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		needSetup := s.CountUsers() == 0
		username, authenticated := extractUserFromToken(r)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": authenticated,
			"username":      username,
			"need_setup":    needSetup,
		})
	}
}

// handleAuthLogin 登录或初始化
func handleAuthLogin(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "参数格式错误"})
			return
		}

		// 若系统中无任何用户，则第一位用户自动作为管理员初始化
		if s.CountUsers() == 0 {
			user, err := s.CreateUser(req.Username, req.Password)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			token := createJWTToken(user.Username)
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "token": token})
			return
		}

		user, ok := s.GetUser(req.Username)
		if !ok || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
			return
		}

		token := createJWTToken(user.Username)
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "token": token})
	}
}

func handleNodes(s *store.Store, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetNodes())
		case http.MethodPost:
			var req struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "节点名称不能为空"})
				return
			}
			node, err := s.CreateNode(req.Name)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, node)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleNodeByID(s *store.Store, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
		if r.Method == http.MethodDelete {
			if err := s.DeleteNode(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleRules(s *store.Store, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.GetRules())
		case http.MethodPost:
			var rule models.Rule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "参数格式错误"})
				return
			}
			if err := s.SaveRule(rule); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			// 触发节点同步
			hub.SyncRulesToNode(rule.NodeID)
			writeJSON(w, http.StatusOK, rule)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleRuleByID(s *store.Store, hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/rules/")
		parts := strings.Split(path, "/")
		id := parts[0]

		rule, ok := s.GetRule(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "规则不存在"})
			return
		}

		if len(parts) == 2 && parts[1] == "toggle" && r.Method == http.MethodPost {
			rule.Enabled = !rule.Enabled
			_ = s.SaveRule(*rule)
			hub.SyncRulesToNode(rule.NodeID)
			writeJSON(w, http.StatusOK, rule)
			return
		}

		if r.Method == http.MethodDelete {
			nodeID := rule.NodeID
			_ = s.DeleteRule(id)
			hub.SyncRulesToNode(nodeID)
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgentInstallScript 输出通用一键安装脚本
func handleAgentInstallScript() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		script := `#!/bin/bash
set -e
echo "========================================="
echo "       RealmHub Agent 一键安装程序       "
echo "========================================="

SERVER=""
SECRET=""
NODE_ID=""

for arg in "$@"; do
  case $arg in
    server=*) SERVER="${arg#*=}" ;;
    secret=*) SECRET="${arg#*=}" ;;
    id=*)     NODE_ID="${arg#*=}" ;;
  esac
done

if [ -z "$SERVER" ] || [ -z "$SECRET" ] || [ -z "$NODE_ID" ]; then
  echo "错误: 缺少必要参数 (server, secret, id)"
  exit 1
fi

INSTALL_DIR="/etc/realm-hub"
mkdir -p "$INSTALL_DIR"

echo "[1/4] 下载 realm-agent..."
# 若安装包在主控端，可直接拉取
ARCH=$(uname -m)
case $ARCH in
  x86_64) AGENT_URL="https://github.com/zhboner/realm/releases/latest/download/realm-x86_64-unknown-linux-musl.tar.gz" ;;
  aarch64|arm64) AGENT_URL="https://github.com/zhboner/realm/releases/latest/download/realm-aarch64-unknown-linux-musl.tar.gz" ;;
esac

# 检查 iptables 依赖
if ! command -v iptables &> /dev/null; then
  echo "[2/4] 安装 iptables 依赖..."
  if command -v apt-get &> /dev/null; then
    apt-get update -qq && apt-get install -y -qq iptables curl wget
  elif command -v yum &> /dev/null; then
    yum install -y iptables curl wget
  fi
fi

# 下载官方 realm 内核（如果本地没有）
if [ ! -f "$INSTALL_DIR/realm" ]; then
  echo "[3/4] 下载官方 Realm 内核..."
  wget -qO /tmp/realm.tar.gz "$AGENT_URL" && tar -xzf /tmp/realm.tar.gz -C "$INSTALL_DIR/" && chmod +x "$INSTALL_DIR/realm" || true
fi

echo "[4/4] 注册 systemd 守护进程服务..."
cat > /etc/systemd/system/realm-agent.service << EOF
[Unit]
Description=RealmHub Agent Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/realm-agent --server "$SERVER" --secret "$SECRET" --id "$NODE_ID" --dir "$INSTALL_DIR"
Restart=always
RestartSec=3s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable realm-agent
systemctl restart realm-agent

echo "✅ RealmHub Agent 已成功启动并在后台运行！"
`
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(script))
	}
}

// 辅助方法
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := extractUserFromToken(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未授权访问，请登录"})
			return
		}
		next(w, r)
	}
}

func createJWTToken(username string) string {
	claims := jwt.MapClaims{
		"sub": username,
		"exp": time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(jwtSecret)
	return tokenStr
}

func extractUserFromToken(r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}

	sub, ok := claims["sub"].(string)
	return sub, ok
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
