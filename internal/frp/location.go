package frp

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const locationCacheTTL = 24 * time.Hour

type locationCache struct {
	value     string
	expiresAt time.Time
}

type ipLocationResponse struct {
	Success bool   `json:"success"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
}

func (s *Service) serverLocation(ctx context.Context, host string) string {
	ip := resolvePublicIP(ctx, host)
	if ip == "" {
		return strings.TrimSpace(host)
	}
	s.locationMu.Lock()
	if cached, ok := s.locations[ip]; ok && time.Now().Before(cached.expiresAt) {
		s.locationMu.Unlock()
		return cached.value
	}
	s.locationMu.Unlock()
	value := lookupIPLocation(ctx, ip)
	// 定位失败只是展示信息缺失：缓存 IP 本身,不产生错误状态,下一次诊断会再次尝试
	s.locationMu.Lock()
	s.locations[ip] = locationCache{value: value, expiresAt: time.Now().Add(locationCacheTTL)}
	s.locationMu.Unlock()
	return value
}

func resolvePublicIP(ctx context.Context, host string) string {
	if ip := net.ParseIP(host); isPublicIP(ip) {
		return ip.String()
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return ""
	}
	for _, address := range addresses {
		if isPublicIP(address.IP) {
			return address.IP.String()
		}
	}
	return ""
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

func lookupIPLocation(ctx context.Context, ip string) string {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/"+ip, nil)
	if err != nil {
		return "未定位"
	}
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "未定位"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "未定位"
	}
	var data ipLocationResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&data); err != nil || !data.Success {
		return "未定位"
	}
	return formatLocation(data)
}

func formatLocation(data ipLocationResponse) string {
	parts := []string{data.City, data.Region, data.Country}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && (len(values) == 0 || values[len(values)-1] != part) {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return "未定位"
	}
	return strings.Join(values, " · ")
}
