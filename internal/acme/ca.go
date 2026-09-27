package acme

import (
	"fmt"
	"strings"

	"github.com/go-acme/lego/v4/lego"
)

const (
	CALetsEncrypt        = "letsencrypt"
	CALetsEncryptStaging = "letsencrypt-staging"
	CAZeroSSL            = "zerossl"
	CABuypass            = "buypass"
)

// caRetired 记录已停止签发的 CA：仍然认识这些值（历史证书记录里可能存着，用于展示与清理），
// 但不允许再用来申请——否则用户只会看到一个难懂的 ACME 报错而不是「这家 CA 不干了」。
// 否则用户只会看到一个难懂的 ACME 报错。
var caRetired = map[string]string{
	CABuypass: "Buypass 已于 2025-10-15 停止签发证书（ACME 服务 2026-04-15 终止），请改用 Let's Encrypt 或 ZeroSSL",
}

// CAUnavailableReason 返回该 CA 已停用的原因；可用的 CA 返回空字符串。
func CAUnavailableReason(ca string) string {
	return caRetired[NormalizeCA(ca)]
}

const (
	zeroSSLDirectoryURL = "https://acme.zerossl.com/v2/DV90"
	buypassDirectoryURL = "https://api.buypass.com/acme/directory"
)

func NormalizeCA(ca string) string {
	switch ca {
	case "", CALetsEncrypt, "production":
		return CALetsEncrypt
	case CALetsEncryptStaging, "staging", "letsencrypt-test":
		return CALetsEncryptStaging
	case CAZeroSSL:
		return CAZeroSSL
	case CABuypass, "buypass-go":
		return CABuypass
	default:
		return ca
	}
}

func DirectoryURL(ca string) (string, error) {
	switch NormalizeCA(ca) {
	case CALetsEncrypt:
		return lego.LEDirectoryProduction, nil
	case CALetsEncryptStaging:
		return lego.LEDirectoryStaging, nil
	case CAZeroSSL:
		return zeroSSLDirectoryURL, nil
	case CABuypass:
		return buypassDirectoryURL, nil
	default:
		return "", fmt.Errorf("不支持的颁发机构：%s", ca)
	}
}

func CALabel(ca string) string {
	switch NormalizeCA(ca) {
	case CALetsEncrypt:
		return "Let's Encrypt"
	case CALetsEncryptStaging:
		return "Let's Encrypt 测试"
	case CAZeroSSL:
		return "ZeroSSL"
	case CABuypass:
		return "Buypass"
	default:
		if ca == "" {
			return "手动导入"
		}
		return ca
	}
}

func ValidateCA(ca string) error {
	_, err := DirectoryURL(ca)
	return err
}

func ValidateCADomains(ca string, domains []string) error {
	switch NormalizeCA(ca) {
	case CABuypass:
		if len(domains) > 5 {
			return fmt.Errorf("Buypass 单张证书最多支持 5 个域名")
		}
		for _, domain := range domains {
			if strings.HasPrefix(domain, "*.") {
				return fmt.Errorf("Buypass 不支持通配符证书")
			}
		}
	}
	return nil
}
