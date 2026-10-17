package site_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type domainDNS struct {
	txt   []string
	cname string
	ips   map[string][]net.IPAddr
	err   error
}

func (f *domainDNS) LookupTXT(context.Context, string) ([]string, error) { return f.txt, f.err }
func (f *domainDNS) LookupCNAME(context.Context, string) (string, error) { return f.cname, f.err }
func (f *domainDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return f.ips[host], f.err
}
func dnsFor(host, token string) *domainDNS {
	return &domainDNS{txt: []string{token}, cname: "edge.rentopia.com.", ips: map[string][]net.IPAddr{
		host: {{IP: net.ParseIP("203.0.113.10")}}, "edge.rentopia.com": {{IP: net.ParseIP("203.0.113.10")}},
	}}
}
func domainService(q usecase.DomainQueries, opts usecase.DomainOptions) *usecase.Domains {
	base, _ := usecase.ParseBaseURL("https://sites.rentopia.com")
	opts.TargetHost = "edge.rentopia.com"
	return usecase.NewDomains(q, base, opts)
}

// Memory persistence exercises state transitions without Redis, live DNS or a database.
type domainMemory struct {
	usecase.DomainQueries
	rows    map[uuid.UUID]db.SiteDomain
	syncErr error
}

func (m *domainMemory) ListSiteDomains(_ context.Context, orgID int64) ([]db.SiteDomain, error) {
	out := []db.SiteDomain{}
	for _, r := range m.rows {
		if r.OrganizationID == orgID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *domainMemory) LookupSiteDomain(_ context.Context, host string) (int64, error) {
	for _, r := range m.rows {
		if r.Host == host && r.Status == "verified" {
			return r.OrganizationID, nil
		}
	}
	return 0, pgx.ErrNoRows
}
func (m *domainMemory) GetSiteDomain(_ context.Context, p db.GetSiteDomainParams) (db.SiteDomain, error) {
	r, ok := m.rows[p.Uuid]
	if !ok || r.OrganizationID != p.OrganizationID {
		return db.SiteDomain{}, pgx.ErrNoRows
	}
	return r, nil
}
func (m *domainMemory) GetPlatformSiteDomain(_ context.Context, id uuid.UUID) (db.SiteDomain, error) {
	r, ok := m.rows[id]
	if !ok {
		return db.SiteDomain{}, pgx.ErrNoRows
	}
	return r, nil
}
func (m *domainMemory) ClaimDueSiteDomains(_ context.Context, now pgtype.Timestamptz) ([]db.SiteDomain, error) {
	rows := []db.SiteDomain{}
	for _, r := range m.rows {
		eligible := r.Status == "pending" || r.Status == "verified" || r.Status == "disabled" || r.OriginsDirty || r.TlsStatus == "failed" || r.TlsStatus == "provisioning"
		if eligible && !r.NextCheckAt.Time.After(now.Time) && (!r.LeaseUntil.Valid || !r.LeaseUntil.Time.After(now.Time)) {
			rows = append(rows, r)
			r.LeaseUntil = stamp(now.Time.Add(10 * time.Minute))
			m.rows[r.Uuid] = r
		}
	}
	return rows, nil
}
func (m *domainMemory) UpdateSiteDomainCheck(_ context.Context, p db.UpdateSiteDomainCheckParams) (db.SiteDomain, error) {
	for id, r := range m.rows {
		if r.ID == p.ID && r.OrganizationID == p.OrganizationID && r.Status == p.Status {
			r.Status, r.LastCheckedAt, r.VerifiedAt, r.LastError, r.NextCheckAt = p.Status_2, p.LastCheckedAt, p.VerifiedAt, p.LastError, p.NextCheckAt
			r.Attempts++
			r.OriginsDirty = true
			m.rows[id] = r
			return r, nil
		}
	}
	return db.SiteDomain{}, pgx.ErrNoRows
}
func (m *domainMemory) UpdateSiteDomainTLS(_ context.Context, p db.UpdateSiteDomainTLSParams) error {
	for id, r := range m.rows {
		if r.ID == p.ID && r.OrganizationID == p.OrganizationID {
			r.TlsStatus, r.ProvisionerID, r.LastError = p.TlsStatus, p.ProvisionerID, p.LastError
			m.rows[id] = r
		}
	}
	return nil
}
func (m *domainMemory) MarkSiteDomainOriginsSynced(_ context.Context, p db.MarkSiteDomainOriginsSyncedParams) error {
	if m.syncErr != nil {
		return m.syncErr
	}
	for id, r := range m.rows {
		if r.ID == p.ID && r.OrganizationID == p.OrganizationID && r.Status == p.Status {
			r.OriginsDirty = false
			m.rows[id] = r
		}
	}
	return nil
}
func (m *domainMemory) DeleteDisabledSiteDomain(_ context.Context, p db.DeleteDisabledSiteDomainParams) error {
	for id, r := range m.rows {
		if r.ID == p.ID && r.OrganizationID == p.OrganizationID && r.Status == "disabled" && !r.OriginsDirty {
			delete(m.rows, id)
		}
	}
	return nil
}
func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func pendingDomain(now time.Time) db.SiteDomain {
	return db.SiteDomain{ID: 1, Uuid: uuid.New(), OrganizationID: 7, Host: "rent.customer.com", Status: "pending", VerificationToken: "proof",
		Provisioner: "manual", TlsStatus: "pending", CreatedAt: stamp(now), NextCheckAt: stamp(now), OriginsDirty: true}
}

func TestDomainDNS(t *testing.T) {
	row := pendingDomain(time.Now())
	cases := []struct {
		name   string
		change func(*domainDNS)
		valid  bool
	}{
		{"matching cname", func(*domainDNS) {}, true},
		{"matching A", func(f *domainDNS) { f.cname = row.Host }, true},
		{"dns failure", func(f *domainDNS) { f.err = errors.New("no records") }, false},
		{"token mismatch", func(f *domainDNS) { f.txt = []string{"another token"} }, false},
		{"wrong target", func(f *domainDNS) {
			f.cname = "elsewhere.com"
			f.ips[row.Host] = []net.IPAddr{{IP: net.ParseIP("203.0.113.99")}}
		}, false},
		{"dangling cname", func(f *domainDNS) { f.ips[row.Host] = nil }, false},
		{"mixed addresses", func(f *domainDNS) {
			f.cname = row.Host
			f.ips[row.Host] = append(f.ips[row.Host], net.IPAddr{IP: net.ParseIP("203.0.113.99")})
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := dnsFor(row.Host, row.VerificationToken)
			tc.change(resolver)
			err := domainService(nil, usecase.DomainOptions{Resolver: resolver}).CheckDNS(context.Background(), row)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
	t.Run("configured IPv6 target", func(t *testing.T) {
		f := dnsFor(row.Host, row.VerificationToken)
		f.cname = row.Host
		f.ips[row.Host] = []net.IPAddr{{IP: net.ParseIP("2001:db8::1")}}
		d := domainService(nil, usecase.DomainOptions{Resolver: f, TargetIPs: []net.IP{net.ParseIP("2001:db8::1")}})
		if err := d.CheckDNS(context.Background(), row); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDomainVerificationLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	row := pendingDomain(now)
	m := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{row.Uuid: row}}
	resolver := dnsFor(row.Host, row.VerificationToken)
	origins := []string{}
	var d *usecase.Domains
	d = domainService(m, usecase.DomainOptions{Resolver: resolver, Now: func() time.Time { return now }, Providers: map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}},
		SyncOrigins: func(ctx context.Context, orgID int64) error {
			hosts, err := d.Hosts(ctx, orgID)
			origins = []string{}
			for _, h := range hosts {
				origins = append(origins, h.Origin)
			}
			return err
		}})
	if _, err := d.Lookup(ctx, row.Host); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("pending lookup: %v", err)
	}
	if _, err := d.Get(ctx, 8, row.Uuid); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	if _, err := d.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	verified := m.rows[row.Uuid]
	if verified.Status != "verified" || verified.TlsStatus != "manual" || verified.VerifiedAt.Time != now {
		t.Fatalf("verified: %+v", verified)
	}
	if id, err := d.Lookup(ctx, "RENT.CUSTOMER.COM:443"); err != nil || id != 7 {
		t.Fatalf("lookup: %d %v", id, err)
	}
	if len(origins) != 1 || origins[0] != "https://"+row.Host {
		t.Fatalf("origins: %v", origins)
	}
	now = now.Add(24 * time.Hour)
	resolver.txt = nil
	if _, err := d.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if m.rows[row.Uuid].Status != "failed" {
		t.Fatal("verified DNS loss did not fail")
	}
	if _, err := d.Lookup(ctx, row.Host); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("failed lookup: %v", err)
	}
	if len(origins) != 0 {
		t.Fatalf("failed origin retained: %v", origins)
	}
}

func TestDomainExpiryAndBackoff(t *testing.T) {
	for _, goodDNS := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing DNS", true: "late DNS"}[goodDNS], func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC()
			row := pendingDomain(now)
			f := dnsFor(row.Host, row.VerificationToken)
			if !goodDNS {
				f.txt = nil
			}
			m := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{row.Uuid: row}}
			d := domainService(m, usecase.DomainOptions{Resolver: f, Now: func() time.Time { return now }, Providers: map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}}})
			if !goodDNS {
				if _, err := d.Sweep(ctx); err != nil {
					t.Fatal(err)
				}
				r := m.rows[row.Uuid]
				if r.Status != "pending" || r.NextCheckAt.Time.Sub(now) != time.Minute {
					t.Fatalf("backoff: %+v", r)
				}
			}
			now = now.Add(72 * time.Hour)
			if _, err := d.Sweep(ctx); err != nil {
				t.Fatal(err)
			}
			if m.rows[row.Uuid].Status != "failed" {
				t.Fatal("domain not expired")
			}
		})
	}
}

