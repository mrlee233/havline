package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client 通过共享 Unix Socket 调用 havline-updater sidecar。
const tokenHeader = "X-Havline-Updater-Token"

type Client struct {
	socketPath string
	token      string
	http       *http.Client
}

func NewClient(socketPath, token string) *Client {
	socketPath = strings.TrimSpace(socketPath)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		socketPath: socketPath,
		token:      strings.TrimSpace(token),
		http:       &http.Client{Transport: transport, Timeout: 15 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	if c == nil || c.socketPath == "" {
		return false
	}
	info, err := os.Stat(c.socketPath)
	return err == nil && !info.IsDir()
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var status Status
	if err := c.request(ctx, http.MethodGet, "/status", nil, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (c *Client) Apply(ctx context.Context) (Status, error) {
	var status Status
	if err := c.request(ctx, http.MethodPost, "/apply", map[string]string{}, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (c *Client) request(ctx context.Context, method, path string, payload any, result any) error {
	if !c.Enabled() {
		return fmt.Errorf("一键升级服务未启用")
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set(tokenHeader, c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("updater 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("updater 响应格式无效: %w", err)
	}
	return nil
}
