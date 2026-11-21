package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"realm-hub/pkg/iptables"
	"realm-hub/pkg/protocol"
	"realm-hub/pkg/realm"
	"realm-hub/pkg/sysinfo"
)

func main() {
	serverAddr := flag.String("server", "", "控制端 WebSocket 地址 (如 ws://1.1.1.1:3000/ws/agent)")
	secret := flag.String("secret", "", "节点连接密钥")
	nodeID := flag.String("id", "", "节点唯一标识")
	workDir := flag.String("dir", "./", "工作与配置目录")
	flag.Parse()

	if *serverAddr == "" || *secret == "" || *nodeID == "" {
		log.Fatal("错误: 必须指定 --server, --secret 和 --id 参数")
	}

	_ = os.MkdirAll(*workDir, 0755)

	log.Println("==================================================")
	log.Printf("🚀 启动 RealmHub Agent (ID: %s)", *nodeID)
	log.Printf("📦 工作目录: %s", filepath.Clean(*workDir))
	log.Println("==================================================")

	// 初始化双执行引擎
	realmManager := realm.NewManager(*workDir)
	kernelManager := iptables.NewManager()

	// 退出信号捕获
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\n[Agent] 收到终止信号，正在清理环境与规则...")
		realmManager.Stop()
		kernelManager.Clear()
		os.Exit(0)
	}()

	// 构建连接 URL
	u, err := url.Parse(*serverAddr)
	if err != nil {
		log.Fatalf("无效的服务端 URL: %v", err)
	}
	q := u.Query()
	q.Set("id", *nodeID)
	q.Set("secret", *secret)
	u.RawQuery = q.Encode()

	// 启动常驻重连循环
	for {
		log.Printf("[Agent] 正在连接管理端: %s ...", u.Host)
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("[Agent] 连接失败: %v, 5 秒后重试...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("[Agent] ✅ 已成功连接至管理端！开始监听控制指令...")

		done := make(chan struct{})

		// 心跳协程 (每 5 秒上报负载)
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					m := sysinfo.GetMetrics()
					hb := protocol.HeartbeatPayload{
						NodeID:    *nodeID,
						CPUUsage:  m.CPUUsage,
						MemUsage:  m.MemUsage,
						MemTotal:  m.MemTotal,
						MemUsed:   m.MemUsed,
						UptimeSec: m.UptimeSec,
					}
					env, err := protocol.NewEnvelope(protocol.TypeHeartbeat, hb)
					if err == nil {
						_ = conn.WriteJSON(env)
					}
				}
			}
		}()

		// 消息接收处理循环
		for {
			var env protocol.Envelope
			if err := conn.ReadJSON(&env); err != nil {
				log.Printf("[Agent] 与管理端连接断开: %v", err)
				close(done)
				break
			}

			switch env.Type {
			case protocol.TypeSyncRules:
				var syncPayload protocol.SyncRulesPayload
				if err := json.Unmarshal(env.Payload, &syncPayload); err != nil {
					log.Printf("[Agent] 解析规则失败: %v", err)
					continue
				}

				fmt.Printf("\n[Agent] 📥 接收到管理端规则同步请求 (共 %d 条规则)\n", len(syncPayload.Rules))

				// 分流执行：
				// 1. Realm 用户态转发引擎
				if err := realmManager.ApplyRules(syncPayload.Rules); err != nil {
					log.Printf("[Realm] 应用规则出现错误: %v", err)
				}

				// 2. 内核端口段转发引擎 (iptables/nftables)
				if err := kernelManager.ApplyRules(syncPayload.Rules); err != nil {
					log.Printf("[Kernel] 应用内核端口段规则出现错误: %v", err)
				}
			}
		}

		time.Sleep(3 * time.Second)
	}
}
