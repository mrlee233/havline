package service

import (
	"context"

	"github.com/havline/havline/internal/chinacidr"
	"github.com/havline/havline/internal/nginx"
	"github.com/havline/havline/internal/settings"
)

func (s *ProxyService) loadGenerateOptions(ctx context.Context) nginx.GenerateOptions {
	opts := nginx.GenerateOptions{}
	if s.settings == nil {
		return opts
	}
	raw, err := s.settings.Get(ctx, settings.KeyTrustedProxy)
	if err == nil {
		opts.TrustedProxy = nginx.ParseTrustedProxyJSON(raw)
	}
	blRaw, err := s.settings.Get(ctx, settings.KeyGlobalIPBlacklist)
	if err == nil {
		opts.GlobalIPBlacklist = chinacidr.ParseGlobalIPList(blRaw)
	}
	wlRaw, err := s.settings.Get(ctx, settings.KeyGlobalIPWhitelist)
	if err == nil {
		opts.GlobalIPWhitelist = chinacidr.ParseGlobalIPList(wlRaw)
	}
	return opts
}
