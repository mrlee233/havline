package nginx

import (
	"fmt"
	"os"
	"strings"

	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/nginxtmpl"
	"github.com/havline/havline/internal/proxy"
)

type GenerateOptions struct {
	TrustedProxy             TrustedProxyConfig
	GlobalIPBlacklist        []string
	GlobalIPWhitelist        []string
	ChinaCIDRAvailable       bool
	ChinaCIDRPathOverride    string
	GlobalCustomOverride     string
	GlobalCustomPathOverride string
	RuleCustomOverrides      map[int64]string
}

func writeGlobalAllowList(b *strings.Builder, cidrs []string) {
	merged := proxy.ChinaBypassCIDRs(cidrs)
	if len(merged) == 0 {
		return
	}
	writeGlobalAllowListComment(b)
	for _, cidr := range merged {
		b.WriteString(fmt.Sprintf("    allow %s;\n", cidr))
	}
	b.WriteString("\n")
}

func writeGlobalDenyList(b *strings.Builder, cidrs []string) {
	if len(cidrs) == 0 {
		return
	}
	writeGlobalDenyListComment(b)
	for _, cidr := range cidrs {
		b.WriteString(fmt.Sprintf("    deny %s;\n", cidr))
	}
	b.WriteString("\n")
}

func writeLimitZones(b *strings.Builder, rules []proxy.Rule) {
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Security.RateLimitEnabled() || rule.Security.ConnLimitEnabled() {
			writeLimitZoneComments(b, rule)
		}
		if rule.Security.RateLimitEnabled() {
			zone := fmt.Sprintf("havline_rule_%d_req", rule.ID)
			writeIndentedZone(b, nginxtmpl.RateLimitZone(zone, rule.Security.RateLimit.Rate))
		}
		if rule.Security.ConnLimitEnabled() {
			writeIndentedZone(b, nginxtmpl.ConnLimitZone(fmt.Sprintf("havline_rule_%d_conn", rule.ID)))
		}
	}
	if hasLimitZones(rules) {
		b.WriteString("\n")
	}
}

func writeIndentedZone(b *strings.Builder, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	b.WriteString("    " + line)
}

func hasLimitZones(rules []proxy.Rule) bool {
	for _, rule := range rules {
		if rule.Enabled && (rule.Security.RateLimitEnabled() || rule.Security.ConnLimitEnabled()) {
			return true
		}
	}
	return false
}

func writePrivateIPGeo(b *strings.Builder) {
	b.WriteString(`    geo $havline_client_ip $havline_is_private {
        default 0;
        10.0.0.0/8 1;
        127.0.0.0/8 1;
        172.16.0.0/12 1;
        192.168.0.0/16 1;
        ::1/128 1;
        fc00::/7 1;
    }

`)
}

func writeChinaGeoBlocks(b *strings.Builder, cfg config.Config, opts GenerateOptions, rules []proxy.Rule) {
	needsChina := false
	for _, rule := range rules {
		if rule.Enabled && rule.Security.ChinaOnly {
			needsChina = true
			break
		}
	}
	if !needsChina {
		return
	}
	writeChinaGeoComment(b)
	writePrivateIPGeo(b)
	if !opts.ChinaCIDRAvailable {
		return
	}
	cidrPath := absNginxPath(cfg.ChinaCIDRPath())
	if opts.ChinaCIDRPathOverride != "" {
		cidrPath = absNginxPath(opts.ChinaCIDRPathOverride)
	}
	b.WriteString(fmt.Sprintf(`    geo $havline_client_ip $havline_is_china {
        default 0;
        include %s;
    }

`, cidrPath))
}

func writeChinaOnlyBypassGeo(b *strings.Builder, rules []proxy.Rule, opts GenerateOptions) {
	if !opts.ChinaCIDRAvailable {
		return
	}
	for _, rule := range rules {
		if !rule.Enabled || !rule.Security.ChinaOnly {
			continue
		}
		sec := rule.Security.Normalize()
		cidrs := chinaBypassCIDRs(sec, opts.GlobalIPWhitelist)
		writeChinaBypassComment(b, rule)
		b.WriteString(fmt.Sprintf("    geo $havline_client_ip $havline_rule_%d_china_bypass {\n", rule.ID))
		b.WriteString("        default 0;\n")
		for _, cidr := range cidrs {
			b.WriteString(fmt.Sprintf("        %s 1;\n", cidr))
		}
		b.WriteString("    }\n\n")
	}
}

