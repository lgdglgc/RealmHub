package iptables

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"realm-hub/pkg/models"
)

// Manager Linux 内核级端口段转发管理器（基于 iptables/nftables）
type Manager struct {
	mu           sync.Mutex
	isLinux      bool
	hasIptables  bool
	activeChains bool
}

const (
	ChainPrerouting  = "REALM_HUB_PREROUTING"
	ChainPostrouting = "REALM_HUB_POSTROUTING"
)

func NewManager() *Manager {
	m := &Manager{
		isLinux: runtime.GOOS == "linux",
	}
	if m.isLinux {
		if _, err := exec.LookPath("iptables"); err == nil {
			m.hasIptables = true
			m.initChains()
		} else {
			log.Println("[Kernel Mux] 未在系统 PATH 中找到 iptables 工具")
		}
	}
	return m
}

// initChains 初始化专属链，避免污染用户原有防火墙规则
func (m *Manager) initChains() {
	if !m.isLinux || !m.hasIptables {
		return
	}
	// 开启内核 IPv4 转发
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()

	// 创建自定义 NAT 链并挂载至 PREROUTING 和 POSTROUTING
	_ = exec.Command("iptables", "-t", "nat", "-N", ChainPrerouting).Run()
	_ = exec.Command("iptables", "-t", "nat", "-N", ChainPostrouting).Run()

	// 确保 PREROUTING 挂载了自定义链（若未挂载则挂载）
	checkPre := exec.Command("iptables", "-t", "nat", "-C", "PREROUTING", "-j", ChainPrerouting)
	if err := checkPre.Run(); err != nil {
		_ = exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-j", ChainPrerouting).Run()
	}

	checkPost := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-j", ChainPostrouting)
	if err := checkPost.Run(); err != nil {
		_ = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-j", ChainPostrouting).Run()
	}

	m.activeChains = true
}

// ApplyRules 同步应用所有内核级端口段规则
func (m *Manager) ApplyRules(rules []models.Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLinux {
		log.Println("[Kernel Mux] 非 Linux 平台，内核端口段转发处于模拟/旁路模式")
		return nil
	}

	if !m.hasIptables {
		return fmt.Errorf("系统缺少 iptables，无法下发内核端口段规则")
	}

	// 1. 清空自定义链现有规则
	_ = exec.Command("iptables", "-t", "nat", "-F", ChainPrerouting).Run()
	_ = exec.Command("iptables", "-t", "nat", "-F", ChainPostrouting).Run()

	// 2. 依次添加启用的内核转发规则
	appliedCount := 0
	for _, r := range rules {
		if !r.Enabled || r.Mode != models.ModeKernel {
			continue
		}

		if err := m.applySingleRule(r); err != nil {
			log.Printf("[Kernel Mux] 规则 %s (%s) 应用失败: %v", r.Name, r.ListenPort, err)
			continue
		}
		appliedCount++
	}

	log.Printf("[Kernel Mux] 内核转发规则更新完毕，当前激活 %d 条规则", appliedCount)
	return nil
}

// applySingleRule 应用单条规则到 iptables
// 端口格式支持: "20000-50000" 或 "20000:50000"
func (m *Manager) applySingleRule(r models.Rule) error {
	listenPort := strings.ReplaceAll(r.ListenPort, "-", ":")
	remoteAddr := r.RemoteAddr

	// 解析目标 IP 和目标端口
	targetHost, targetPort, err := splitHostPort(remoteAddr)
	if err != nil {
		return fmt.Errorf("无效的远程地址格式: %s", remoteAddr)
	}
	targetPort = strings.ReplaceAll(targetPort, "-", ":")

	protocols := []string{}
	switch r.Protocol {
	case models.ProtoTCP:
		protocols = append(protocols, "tcp")
	case models.ProtoUDP:
		protocols = append(protocols, "udp")
	case models.ProtoALL, "":
		protocols = append(protocols, "tcp", "udp")
	}

	destination := fmt.Sprintf("%s:%s", targetHost, targetPort)

	for _, proto := range protocols {
		// DNAT 规则 (入站改写目标 IP:Port)
		cmdDnat := exec.Command("iptables", "-t", "nat", "-A", ChainPrerouting,
			"-p", proto, "--dport", listenPort, "-j", "DNAT", "--to-destination", destination)
		if out, err := cmdDnat.CombinedOutput(); err != nil {
			return fmt.Errorf("DNAT 错误 (%s): %s", err, string(out))
		}

		// MASQUERADE 规则 (出站源地址改写，保证回包路由正确)
		cmdMasq := exec.Command("iptables", "-t", "nat", "-A", ChainPostrouting,
			"-p", proto, "-d", targetHost, "--dport", targetPort, "-j", "MASQUERADE")
		if out, err := cmdMasq.CombinedOutput(); err != nil {
			return fmt.Errorf("MASQUERADE 错误 (%s): %s", err, string(out))
		}
	}

	log.Printf("[Kernel Mux] 成功配置端口段: [%s] %s -> %s", strings.Join(protocols, "/"), listenPort, destination)
	return nil
}

// Clear 清理所有规则
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isLinux || !m.hasIptables {
		return
	}

	_ = exec.Command("iptables", "-t", "nat", "-F", ChainPrerouting).Run()
	_ = exec.Command("iptables", "-t", "nat", "-F", ChainPostrouting).Run()
	log.Println("[Kernel Mux] 已清空所有内核转发规则")
}

func splitHostPort(hostport string) (string, string, error) {
	idx := strings.LastIndex(hostport, ":")
	if idx == -1 {
		return "", "", fmt.Errorf("缺少冒号分隔符")
	}
	host := hostport[:idx]
	port := hostport[idx+1:]
	// 移除 IPv6 方括号
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return host, port, nil
}
