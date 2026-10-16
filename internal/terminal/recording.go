package terminal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// MaxRecordingBytes 单个录制文件的大小上限：超过就停写，
	// 避免 `yes`、无限日志之类的输出把 NAS 磁盘写满。
	MaxRecordingBytes = 32 << 20
	// backingExt 是录制文件扩展名；内容为 asciinema v2 文本格式。
	backingExt = ".cast"
)

// sessionIDPattern 限定会话 ID 形态：录制文件按会话 ID 命名，必须挡住路径穿越。
var sessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{4,64}$`)

// OutputRecorder 记录一次会话的输出；实现需自行保证并发安全。
type OutputRecorder interface {
	Write(p []byte)
	Close()
}

// Recording 是一条录制文件的元信息。
type Recording struct {
	SessionID string `json:"session_id"`
	Size      int64  `json:"size"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
}

// RecordingStore 管理 logs/terminal 下的录制文件：列表、读取与清理。
type RecordingStore struct {
	dir string
}

func NewRecordingStore(dir string) *RecordingStore {
	return &RecordingStore{dir: dir}
}

// Dir 返回录制目录（启动时需要先建好）。
func (s *RecordingStore) Dir() string { return s.dir }

// EnsureDir 建好目录：录制开关打开前也应该能列出空目录。
func (s *RecordingStore) EnsureDir() error {
	return os.MkdirAll(s.dir, 0o755)
}

// Path 返回某个会话的录制文件路径；会话 ID 非法时返回 false，避免路径穿越。
func (s *RecordingStore) Path(sessionID string) (string, bool) {
	if !sessionIDPattern.MatchString(sessionID) {
		return "", false
	}
	return filepath.Join(s.dir, sessionID+backingExt), true
}

// List 按录制结束时间倒序返回全部录制；started_at 取自 cast 文件头。
func (s *RecordingStore) List() ([]Recording, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Recording{}, nil
		}
		return nil, err
	}
	out := make([]Recording, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), backingExt) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), backingExt)
		out = append(out, Recording{
			SessionID: sessionID,
			Size:      info.Size(),
			StartedAt: s.readStartedAt(filepath.Join(s.dir, entry.Name())),
			EndedAt:   info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndedAt > out[j].EndedAt })
	return out, nil
}

// Prune 删除修改时间早于 before 的录制，返回删除条数。
func (s *RecordingStore) Prune(before time.Time) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), backingExt) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(before) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// readStartedAt 读 cast 头里的 timestamp；读不到就用空串。
func (s *RecordingStore) readStartedAt(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	var header struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &header); err != nil || header.Timestamp <= 0 {
		return ""
	}
	return time.Unix(header.Timestamp, 0).UTC().Format(time.RFC3339)
}

// castRecorder 以 asciinema v2 文本格式记录一段会话：
// 第一行是头（版本/宽高/起始时间），之后每行是一条 [相对秒数, "o", 输出]。
type castRecorder struct {
	mu      sync.Mutex
	file    *os.File
	writer  *bufio.Writer
	started time.Time
	written int64
	stopped bool
}

// NewCastRecorder 建好录制文件并写下 cast 头。
func NewCastRecorder(store *RecordingStore, sessionID, title string, cols, rows int, startedAt time.Time) (OutputRecorder, error) {
	path, ok := store.Path(sessionID)
	if !ok {
		return nil, fmt.Errorf("会话 ID 不合法")
	}
	if err := store.EnsureDir(); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	recorder := &castRecorder{file: file, writer: bufio.NewWriter(file), started: startedAt}
	header := map[string]any{
		"version":   2,
		"width":     cols,
		"height":    rows,
		"timestamp": startedAt.Unix(),
		"title":     title,
		"env":       map[string]string{"TERM": "xterm-256color", "SHELL": "/bin/sh"},
	}
	encoded, err := json.Marshal(header)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := recorder.writer.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return nil, err
	}
	return recorder, nil
}

// Write 追加一帧输出；超过大小上限后停止写入，避免把磁盘写满。
func (r *castRecorder) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped || r.file == nil || r.written >= MaxRecordingBytes {
		return
	}
	elapsed := time.Since(r.started).Seconds()
	chunk := strings.ToValidUTF8(string(p), "\uFFFD")
	payload, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	line := fmt.Sprintf("[%.3f, \"o\", %s]\n", elapsed, payload)
	if _, err := r.writer.WriteString(line); err != nil {
		r.stopped = true
		return
	}
	r.written += int64(len(line))
	if r.written >= MaxRecordingBytes {
		r.stopped = true
	}
}

// Close 落盘并关闭文件；可重复调用。
func (r *castRecorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	_ = r.writer.Flush()
	_ = r.file.Close()
	r.file = nil
}
