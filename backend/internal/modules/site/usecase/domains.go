package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrDomainTaken = errors.New("domain already registered")
	ErrDomainLimit = errors.New("maximum three domains per organization")
	ErrDomainMode  = errors.New("operation requires a verified domain using the manual provisioner")
)

// DomainQueries is the persisted verification and reconciliation state.
type DomainQueries interface {
	ReleaseSiteDomainLease(context.Context, db.ReleaseSiteDomainLeaseParams) error
	RetrySiteDomainReconciliation(context.Context, db.RetrySiteDomainReconciliationParams) error
	ListSiteDomainHealthIssues(context.Context, int32) ([]db.SiteDomain, error)
	SiteDomainHealthCounts(context.Context) (db.SiteDomainHealthCountsRow, error)
	CreateSiteDomain(context.Context, db.CreateSiteDomainParams) (db.SiteDomain, error)
	ListSiteDomains(context.Context, int64) ([]db.SiteDomain, error)
	ListPlatformSiteDomains(context.Context, db.ListPlatformSiteDomainsParams) ([]db.SiteDomain, error)
	GetSiteDomain(context.Context, db.GetSiteDomainParams) (db.SiteDomain, error)
	GetPlatformSiteDomain(context.Context, uuid.UUID) (db.SiteDomain, error)
	LookupSiteDomain(context.Context, string) (int64, error)
	RequestSiteDomainCheck(context.Context, db.RequestSiteDomainCheckParams) (int64, error)
	DisableSiteDomain(context.Context, db.DisableSiteDomainParams) (int64, error)
	ClaimDueSiteDomains(context.Context, pgtype.Timestamptz) ([]db.SiteDomain, error)
	UpdateSiteDomainCheck(context.Context, db.UpdateSiteDomainCheckParams) (db.SiteDomain, error)
	UpdateSiteDomainTLS(context.Context, db.UpdateSiteDomainTLSParams) error
	MarkSiteDomainOriginsSynced(context.Context, db.MarkSiteDomainOriginsSyncedParams) error
	DeleteDisabledSiteDomain(context.Context, db.DeleteDisabledSiteDomainParams) error
	MarkSiteDomainTLSActive(context.Context, db.MarkSiteDomainTLSActiveParams) (int64, error)
}

// DNSResolver is implemented by net.Resolver and by deterministic test resolvers.
type DNSResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
	LookupCNAME(context.Context, string) (string, error)
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// DomainProvisioner stages routing changes. Commit applies one deployment per batch.
// Ensure and Remove must be idempotent across worker retries.
type DomainProvisioner interface {
	Ensure(context.Context, db.SiteDomain) (string, string, error)
	Remove(context.Context, db.SiteDomain) error
	Commit(context.Context) error
}

type DomainOptions struct {
	TargetHost  string
	TargetIPs   []net.IP
	Provisioner string
	Resolver    DNSResolver
	Providers   map[string]DomainProvisioner
	SyncOrigins func(context.Context, int64) error
	Enqueue     func(context.Context) error
	Now         func() time.Time
}

type Domains struct {
	q    DomainQueries
	base BaseURL
	opts DomainOptions
}