func TestDomainHostsValidation(t *testing.T) {
	d := domainService(nil, usecase.DomainOptions{})
	for _, host := range []string{"sites.rentopia.com", "rent.sites.rentopia.com", "a.b.sites.rentopia.com", "127.0.0.1", "::1", "localhost", "foo.localhost", "foo.local", "api", "www", "api.sites.rentopia.com", "-bad.com", "bad-.com", "bad..com", "https://customer.com", "customer.com:443", "*.customer.com", strings.Repeat("a", 64) + ".com", "x.home.arpa"} {
		if _, err := d.ValidateHost(host); err == nil {
			t.Errorf("accepted %q", host)
		}
	}
	if _, err := d.ValidateHost("www.customer.com"); err != nil {
		t.Fatalf("external www host: %v", err)
	}
	if got, err := d.ValidateHost("Rent.Customer.COM."); err != nil || got != "rent.customer.com" {
		t.Fatalf("normalization: %s %v", got, err)
	}
}

func TestInternalDomainAllowed(t *testing.T) {
	ctx := context.Background()
	row := pendingDomain(time.Now())
	m := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{row.Uuid: row}}
	d := domainService(m, usecase.DomainOptions{})
	h := handler.New(nil, nil, nil)
	h.SetDomains(d, "shared-key")
	mux := http.NewServeMux()
	site.RegisterRoutes(mux, h, nil, nil, stubResolver{})
	for _, tc := range []struct {
		status, secret string
		want           int
	}{
		{"pending", "shared-key", 404}, {"failed", "shared-key", 404}, {"disabled", "shared-key", 404}, {"verified", "", 401}, {"verified", "wrong", 401}, {"verified", "shared-key", 200},
	} {
		row.Status = tc.status
		m.rows[row.Uuid] = row
		req := httptest.NewRequest(http.MethodGet, "/v1/internal/site/domain-allowed?host="+row.Host, nil).WithContext(ctx)
		req.Header.Set("X-Auth-Adapter-Key", tc.secret)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
	h.SetDomains(d, "")
	w := httptest.NewRecorder()
	h.DomainAllowed(w, httptest.NewRequest(http.MethodGet, "/?host="+row.Host, nil))
	if w.Code != 401 {
		t.Fatal("empty secret must deny")
	}
}

