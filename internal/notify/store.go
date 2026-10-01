package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/havline/havline/internal/secret"
	"github.com/havline/havline/internal/settings"
)

type Store struct {
	settings *settings.Store
	secret   *secret.Box
}

func NewStore(settingsStore *settings.Store, secretBox *secret.Box) *Store {
	return &Store{settings: settingsStore, secret: secretBox}
}

func (s *Store) Load(ctx context.Context) (Config, error) {
	cfg, _, err := s.load(ctx, false)
	return cfg, err
}

func (s *Store) LoadRuntime(ctx context.Context) (RuntimeConfig, error) {
	cfg, runtime, err := s.load(ctx, true)
	if err != nil {
		return RuntimeConfig{}, err
	}
	return cfg.Runtime(runtime.smtpPassword, runtime.webhookSecret, runtime.telegramToken), nil
}

type runtimeSecrets struct {
	smtpPassword  string
	webhookSecret string
	telegramToken string
}

func (s *Store) load(ctx context.Context, decrypt bool) (Config, runtimeSecrets, error) {
	cfg := Config{
		Email:    EmailConfig{Port: 587, TLS: true},
		Webhook:  WebhookConfig{Provider: WebhookBark, Server: DefaultBarkServer},
		Telegram: TelegramConfig{},
	}
	secrets := runtimeSecrets{}

	typeRaw, _ := s.settings.Get(ctx, settings.KeyNotifyType)
	cfg.Type = NotifyType(strings.TrimSpace(typeRaw))

	emailRaw, _ := s.settings.Get(ctx, settings.KeyNotifyEmailJSON)
	if emailRaw != "" {
		_ = json.Unmarshal([]byte(emailRaw), &cfg.Email)
	}
	if cfg.Email.Port <= 0 {
		cfg.Email.Port = 587
	}

	webhookRaw, _ := s.settings.Get(ctx, settings.KeyNotifyWebhookJSON)
	if webhookRaw != "" {
		_ = json.Unmarshal([]byte(webhookRaw), &cfg.Webhook)
	}
	if cfg.Webhook.Server == "" && cfg.Webhook.Provider == WebhookBark {
		cfg.Webhook.Server = DefaultBarkServer
	}

	telegramRaw, _ := s.settings.Get(ctx, settings.KeyNotifyTelegramJSON)
	if telegramRaw != "" {
		_ = json.Unmarshal([]byte(telegramRaw), &cfg.Telegram)
	}

	cfg.IPFrequentThreshold = intSetting(s.settings, ctx, settings.KeyNotifyIPFrequentThreshold, DefaultIPFrequentThreshold)
	cfg.IPFrequentWindowSec = intSetting(s.settings, ctx, settings.KeyNotifyIPFrequentWindowSec, DefaultIPFrequentWindowSec)

	cfg.LoginFailureThreshold = intSetting(s.settings, ctx, settings.KeyNotifyLoginFailureThreshold, DefaultLoginFailureThreshold)
	cfg.LoginFailureWindowSec = intSetting(s.settings, ctx, settings.KeyNotifyLoginFailureWindowSec, DefaultLoginFailureWindowSec)
	cfg.CPUThreshold = intSetting(s.settings, ctx, settings.KeyNotifyCPUThreshold, DefaultCPUThreshold)
	cfg.MemoryThreshold = intSetting(s.settings, ctx, settings.KeyNotifyMemoryThreshold, DefaultMemoryThreshold)
	cfg.DiskThreshold = intSetting(s.settings, ctx, settings.KeyNotifyDiskThreshold, DefaultDiskThreshold)
	cfg.TunnelFailThreshold = intSetting(s.settings, ctx, settings.KeyNotifyTunnelFailThreshold, DefaultTunnelFailThreshold)
	cfg.FrpsFailThreshold = intSetting(s.settings, ctx, settings.KeyNotifyFrpsFailThreshold, DefaultFrpsFailThreshold)
	cfg.AgentFailThreshold = intSetting(s.settings, ctx, settings.KeyNotifyAgentFailThreshold, DefaultAgentFailThreshold)
	cfg.DriftCooldownHours = intSetting(s.settings, ctx, settings.KeyNotifyDriftCooldownHours, DefaultDriftCooldownHours)

	if hasNotifyEventSettings(ctx, s.settings) {
		cfg.OnDDNSIPChange = flag(s.settings, ctx, settings.KeyNotifyOnDDNSIPChange, true)
		cfg.OnDDNSFailure = flag(s.settings, ctx, settings.KeyNotifyOnDDNSFailure, true)
		cfg.OnCertExpiry = flag(s.settings, ctx, settings.KeyNotifyOnCertExpiry, true)
		cfg.OnCertRenewSuccess = flag(s.settings, ctx, settings.KeyNotifyOnCertRenewSuccess, true)
		cfg.OnCertRenewFailure = flag(s.settings, ctx, settings.KeyNotifyOnCertRenewFailure, true)
		cfg.OnIPFrequentAccess = flag(s.settings, ctx, settings.KeyNotifyOnIPFrequentAccess, false)
		cfg.OnLoginFailure = flag(s.settings, ctx, settings.KeyNotifyOnLoginFailure, false)
		cfg.OnNginxReloadFailure = flag(s.settings, ctx, settings.KeyNotifyOnNginxReloadFailure, true)
		// 资源与隧道告警默认关闭：升级后不改变现有用户收到的通知
		cfg.OnResourceThreshold = flag(s.settings, ctx, settings.KeyNotifyOnResourceThreshold, false)
		cfg.OnTunnelDown = flag(s.settings, ctx, settings.KeyNotifyOnTunnelDown, false)
		cfg.OnFrpsDown = flag(s.settings, ctx, settings.KeyNotifyOnFrpsDown, false)
		cfg.OnAgentUnreachable = flag(s.settings, ctx, settings.KeyNotifyOnAgentUnreachable, false)
		cfg.OnRouteDrift = flag(s.settings, ctx, settings.KeyNotifyOnRouteDrift, false)
		// 备份失败属于「你以为有备份其实没有」，默认开启
		cfg.OnBackupFailure = flag(s.settings, ctx, settings.KeyNotifyOnBackupFailure, true)
	} else {
		legacyDDNS := flag(s.settings, ctx, settings.KeyNotifyOnDDNSError, true)
		legacyCert := flag(s.settings, ctx, settings.KeyNotifyOnCertError, true)
		cfg.OnDDNSFailure = legacyDDNS
		cfg.OnCertExpiry = legacyCert
		cfg.OnCertRenewFailure = legacyCert
		cfg.OnDDNSIPChange = true
		cfg.OnCertRenewSuccess = true
		cfg.OnNginxReloadFailure = true
		cfg.OnIPFrequentAccess = false
		cfg.OnLoginFailure = false
	}

	smtpEnc, _ := s.settings.Get(ctx, settings.KeyNotifySMTPPassword)
	if smtpEnc != "" {
		cfg.Email.HasPassword = true
		if decrypt && s.secret != nil {
			plain, err := s.secret.Decrypt(smtpEnc)
			if err != nil {
				// 单个通道的密文损坏（例如更换过加密密钥）不应让整份配置加载失败：
				// 仅标记该通道不可用并继续解析其它通道，避免 Telegram/Webhook 一起失效
				cfg.Email.HasPassword = false
			} else {
				secrets.smtpPassword = plain
			}
		}
	}

	webhookEnc, _ := s.settings.Get(ctx, settings.KeyNotifyWebhookSecret)
	if webhookEnc != "" {
		cfg.Webhook.HasSecret = true
		if decrypt && s.secret != nil {
			plain, err := s.secret.Decrypt(webhookEnc)
			if err != nil {
				// 同上：只让该通道不可用，不影响其它通道
				cfg.Webhook.HasSecret = false
			} else {
				secrets.webhookSecret = plain
			}
		}
	}

	telegramEnc, _ := s.settings.Get(ctx, settings.KeyNotifyTelegramToken)
	if telegramEnc != "" {
		cfg.Telegram.HasBotToken = true
		if decrypt && s.secret != nil {
			plain, err := s.secret.Decrypt(telegramEnc)
			if err != nil {
				// 同上：只让该通道不可用，不影响其它通道
				cfg.Telegram.HasBotToken = false
			} else {
				secrets.telegramToken = plain
			}
		}
	}

	if cfg.Type == "" {
		legacyURL, _ := s.settings.Get(ctx, settings.KeyNotifyWebhookURL)
		if strings.TrimSpace(legacyURL) != "" {
			cfg.Type = NotifyTypeWebhook
			cfg.Webhook.Provider = WebhookCustom
			cfg.Webhook.URL = strings.TrimSpace(legacyURL)
		}
	}

	return cfg, secrets, nil
}

