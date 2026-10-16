package api

import (
	"sync"
	"time"
)

// terminalUnlockTTL 是一次二次认证的有效期：输一次管理员密码，十分钟内新建会话不再重复输入。
const terminalUnlockTTL = 10 * time.Minute

// terminalUnlockStore 记录「哪个登录会话已通过终端二次认证」。
//
// 只放内存、不落盘：进程重启即全部失效，符合「短时解锁」的定位；
// 键是会话 Cookie 的会话 ID 而不是管理员 ID，换个浏览器/会话就要重新解锁。
type terminalUnlockStore struct {
	mu      sync.Mutex
	expires map[string]time.Time
}

func newTerminalUnlockStore() *terminalUnlockStore {
	return &terminalUnlockStore{expires: make(map[string]time.Time)}
}

// grant 记下这次解锁并返回到期时间。
func (s *terminalUnlockStore) grant(sessionID string) time.Time {
	now := time.Now()
	expires := now.Add(terminalUnlockTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.expires[sessionID] = expires
	return expires
}

// expiresAt 返回该会话的解锁到期时间；未解锁或已过期返回 false。
func (s *terminalUnlockStore) expiresAt(sessionID string) (time.Time, bool) {
	if sessionID == "" {
		return time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.expires[sessionID]
	if !ok {
		return time.Time{}, false
	}
	if time.Now().After(expires) {
		delete(s.expires, sessionID)
		return time.Time{}, false
	}
	return expires, true
}

// revoke 主动锁定：用户点锁定，或改密后让当前会话立即失效。
func (s *terminalUnlockStore) revoke(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.expires, sessionID)
}

// pruneLocked 清理过期条目，避免长期运行后 map 里堆积失效的旧会话（调用方需持锁）。
func (s *terminalUnlockStore) pruneLocked(now time.Time) {
	for sessionID, expires := range s.expires {
		if now.After(expires) {
			delete(s.expires, sessionID)
		}
	}
}