func TestManualDomainProvisioner(t *testing.T) {
	p := usecase.ManualProvisioner{}
	ctx := context.Background()
	row := pendingDomain(time.Now())
	state, _, err := p.Ensure(ctx, row)
	if err != nil || state != "manual" {
		t.Fatalf("%s %v", state, err)
	}
	row.TlsStatus = "active"
	state, _, err = p.Ensure(ctx, row)
	if err != nil || state != "active" {
		t.Fatalf("%s %v", state, err)
	}
	if err := p.Remove(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := p.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTraefikDomainProvisioner(t *testing.T) {
	p := &usecase.TraefikFileProvisioner{Dir: t.TempDir(), Network: "rentopia-app"}
	row := pendingDomain(time.Now())
	ctx := context.Background()
	state, _, err := p.Ensure(ctx, row)
	if err != nil || state != "active" {
		t.Fatalf("%s %v", state, err)
	}
	file := filepath.Join(p.Dir, "site-domain-"+row.Host+".yml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"Host(`" + row.Host + "`)", "websecure", "certResolver: letsencrypt", "redirectScheme:", "http://site.rentopia-app:3000"} {
		if !strings.Contains(string(data), part) {
			t.Errorf("missing %s", part)
		}
	}
	if _, _, err = p.Ensure(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err = p.Remove(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err = p.Remove(ctx, row); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("file not removed")
	}
	row.Host = "../../escape"
	if _, _, err = p.Ensure(ctx, row); err == nil {
		t.Fatal("accepted path traversal")
	}
}

func TestDokployDomainProvisioner(t *testing.T) {
	domains := map[string]string{}
	creates, deletes, deploys := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "private-key" {
			t.Error("missing api key")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/domain.byComposeId":
			if r.URL.Query().Get("composeId") != "compose-1" {
				t.Error("wrong compose")
			}
			out := []map[string]string{}
			for host, id := range domains {
				out = append(out, map[string]string{"domainId": id, "host": host, "serviceName": "site"})
			}
			_ = json.NewEncoder(w).Encode(out)
		case "/api/domain.create":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["certificateType"] != "letsencrypt" || body["composeId"] != "compose-1" || body["serviceName"] != "site" || body["port"] != float64(3000) || body["https"] != true {
				t.Errorf("create body %v", body)
			}
			creates++
			id := uuid.NewString()
			domains[body["host"].(string)] = id
			_ = json.NewEncoder(w).Encode(map[string]string{"domainId": id})
		case "/api/domain.delete":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			for host, id := range domains {
				if body["domainId"] == id {
					delete(domains, host)
				}
			}
			deletes++
			_, _ = w.Write([]byte("{}"))
		case "/api/compose.deploy":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["composeId"] != "compose-1" {
				t.Error("wrong deploy")
			}
			deploys++
			_, _ = w.Write([]byte("{}"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p := &usecase.DokployProvisioner{URL: server.URL, APIKey: "private-key", ComposeID: "compose-1", Service: "site", Port: 3000}
	ctx := context.Background()
	a := pendingDomain(time.Now())
	b := a
	b.Host = "other.customer.com"
	_, id, err := p.Ensure(ctx, a)
	if err != nil || id == "" {
		t.Fatalf("ensure: %s %v", id, err)
	}
	// A crash before persisting the id must discover the previously created route.
	_, same, err := p.Ensure(ctx, a)
	if err != nil || same != id || creates != 1 {
		t.Fatalf("retry: %s %v creates=%d", same, err, creates)
	}
	if _, _, err = p.Ensure(ctx, b); err != nil {
		t.Fatal(err)
	}
	if deploys != 0 {
		t.Fatal("deployed before commit")
	}
	if err = p.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if deploys != 1 {
		t.Fatal("expected one deploy for two changes")
	}
	a.ProvisionerID = id
	if err = p.Remove(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err = p.Remove(ctx, a); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatal("delete not idempotent")
	}
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer failed.Close()
	p.URL = failed.URL
	if _, _, err = p.Ensure(ctx, b); err == nil {
		t.Fatal("accepted failed API")
	}
	p = &usecase.DokployProvisioner{URL: failed.URL, APIKey: "private-key", ComposeID: "compose-1", Service: "site", Port: 3000}
	b.TlsStatus = "provisioning"
	b.ProvisionerID = "existing"
	_, _, _ = p.Ensure(ctx, b)
	if err = p.Commit(ctx); err == nil {
		t.Fatal("accepted failed deploy")
	}
}

// The database integration checks the real hook, allowed_origins, global uniqueness and tenant isolation.
func TestCustomDomainsIntegration(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()
	base, err := usecase.ParseBaseURL("https://sites.test")
	e.must(err)
	resolver := dnsFor("rent.customer.com", "")
	d := usecase.NewDomains(e.q, base, usecase.DomainOptions{TargetHost: "edge.rentopia.com", Resolver: resolver,
		Providers: map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}}, SyncOrigins: e.svc.SyncDomainOrigins, Now: func() time.Time { return now }})
	e.svc.SetCustomDomains(d)
	on := true
	settings, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: &on})
	e.must(err)
	_, _, err = e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(settings.Version)})
	e.must(err)
	host := "rent-" + uuid.NewString() + ".customer.com"
	domain, err := d.Create(ctx, e.a.id, 0, strings.ToUpper(host)+".")
	e.must(err)
	resolver.ips[host] = resolver.ips["rent.customer.com"]
	resolver.txt = []string{domain.VerificationToken}
	if _, err = e.svc.Resolve(ctx, host, ""); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("pending resolve: %v", err)
	}
	if _, err = d.Get(ctx, e.b.id, domain.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	if err = d.Remove(ctx, e.b.id, domain.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-org delete: %v", err)
	}
	if _, err = d.Recheck(ctx, e.b.id, domain.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-org verify: %v", err)
	}
	if _, err = d.Create(ctx, e.b.id, 0, host); !errors.Is(err, usecase.ErrDomainTaken) {
		t.Fatalf("global duplicate: %v", err)
	}
	for i := 0; i < 2; i++ {
		_, err = d.Create(ctx, e.a.id, 0, "other-"+uuid.NewString()+".customer.com")
		e.must(err)
	}
	if _, err = d.Create(ctx, e.a.id, 0, "fourth-"+uuid.NewString()+".customer.com"); !errors.Is(err, usecase.ErrDomainLimit) {
		t.Fatalf("limit: %v", err)
	}
	checked, err := d.Sweep(ctx)
	e.must(err)
	if checked < 3 {
		t.Fatalf("initial sweep claimed %d domains, want at least 3", checked)
	}
	stored, err := e.q.GetSiteDomain(ctx, db.GetSiteDomainParams{OrganizationID: e.a.id, Uuid: domain.UUID})
	e.must(err)
	if stored.Status != "verified" || !stored.CreatedAt.Time.Equal(now.Truncate(time.Microsecond)) {
		t.Fatalf("created domain was not verified at the injected clock: %+v", stored)
	}
	resolved, err := e.svc.Resolve(ctx, host, "")
	e.must(err)
	if resolved.Organization.Slug != e.a.slug {
		t.Fatal("wrong organization resolved")
	}
	key, err := e.keys.GetKey(ctx, e.a.id, settings.APIKey.UUID)
	e.must(err)
	if !containsOrigin(key.AllowedOrigins, "https://"+host) {
		t.Fatalf("origins %v", key.AllowedOrigins)
	}
	rotated, err := e.keys.RotateKey(ctx, e.a.id, settings.APIKey.UUID, pamodel.RotateInput{})
	e.must(err)
	now = now.Add(24 * time.Hour)
	resolver.txt = nil
	_, err = d.Sweep(ctx)
	e.must(err)
	if _, err = e.svc.Resolve(ctx, host, ""); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("failed resolve: %v", err)
	}
	key, err = e.keys.GetKey(ctx, e.a.id, settings.APIKey.UUID)
	e.must(err)
	if containsOrigin(key.AllowedOrigins, "https://"+host) {
		t.Fatalf("failed origin retained %v", key.AllowedOrigins)
	}
	successor, err := e.keys.GetKey(ctx, e.a.id, rotated.Key.UUID)
	e.must(err)
	if containsOrigin(successor.AllowedOrigins, "https://"+host) {
		t.Fatalf("rotated origin retained %v", successor.AllowedOrigins)
	}

}
func containsOrigin(origins []string, origin string) bool {
	for _, v := range origins {
		if v == origin {
			return true
		}
	}
	return false
}

