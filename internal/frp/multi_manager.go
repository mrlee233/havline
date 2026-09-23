package frp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (s *Service) managerForServer(serverID int64) *Manager {
	s.managersMu.Lock()
	defer s.managersMu.Unlock()
	if manager, ok := s.managers[serverID]; ok {
		return manager
	}
	bin, err := s.manager.BinaryPath()
	if err != nil {
		bin = s.cfg.FrpcBin
	}
	manager := NewManager(
		bin,
		s.serverConfigPath(serverID),
		s.serverLogPath(serverID),
		s.logger,
	)
	s.managers[serverID] = manager
	return manager
}

func (s *Service) serverConfigPath(serverID int64) string {
	return filepath.Join(s.cfg.FrpDir(), fmt.Sprintf("frpc-server-%d.toml", serverID))
}

func (s *Service) serverLogPath(serverID int64) string {
	return filepath.Join(s.cfg.FrpDir(), fmt.Sprintf("frpc-server-%d.log", serverID))
}

func (s *Service) managerSnapshot() map[int64]*Manager {
	s.managersMu.RLock()
	defer s.managersMu.RUnlock()
	result := make(map[int64]*Manager, len(s.managers))
	for id, manager := range s.managers {
		result[id] = manager
	}
	return result
}

func (s *Service) setManagerBinary(path string) {
	s.manager.SetBinary(path)
	for _, manager := range s.managerSnapshot() {
		manager.SetBinary(path)
	}
}

func (s *Service) removeManager(serverID int64) {
	s.managersMu.Lock()
	delete(s.managers, serverID)
	s.managersMu.Unlock()
}

func latestRuntimeStatus(statuses []RuntimeStatus) RuntimeStatus {
	result := RuntimeStatus{Status: "stopped"}
	for _, status := range statuses {
		if status.Status == "running" {
			result.Status = "running"
		}
		if status.Configured {
			result.Configured = true
		}
		result.EnabledProxies += status.EnabledProxies
		if result.PID == 0 && status.PID != 0 {
			result.PID = status.PID
		}
		if status.LastStartedAt > result.LastStartedAt {
			result.LastStartedAt = status.LastStartedAt
		}
		if status.LastReloadedAt > result.LastReloadedAt {
			result.LastReloadedAt = status.LastReloadedAt
		}
		if result.LastError == "" && status.LastError != "" {
			result.LastError = status.LastError
		}
		if status.Version != "" && result.Version == "" {
			result.Version = status.Version
		}
	}
	if len(statuses) == 0 {
		result.Status = "unknown"
	}
	return result
}

func (s *Service) waitForManagers(ctx context.Context, action func(*Manager) error) error {
	var messages []string
	for id, manager := range s.managerSnapshot() {
		if err := action(manager); err != nil {
			messages = append(messages, fmt.Sprintf("服务端 %d：%v", id, err))
		}
	}
	if len(messages) > 0 {
		return fmt.Errorf("%s", strings.Join(messages, "；"))
	}
	return nil
}
