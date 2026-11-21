package models

import (
	"time"
)

// ForwardMode 转发模式
type ForwardMode string

const (
	ModeRealm  ForwardMode = "realm"  // Realm 用户态高性能中继（单端口、多端口、负载均衡、DNS动态解析）
	ModeKernel ForwardMode = "kernel" // Linux 内核级转发（iptables/nftables，极适合 20000-50000 大端口段）
)

// Protocol 协议类型
type Protocol string

const (
	ProtoTCP Protocol = "tcp"
	ProtoUDP Protocol = "udp"
	ProtoALL Protocol = "all"
)

// Rule 转发规则
type Rule struct {
	ID         string      `json:"id"`
	NodeID     string      `json:"node_id"`
	Name       string      `json:"name"`
	Mode       ForwardMode `json:"mode"`        // realm 或 kernel
	Protocol   Protocol    `json:"protocol"`    // tcp, udp, all
	ListenPort string      `json:"listen_port"` // 例如 "8080" 或 "20000-50000"
	RemoteAddr string      `json:"remote_addr"` // 例如 "1.1.1.1:8080" 或 "1.1.1.1:20000-50000"
	Enabled    bool        `json:"enabled"`
	Status     string      `json:"status"` // running, stopped, error
	ErrorMsg   string      `json:"error_msg,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// Node 节点信息
type Node struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Secret    string    `json:"secret"` // Agent 连接凭证
	IP        string    `json:"ip"`
	Version   string    `json:"version"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Online    bool      `json:"online"`
	LastSeen  time.Time `json:"last_seen"`
	CPUUsage  float64   `json:"cpu_usage"`  // 百分比
	MemUsage  float64   `json:"mem_usage"`  // 百分比
	MemTotal  uint64    `json:"mem_total"`  // 字节
	MemUsed   uint64    `json:"mem_used"`   // 字节
	UptimeSec uint64    `json:"uptime_sec"` // 秒
	CreatedAt time.Time `json:"created_at"`
}

// User 管理员用户
type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
}