func NewDomains(q DomainQueries, base BaseURL, opts DomainOptions) *Domains {
	if opts.TargetHost == "" {
		opts.TargetHost = base.host
	}
	opts.TargetHost = strings.TrimSuffix(strings.ToLower(opts.TargetHost), ".")
	if opts.Provisioner == "" {
		opts.Provisioner = "manual"
	}
	if opts.Resolver == nil {
		opts.Resolver = net.DefaultResolver
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Domains{q: q, base: base, opts: opts}
}

// ValidateHost refuses platform names, local names and malformed DNS labels.
func (d *Domains) ValidateHost(raw string) (string, error) {
	host, ok := NormalizeHost(raw)
	invalid := !ok || strings.Contains(raw, ":") || net.ParseIP(host) != nil || !strings.Contains(host, ".")
	invalid = invalid || len("_rentopia-verify."+host) > 253
	invalid = invalid || host == strings.TrimSuffix(d.base.host, ".") || strings.HasSuffix(host, "."+strings.TrimSuffix(d.base.host, "."))
	for _, suffix := range []string{"localhost", "local", "internal", "test", "invalid", "example", "home.arpa"} {
		invalid = invalid || host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	for _, label := range strings.Split(host, ".") {
		invalid = invalid || len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-")
	}
	// Bare platform service names are reserved; fully qualified external hosts may use labels such as www.
	invalid = invalid || slices.Contains(model.ReservedSubdomains, host)
	if invalid {
		ve := &ValidationError{}
		ve.add("host", "invalid", "use a public DNS name outside the platform domain")
		return "", ve
	}
	return host, nil
}

func (d *Domains) view(r db.SiteDomain) model.Domain {
	records := []model.DNSRecord{{Type: "TXT", Name: "_rentopia-verify." + r.Host, Value: r.VerificationToken}}
	if len(d.opts.TargetIPs) == 0 {
		records = append(records, model.DNSRecord{Type: "CNAME", Name: r.Host, Value: d.opts.TargetHost})
	} else {
		for _, ip := range d.opts.TargetIPs {
			kind := "AAAA"
			if ip.To4() != nil {
				kind = "A"
			}
			records = append(records, model.DNSRecord{Type: kind, Name: r.Host, Value: ip.String()})
		}
	}
	out := model.Domain{UUID: r.Uuid, OrganizationID: r.OrganizationID, Host: r.Host, Status: r.Status,
		VerificationToken: r.VerificationToken, LastCheckedAt: domainTime(r.LastCheckedAt), VerifiedAt: domainTime(r.VerifiedAt),
		LastError: r.LastError, TLSStatus: r.TlsStatus, Provisioner: r.Provisioner, CreatedAt: r.CreatedAt.Time,
		UpdatedAt: r.UpdatedAt.Time, DNSRecords: records}
	if r.Provisioner == "manual" {
		out.TLSInstructions = "Add " + r.Host + " in Dokploy: compose service site, port 3000, HTTPS, Let's Encrypt; then mark TLS active. Remove its route when disabled or failed."
	}
	return out
}

func domainTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}
func domainTS(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func (d *Domains) Create(ctx context.Context, orgID, userID int64, raw string) (model.Domain, error) {
	host, err := d.ValidateHost(raw)
	if err != nil {
		return model.Domain{}, err
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return model.Domain{}, fmt.Errorf("domain token: %w", err)
	}
	r, err := d.q.CreateSiteDomain(ctx, db.CreateSiteDomainParams{OrganizationID: orgID, Host: host,
		Now: domainTS(d.opts.Now()), VerificationToken: hex.EncodeToString(token), Provisioner: d.opts.Provisioner, CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0}})
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if pe.Code == "23505" {
			return model.Domain{}, ErrDomainTaken
		}
		if pe.ConstraintName == "site_domains_limit" {
			return model.Domain{}, ErrDomainLimit
		}
	}
	if err != nil {
		return model.Domain{}, err
	}
	// The durable next_check_at also schedules the check when Redis is unavailable.
	if d.opts.Enqueue != nil {
		if err := d.opts.Enqueue(ctx); err != nil {
			slog.WarnContext(ctx, "site_domain_enqueue_failed", "domain_uuid", r.Uuid, "error", err)
		}
	}
	return d.view(r), nil
}

