package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"realm-hub/pkg/models"
)

type Database struct {
	Users []models.User `json:"users"`
	Nodes []models.Node `json:"nodes"`
	Rules []models.Rule `json:"rules"`
}

type Store struct {
	mu       sync.RWMutex
	filePath string
	db       Database
}

func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	filePath := filepath.Join(dataDir, "db.json")
	s := &Store{
		filePath: filePath,
		db: Database{
			Users: make([]models.User, 0),
			Nodes: make([]models.Node, 0),
			Rules: make([]models.Rule, 0),
		},
	}

	if _, err := os.Stat(filePath); err == nil {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取数据文件失败: %w", err)
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &s.db); err != nil {
				return nil, fmt.Errorf("解析数据文件失败: %w", err)
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// 初始化空数据库文件
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.filePath)
}

// --- 用户接口 ---

func (s *Store) CountUsers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.db.Users)
}

func (s *Store) GetUser(username string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.db.Users {
		if u.Username == username {
			return &u, true
		}
	}
	return nil, false
}

func (s *Store) CreateUser(username, plainPassword string) (*models.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.db.Users {
		if u.Username == username {
			return nil, fmt.Errorf("用户名已存在")
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := models.User{
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}
	s.db.Users = append(s.db.Users, user)
	return &user, s.saveLocked()
}

// --- 节点接口 ---

func (s *Store) GetNodes() []models.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodes := make([]models.Node, len(s.db.Nodes))
	copy(nodes, s.db.Nodes)
	return nodes
}

func (s *Store) GetNode(id string) (*models.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.db.Nodes {
		if n.ID == id {
			return &n, true
		}
	}
	return nil, false
}

func (s *Store) GetNodeBySecret(secret string) (*models.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.db.Nodes {
		if n.Secret == secret {
			return &n, true
		}
	}
	return nil, false
}

func (s *Store) CreateNode(name string) (*models.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateRandomID("node_")
	secret := generateRandomToken(24)

	node := models.Node{
		ID:        id,
		Name:      name,
		Secret:    secret,
		Online:    false,
		CreatedAt: time.Now(),
	}

	s.db.Nodes = append(s.db.Nodes, node)
	return &node, s.saveLocked()
}

func (s *Store) SaveNode(node models.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, n := range s.db.Nodes {
		if n.ID == node.ID {
			s.db.Nodes[i] = node
			found = true
			break
		}
	}
	if !found {
		s.db.Nodes = append(s.db.Nodes, node)
	}
	return s.saveLocked()
}

func (s *Store) DeleteNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 删除节点
	newNodes := make([]models.Node, 0, len(s.db.Nodes))
	for _, n := range s.db.Nodes {
		if n.ID != id {
			newNodes = append(newNodes, n)
		}
	}
	s.db.Nodes = newNodes

	// 级联删除属于该节点的规则
	newRules := make([]models.Rule, 0, len(s.db.Rules))
	for _, r := range s.db.Rules {
		if r.NodeID != id {
			newRules = append(newRules, r)
		}
	}
	s.db.Rules = newRules

	return s.saveLocked()
}

// --- 规则接口 ---

func (s *Store) GetRules() []models.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]models.Rule, len(s.db.Rules))
	copy(rules, s.db.Rules)
	return rules
}

func (s *Store) GetRulesByNode(nodeID string) []models.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []models.Rule
	for _, r := range s.db.Rules {
		if r.NodeID == nodeID {
			result = append(result, r)
		}
	}
	return result
}

func (s *Store) GetRule(id string) (*models.Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.db.Rules {
		if r.ID == id {
			return &r, true
		}
	}
	return nil, false
}

func (s *Store) SaveRule(rule models.Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	now := time.Now()
	for i, r := range s.db.Rules {
		if r.ID == rule.ID {
			rule.UpdatedAt = now
			s.db.Rules[i] = rule
			found = true
			break
		}
	}
	if !found {
		if rule.ID == "" {
			rule.ID = generateRandomID("rule_")
		}
		rule.CreatedAt = now
		rule.UpdatedAt = now
		s.db.Rules = append(s.db.Rules, rule)
	}

	return s.saveLocked()
}

func (s *Store) DeleteRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	newRules := make([]models.Rule, 0, len(s.db.Rules))
	for _, r := range s.db.Rules {
		if r.ID != id {
			newRules = append(newRules, r)
		}
	}
	s.db.Rules = newRules
	return s.saveLocked()
}

func generateRandomID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func generateRandomToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