func flag(store *settings.Store, ctx context.Context, key string, fallback bool) bool {
	raw, err := store.Get(ctx, key)
	if err != nil || raw == "" {
		return fallback
	}
	return raw == "1" || strings.EqualFold(raw, "true")
}

func intSetting(store *settings.Store, ctx context.Context, key string, fallback int) int {
	raw, err := store.Get(ctx, key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func hasNotifyEventSettings(ctx context.Context, store *settings.Store) bool {
	keys := []string{
		settings.KeyNotifyOnDDNSIPChange,
		settings.KeyNotifyOnDDNSFailure,
		settings.KeyNotifyOnCertExpiry,
		settings.KeyNotifyOnCertRenewSuccess,
		settings.KeyNotifyOnIPFrequentAccess,
		settings.KeyNotifyOnCertRenewFailure,
		settings.KeyNotifyOnLoginFailure,
		settings.KeyNotifyOnNginxReloadFailure,
		settings.KeyNotifyOnResourceThreshold,
		settings.KeyNotifyOnTunnelDown,
		settings.KeyNotifyOnFrpsDown,
		settings.KeyNotifyOnAgentUnreachable,
		settings.KeyNotifyOnRouteDrift,
		settings.KeyNotifyOnBackupFailure,
	}
	for _, key := range keys {
		raw, err := store.Get(ctx, key)
		if err == nil && raw != "" {
			return true
		}
	}
	return false
}

func (s *Store) Save(ctx context.Context, in SaveInput) error {
	if err := validateSaveInput(in); err != nil {
		return err
	}

	if err := s.settings.Set(ctx, settings.KeyNotifyType, string(in.Type)); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnDDNSIPChange, in.OnDDNSIPChange); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnDDNSFailure, in.OnDDNSFailure); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnCertExpiry, in.OnCertExpiry); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnCertRenewSuccess, in.OnCertRenewSuccess); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnIPFrequentAccess, in.OnIPFrequentAccess); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnCertRenewFailure, in.OnCertRenewFailure); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnLoginFailure, in.OnLoginFailure); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnNginxReloadFailure, in.OnNginxReloadFailure); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnResourceThreshold, in.OnResourceThreshold); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnTunnelDown, in.OnTunnelDown); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyCPUThreshold, clampThreshold(in.CPUThreshold, DefaultCPUThreshold)); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyMemoryThreshold, clampThreshold(in.MemoryThreshold, DefaultMemoryThreshold)); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyDiskThreshold, clampThreshold(in.DiskThreshold, DefaultDiskThreshold)); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyTunnelFailThreshold, clampThreshold(in.TunnelFailThreshold, DefaultTunnelFailThreshold)); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnFrpsDown, in.OnFrpsDown); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnAgentUnreachable, in.OnAgentUnreachable); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnRouteDrift, in.OnRouteDrift); err != nil {
		return err
	}
	if err := s.settings.SetBool(ctx, settings.KeyNotifyOnBackupFailure, in.OnBackupFailure); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyFrpsFailThreshold, clampThreshold(in.FrpsFailThreshold, DefaultFrpsFailThreshold)); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyAgentFailThreshold, clampThreshold(in.AgentFailThreshold, DefaultAgentFailThreshold)); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyDriftCooldownHours, clampThreshold(in.DriftCooldownHours, DefaultDriftCooldownHours)); err != nil {
		return err
	}
	threshold := in.IPFrequentThreshold
	if threshold <= 0 {
		threshold = DefaultIPFrequentThreshold
	}
	windowSec := in.IPFrequentWindowSec
	if windowSec <= 0 {
		windowSec = DefaultIPFrequentWindowSec
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyIPFrequentThreshold, threshold); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyIPFrequentWindowSec, windowSec); err != nil {
		return err
	}
	loginThreshold := in.LoginFailureThreshold
	if loginThreshold <= 0 {
		loginThreshold = DefaultLoginFailureThreshold
	}
	loginWindowSec := in.LoginFailureWindowSec
	if loginWindowSec <= 0 {
		loginWindowSec = DefaultLoginFailureWindowSec
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyLoginFailureThreshold, loginThreshold); err != nil {
		return err
	}
	if err := s.settings.SetInt(ctx, settings.KeyNotifyLoginFailureWindowSec, loginWindowSec); err != nil {
		return err
	}

	email := in.Email
	email.HasPassword = email.HasPassword || strings.TrimSpace(in.SMTPPassword) != "" && in.SMTPPassword != MaskedSecret
	emailJSON, err := json.Marshal(email)
	if err != nil {
		return err
	}
	if err := s.settings.Set(ctx, settings.KeyNotifyEmailJSON, string(emailJSON)); err != nil {
		return err
	}

	webhook := in.Webhook
	secretValue := strings.TrimSpace(in.WebhookSecret)
	if secretValue != "" && secretValue != MaskedSecret {
		webhook.HasSecret = true
	} else if providerUsesKey(webhook.Provider) {
		if strings.TrimSpace(webhook.Key) != "" && webhook.Key != MaskedSecret {
			webhook.HasSecret = true
		}
	}
	webhookJSON, err := json.Marshal(webhook)
	if err != nil {
		return err
	}
	if err := s.settings.Set(ctx, settings.KeyNotifyWebhookJSON, string(webhookJSON)); err != nil {
		return err
	}

	telegram := in.Telegram
	if strings.TrimSpace(in.TelegramToken) != "" && in.TelegramToken != MaskedSecret {
		telegram.HasBotToken = true
	}
	telegramJSON, err := json.Marshal(telegram)
	if err != nil {
		return err
	}
	if err := s.settings.Set(ctx, settings.KeyNotifyTelegramJSON, string(telegramJSON)); err != nil {
		return err
	}

	if in.Type == NotifyTypeWebhook && webhook.Provider == WebhookCustom {
		if err := s.settings.Set(ctx, settings.KeyNotifyWebhookURL, strings.TrimSpace(webhook.URL)); err != nil {
			return err
		}
	}

	if err := s.saveSecret(ctx, settings.KeyNotifySMTPPassword, in.SMTPPassword, in.Email.HasPassword); err != nil {
		return err
	}

	webhookPlain := secretValue
	if webhookPlain == "" || webhookPlain == MaskedSecret {
		if providerUsesKey(webhook.Provider) {
			if strings.TrimSpace(webhook.Key) != "" && webhook.Key != MaskedSecret {
				webhookPlain = strings.TrimSpace(webhook.Key)
			}
		}
	}
	if err := s.saveSecret(ctx, settings.KeyNotifyWebhookSecret, webhookPlain, webhook.HasSecret); err != nil {
		return err
	}

	if err := s.saveSecret(ctx, settings.KeyNotifyTelegramToken, in.TelegramToken, telegram.HasBotToken); err != nil {
		return err
	}

	return nil
}

