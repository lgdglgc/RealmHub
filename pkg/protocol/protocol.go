package protocol

import (
	"encoding/json"
	"time"

	"realm-hub/pkg/models"
)

// MessageType 通信消息类型
type MessageType string

const (
	TypeRegister  MessageType = "register"   // Agent -> Server: 节点上线注册
	TypeHeartbeat MessageType = "heartbeat"  // Agent -> Server: 心跳及负载上报
	TypeSyncRules MessageType = "sync_rules" // Server -> Agent: 下发该节点所有最新规则
	TypeRuleAck   MessageType = "rule_ack"   // Agent -> Server: 规则应用状态反馈
)

// Envelope 统一 WebSocket 消息封装
type Envelope struct {
	Type      MessageType     `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp int64           `json:"timestamp"`
}

// RegisterPayload 注册载荷
type RegisterPayload struct {
	NodeID   string `json:"node_id"`
	Secret   string `json:"secret"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
}

// HeartbeatPayload 心跳载荷
type HeartbeatPayload struct {
	NodeID     string                    `json:"node_id"`
	CPUUsage   float64                   `json:"cpu_usage"`
	MemUsage   float64                   `json:"mem_usage"`
	MemTotal   uint64                    `json:"mem_total"`
	MemUsed    uint64                    `json:"mem_used"`
	UptimeSec  uint64                    `json:"uptime_sec"`
	RuleStatus map[string]RuleStatusItem `json:"rule_status"` // ruleID -> status
}

// RuleStatusItem 单条规则运行情况
type RuleStatusItem struct {
	Status   string `json:"status"` // running, failed, stopped
	ErrorMsg string `json:"error_msg,omitempty"`
}

// SyncRulesPayload 规则同步载荷
type SyncRulesPayload struct {
	Rules []models.Rule `json:"rules"`
}

// NewEnvelope 构造消息封装
func NewEnvelope(msgType MessageType, payload interface{}) (*Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Type:      msgType,
		Payload:   data,
		Timestamp: time.Now().Unix(),
	}, nil
}
