package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

type firewallStatus struct {
	Tool     string `json:"tool"`
	Active   bool   `json:"active"`
	PortOpen bool   `json:"port_open"`
	Message  string `json:"message,omitempty"`
}

// firewallStatus 只检测主机防火墙，不修改规则；云安全组无法从 VPS 内部感知。
func (s *Server) firewallStatus(w http.ResponseWriter, r *http.Request) {
	port := 7443
	if raw := strings.TrimSpace(r.URL.Query().Get("port")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 65535 {
			port = parsed
		}
	}
	writeJSON(w, http.StatusOK, detectFirewall(port))
}

// firewallAllow 仅在用户显式确认后调用；只自动处理 ufw 与 firewalld。
func (s *Server) firewallAllow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "端口无效"})
		return
	}
	output, err := allowFirewallPort(req.Port)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output, "status": detectFirewall(req.Port)})
}

func detectFirewall(port int) firewallStatus {
	if path, err := exec.LookPath("ufw"); err == nil {
		if out, err := exec.Command(path, "status").Output(); err == nil {
			active, open := parseUFWStatus(string(out), port)
			if active {
				return firewallStatus{Tool: "ufw", Active: true, PortOpen: open}
			}
		}
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		if out, err := exec.Command(path, "--state").Output(); err == nil && strings.TrimSpace(string(out)) == "running" {
			return firewallStatus{Tool: "firewalld", Active: true, PortOpen: firewalldPortOpen(path, port)}
		}
	}
	if _, err := exec.LookPath("nft"); err == nil {
		return firewallStatus{Tool: "nftables", Active: true, Message: "nftables 需要手工放行，Agent 不会自动修改规则"}
	}
	if _, err := exec.LookPath("iptables"); err == nil {
		return firewallStatus{Tool: "iptables", Active: true, Message: "iptables 需要手工放行，Agent 不会自动修改规则"}
	}
	return firewallStatus{Tool: "none", Message: "未检测到主机防火墙"}
}

// parseUFWStatus 纯解析 ufw status 输出：是否启用、目标端口是否已放行。
func parseUFWStatus(output string, port int) (bool, bool) {
	active := false
	open := false
	needle := strconv.Itoa(port)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Status:") {
			active = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Status:")), "active")
			continue
		}
		if !strings.Contains(line, needle) || !strings.Contains(line, "ALLOW") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && (fields[0] == needle || strings.HasPrefix(fields[0], needle+"/")) {
			open = true
		}
	}
	return active, open
}

func firewalldPortOpen(path string, port int) bool {
	out, err := exec.Command(path, "--list-ports").Output()
	if err != nil {
		return false
	}
	for _, item := range strings.Fields(string(out)) {
		if strings.HasPrefix(item, strconv.Itoa(port)+"/") {
			return true
		}
	}
	return false
}

func allowFirewallPort(port int) (string, error) {
	status := detectFirewall(port)
	switch status.Tool {
	case "ufw":
		out, err := exec.Command("ufw", "allow", strconv.Itoa(port)+"/tcp").CombinedOutput()
		return strings.TrimSpace(string(out)), err
	case "firewalld":
		first, err := exec.Command("firewall-cmd", "--permanent", "--add-port="+strconv.Itoa(port)+"/tcp").CombinedOutput()
		if err != nil {
			return strings.TrimSpace(string(first)), err
		}
		second, err := exec.Command("firewall-cmd", "--reload").CombinedOutput()
		return strings.TrimSpace(string(first) + "\n" + string(second)), err
	case "nftables", "iptables":
		return "", errors.New("当前防火墙需要手工放行，Agent 不会自动修改 nftables / iptables 规则")
	default:
		return "", errors.New("未检测到可自动放行的主机防火墙（仅支持 ufw / firewalld）")
	}
}
