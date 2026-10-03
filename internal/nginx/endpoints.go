package nginx

import (
	"strconv"
	"strings"

	"github.com/havline/havline/internal/proxy"
)

// ParseServerEndpoints 从 Nginx 配置文本里抽取各 server 块的 (server_name, listen 端口) 组合，
// 返回 proxy.Endpoint（与表单规则的端点同一类型），用于手动（custom）规则的监听占用校验。
//
// 采取保守策略：解析不出的写法（include 片段、正则 server_name、listen unix:、变量）一律忽略。
// 该校验只用于保存时提示，宁可漏报也不能因为解析失败而拦住保存。
func ParseServerEndpoints(content string) []proxy.Endpoint {
	var out []proxy.Endpoint
	seen := map[string]bool{}

	var (
		inServer bool
		depth    int
		ports    []int
		names    []string
	)

	flush := func() {
		// 未写 listen 的 server 块按 Nginx 默认监听端口 80 处理
		if len(ports) == 0 && len(names) > 0 {
			ports = []int{80}
		}
		for _, port := range ports {
			for _, name := range names {
				key := name + ":" + strconv.Itoa(port)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, proxy.Endpoint{Hostname: name, Port: port})
			}
		}
		inServer, depth, ports, names = false, 0, nil, nil
	}

	for _, raw := range strings.Split(content, "\n") {
		line := stripInlineComment(raw)
		if line == "" {
			continue
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		if !inServer {
			if opens > 0 && serverBlockOpens(line) {
				inServer, depth = true, opens-closes
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "listen"):
			if port, ok := listenPort(line); ok {
				ports = append(ports, port)
			}
		case strings.HasPrefix(line, "server_name"):
			names = append(names, serverNames(line)...)
		}
		depth += opens - closes
		if depth <= 0 {
			flush()
		}
	}
	return out
}

func serverBlockOpens(line string) bool {
	fields := strings.Fields(line)
	return len(fields) > 0 && fields[0] == "server" && strings.Contains(line, "{")
}

// listenPort 从 listen 指令取出端口号；unix socket 等无法判定的写法返回 false
func listenPort(line string) (int, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	addr := strings.TrimSuffix(fields[1], ";")
	if strings.Contains(addr, "unix:") {
		return 0, false
	}
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		addr = addr[idx+1:]
	}
	port, err := strconv.Atoi(addr)
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

// serverNames 从 server_name 指令取出字面量主机名；正则（~ 开头）与占位符（含 $）跳过
func serverNames(line string) []string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil
	}
	var names []string
	for _, field := range fields[1:] {
		name := strings.ToLower(strings.TrimSuffix(field, ";"))
		if name == "" || name == "_" || strings.HasPrefix(name, "~") || strings.Contains(name, "$") {
			continue
		}
		names = append(names, name)
	}
	return names
}