func (s *Store) saveSecret(ctx context.Context, key, plain string, hasExisting bool) error {
	plain = strings.TrimSpace(plain)
	if plain == "" || plain == MaskedSecret {
		if !hasExisting {
			return nil
		}
		return nil
	}
	if s.secret == nil {
		return fmt.Errorf("加密模块未初始化")
	}
	enc, err := s.secret.Encrypt(plain)
	if err != nil {
		return err
	}
	return s.settings.Set(ctx, key, enc)
}

func validateSaveInput(in SaveInput) error {
	switch in.Type {
	case "":
		return nil
	case NotifyTypeEmail:
		if strings.TrimSpace(in.Email.Host) == "" {
			return fmt.Errorf("请填写 SMTP 主机")
		}
		if len(in.Email.To) == 0 {
			return fmt.Errorf("请填写收件人邮箱")
		}
		password := strings.TrimSpace(in.SMTPPassword)
		if password == "" || password == MaskedSecret {
			if !in.Email.HasPassword {
				return fmt.Errorf("请填写 SMTP 密码")
			}
		}
	case NotifyTypeWebhook:
		switch in.Webhook.Provider {
		case WebhookBark:
			if strings.TrimSpace(in.Webhook.Key) == "" && !in.Webhook.HasSecret {
				return fmt.Errorf("请填写 Bark Device Key")
			}
		case WebhookNtfy:
			if strings.TrimSpace(in.Webhook.Topic) == "" {
				return fmt.Errorf("请填写 ntfy Topic")
			}
		case WebhookGotify:
			if strings.TrimSpace(in.Webhook.Key) == "" && !in.Webhook.HasSecret {
				return fmt.Errorf("请填写 Gotify App Token")
			}
		case WebhookWecom:
			if strings.TrimSpace(in.Webhook.Key) == "" && !in.Webhook.HasSecret {
				return fmt.Errorf("请填写企业微信机器人 Key")
			}
		case WebhookDingtalk:
			if strings.TrimSpace(in.Webhook.Key) == "" && !in.Webhook.HasSecret {
				return fmt.Errorf("请填写钉钉机器人 Access Token")
			}
		case WebhookFeishu:
			if strings.TrimSpace(in.Webhook.Key) == "" && !in.Webhook.HasSecret {
				return fmt.Errorf("请填写飞书机器人 Key")
			}
		case WebhookCustom:
			if strings.TrimSpace(in.Webhook.URL) == "" {
				return fmt.Errorf("请填写 Webhook URL")
			}
		default:
			return fmt.Errorf("不支持的 Webhook 预设")
		}
	case NotifyTypeTelegram:
		token := strings.TrimSpace(in.TelegramToken)
		if token == "" && !in.Telegram.HasBotToken {
			return fmt.Errorf("请填写 Telegram Bot Token")
		}
		if strings.TrimSpace(in.Telegram.ChatID) == "" {
			return fmt.Errorf("请填写 Telegram Chat ID")
		}
	default:
		return fmt.Errorf("不支持的通知类型")
	}
	return nil
}

