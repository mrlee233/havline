package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/havline/havline/internal/acme"
	certstore "github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/cloudflared"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/ddns"
	"github.com/havline/havline/internal/logstore"
	"github.com/havline/havline/internal/publicip"
	"github.com/havline/havline/internal/service"
	"github.com/havline/havline/internal/sysinfo"
)

type StatusHandler struct {
	cfg           config.Config
	proxySvc      *service.ProxyService
	ddnsSvc       *ddns.Service
	acmeSvc       *acme.Service
	cloudflareSvc *cloudflared.Service
	startedAt     string
	nginxPIDFile  string
	cpuMu         sync.Mutex
	cpuSample     sysinfo.CPUSample
}

func NewStatusHandler(cfg config.Config, proxySvc *service.ProxyService, ddnsSvc *ddns.Service, acmeSvc *acme.Service, cloudflareSvc *cloudflared.Service, startedAt, nginxPIDFile string) *StatusHandler {
	// 先采一次 CPU 计数器，使首次请求就能算出占用率（否则只能返回 0）
	_, sample := sysinfo.Read(sysinfo.CPUSample{})
	return &StatusHandler{
		cfg:           cfg,
		proxySvc:      proxySvc,
		ddnsSvc:       ddnsSvc,
		acmeSvc:       acmeSvc,
		cloudflareSvc: cloudflareSvc,
		startedAt:     startedAt,
		nginxPIDFile:  nginxPIDFile,
		cpuSample:     sample,
	}
}

type statusResponse struct {
	PublicIPv4           string   `json:"public_ipv4"`
	PublicIPv6           string   `json:"public_ipv6"`
	PublicIPSource       string   `json:"public_ip_source"`
	DDNSStatus           string   `json:"ddns_status"`
	DDNSCount            int      `json:"ddns_count"`
	DDNSLastUpdated      string   `json:"ddns_last_updated,omitempty"`
	CertificateStatus    string   `json:"certificate_status"`
	CertificateDays      int      `json:"certificate_days"`
	CertificateCount     int      `json:"certificate_count"`
	CertificateSummary   string   `json:"certificate_summary,omitempty"`
	ProxyCount           int      `json:"proxy_count"`
	NginxStatus          string   `json:"nginx_status"`
	RequestToday         int      `json:"request_today"`
	RequestYesterday     int      `json:"request_yesterday"`
	RequestTrend         *int     `json:"request_trend,omitempty"`
	RequestsHourly       []int    `json:"requests_hourly"`
	RequestsHourlyLabels []string `json:"requests_hourly_labels"`
	ErrorToday           int      `json:"error_today"`
	AvgResponseMs        float64  `json:"avg_response_ms"`
	CPUPercent           float64  `json:"cpu_percent"`
	MemUsedBytes         uint64   `json:"mem_used_bytes"`
	MemTotalBytes        uint64   `json:"mem_total_bytes"`
	DiskUsedBytes        uint64   `json:"disk_used_bytes"`
	DiskTotalBytes       uint64   `json:"disk_total_bytes"`
	CloudflareCount      int      `json:"cloudflare_count"`
	CloudflareOnline     int      `json:"cloudflare_online"`
	CloudflareStatus     string   `json:"cloudflare_status"`
	CloudflareMessage    string   `json:"cloudflare_message,omitempty"`
	StartedAt            string   `json:"started_at"`
	UptimeSeconds        int64    `json:"uptime_seconds"`
}

