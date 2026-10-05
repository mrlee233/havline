package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr            string
	GatewaySocket         string
	GatewayPrefix         string
	DataDir               string
	SessionSecret         string
	NginxBin              string
	NginxPIDFile          string
	NginxMimeTypes        string
	NginxDefaultHTTPPort  int
	NginxDefaultHTTPSPort int
	FrpcBin               string
	FrpPIDFile            string
	UpdaterSocket         string
	UpdaterToken          string
	UpdaterTokenFile      string
}

func Load() Config {
	dataDir := envOr("HAVLINE_DATA_DIR", defaultDataDir())
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	return Config{
		ListenAddr:            envOr("HAVLINE_LISTEN", ":6893"),
		GatewaySocket:         envOr("HAVLINE_GATEWAY_SOCKET", ""),
		GatewayPrefix:         envOr("HAVLINE_GATEWAY_PREFIX", ""),
		DataDir:               dataDir,
		SessionSecret:         envOr("HAVLINE_SESSION_SECRET", ""),
		NginxBin:              envOr("HAVLINE_NGINX_BIN", "nginx"),
		NginxPIDFile:          envOr("HAVLINE_NGINX_PID", dataDir+"/nginx/nginx.pid"),
		NginxMimeTypes:        envOr("HAVLINE_NGINX_MIME_TYPES", defaultMimeTypes()),
		NginxDefaultHTTPPort:  envIntOr("HAVLINE_NGINX_HTTP_PORT", 80),
		NginxDefaultHTTPSPort: envIntOr("HAVLINE_NGINX_HTTPS_PORT", 443),
		FrpcBin:               envOr("HAVLINE_FRPC_BIN", "frpc"),
		FrpPIDFile:            envOr("HAVLINE_FRP_PID", dataDir+"/frp/frpc.pid"),
		UpdaterSocket:         updaterSocket(),
		UpdaterToken:          envOr("HAVLINE_UPDATER_TOKEN", ""),
		UpdaterTokenFile:      envOr("HAVLINE_UPDATER_TOKEN_FILE", defaultUpdaterTokenFile()),
	}
}

func (c Config) DefaultListenPort(httpsEnabled bool) int {
	if httpsEnabled {
		return c.NginxDefaultHTTPSPort
	}
	return c.NginxDefaultHTTPPort
}

func (c Config) DBPath() string {
	return c.DataDir + "/havline.db"
}

func (c Config) NginxDir() string {
	return c.DataDir + "/nginx"
}

func (c Config) NginxConfigPath() string {
	return c.NginxDir() + "/nginx.conf"
}

func (c Config) NginxCustomDir() string {
	return c.NginxDir() + "/custom"
}

func (c Config) NginxGlobalCustomPath() string {
	return c.NginxCustomDir() + "/global.conf"
}

func (c Config) NginxRulesDir() string {
	return c.NginxDir() + "/rules"
}

func (c Config) NginxRuleCustomPath(ruleID int64) string {
	return filepath.Join(c.NginxRulesDir(), fmt.Sprintf("rule_%d.conf", ruleID))
}

func (c Config) NginxBackupsDir() string {
	return c.NginxDir() + "/backups"
}

func (c Config) NginxGlobalBackupsDir() string {
	return c.NginxBackupsDir() + "/global"
}

func (c Config) NginxRuleBackupsDir(ruleID int64) string {
	return filepath.Join(c.NginxBackupsDir(), "rules", fmt.Sprintf("%d", ruleID))
}

func (c Config) LogsDir() string {
	return c.DataDir + "/logs"
}

func (c Config) CertsDir() string {
	return c.DataDir + "/certs"
}

// SSHKnownHostsPath 是 SSH 主机密钥的 TOFU 记录文件（首次连接记录指纹，之后必须一致）。
func (c Config) SSHKnownHostsPath() string {
	return filepath.Join(c.DataDir, "ssh_known_hosts")
}

func (c Config) ErrorsDir() string {
	return c.NginxDir() + "/errors"
}

func (c Config) ChinaCIDRPath() string {
	return c.NginxDir() + "/china_cidr.conf"
}

func (c Config) ChinaCIDRTempPath() string {
	return c.NginxDir() + "/china_cidr.conf.tmp"
}

func (c Config) FrpDir() string {
	return c.DataDir + "/frp"
}

func (c Config) FrpConfigPath() string {
	return c.FrpDir() + "/frpc.toml"
}

func (c Config) FrpLogPath() string {
	return filepath.Join(c.LogsDir(), "frpc.log")
}

// LegacySessionSecret 是历史版本在未配置 HAVLINE_SESSION_SECRET 时使用的固定值。
// 它同时是 secret box 的加密密钥，所以只提醒、不自动替换：
// 换掉它会让库里已加密的凭据（frp token、通知密码等）再也解不开。
const LegacySessionSecret = "change-me-in-production"

// sessionSecretPath 会话密钥的持久化位置（与数据库同在数据目录，重建容器/升级都不受影响）
func (c Config) sessionSecretPath() string {
	return filepath.Join(c.DataDir, "session_secret")
}

// ResolveSessionSecret 补齐会话密钥并返回它的来源（写进启动日志，便于确认到底用的哪一个）：
//   - 显式配置了（包括历史默认值）→ 原样使用，与既有数据保持兼容；
//   - 没配置 → 读数据目录里的 session_secret；不存在或内容明显无效就生成一个（0600）。
//
// 这样 Docker 与原生部署都不必手工准备密钥，也不会出现「忘配就用固定弱密钥跑在公网」。
func (c *Config) ResolveSessionSecret() (string, error) {
	if secret := strings.TrimSpace(c.SessionSecret); secret != "" {
		if secret == LegacySessionSecret {
			return "环境变量（仍是历史默认值，建议改成随机值，注意更换后需重新登录）", nil
		}
		return "环境变量", nil
	}
	path := c.sessionSecretPath()
	if data, err := os.ReadFile(path); err == nil {
		// 长度不足或带空白的视为损坏（人工改坏、写了一半）：重新生成，而不是带着坏密钥启动
		if secret := strings.TrimSpace(string(data)); len(secret) >= 32 && !strings.ContainsAny(secret, " \t\r\n") {
			c.SessionSecret = secret
			return path, nil
		}
	}
	secret, err := randomSessionSecret()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return "", err
	}
	// 先写临时文件再改名：避免中途失败留下半个密钥
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// WriteFile 的权限会被 umask 干扰，这里再显式收一道（密钥文件不该让同机其他用户读到）
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	c.SessionSecret = secret
	return path, nil
}

// randomSessionSecret 32 字节随机数的十六进制表示（64 字符）
func randomSessionSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成会话密钥失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func defaultDataDir() string {
	if _, err := os.Stat("/data"); err == nil {
		return "/data"
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, ".data")
	}
	return "/data"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultMimeTypes() string {
	candidates := []string{
		"/etc/nginx/mime.types",
		"deploy/nginx/mime.types",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "/etc/nginx/mime.types"
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// 原生应用显式传空值以禁用 Docker 升级；未配置时保留容器默认值。
func updaterSocket() string {
	if value, ok := os.LookupEnv("HAVLINE_UPDATER_SOCKET"); ok {
		return value
	}
	return "/run/havline-updater/updater.sock"
}

func defaultUpdaterTokenFile() string {
	return "/run/havline-updater/updater.token"
}
