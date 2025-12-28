package server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"kiro2api/logger"
)

const sessionCookieName = "kiro_sid"

// Session 会话数据结构
type Session struct {
	ID        string
	User      string
	CreatedAt time.Time
	LastSeen  time.Time
}

// SessionManager 会话管理器
type SessionManager struct {
	mu              sync.RWMutex
	sessions        map[string]Session
	idleTimeout     time.Duration
	absoluteTimeout time.Duration
	stop            chan struct{}
}

// NewSessionManager 创建会话管理器
func NewSessionManager(idleTimeout, absoluteTimeout time.Duration) *SessionManager {
	m := &SessionManager{
		sessions:        make(map[string]Session),
		idleTimeout:     idleTimeout,
		absoluteTimeout: absoluteTimeout,
		stop:            make(chan struct{}),
	}
	go m.cleanupLoop()
	logger.Info("会话管理器已启动",
		logger.String("idle_timeout", idleTimeout.String()),
		logger.String("absolute_timeout", absoluteTimeout.String()))
	return m
}

// CreateSession 创建会话
func (m *SessionManager) CreateSession(user string) (Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return Session{}, err
	}

	now := time.Now()
	s := Session{
		ID:        id,
		User:      user,
		CreatedAt: now,
		LastSeen:  now,
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	logger.Debug("创建会话",
		logger.String("user", user))
	return s, nil
}

// Validate 验证会话是否有效
func (m *SessionManager) Validate(id string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return Session{}, false
	}

	now := time.Now()
	if m.isExpired(s, now) {
		delete(m.sessions, id)
		logger.Debug("会话已过期")
		return Session{}, false
	}

	s.LastSeen = now
	m.sessions[id] = s
	return s, true
}

// Delete 删除会话
func (m *SessionManager) Delete(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	logger.Debug("删除会话")
}

// Close 关闭管理器
func (m *SessionManager) Close() {
	close(m.stop)
}

// Count 获取会话数量
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// cleanupLoop 定期清理过期会话
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupExpired()
		case <-m.stop:
			return
		}
	}
}

// cleanupExpired 清理过期会话
func (m *SessionManager) cleanupExpired() {
	now := time.Now()
	expired := 0

	m.mu.Lock()
	for id, s := range m.sessions {
		if m.isExpired(s, now) {
			delete(m.sessions, id)
			expired++
		}
	}
	m.mu.Unlock()

	if expired > 0 {
		logger.Debug("清理过期会话",
			logger.Int("count", expired))
	}
}

// isExpired 判断会话是否过期
func (m *SessionManager) isExpired(s Session, now time.Time) bool {
	if m.idleTimeout > 0 && now.Sub(s.LastSeen) > m.idleTimeout {
		return true
	}
	if m.absoluteTimeout > 0 && now.Sub(s.CreatedAt) > m.absoluteTimeout {
		return true
	}
	return false
}

// generateSessionID 生成随机会话ID
func generateSessionID() (string, error) {
	b := make([]byte, 32) // 256 bits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