func (m *domainMemory) RetrySiteDomainReconciliation(_ context.Context, p db.RetrySiteDomainReconciliationParams) error {
	for id, r := range m.rows {
		if r.ID == p.ID && r.OrganizationID == p.OrganizationID {
			r.NextCheckAt = p.NextCheckAt
			m.rows[id] = r
		}
	}
	return nil
}

func (m *domainMemory) ReleaseSiteDomainLease(_ context.Context, p db.ReleaseSiteDomainLeaseParams) error {
	for id, r := range m.rows {
		if r.ID == p.ID && r.OrganizationID == p.OrganizationID {
			r.LeaseUntil = pgtype.Timestamptz{}
			m.rows[id] = r
		}
	}
	return nil
}

func (m *domainMemory) DisableSiteDomain(_ context.Context, p db.DisableSiteDomainParams) (int64, error) {
	r, ok := m.rows[p.Uuid]
	if !ok || r.OrganizationID != p.OrganizationID {
		return 0, nil
	}
	r.Status = "disabled"
	r.OriginsDirty = true
	r.NextCheckAt = p.NextCheckAt
	m.rows[p.Uuid] = r
	return 1, nil
}
func (m *domainMemory) RequestSiteDomainCheck(_ context.Context, p db.RequestSiteDomainCheckParams) (int64, error) {
	r, ok := m.rows[p.Uuid]
	if !ok || r.OrganizationID != p.OrganizationID || r.Status == "disabled" {
		return 0, nil
	}
	r.OriginsDirty = true
	r.NextCheckAt = p.NextCheckAt
	m.rows[p.Uuid] = r
	return 1, nil
}
func (m *domainMemory) MarkSiteDomainTLSActive(_ context.Context, p db.MarkSiteDomainTLSActiveParams) (int64, error) {
	r, ok := m.rows[p.Uuid]
	if !ok || r.OrganizationID != p.OrganizationID || r.Status != "verified" || r.Provisioner != "manual" {
		return 0, nil
	}
	r.TlsStatus = "active"
	m.rows[p.Uuid] = r
	return 1, nil
}