func (d *Domains) List(ctx context.Context, orgID int64) ([]model.Domain, error) {
	rows, err := d.q.ListSiteDomains(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return d.views(rows), nil
}
func (d *Domains) ListPlatform(ctx context.Context, limit, offset int32) ([]model.Domain, error) {
	rows, err := d.q.ListPlatformSiteDomains(ctx, db.ListPlatformSiteDomainsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	return d.views(rows), nil
}
func (d *Domains) views(rows []db.SiteDomain) []model.Domain {
	out := make([]model.Domain, 0, len(rows))
	for _, r := range rows {
		out = append(out, d.view(r))
	}
	return out
}
func (d *Domains) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Domain, error) {
	var r db.SiteDomain
	var err error
	if orgID == 0 {
		r, err = d.q.GetPlatformSiteDomain(ctx, id)
	} else {
		r, err = d.q.GetSiteDomain(ctx, db.GetSiteDomainParams{OrganizationID: orgID, Uuid: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Domain{}, ErrNotFound
	}
	if err != nil {
		return model.Domain{}, err
	}
	return d.view(r), nil
}
func (d *Domains) Recheck(ctx context.Context, orgID int64, id uuid.UUID) (model.Domain, error) {
	out, err := d.Get(ctx, orgID, id)
	if err != nil {
		return out, err
	}
	if out.Status == "disabled" {
		return model.Domain{}, ErrNotFound
	}
	if _, err = d.q.RequestSiteDomainCheck(ctx, db.RequestSiteDomainCheckParams{Uuid: id, OrganizationID: out.OrganizationID, NextCheckAt: domainTS(d.opts.Now())}); err != nil {
		return out, err
	}
	if d.opts.Enqueue != nil {
		if err = d.opts.Enqueue(ctx); err != nil {
			return out, err
		}
	}
	return out, nil
}
func (d *Domains) Remove(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := d.q.DisableSiteDomain(ctx, db.DisableSiteDomainParams{OrganizationID: orgID, Uuid: id, NextCheckAt: domainTS(d.opts.Now())})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	// Keep the disabled row until routing cleanup and origin sync both succeed.
	if d.opts.SyncOrigins != nil {
		if err := d.opts.SyncOrigins(ctx, orgID); err != nil {
			return err
		}
	}
	if d.opts.Enqueue != nil {
		return d.opts.Enqueue(ctx)
	}
	return nil
}
func (d *Domains) MarkTLSActive(ctx context.Context, id uuid.UUID) (model.Domain, error) {
	out, err := d.Get(ctx, 0, id)
	if err != nil {
		return model.Domain{}, err
	}
	n, err := d.q.MarkSiteDomainTLSActive(ctx, db.MarkSiteDomainTLSActiveParams{Uuid: id, OrganizationID: out.OrganizationID})
	if err != nil {
		return model.Domain{}, err
	}
	if n == 0 {
		return model.Domain{}, ErrDomainMode
	}
	return d.Get(ctx, 0, id)
}
func (d *Domains) Lookup(ctx context.Context, host string) (int64, error) {
	h, ok := NormalizeHost(host)
	if !ok {
		return 0, ErrNotFound
	}
	id, err := d.q.LookupSiteDomain(ctx, h)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}
func (d *Domains) Hosts(ctx context.Context, orgID int64) ([]model.Host, error) {
	rows, err := d.q.ListSiteDomains(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := []model.Host{}
	for _, r := range rows {
		if r.Status == "verified" {
			out = append(out, model.Host{Host: r.Host, Origin: "https://" + r.Host, Kind: model.HostCustom})
		}
	}
	return out, nil
}

// CheckDNS requires both ownership proof and routing to the configured platform.
func (d *Domains) CheckDNS(ctx context.Context, r db.SiteDomain) error {
	txt, err := d.opts.Resolver.LookupTXT(ctx, "_rentopia-verify."+r.Host)
	if err != nil {
		return errors.New("verification TXT lookup failed")
	}
	if !slices.Contains(txt, r.VerificationToken) {
		return errors.New("verification TXT token mismatch")
	}
	// Even a matching CNAME must resolve to an address (a dangling alias cannot verify).
	addresses, err := d.opts.Resolver.LookupIPAddr(ctx, r.Host)
	if err != nil || len(addresses) == 0 {
		return errors.New("host has no A or AAAA address")
	}
	cname, err := d.opts.Resolver.LookupCNAME(ctx, r.Host)
	if err == nil && strings.EqualFold(strings.TrimSuffix(cname, "."), d.opts.TargetHost) {
		return nil
	}
	targets := slices.Clone(d.opts.TargetIPs)
	resolved, err := d.opts.Resolver.LookupIPAddr(ctx, d.opts.TargetHost)
	if err == nil {
		for _, ip := range resolved {
			targets = append(targets, ip.IP)
		}
	}
	// Every returned address must belong to the platform, preventing mixed routing.
	for _, address := range addresses {
		if !slices.ContainsFunc(targets, func(ip net.IP) bool { return ip.Equal(address.IP) }) {
			return errors.New("host address does not match the platform target")
		}
	}
	if len(targets) == 0 {
		return errors.New("platform target has no addresses")
	}
	return nil
}

func (d *Domains) check(ctx context.Context, r db.SiteDomain) (db.SiteDomain, error) {
	if r.Status != "pending" && r.Status != "verified" && r.Status != "failed" {
		return r, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	now := d.opts.Now()
	status, lastError := "verified", ""
	verified := r.VerifiedAt
	if !r.VerifiedAt.Valid && !now.Before(r.CreatedAt.Time.Add(72*time.Hour)) {
		status, lastError = "failed", "verification expired after 72 hours; remove and register again"
	} else if err := d.CheckDNS(ctx, r); err != nil {
		lastError = err.Error()
		status = "pending"
		if r.Status != "pending" {
			status = "failed"
		}
	} else if !verified.Valid {
		verified = domainTS(now)
	}

	delay := 24 * time.Hour
	if status == "pending" {
		delay = min(time.Minute*time.Duration(1<<min(r.Attempts, 10)), 6*time.Hour, r.CreatedAt.Time.Add(72*time.Hour).Sub(now))
	}
	return d.q.UpdateSiteDomainCheck(ctx, db.UpdateSiteDomainCheckParams{ID: r.ID, OrganizationID: r.OrganizationID, Status: r.Status, Status_2: status,
		LastCheckedAt: domainTS(now), VerifiedAt: verified, LastError: lastError, NextCheckAt: domainTS(now.Add(delay))})
}

// Sweep processes leased rows, syncing origins before making routing changes.
func (d *Domains) Sweep(ctx context.Context) (int, error) {
	rows, err := d.q.ClaimDueSiteDomains(ctx, domainTS(d.opts.Now()))
	if err != nil {
		return 0, err
	}
	done := []db.SiteDomain{}
	failures := []error{}
	providers := map[string]DomainProvisioner{}
	for _, r := range rows {
		// Failed rows are retried only for reconciliation, or on an explicit recheck.
		checked, err := d.check(ctx, r)
		if err != nil {
			failures = append(failures, domainSweepError(r, "check DNS and persist verification", err))
			continue
		}
		r = checked
		if d.opts.SyncOrigins != nil {
			if err = d.opts.SyncOrigins(ctx, r.OrganizationID); err != nil {
				failures = append(failures, domainSweepError(r, "sync origins", err))
				continue
			}
		}
		if err = d.q.MarkSiteDomainOriginsSynced(ctx, db.MarkSiteDomainOriginsSyncedParams{ID: r.ID, OrganizationID: r.OrganizationID, Status: r.Status}); err != nil {
			failures = append(failures, domainSweepError(r, "mark origins synced", err))
			continue
		}
		p := d.opts.Providers[r.Provisioner]
		if p == nil {
			persistErr := d.q.UpdateSiteDomainTLS(ctx, db.UpdateSiteDomainTLSParams{ID: r.ID, OrganizationID: r.OrganizationID,
				TlsStatus: "failed", ProvisionerID: r.ProvisionerID, LastError: "domain provisioner is not configured"})
			failures = append(failures, domainSweepError(r, "select provisioner", fmt.Errorf("domain provisioner %s unavailable", r.Provisioner)),
				domainSweepError(r, "persist missing provisioner", persistErr))
			continue
		}
		var state, providerID string
		if r.Status == "verified" {
			state, providerID, err = p.Ensure(ctx, r)
		} else {
			state = "pending"
			err = p.Remove(ctx, r)
		}
		if err != nil {
			persistErr := d.q.UpdateSiteDomainTLS(ctx, db.UpdateSiteDomainTLSParams{ID: r.ID, OrganizationID: r.OrganizationID, TlsStatus: "failed", ProvisionerID: r.ProvisionerID, LastError: "routing provisioning failed; check worker logs"})
			failures = append(failures, domainSweepError(r, "provision routing", err), domainSweepError(r, "persist provisioning failure", persistErr))
			continue
		}
		r.TlsStatus, r.ProvisionerID = state, providerID
		// Persist external IDs before deploying; a retry can reuse them after a crash.
		if err = d.q.UpdateSiteDomainTLS(ctx, db.UpdateSiteDomainTLSParams{ID: r.ID, OrganizationID: r.OrganizationID, TlsStatus: "provisioning", ProvisionerID: providerID, LastError: r.LastError}); err != nil {
			failures = append(failures, domainSweepError(r, "persist provisioning", err))
			continue
		}
		providers[r.Provisioner] = p
		done = append(done, r)
	}
	for mode, p := range providers {
		err := p.Commit(ctx)
		if err != nil {
			failures = append(failures, fmt.Errorf("domain provisioner %s: commit batch: %w", mode, err))
			for _, r := range done {
				if r.Provisioner == mode {
					persistErr := d.q.UpdateSiteDomainTLS(ctx, db.UpdateSiteDomainTLSParams{
						ID: r.ID, OrganizationID: r.OrganizationID, TlsStatus: "failed", ProvisionerID: r.ProvisionerID,
						LastError: "routing deployment failed; check worker logs",
					})
					failures = append(failures, domainSweepError(r, "persist deployment failure", persistErr))
				}
			}
			continue
		}
		for _, r := range done {
			if r.Provisioner == mode {
				if err := d.q.UpdateSiteDomainTLS(ctx, db.UpdateSiteDomainTLSParams{ID: r.ID, OrganizationID: r.OrganizationID, TlsStatus: r.TlsStatus, ProvisionerID: r.ProvisionerID, LastError: r.LastError}); err != nil {
					failures = append(failures, domainSweepError(r, "persist TLS status", err))
					continue
				}
				if r.Status == "disabled" {
					failures = append(failures, domainSweepError(r, "delete disabled domain", d.q.DeleteDisabledSiteDomain(ctx, db.DeleteDisabledSiteDomainParams{ID: r.ID, OrganizationID: r.OrganizationID})))
				}
			}
		}
	}
	joined := errors.Join(failures...)
	if joined != nil {
		for _, r := range rows {
			if err := d.q.RetrySiteDomainReconciliation(ctx, db.RetrySiteDomainReconciliationParams{
				OrganizationID: r.OrganizationID, ID: r.ID, NextCheckAt: domainTS(d.opts.Now().Add(time.Minute)),
			}); err != nil {
				joined = errors.Join(joined, domainSweepError(r, "schedule reconciliation retry", err))
			}
		}
	}
	for _, r := range rows {
		if err := d.q.ReleaseSiteDomainLease(ctx, db.ReleaseSiteDomainLeaseParams{OrganizationID: r.OrganizationID, ID: r.ID}); err != nil {
			joined = errors.Join(joined, domainSweepError(r, "release lease", err))
		}
	}
	return len(rows), joined
}

func domainSweepError(r db.SiteDomain, stage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("site domain %s (%s): %s: %w", r.Uuid, r.Host, stage, err)
}

// SyncDomainOrigins reuses site key synchronization without changing the draft version.
func (s *Service) SyncDomainOrigins(ctx context.Context, orgID int64) error {
	row, err := s.q.GetSiteSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := s.q.ListSiteDomains(ctx, orgID)
	if err != nil {
		return err
	}
	previous := []string{s.base.Origin(row.Subdomain)}
	for _, r := range rows {
		previous = append(previous, "https://"+r.Host)
	}
	hosts, err := s.siteHosts(ctx, orgID, row.Subdomain)
	if err != nil {
		return err
	}
	next := origins(hosts)
	keys, err := s.q.ListSiteDomainKeys(ctx, orgID)
	if err != nil {
		return err
	}
	for _, id := range keys {
		key, err := s.keys.GetKey(ctx, orgID, id)
		if err != nil {
			return err
		}
		if key.Status != "active" {
			continue
		}
		merged := []string{}
		for _, origin := range key.AllowedOrigins {
			if !slices.Contains(previous, origin) && !slices.Contains(next, origin) {
				merged = append(merged, origin)
			}
		}
		merged = append(merged, next...)
		if sameSet(merged, key.AllowedOrigins) {
			continue
		}
		if _, err = s.keys.PatchKey(ctx, orgID, id, pamodel.KeyInput{AllowedOrigins: &merged}); err != nil {
			return err
		}
	}
	return nil
}
