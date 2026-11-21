package realm

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"realm-hub/pkg/models"
)

// Config Realm 配置文件结构
type Config struct {
	Dns       DnsConf        `json:"dns"`
	Network   NetworkConf    `json:"network"`
	Endpoints []EndpointConf `json:"endpoints"`
}

type DnsConf struct {
	Mode        string   `json:"mode"`
	Nameservers []string `json:"nameservers"`
	Timeout     int      `json:"timeout"`
	CacheSize   int      `json:"cache_size"`
}

type NetworkConf struct {
	NoTCP        bool `json:"no_tcp"`
	UseUDP       bool `json:"use_udp"`
	TcpKeepalive int  `json:"tcp_keepalive"`
	TcpTimeout   int  `json:"tcp_timeout"`
	UdpTimeout   int  `json:"udp_timeout"`
}

type EndpointConf struct {
	Listen string `json:"listen"`
	Remote string `json:"remote"`
}

// Manager Realm 进程管理器
type Manager struct {
	mu         sync.Mutex
	binPath    string
	configPath string
	cmd        *exec.Cmd
}

func NewManager(workDir string) *Manager {
	binName := "realm"
	if runtime.GOOS == "windows" {
		binName = "realm.exe"
	}
	binPath := filepath.Join(workDir, binName)
	configPath := filepath.Join(workDir, "realm.json")

	return &Manager{
		binPath:    binPath,
		configPath: configPath,
	}
}

// ApplyRules 根据模型规则更新 realm 配置并重启/重载进程
func (m *Manager) ApplyRules(rules []models.Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var endpoints []EndpointConf
	for _, r := range rules {
		if !r.Enabled || r.Mode != models.ModeRealm {
			continue
		}
		// 构造规范的 listen 与 remote
		listen := r.ListenPort
		if !hasHost(listen) {
			listen = "0.0.0.0:" + listen
		}

		endpoints = append(endpoints, EndpointConf{
			Listen: listen,
			Remote: r.RemoteAddr,
		})
	}

	conf := Config{
		Dns: DnsConf{
			Mode:        "ipv4_then_ipv6",
			Nameservers: []string{"223.5.5.5:53", "119.29.29.29:53", "1.1.1.1:53"},
			Timeout:     5,
			CacheSize:   256,
		},
		Network: NetworkConf{
			NoTCP:        false,
			UseUDP:       true,
			TcpKeepalive: 60,
			TcpTimeout:   10,
			UdpTimeout:   30,
		},
		Endpoints: endpoints,
	}

	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 realm 配置失败: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("写入 realm 配置文件失败: %w", err)
	}

	log.Printf("[Realm] 配置更新成功，当前激活 %d 个端点规则", len(endpoints))

	// 如果没有活跃端点，且进程正在运行，则停止进程释放资源
	if len(endpoints) == 0 {
		m.stopProcessLocked()
		return nil
	}

	return m.reloadOrStartLocked()
}

// reloadOrStartLocked 启动或热重载
func (m *Manager) reloadOrStartLocked() error {
	// 检查二进制是否存在
	if _, err := os.Stat(m.binPath); os.IsNotExist(err) {
		// 尝试从系统 PATH 中寻找
		if p, err := exec.LookPath("realm"); err == nil {
			m.binPath = p
		} else {
			log.Printf("[Realm] 警告: 未在本地或系统路径中找到 realm 可执行文件 (%s)", m.binPath)
			return fmt.Errorf("realm 可执行文件未找到，请下载官方 realm 放置于 %s", m.binPath)
		}
	}

	// 若已有正在运行的进程，在 Linux 上发送 SIGHUP 热重载
	if m.cmd != nil && m.cmd.Process != nil {
		if runtime.GOOS == "linux" {
			log.Println("[Realm] 向正在运行的 Realm 进程发送 SIGHUP 热重载信号")
			if err := m.cmd.Process.Signal(syscall.SIGHUP); err == nil {
				return nil
			}
		}
		// 如果 SIGHUP 失败或非 Linux 平台，则先杀死旧进程再重新启动
		m.stopProcessLocked()
	}

	// 启动新进程
	log.Printf("[Realm] 正在启动 Realm: %s -c %s", m.binPath, m.configPath)
	cmd := exec.Command(m.binPath, "-c", m.configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Realm 失败: %w", err)
	}

	m.cmd = cmd
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
		}
		m.mu.Unlock()
		log.Println("[Realm] Realm 进程已退出")
	}()

	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopProcessLocked()
}

func (m *Manager) stopProcessLocked() {
	if m.cmd != nil && m.cmd.Process != nil {
		log.Println("[Realm] 正在终止 Realm 进程...")
		_ = m.cmd.Process.Kill()
		m.cmd = nil
	}
}

func hasHost(addr string) bool {
	for i := 0; i < len(addr); i++ {
		if addr[i] == ':' {
			return true
		}
	}
	return false
}