type stagingProvisioner struct {
	commits, removes int
	failCommit       bool
}

func (p *stagingProvisioner) Ensure(context.Context, db.SiteDomain) (string, string, error) {
	return "active", "external-id", nil
}
func (p *stagingProvisioner) Remove(context.Context, db.SiteDomain) error { p.removes++; return nil }
func (p *stagingProvisioner) Commit(context.Context) error {
	p.commits++
	if p.failCommit {
		return errors.New("deployment unavailable")
	}
	return nil
}

func TestDomainRemovalAndTLSRetry(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	row := pendingDomain(now)
	row.Status = "verified"
	row.VerifiedAt = stamp(now)
	m := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{row.Uuid: row}}
	p := &stagingProvisioner{failCommit: true}
	resolver := dnsFor(row.Host, row.VerificationToken)
	syncs := 0
	d := domainService(m, usecase.DomainOptions{Resolver: resolver, Now: func() time.Time { return now }, Providers: map[string]usecase.DomainProvisioner{"manual": p},
		SyncOrigins: func(context.Context, int64) error { syncs++; return nil }})
	if _, err := d.Sweep(ctx); err == nil {
		t.Fatal("deployment failure hidden")
	}
	r := m.rows[row.Uuid]
	if r.TlsStatus != "failed" || r.ProvisionerID != "external-id" || r.NextCheckAt.Time.Sub(now) != time.Minute || r.LeaseUntil.Valid {
		t.Fatalf("retry state: %+v", r)
	}
	p.failCommit = false
	now = now.Add(time.Minute)
	if _, err := d.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if m.rows[row.Uuid].TlsStatus != "active" {
		t.Fatal("TLS did not recover")
	}
	if err := d.Remove(ctx, 8, row.Uuid); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("cross-org delete %v", err)
	}
	if err := d.Remove(ctx, 7, row.Uuid); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Lookup(ctx, row.Host); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal("disabled domain still resolves")
	}
	if _, err := d.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(m.rows) != 0 || p.removes != 1 || syncs < 3 {
		t.Fatalf("cleanup: rows=%d removes=%d syncs=%d", len(m.rows), p.removes, syncs)
	}
}

