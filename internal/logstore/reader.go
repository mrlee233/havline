package logstore

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AccessEntry struct {
	Time          string  `json:"time"`
	Domain        string  `json:"domain"`
	ServerPort    int     `json:"server_port,omitempty"`
	Method        string  `json:"method"`
	Path          string  `json:"path"`
	Status        int     `json:"status"`
	ResponseTime  float64 `json:"response_time"`
	ClientIP      string  `json:"client_ip"`
	Upstream      string  `json:"upstream"`
	RequestLength int64   `json:"request_length,omitempty"`
	BytesSent     int64   `json:"bytes_sent,omitempty"`
}

type SystemEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Module  string `json:"module"`
	Message string `json:"message"`
}

var accessRe = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\d{3})\s+([\d.]+)\s+(\S+)\s+(\S+)$`)
var accessReWithBytes = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\d{3})\s+([\d.]+)\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d+)$`)
var accessReWithPort = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+(\S+)\s+(\S+)\s+(\d{3})\s+([\d.]+)\s+(\S+)\s+(\S+)$`)
var accessReWithPortAndBytes = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\d+)\s+(\S+)\s+(\S+)\s+(\d{3})\s+([\d.]+)\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d+)$`)

func ParseAccess(line string) (AccessEntry, bool) {
	return parseAccess(line)
}

