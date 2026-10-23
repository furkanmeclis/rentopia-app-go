package sms

import (
	"log/slog"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/envguard"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	platformsms "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/logsms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// Providers builds the SMS providers of the environment: Netgsm always,
// the log provider only when enabled outside production. Outside
// production Netgsm's official live endpoint is refused (TEC-744) unless
// NETGSM_ALLOW_PRODUCTION_IN_NON_PROD is set.
func Providers(cfg config.SMSConfig, appEnv string, log *slog.Logger) []platformsms.Provider {
	refusal := envguard.Refusal(appEnv, "netgsm", envguard.AllowNetgsmKey, cfg.NetgsmAllowProduction)
	out := []platformsms.Provider{
		netgsm.New(netgsm.Config{LiveBaseURL: cfg.NetgsmLiveBaseURL, SandboxBaseURL: cfg.NetgsmSandboxBaseURL, RefuseOfficial: refusal}),
	}
	if cfg.LogProvider && !envguard.IsProduction(appEnv) {
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