func TestDomainOriginSyncRetry(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	row := pendingDomain(now)
	m := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{row.Uuid: row}}
	fail := true
	d := domainService(m, usecase.DomainOptions{Resolver: dnsFor(row.Host, row.VerificationToken), Now: func() time.Time { return now },
		Providers: map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}}, SyncOrigins: func(context.Context, int64) error {
			if fail {
				return errors.New("origin update unavailable")
			}
			return nil
		}})
	if _, err := d.Sweep(ctx); err == nil {
		t.Fatal("origin sync error hidden")
	}
	if !m.rows[row.Uuid].OriginsDirty {
		t.Fatal("lost retry marker")
	}
	now = now.Add(time.Minute)
	fail = false
	if _, err := d.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if m.rows[row.Uuid].OriginsDirty || m.rows[row.Uuid].TlsStatus != "manual" {
		t.Fatal("origin sync retry did not finish")
	}
	out, err := d.MarkTLSActive(ctx, row.Uuid)
	if err != nil || out.TLSStatus != "active" {
		t.Fatalf("manual mark %s %v", out.TLSStatus, err)
	}
	now = now.Add(24 * time.Hour)
	if _, err = d.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if m.rows[row.Uuid].TlsStatus != "active" {
		t.Fatal("daily check lost manual activation")
	}
}