func chinaOnlyCheckDirectives(rule proxy.Rule, opts GenerateOptions) string {
	sec := rule.Security.Normalize()
	if !sec.ChinaOnly {
		return ""
	}
	if !opts.ChinaCIDRAvailable {
		return `        if ($havline_is_private = 0) {
            return 403;
        }
`
	}
	var b strings.Builder
	b.WriteString(`        set $havline_china_block 0;
        if ($havline_is_china = 0) {
            set $havline_china_block 1;
        }
        if ($havline_is_private = 1) {
            set $havline_china_block 0;
        }
`)
	b.WriteString(fmt.Sprintf(`        if ($havline_rule_%d_china_bypass = 1) {
            set $havline_china_block 0;
        }
`, rule.ID))
	b.WriteString(`        if ($havline_china_block = 1) {
            return 403;
        }
`)
	return b.String()
}

func writeServerSecurityHeaders(b *strings.Builder, sec proxy.SecurityConfig, https bool) {
	if !https || !sec.SecurityHeaders {
		return
	}
	writeSecurityHeadersComment(b)
	b.WriteString(nginxtmpl.ServerSecurityHeaders(nginxtmpl.SecurityHeaders{Enabled: true, HTTPS: true}))
	b.WriteString("\n")
}

func writeErrorPages(b *strings.Builder, cfg config.Config) {
	errorsDir := absNginxPath(cfg.ErrorsDir())
	writeErrorPagesComment(b)
	// HTML is served via internal error_page redirect; images are fetched by the browser
	// as separate requests and must not use the internal-only location.
	b.WriteString(`    error_page 403 /havline-errors/403.html;
    error_page 404 /havline-errors/404.html;
    error_page 429 /havline-errors/429.html;
    error_page 500 /havline-errors/500.html;
    error_page 502 503 /havline-errors/503.html;

    location = /havline-errors/error.png {
        alias ` + errorsDir + `/error.png;
    }
    location = /havline-errors/429.png {
        alias ` + errorsDir + `/429.png;
    }

    location ^~ /havline-errors/ {
        internal;
        alias ` + errorsDir + `/;
    }

`)
}

func writeLocationSecurity(b *strings.Builder, cfg config.Config, rule proxy.Rule, opts GenerateOptions) {
	sec := rule.Security.Normalize()
	writeLocationSecurityComments(b, rule, opts)

	acl := nginxtmpl.AccessControl{
		Deny:      sec.IPBlacklist,
		ChinaDeny: chinaOnlyCheckDirectives(rule, opts),
	}
	if sec.WhitelistEnabled() {
		acl.Allow = sec.IPWhitelist
	}
	b.WriteString(nginxtmpl.AccessDirectives(acl, "        "))

	limits := nginxtmpl.LimitConfig{Zone: fmt.Sprintf("havline_rule_%d", rule.ID)}
	if sec.ConnLimitEnabled() {
		limits.Conn = sec.ConnLimit.Max
	}
	if sec.RateLimitEnabled() {
		limits.Rate = sec.RateLimit.Rate
		limits.Burst = sec.RateLimit.Burst
	}
	b.WriteString(nginxtmpl.LimitDirectives(limits, "        "))

	if sec.BasicAuthEnabled() {
		path := absNginxPath(HtpasswdPath(cfg, rule.ID))
		b.WriteString(fmt.Sprintf(`        auth_basic "Restricted";
        auth_basic_user_file %s;
`, path))
	}
}

func chinaBypassCIDRs(sec proxy.SecurityConfig, globalWhitelist []string) []string {
	extra := make([]string, 0, len(globalWhitelist)+len(sec.IPWhitelist))
	extra = append(extra, globalWhitelist...)
	extra = append(extra, sec.IPWhitelist...)
	return proxy.ChinaBypassCIDRs(extra)
}

func chinaCIDRExists(cfg config.Config) bool {
	info, err := os.Stat(cfg.ChinaCIDRPath())
	return err == nil && !info.IsDir() && info.Size() > 0
}
