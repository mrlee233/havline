package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type zeroSSLEABResponse struct {
	Success    int    `json:"success"`
	EABKid     string `json:"eab_kid"`
	EABHmacKey string `json:"eab_hmac_key"`
	Error      struct {
		Code int    `json:"code"`
		Type string `json:"type"`
	} `json:"error"`
}

// zeroSSLEABEndpoint 单独成变量，测试可以替换成本地 httptest 服务。
var zeroSSLEABEndpoint = "https://api.zerossl.com/acme/eab-credentials"

func zerosslEABCredentials(ctx context.Context, apiKey string) (kid, hmac string, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", "", fmt.Errorf("请在设置中配置 ZeroSSL API Key")
	}

	endpoint := zeroSSLEABEndpoint + "?access_key=" + url.QueryEscape(apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// 不能把 url.Error 整个包进错误：它的字符串里是带 access_key 的完整 URL，
		// 而这条错误会进任务日志与 <数据目录>/logs/app.log，等于把密钥写进日志。
		return "", "", fmt.Errorf("请求 ZeroSSL API 失败：%s", redactURLError(err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	var payload zeroSSLEABResponse
	parseErr := json.Unmarshal(body, &payload)
	if resp.StatusCode != http.StatusOK {
		// 422 基本都是 access_key 无效（也可能是账号/权限问题）。ZeroSSL 会在
		// error.type 与 error.code 里写清原因，所以带上它，而不是只回状态码让用户猜。
		if parseErr == nil && payload.Error.Type != "" {
			return "", "", fmt.Errorf("ZeroSSL API 返回 %d：%s（错误码 %d）", resp.StatusCode, payload.Error.Type, payload.Error.Code)
		}
		return "", "", fmt.Errorf("ZeroSSL API 返回 %d：%s", resp.StatusCode, snippet(body))
	}
	if parseErr != nil {
		return "", "", fmt.Errorf("解析 ZeroSSL 响应失败：%w", parseErr)
	}
	if payload.Success != 1 || payload.EABKid == "" || payload.EABHmacKey == "" {
		if payload.Error.Type != "" {
			return "", "", fmt.Errorf("ZeroSSL API 错误：%s（错误码 %d）", payload.Error.Type, payload.Error.Code)
		}
		return "", "", fmt.Errorf("ZeroSSL API 未返回有效的 EAB 凭据")
	}
	return payload.EABKid, payload.EABHmacKey, nil
}

// redactURLError 只保留底层错误原因，避免把带 access_key 的 URL 写进日志。
func redactURLError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

// snippet 压平并截断响应正文：既不把整页 HTML 灌进日志，也不让换行破坏排版。
func snippet(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return "（响应体为空）"
	}
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return text
}