func ReadAccess(path string, limit int, keyword string, status int) ([]AccessEntry, error) {
	lines, err := tailLines(path, limit*4)
	if err != nil {
		return nil, err
	}
	var out []AccessEntry
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		entry, ok := parseAccess(lines[i])
		if !ok {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(entry.Domain+" "+entry.Path), strings.ToLower(keyword)) {
			continue
		}
		if status > 0 && entry.Status != status {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func AccessHostFilter(hosts []string) LineFilter {
	if len(hosts) == 0 {
		return nil
	}
	endpoints := make([]AccessEndpoint, 0, len(hosts))
	for _, host := range hosts {
		endpoints = append(endpoints, AccessEndpoint{Host: host, Port: 0})
	}
	return AccessEndpointFilter(endpoints)
}

type AccessEndpoint struct {
	Host string
	Port int
}

func AccessEndpointFilter(endpoints []AccessEndpoint) LineFilter {
	if len(endpoints) == 0 {
		return nil
	}
	exact := make(map[string]bool, len(endpoints))
	hosts := make(map[string]bool, len(endpoints))
	for _, ep := range endpoints {
		host := normalizeAccessHost(ep.Host)
		if host == "" {
			continue
		}
		hosts[host] = true
		if ep.Port > 0 {
			exact[endpointMatchKey(host, ep.Port)] = true
		}
	}
	if len(hosts) == 0 {
		return nil
	}
	return func(line string) bool {
		entry, ok := parseAccess(line)
		if !ok {
			return false
		}
		host := normalizeAccessHost(entry.Domain)
		if entry.ServerPort > 0 {
			return exact[endpointMatchKey(host, entry.ServerPort)]
		}
		return hosts[host]
	}
}

func parseAccess(line string) (AccessEntry, bool) {
	line = strings.TrimSpace(line)
	if m := accessReWithPortAndBytes.FindStringSubmatch(line); len(m) == 12 {
		status, _ := strconv.Atoi(m[6])
		rt, _ := strconv.ParseFloat(m[7], 64)
		port, _ := strconv.Atoi(m[3])
		reqLen, _ := strconv.ParseInt(m[10], 10, 64)
		bytesSent, _ := strconv.ParseInt(m[11], 10, 64)
		return AccessEntry{
			Time:          m[1],
			Domain:        m[2],
			ServerPort:    port,
			Method:        m[4],
			Path:          m[5],
			Status:        status,
			ResponseTime:  rt,
			ClientIP:      m[8],
			Upstream:      m[9],
			RequestLength: reqLen,
			BytesSent:     bytesSent,
		}, true
	}
	if m := accessReWithPort.FindStringSubmatch(line); len(m) == 10 {
		status, _ := strconv.Atoi(m[6])
		rt, _ := strconv.ParseFloat(m[7], 64)
		port, _ := strconv.Atoi(m[3])
		return AccessEntry{
			Time:         m[1],
			Domain:       m[2],
			ServerPort:   port,
			Method:       m[4],
			Path:         m[5],
			Status:       status,
			ResponseTime: rt,
			ClientIP:     m[8],
			Upstream:     m[9],
		}, true
	}
	if m := accessReWithBytes.FindStringSubmatch(line); len(m) == 11 {
		status, _ := strconv.Atoi(m[5])
		rt, _ := strconv.ParseFloat(m[6], 64)
		reqLen, _ := strconv.ParseInt(m[9], 10, 64)
		bytesSent, _ := strconv.ParseInt(m[10], 10, 64)
		return AccessEntry{
			Time:          m[1],
			Domain:        m[2],
			Method:        m[3],
			Path:          m[4],
			Status:        status,
			ResponseTime:  rt,
			ClientIP:      m[7],
			Upstream:      m[8],
			RequestLength: reqLen,
			BytesSent:     bytesSent,
		}, true
	}
	m := accessRe.FindStringSubmatch(line)
	if len(m) != 9 {
		return AccessEntry{}, false
	}
	status, _ := strconv.Atoi(m[5])
	rt, _ := strconv.ParseFloat(m[6], 64)
	return AccessEntry{
		Time:         m[1],
		Domain:       m[2],
		Method:       m[3],
		Path:         m[4],
		Status:       status,
		ResponseTime: rt,
		ClientIP:     m[7],
		Upstream:     m[8],
	}, true
}

func normalizeAccessHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func endpointMatchKey(host string, port int) string {
	return fmt.Sprintf("%s:%d", normalizeAccessHost(host), port)
}

func ReadError(path string, limit int, keyword string) ([]string, error) {
	lines, err := tailLines(path, limit*2)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

func isNoiseSystemEntry(entry SystemEntry) bool {
	return entry.Module == "HTTP" && strings.EqualFold(entry.Level, "INFO")
}

func ReadSystem(path string, limit int, level string, keyword string) ([]SystemEntry, error) {
	lines, err := tailLines(path, limit*20)
	if err != nil {
		return nil, err
	}
	var out []SystemEntry
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		entry, ok := parseSystem(lines[i])
		if !ok {
			continue
		}
		if isNoiseSystemEntry(entry) {
			continue
		}
		if level != "" && !strings.EqualFold(entry.Level, level) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(entry.Message+" "+entry.Module), strings.ToLower(keyword)) {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseSystem(line string) (SystemEntry, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return SystemEntry{}, false
	}
	entry := SystemEntry{
		Time:    asString(raw["time"]),
		Level:   asString(raw["level"]),
		Message: asString(raw["msg"]),
		Module:  asString(raw["module"]),
	}
	if entry.Time == "" {
		entry.Time = time.Now().UTC().Format(time.RFC3339)
	}
	entry = localizeSystemEntry(entry, raw)
	return entry, entry.Message != ""
}

const hourlyBucketCount = 12

func isDashboardAccess(entry AccessEntry) bool {
	switch entry.Path {
	case "/favicon.ico", "/robots.txt":
		return false
	}
	if strings.HasPrefix(entry.Path, "/havline-errors/") {
		return false
	}
	return true
}

func isSameCalendarDay(t, day time.Time) bool {
	lt := t.In(day.Location())
	dy, dm, dd := day.Date()
	ty, tm, td := lt.Date()
	return ty == dy && tm == dm && td == dd
}

// HourlyAccessCounts returns today's request counts on a fixed 0–24h axis (12 two-hour buckets).
// Future buckets (after the current time) stay zero.
func HourlyAccessCounts(path string) ([]int, []string) {
	return hourlyAccessCountsForDay(path, time.Now())
}

func hourlyAccessCountsForDay(path string, day time.Time) ([]int, []string) {
	now := time.Now()
	currentBucket := now.Hour() / 2
	if !isSameCalendarDay(day, now) {
		currentBucket = hourlyBucketCount - 1
	}
	counts := make([]int, hourlyBucketCount)
	labels := calendarHourlyLabels()

	// 轮转后的历史内容在归档文件里：只读当前文件会让「今日统计」在轮转后突然掉一半。
	for _, line := range accessLinesForDay(path, day) {
		entry, ok := parseAccess(line)
		if !ok || !isDashboardAccess(entry) {
			continue
		}
		t, ok := parseAccessTime(entry.Time)
		if !ok || !isSameCalendarDay(t, day) {
			continue
		}
		bucket := t.In(day.Location()).Hour() / 2
		if bucket < 0 || bucket > currentBucket {
			continue
		}
		counts[bucket]++
	}
	return counts, labels
}

// accessLinesForDay 返回某天的原始日志行：当前文件 + 该日期的归档文件。
// 只挑文件名里带目标日期的归档，避免把几个月的归档都解开。
func accessLinesForDay(path string, day time.Time) []string {
	var out []string
	stamp := day.Format("20060102")
	for _, archive := range archivesForDay(path, stamp) {
		out = append(out, readGzipLines(archive)...)
	}
	out = append(out, readFileLines(path)...)
	return out
}

// archivesForDay 列出与目标日期匹配的归档（access-20261024-1305.log.gz）：
// 文件名里的日期是按轮转时刻取的，所以既可能属于当天，也可能属于前一天跨零点的那一段。
func archivesForDay(path, day string) []string {
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), ".log")
	prefix := base + "-" + day
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, 4)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log.gz") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out
}

func readFileLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func readGzipLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil
	}
	defer gz.Close()
	var lines []string
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func calendarHourlyLabels() []string {
	labels := make([]string, hourlyBucketCount)
	for i := 0; i < hourlyBucketCount; i++ {
		labels[i] = fmt.Sprintf("%02d:00", i*2)
	}
	return labels
}

func parseAccessTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func isLocalToday(t time.Time, now time.Time) bool {
	return isSameCalendarDay(t, now)
}

func CountTodayAccess(path string) (total int, errors int, avgMs float64) {
	return CountAccessForDay(path, time.Now())
}

func CountAccessForDay(path string, day time.Time) (total int, errors int, avgMs float64) {
	var sum float64
	// 与小时曲线一致：轮转后的内容在归档里，只扫当前文件会漏掉当天早些时候的请求
	for _, line := range accessLinesForDay(path, day) {
		entry, ok := parseAccess(line)
		if !ok || !isDashboardAccess(entry) {
			continue
		}
		t, ok := parseAccessTime(entry.Time)
		if !ok || !isSameCalendarDay(t, day) {
			continue
		}
		total++
		sum += entry.ResponseTime
		if entry.Status >= 400 {
			errors++
		}
	}
	if total > 0 {
		avgMs = sum / float64(total) * 1000
	}
	return total, errors, avgMs
}

// tailLines 返回文件末尾的至多 max 行（保持原顺序，旧→新）。
//
// 之所以从文件尾往前读：日志可能上到几百 MB，而调用方只要最后几十行。
// 顺序扫描等于每刷新一次页面就把整个文件读一遍，NAS 这种长期运行的机器上很吃亏。
func tailLines(path string, max int) ([]string, error) {
	if max <= 0 {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	// 从尾部往前读块，攒够 max 个换行就停。64KB 起步，最多扩到 8MB：
	// 单行上限 1MB（与 scanner 的上限一致），保证超长行不会被截断成多条。
	const (
		startChunk = 64 << 10
		maxChunk   = 8 << 20
	)
	chunk := int64(startChunk)
	var tail []byte
	offset := size
	for offset > 0 {
		readSize := chunk
		if readSize > offset {
			readSize = offset
		}
		offset -= readSize
		buf := make([]byte, readSize)
		if _, err := file.ReadAt(buf, offset); err != nil && err != io.EOF {
			return nil, err
		}
		tail = append(buf, tail...)
		// 已攒到足够的行就不必继续往前读（末尾若没有换行，也算一行）
		if strings.Count(string(tail), "\n") > max {
			break
		}
		if chunk < maxChunk {
			chunk *= 2
		}
	}

	text := string(tail)
	if offset > 0 {
		// 从中间开始读时，第一行很可能是半行：丢掉它
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