func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	rules, err := h.proxySvc.List(r.Context())
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, "读取状态失败")
		return
	}

	nginxStatus := "unknown"
	if _, err := os.Stat(h.nginxPIDFile); err == nil {
		nginxStatus = "running"
	} else if os.IsNotExist(err) {
		nginxStatus = "stopped"
	}

	ddnsStatus, ddnsLastUpdated, ddnsCount := h.ddnsSvc.Summary(r.Context())
	ipv4, ipv6, ipSource := h.resolvePublicIPs(r.Context())

	records, _ := h.acmeSvc.List(r.Context())
	certStatus, certDays, certCount, certSummary := summarizeCertificates(records)

	resources, cpuSample := sysinfo.Read(h.currentCPUSample())
	h.setCPUSample(cpuSample)
	cfSummary := cloudflared.TunnelSummary{Status: "none", Message: "未创建隧道"}
	if h.cloudflareSvc != nil {
		cfSummary = h.cloudflareSvc.Summary(r.Context())
	}

	accessLog := filepath.Join(h.cfg.LogsDir(), "access.log")
	total, errors, avgMs := logstore.CountTodayAccess(accessLog)
	yesterday, _, _ := logstore.CountAccessForDay(accessLog, time.Now().AddDate(0, 0, -1))
	hourly, hourlyLabels := logstore.HourlyAccessCounts(accessLog)

	started, _ := time.Parse(time.RFC3339, h.startedAt)
	uptime := time.Since(started).Seconds()

	writeJSON(w, http.StatusOK, statusResponse{
		PublicIPv4:           ipv4,
		PublicIPv6:           ipv6,
		PublicIPSource:       ipSource,
		DDNSStatus:           ddnsStatus,
		DDNSCount:            ddnsCount,
		DDNSLastUpdated:      ddnsLastUpdated,
		CertificateStatus:    certStatus,
		CertificateDays:      certDays,
		CertificateCount:     certCount,
		CertificateSummary:   certSummary,
		ProxyCount:           len(rules),
		NginxStatus:          nginxStatus,
		RequestToday:         total,
		RequestYesterday:     yesterday,
		RequestTrend:         requestTrendPercent(total, yesterday),
		RequestsHourly:       hourly,
		RequestsHourlyLabels: hourlyLabels,
		ErrorToday:           errors,
		AvgResponseMs:        avgMs,
		CPUPercent:           resources.CPUPercent,
		MemUsedBytes:         resources.MemUsedBytes,
		MemTotalBytes:        resources.MemTotalBytes,
		DiskUsedBytes:        resources.DiskUsedBytes,
		DiskTotalBytes:       resources.DiskTotalBytes,
		CloudflareCount:      cfSummary.Count,
		CloudflareOnline:     cfSummary.Online,
		CloudflareStatus:     cfSummary.Status,
		CloudflareMessage:    cfSummary.Message,
		StartedAt:            h.startedAt,
		UptimeSeconds:        int64(uptime),
	})
}

// currentCPUSample 与 setCPUSample 用锁保护上次采样，/api/status 可能被并发调用
func (h *StatusHandler) currentCPUSample() sysinfo.CPUSample {
	h.cpuMu.Lock()
	defer h.cpuMu.Unlock()
	return h.cpuSample
}

func (h *StatusHandler) setCPUSample(sample sysinfo.CPUSample) {
	h.cpuMu.Lock()
	defer h.cpuMu.Unlock()
	h.cpuSample = sample
}

func (h *StatusHandler) resolvePublicIPs(ctx context.Context) (ipv4, ipv6, source string) {
	if ipv4, ipv6, ok := h.ddnsSvc.PublicIPs(ctx); ok {
		return ipv4, ipv6, "ddns"
	}
	if ipv4, ipv6, ok := h.ddnsSvc.LastKnownPublicIPs(ctx); ok {
		return ipv4, ipv6, "ddns"
	}
	detectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ipv4, ipv6, err := publicip.Detect(detectCtx)
	if err != nil {
		return "", ipv6, "none"
	}
	return ipv4, ipv6, "detect"
}

func summarizeCertificates(records []certstore.Record) (status string, days int, count int, summary string) {
	if len(records) == 0 {
		return "none", 0, 0, ""
	}
	count = len(records)
	status = "ok"
	days = records[0].DaysLeft
	for _, rec := range records {
		if rec.DaysLeft < days {
			days = rec.DaysLeft
		}
		switch {
		case rec.Status == "error" || rec.DaysLeft <= 0:
			status = "error"
		case rec.DaysLeft <= 30 && status != "error":
			status = "warning"
		}
	}
	if count == 1 {
		summary = certificateSummaryName(records[0])
	} else {
		summary = fmt.Sprintf("%d 张证书", count)
	}
	return status, days, count, summary
}

func requestTrendPercent(today, yesterday int) *int {
	if yesterday <= 0 {
		if today <= 0 {
			return nil
		}
		v := 100
		return &v
	}
	v := int(math.Round(float64(today-yesterday) / float64(yesterday) * 100))
	return &v
}

func certificateSummaryName(rec certstore.Record) string {
	for _, domain := range rec.Domains {
		if strings.HasPrefix(domain, "*.") {
			return domain
		}
	}
	if rec.Wildcard {
		return "*." + rec.Domain
	}
	return rec.Domain
}
