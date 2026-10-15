package sms

import (
	"log/slog"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	platformsms "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/logsms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// Providers builds the SMS providers of the environment: Netgsm always,
// the log provider only when enabled outside production.
func Providers(cfg config.SMSConfig, appEnv string, log *slog.Logger) []platformsms.Provider {
	out := []platformsms.Provider{
		netgsm.New(netgsm.Config{LiveBaseURL: cfg.NetgsmLiveBaseURL, SandboxBaseURL: cfg.NetgsmSandboxBaseURL}),
	}
	if cfg.LogProvider && !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		out = append(out, logsms.New(log))
	}
	return out
}

// Adapters lists providers as integration adapters (account registry).
func Adapters(providers []platformsms.Provider) []integrations.Adapter {
	out := make([]integrations.Adapter, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out
}