func SanitizeSettings(values map[string]string) {
	delete(values, settings.KeyNotifySMTPPassword)
	delete(values, settings.KeyNotifyWebhookSecret)
	delete(values, settings.KeyNotifyTelegramToken)
	// ZeroSSL API Key 属证书申请凭据：API 永不回明文，只回掩码。前端以「非空」判断是否已配置，
	// 且只在用户重新输入时才提交该字段，因此掩码不会被当成新密钥写回。
	if values[settings.KeyZeroSSLAPIKey] != "" {
		values[settings.KeyZeroSSLAPIKey] = MaskedSecret
	}
	// LiteSSL 的 EAB HMAC 同理：属于证书申请凭据，只回掩码（EAB Kid 不是秘密，照常回显）
	if values[settings.KeyLiteSSLEABHMAC] != "" {
		values[settings.KeyLiteSSLEABHMAC] = MaskedSecret
	}
}

func ParseSaveInputFromMap(values map[string]string) (SaveInput, bool) {
	if values[settings.KeyNotifyType] == "" &&
		values[settings.KeyNotifyEmailJSON] == "" &&
		values[settings.KeyNotifyWebhookJSON] == "" &&
		values[settings.KeyNotifyTelegramJSON] == "" &&
		values[settings.KeyNotifySMTPPassword] == "" &&
		values[settings.KeyNotifyWebhookSecret] == "" &&
		values[settings.KeyNotifyTelegramToken] == "" {
		return SaveInput{}, false
	}

	in := SaveInput{
		Type:          NotifyType(values[settings.KeyNotifyType]),
		SMTPPassword:  values[settings.KeyNotifySMTPPassword],
		WebhookSecret: values[settings.KeyNotifyWebhookSecret],
		TelegramToken: values[settings.KeyNotifyTelegramToken],
	}
	if raw := values[settings.KeyNotifyEmailJSON]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &in.Email)
	}
	if raw := values[settings.KeyNotifyWebhookJSON]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &in.Webhook)
	}
	if raw := values[settings.KeyNotifyTelegramJSON]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &in.Telegram)
	}
	in.OnDDNSIPChange = values[settings.KeyNotifyOnDDNSIPChange] == "1" || strings.EqualFold(values[settings.KeyNotifyOnDDNSIPChange], "true")
	in.OnDDNSFailure = values[settings.KeyNotifyOnDDNSFailure] == "1" || strings.EqualFold(values[settings.KeyNotifyOnDDNSFailure], "true")
	if !in.OnDDNSFailure {
		in.OnDDNSFailure = values[settings.KeyNotifyOnDDNSError] == "1" || strings.EqualFold(values[settings.KeyNotifyOnDDNSError], "true")
	}
	in.OnCertExpiry = values[settings.KeyNotifyOnCertExpiry] == "1" || strings.EqualFold(values[settings.KeyNotifyOnCertExpiry], "true")
	if !in.OnCertExpiry {
		in.OnCertExpiry = values[settings.KeyNotifyOnCertError] == "1" || strings.EqualFold(values[settings.KeyNotifyOnCertError], "true")
	}
	in.OnCertRenewSuccess = values[settings.KeyNotifyOnCertRenewSuccess] == "1" || strings.EqualFold(values[settings.KeyNotifyOnCertRenewSuccess], "true")
	in.OnIPFrequentAccess = values[settings.KeyNotifyOnIPFrequentAccess] == "1" || strings.EqualFold(values[settings.KeyNotifyOnIPFrequentAccess], "true")
	in.OnCertRenewFailure = values[settings.KeyNotifyOnCertRenewFailure] == "1" || strings.EqualFold(values[settings.KeyNotifyOnCertRenewFailure], "true")
	in.OnLoginFailure = values[settings.KeyNotifyOnLoginFailure] == "1" || strings.EqualFold(values[settings.KeyNotifyOnLoginFailure], "true")
	in.OnNginxReloadFailure = values[settings.KeyNotifyOnNginxReloadFailure] == "1" || strings.EqualFold(values[settings.KeyNotifyOnNginxReloadFailure], "true")
	in.IPFrequentThreshold = intSettingFromMap(values, settings.KeyNotifyIPFrequentThreshold, DefaultIPFrequentThreshold)
	in.IPFrequentWindowSec = intSettingFromMap(values, settings.KeyNotifyIPFrequentWindowSec, DefaultIPFrequentWindowSec)
	in.LoginFailureThreshold = intSettingFromMap(values, settings.KeyNotifyLoginFailureThreshold, DefaultLoginFailureThreshold)
	in.LoginFailureWindowSec = intSettingFromMap(values, settings.KeyNotifyLoginFailureWindowSec, DefaultLoginFailureWindowSec)
	in.OnResourceThreshold = values[settings.KeyNotifyOnResourceThreshold] == "1" || strings.EqualFold(values[settings.KeyNotifyOnResourceThreshold], "true")
	in.OnTunnelDown = values[settings.KeyNotifyOnTunnelDown] == "1" || strings.EqualFold(values[settings.KeyNotifyOnTunnelDown], "true")
	in.CPUThreshold = intSettingFromMap(values, settings.KeyNotifyCPUThreshold, DefaultCPUThreshold)
	in.MemoryThreshold = intSettingFromMap(values, settings.KeyNotifyMemoryThreshold, DefaultMemoryThreshold)
	in.DiskThreshold = intSettingFromMap(values, settings.KeyNotifyDiskThreshold, DefaultDiskThreshold)
	in.TunnelFailThreshold = intSettingFromMap(values, settings.KeyNotifyTunnelFailThreshold, DefaultTunnelFailThreshold)
	in.OnFrpsDown = values[settings.KeyNotifyOnFrpsDown] == "1" || strings.EqualFold(values[settings.KeyNotifyOnFrpsDown], "true")
	in.OnAgentUnreachable = values[settings.KeyNotifyOnAgentUnreachable] == "1" || strings.EqualFold(values[settings.KeyNotifyOnAgentUnreachable], "true")
	in.OnRouteDrift = values[settings.KeyNotifyOnRouteDrift] == "1" || strings.EqualFold(values[settings.KeyNotifyOnRouteDrift], "true")
	in.OnBackupFailure = values[settings.KeyNotifyOnBackupFailure] == "1" || strings.EqualFold(values[settings.KeyNotifyOnBackupFailure], "true")
	in.FrpsFailThreshold = intSettingFromMap(values, settings.KeyNotifyFrpsFailThreshold, DefaultFrpsFailThreshold)
	in.AgentFailThreshold = intSettingFromMap(values, settings.KeyNotifyAgentFailThreshold, DefaultAgentFailThreshold)
	in.DriftCooldownHours = intSettingFromMap(values, settings.KeyNotifyDriftCooldownHours, DefaultDriftCooldownHours)
	return in, true
}

// clampThreshold 把未配置或越界的阈值拉回合法范围：0/负数用默认值，超过 100 的按 100 处理
// （百分比阈值与「连续失败次数」的上限都用这一条）。
func clampThreshold(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	if value > 100 {
		return 100
	}
	return value
}

func intSettingFromMap(values map[string]string, key string, fallback int) int {
	raw := strings.TrimSpace(values[key])
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
