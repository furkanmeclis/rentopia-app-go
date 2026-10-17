package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
)

// ManualProvisioner leaves certificate issuance to platform administrators.
type ManualProvisioner struct{}

func (ManualProvisioner) Ensure(_ context.Context, r db.SiteDomain) (string, string, error) {
	if r.TlsStatus == "active" {
		return "active", "", nil
	}
	return "manual", "", nil
}
func (ManualProvisioner) Remove(context.Context, db.SiteDomain) error { return nil }
func (ManualProvisioner) Commit(context.Context) error                { return nil }

// DokployProvisioner uses the compose domain API, deploying once at the end of a batch.
type DokployProvisioner struct {
	mu                              sync.Mutex
	dirty                           bool
	URL, APIKey, ComposeID, Service string
	Port                            int
	Client                          *http.Client
}

func (p *DokployProvisioner) request(ctx context.Context, method, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.URL, "/")+"/api/"+path, bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid Dokploy URL")
	}
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("dokploy request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dokploy %s returned HTTP %d", strings.Split(path, "?")[0], resp.StatusCode)
	}
	if out != nil {
		if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
			return errors.New("invalid Dokploy response")
		}
	}
	return nil
}
func (p *DokployProvisioner) find(ctx context.Context, host string) (string, error) {
	var domains []struct {
		DomainID string `json:"domainId"`
		Host     string `json:"host"`
		Service  string `json:"serviceName"`
	}
	if err := p.request(ctx, http.MethodGet, "domain.byComposeId?composeId="+url.QueryEscape(p.ComposeID), nil, &domains); err != nil {
		return "", err
	}
	for _, domain := range domains {
		if domain.Host == host && domain.Service == p.Service {
			return domain.DomainID, nil
		}
	}
	return "", nil
}
func (p *DokployProvisioner) Ensure(ctx context.Context, r db.SiteDomain) (string, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.TlsStatus == "provisioning" || r.TlsStatus == "failed" {
		p.dirty = true
	}
	id := r.ProvisionerID
	if id == "" {
		var err error
		id, err = p.find(ctx, r.Host)
		if err != nil {
			return "", "", err
		}
	}
	if id == "" {
		var out struct {
			DomainID string `json:"domainId"`
		}
		err := p.request(ctx, http.MethodPost, "domain.create", map[string]any{
			"host": r.Host, "composeId": p.ComposeID, "serviceName": p.Service, "port": p.Port,
			"https": true, "certificateType": "letsencrypt", "domainType": "compose", "path": "/",
		}, &out)
		if err != nil {
			return "", "", err
		}
		p.dirty = true
		id = out.DomainID
		if id == "" {
			return "", "", errors.New("dokploy domain.create returned no domainId")
		}
	}
	// Active means the routing configuration was accepted, not a certificate probe.
	return "active", id, nil
}
func (p *DokployProvisioner) Remove(ctx context.Context, r db.SiteDomain) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.TlsStatus == "provisioning" || r.TlsStatus == "failed" {
		p.dirty = true
	}
	// Discover on retries too: an earlier delete may have succeeded before persistence.
	id, err := p.find(ctx, r.Host)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	if err := p.request(ctx, http.MethodPost, "domain.delete", map[string]string{"domainId": id}, nil); err != nil {
		return err
	}
	p.dirty = true
	return nil
}
func (p *DokployProvisioner) Commit(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.dirty {
		return nil
	}
	if err := p.request(ctx, http.MethodPost, "compose.deploy", map[string]string{"composeId": p.ComposeID}, nil); err != nil {
		return err
	}
	p.dirty = false
	return nil
}

// TraefikFileProvisioner writes one dynamic configuration file per host.
type TraefikFileProvisioner struct{ Dir, Network string }

var networkRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func (p *TraefikFileProvisioner) path(host string) (string, error) {
	normalized, ok := NormalizeHost(host)
	if !ok || normalized != host || p.Dir == "" || !networkRE.MatchString(p.Network) {
		return "", errors.New("invalid Traefik domain configuration")
	}
	return filepath.Join(p.Dir, "site-domain-"+host+".yml"), nil
}
func (p *TraefikFileProvisioner) Ensure(_ context.Context, r db.SiteDomain) (string, string, error) {
	path, err := p.path(r.Host)
	if err != nil {
		return "", "", err
	}
	// Host is restricted to DNS characters; network is restricted to Docker name characters.
	digest := sha256.Sum256([]byte(r.Host))
	key := "site-" + hex.EncodeToString(digest[:])
	config := fmt.Sprintf(`http:
  routers:
    %[1]s:
      rule: 'Host(%[2]s)'
      entryPoints: [websecure]
      service: %[1]s
      tls:
        certResolver: letsencrypt
    %[1]s-redirect:
      rule: 'Host(%[2]s)'
      entryPoints: [web]
      middlewares: [%[1]s-https]
      service: %[1]s
  middlewares:
    %[1]s-https:
      redirectScheme:
        scheme: https
        permanent: true
  services:
    %[1]s:
      loadBalancer:
        servers:
          - url: 'http://site.%[3]s:3000'
`, key, "`"+r.Host+"`", p.Network)
	if err = os.MkdirAll(p.Dir, 0750); err != nil {
		return "", "", err
	}
	file, err := os.CreateTemp(p.Dir, ".site-domain-*")
	if err != nil {
		return "", "", err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err = file.WriteString(config); err != nil {
		_ = file.Close()
		return "", "", err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return "", "", err
	}
	if err = file.Close(); err != nil {
		return "", "", err
	}
	if err = os.Chmod(file.Name(), 0644); err != nil {
		return "", "", err
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return "", "", err
	}
	return "active", r.Host, nil
}
func (p *TraefikFileProvisioner) Remove(_ context.Context, r db.SiteDomain) error {
	path, err := p.path(r.Host)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (p *TraefikFileProvisioner) Commit(context.Context) error { return nil }