func (m *domainMemory) ListSiteDomainHealthIssues(_ context.Context, limit int32) ([]db.SiteDomain, error) {
	rows := []db.SiteDomain{}
	for _, r := range m.rows {
		if r.Status == "failed" || r.TlsStatus == "failed" || r.TlsStatus == "manual" {
			rows = append(rows, r)
		}
	}
	return rows, nil
}
func (m *domainMemory) SiteDomainHealthCounts(context.Context) (db.SiteDomainHealthCountsRow, error) {
	var counts db.SiteDomainHealthCountsRow
	for _, r := range m.rows {
		switch r.Status {
		case "verified":
			counts.Verified++
		case "pending":
			counts.Pending++
		case "failed":
			counts.Failed++
		}
	}
	return counts, nil
}
func TestDomainHealth(t *testing.T) {
	row := pendingDomain(time.Now())
	row.Status = "failed"
	row.AttentionSince = stamp(time.Now().Add(-time.Hour))
	row.LastError = "verification TXT token mismatch"
	m := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{row.Uuid: row}}
	d := domainService(m, usecase.DomainOptions{})
	report, err := d.Collect(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Code() != "custom_domains" || len(report.Issues) != 1 || report.Issues[0].Severity != "critical" || report.Issues[0].RefUUID != row.Uuid.String() {
		t.Fatalf("health %+v", report)
	}
	if !report.Issues[0].OccurredAt.Equal(row.AttentionSince.Time) {
		t.Fatal("health alert time changed")
	}
	row.Status = "verified"
	row.TlsStatus = "manual"
	m.rows[row.Uuid] = row
	report, err = d.Collect(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Severity != "warning" {
		t.Fatalf("manual health %+v", report)
	}
}

// A frozen clock behind PostgreSQL must still schedule create, recheck and removal immediately.
func TestCustomDomainsFrozenClock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	host := "clock-" + uuid.NewString() + ".customer.com"
	resolver := dnsFor(host, "")
	base, err := usecase.ParseBaseURL("https://sites.test")
	e.must(err)
	d := usecase.NewDomains(e.q, base, usecase.DomainOptions{TargetHost: "edge.rentopia.com", Resolver: resolver,
		Providers: map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}}, Now: func() time.Time { return now }})
	domain, err := d.Create(ctx, e.a.id, 0, host)
	e.must(err)
	resolver.txt = []string{domain.VerificationToken}
	stored, err := e.q.GetSiteDomain(ctx, db.GetSiteDomainParams{OrganizationID: e.a.id, Uuid: domain.UUID})
	e.must(err)
	if !stored.CreatedAt.Time.Equal(now) || !stored.NextCheckAt.Time.Equal(now) {
		t.Fatalf("domain creation used a different clock: created=%v due=%v now=%v", stored.CreatedAt.Time, stored.NextCheckAt.Time, now)
	}
	_, err = d.Sweep(ctx)
	e.must(err)
	if orgID, err := d.Lookup(ctx, host); err != nil || orgID != e.a.id {
		t.Fatalf("new domain lookup: %d %v", orgID, err)
	}
	resolver.txt = nil
	_, err = d.Recheck(ctx, e.a.id, domain.UUID)
	e.must(err)
	_, err = d.Sweep(ctx)
	e.must(err)
	failed, err := d.Get(ctx, e.a.id, domain.UUID)
	e.must(err)
	if failed.Status != "failed" {
		t.Fatalf("immediate recheck did not run: %+v", failed)
	}
	e.must(d.Remove(ctx, e.a.id, domain.UUID))
	_, err = d.Sweep(ctx)
	e.must(err)
	if _, err = d.Get(ctx, e.a.id, domain.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("immediate removal cleanup: %v", err)
	}
	domain, err = d.Create(ctx, e.a.id, 0, host)
	e.must(err)
	resolver.txt = []string{domain.VerificationToken}
	now = now.Add(72 * time.Hour)
	_, err = d.Sweep(ctx)
	e.must(err)
	expired, err := d.Get(ctx, e.a.id, domain.UUID)
	e.must(err)
	if expired.Status != "failed" || !strings.Contains(expired.LastError, "72 hours") {
		t.Fatalf("expiry did not use the creation clock: %+v", expired)
	}
}

