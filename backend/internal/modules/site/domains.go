package site

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
)

// ConfigureDomains shares the same providers and verification configuration between API and worker.
func ConfigureDomains(q *db.Queries, s *usecase.Service, c config.SiteConfig, enqueue func(context.Context) error) (*usecase.Domains, error) {
	raw := c.BaseURL
	if raw == "" {
		raw = usecase.DefaultBaseURL
	}
	base, err := usecase.ParseBaseURL(raw)
	if err != nil {
		return nil, err
	}
	ips := []net.IP{}
	for _, rawIP := range strings.Split(c.DomainTargetIPs, ",") {
		if strings.TrimSpace(rawIP) == "" {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(rawIP))
		if ip == nil {
			return nil, fmt.Errorf("site: invalid SITE_DOMAIN_TARGET_IPS")
		}
		ips = append(ips, ip)
	}
	if c.DomainTargetHost != "" {
		normalized, ok := usecase.NormalizeHost(c.DomainTargetHost)
		if !ok || normalized != c.DomainTargetHost {
			return nil, fmt.Errorf("site: invalid SITE_DOMAIN_TARGET_HOST")
		}
	}
	mode := c.DomainProvisioner
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "dokploy" && mode != "traefik_file" {
		return nil, fmt.Errorf("site: invalid SITE_DOMAIN_PROVISIONER")
	}
	if mode == "dokploy" {
		u, err := url.Parse(c.DokployURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return nil, fmt.Errorf("site: invalid DOKPLOY_URL")
		}
		if c.DokployAPIKey == "" || c.DokployComposeID == "" {
			return nil, fmt.Errorf("site: Dokploy credentials and compose ID required")
		}
	}
	if mode == "traefik_file" && (c.DomainTraefikDir == "" || c.AppNetworkName == "") {
		return nil, fmt.Errorf("site: SITE_DOMAIN_TRAEFIK_DIR and APP_NETWORK_NAME required")
	}
	service, port := c.DokploySiteService, c.DokploySitePort
	if service == "" {
		service = "site"
	}
	if port == 0 {
		port = 3000
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("site: invalid DOKPLOY_SITE_PORT")
	}
	providers := map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}}
	if c.DokployURL != "" && c.DokployAPIKey != "" && c.DokployComposeID != "" {
		providers["dokploy"] = &usecase.DokployProvisioner{URL: c.DokployURL, APIKey: c.DokployAPIKey, ComposeID: c.DokployComposeID, Service: service, Port: port}
	}
	if c.DomainTraefikDir != "" && c.AppNetworkName != "" {
		providers["traefik_file"] = &usecase.TraefikFileProvisioner{Dir: c.DomainTraefikDir, Network: c.AppNetworkName}
	}
	d := usecase.NewDomains(q, base, usecase.DomainOptions{TargetHost: c.DomainTargetHost, TargetIPs: ips,
		Provisioner: mode, Providers: providers, Enqueue: enqueue, SyncOrigins: s.SyncDomainOrigins})
	s.SetCustomDomains(d)
	return d, nil
}