type domainCheckFailures struct {
	*domainMemory
	byID map[int64]error
}

func (m *domainCheckFailures) UpdateSiteDomainCheck(ctx context.Context, p db.UpdateSiteDomainCheckParams) (db.SiteDomain, error) {
	if err := m.byID[p.ID]; err != nil {
		return db.SiteDomain{}, err
	}
	return m.domainMemory.UpdateSiteDomainCheck(ctx, p)
}

func TestDomainSweepAggregatesRowErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	missing := pendingDomain(now)
	broken := pendingDomain(now)
	broken.ID = 2
	broken.OrganizationID = 8
	broken.Host = "broken.customer.com"
	healthy := pendingDomain(now)
	healthy.ID = 3
	healthy.Host = "healthy.customer.com"
	memory := &domainMemory{rows: map[uuid.UUID]db.SiteDomain{missing.Uuid: missing, broken.Uuid: broken, healthy.Uuid: healthy}}
	q := &domainCheckFailures{domainMemory: memory, byID: map[int64]error{missing.ID: pgx.ErrNoRows}}
	originErr := errors.New("origins unavailable")
	resolver := dnsFor(missing.Host, missing.VerificationToken)
	resolver.ips[broken.Host] = resolver.ips[missing.Host]
	resolver.ips[healthy.Host] = resolver.ips[missing.Host]
	d := domainService(q, usecase.DomainOptions{Resolver: resolver, Now: func() time.Time { return now },
		Providers: map[string]usecase.DomainProvisioner{"manual": usecase.ManualProvisioner{}},
		SyncOrigins: func(_ context.Context, orgID int64) error {
			if orgID == broken.OrganizationID {
				return originErr
			}
			return nil
		},
	})
	count, err := d.Sweep(ctx)
	if count != 3 || !errors.Is(err, pgx.ErrNoRows) || !errors.Is(err, originErr) {
		t.Fatalf("sweep did not return every row failure: count=%d err=%v", count, err)
	}
	for _, message := range []string{missing.Host, broken.Host, "check DNS and persist verification", "sync origins"} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("missing error context %q: %v", message, err)
		}
	}
	if memory.rows[healthy.Uuid].TlsStatus != "manual" {
		t.Fatal("failed rows prevented healthy row processing")
	}
	for _, r := range memory.rows {
		if r.LeaseUntil.Valid {
			t.Fatalf("row lease not released: %s", r.Host)
		}
	}
}
