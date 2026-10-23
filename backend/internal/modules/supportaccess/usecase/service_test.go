package usecase

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// memRepo keeps grants per organization, with the same "active" rule as
// the SQL (not revoked, expires_at > now).
type memRepo struct {
	seq     int64
	grants  []*row
	members map[int64][]int64 // user -> organizations
}

type row struct {
	id, org, createdBy, revokedBy int64
	uuid                          uuid.UUID
	scope, note                   string
	expiresAt, createdAt          time.Time
	revokedAt                     *time.Time
}

func newMemRepo() *memRepo { return &memRepo{members: map[int64][]int64{}} }

func active(r *row, now time.Time) bool { return r.revokedAt == nil && r.expiresAt.After(now) }

func (m *memRepo) Open(_ context.Context, orgID, actorID int64, scope string, expiresAt time.Time, note string, now time.Time) (int64, uuid.UUID, error) {
	for _, r := range m.grants {
		if r.org == orgID && active(r, now) {
			t := now
			r.revokedAt, r.revokedBy = &t, actorID
		}
	}
	m.seq++
	r := &row{id: m.seq, org: orgID, createdBy: actorID, uuid: uuid.New(), scope: scope, note: note, expiresAt: expiresAt, createdAt: now}
	m.grants = append(m.grants, r)
	return r.id, r.uuid, nil
}

func (m *memRepo) find(orgID int64, id uuid.UUID) *row {
	for _, r := range m.grants {
		if r.org == orgID && r.uuid == id {
			return r
		}
	}
	return nil
}

func (m *memRepo) Revoke(_ context.Context, orgID, actorID int64, id uuid.UUID, now time.Time) error {
	r := m.find(orgID, id)
	if r == nil || !active(r, now) {
		return repository.ErrNotFound
	}
	t := now
	r.revokedAt, r.revokedBy = &t, actorID
	return nil
}

func (m *memRepo) toModel(r *row, now time.Time) model.Grant {
	return model.Grant{
		ID: r.id, UUID: r.uuid, Scope: r.scope, Note: r.note, ExpiresAt: r.expiresAt, RevokedAt: r.revokedAt,
		CreatedAt: r.createdAt, Status: model.StatusAt(r.revokedAt, r.expiresAt, now),
	}
}

func (m *memRepo) Get(_ context.Context, orgID int64, id uuid.UUID, now time.Time) (model.Grant, error) {
	r := m.find(orgID, id)
	if r == nil {
		return model.Grant{}, repository.ErrNotFound
	}
	return m.toModel(r, now), nil
}

func (m *memRepo) List(_ context.Context, orgID int64, f model.ListFilter, now time.Time) ([]model.Grant, int64, error) {
	var out []model.Grant
	for _, r := range m.grants {
		if r.org != orgID {
			continue
		}
		g := m.toModel(r, now)
		if len(f.Statuses) > 0 && !contains(f.Statuses, g.Status) {
			continue
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, int64(len(out)), nil
}

func (m *memRepo) Active(_ context.Context, orgID int64, now time.Time) (model.Grant, error) {
	for i := len(m.grants) - 1; i >= 0; i-- {
		if r := m.grants[i]; r.org == orgID && active(r, now) {
			return m.toModel(r, now), nil
		}
	}
	return model.Grant{}, repository.ErrNotFound
}

func (m *memRepo) UserSummary(_ context.Context, userID int64, now time.Time) (int64, int64, error) {
	var granted int64
	for _, org := range m.members[userID] {
		if _, err := m.Active(context.Background(), org, now); err == nil {
			granted++
		}
	}
	return int64(len(m.members[userID])), granted, nil
}

func (m *memRepo) ListActivePlatform(_ context.Context, now time.Time) ([]model.PlatformGrant, error) {
	var out []model.PlatformGrant
	for _, r := range m.grants {
		if active(r, now) {
			out = append(out, model.PlatformGrant{UUID: r.uuid, Scope: r.scope, ExpiresAt: r.expiresAt})
		}
	}
	return out, nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newService() (*Service, *memRepo, *clock) {
	repo := newMemRepo()
	c := &clock{t: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	s := New(repo)
	s.SetClock(c.now)
	return s, repo, c
}

const (
	orgA  int64 = 1
	orgB  int64 = 2
	owner int64 = 10
)

func TestOpenValidates(t *testing.T) {
	s, _, _ := newService()
	ctx := context.Background()
	long := make([]rune, model.MaxNoteLength+1)
	for i := range long {
		long[i] = 'a'
	}
	cases := map[string]struct {
		in    model.OpenInput
		field string
	}{
		"unknown scope":  {model.OpenInput{Scope: "admin", DurationHours: 1}, "scope"},
		"zero duration":  {model.OpenInput{Scope: model.ScopeFull, DurationHours: 0}, "duration_hours"},
		"over 7 days":    {model.OpenInput{Scope: model.ScopeFull, DurationHours: 169}, "duration_hours"},
		"note too long":  {model.OpenInput{Scope: model.ScopeFull, DurationHours: 1, Note: string(long)}, "note"},
		"negative hours": {model.OpenInput{Scope: model.ScopeReadOnly, DurationHours: -3}, "duration_hours"},
	}
	for name, tc := range cases {
		_, err := s.Open(ctx, orgA, owner, tc.in)
		var ve *apiquery.ValidationError
		if !errors.As(err, &ve) || ve.Details[0].Field != tc.field {
			t.Errorf("%s: err = %v, want validation error on %s", name, err, tc.field)
		}
	}
	for _, h := range []int{1, 168} {
		if _, err := s.Open(ctx, orgA, owner, model.OpenInput{Scope: model.ScopeReadOnly, DurationHours: h}); err != nil {
			t.Errorf("duration %dh: %v", h, err)
		}
	}
}

func TestGrantLifecycle(t *testing.T) {
	s, _, c := newService()
	ctx := context.Background()

	if g, err := s.ActiveGrant(ctx, orgA); err != nil || g != nil {
		t.Fatalf("no grant yet: %v %v", g, err)
	}
	g, err := s.Open(ctx, orgA, owner, model.OpenInput{Scope: model.ScopeReadOnly, DurationHours: 4, Note: "  invoice issue  "})
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != model.StatusActive || g.Note != "invoice issue" || !g.ExpiresAt.Equal(c.t.Add(4*time.Hour)) {
		t.Fatalf("opened grant = %+v", g)
	}
	act, err := s.ActiveGrant(ctx, orgA)
	if err != nil || act == nil || act.UUID != g.UUID || !act.ReadOnly() {
		t.Fatalf("active = %+v, %v", act, err)
	}
	if other, _ := s.ActiveGrant(ctx, orgB); other != nil {
		t.Fatal("grant of A visible in B")
	}

	// A new grant replaces the active one.
	full, err := s.Open(ctx, orgA, owner, model.OpenInput{Scope: model.ScopeFull, DurationHours: 1})
	if err != nil {
		t.Fatal(err)
	}
	if act, _ := s.ActiveGrant(ctx, orgA); act == nil || act.UUID != full.UUID || act.ReadOnly() {
		t.Fatalf("active after replace = %+v", act)
	}
	items, _, _ := s.List(ctx, orgA, model.ListFilter{Statuses: []string{model.StatusRevoked}})
	if len(items) != 1 || items[0].UUID != g.UUID {
		t.Fatalf("revoked list = %+v", items)
	}

	// Revoke: another organization's uuid is not found; the own one ends at once.
	if _, err := s.Revoke(ctx, orgB, owner, full.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization revoke = %v", err)
	}
	revoked, err := s.Revoke(ctx, orgA, owner, full.UUID)
	if err != nil || revoked.Status != model.StatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("revoke = %+v, %v", revoked, err)
	}
	if act, _ := s.ActiveGrant(ctx, orgA); act != nil {
		t.Fatal("revoked grant still active")
	}
	if _, err := s.Revoke(ctx, orgA, owner, full.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke = %v", err)
	}

	// Expiry at request time: no job needed.
	short, _ := s.Open(ctx, orgA, owner, model.OpenInput{Scope: model.ScopeFull, DurationHours: 1})
	c.t = c.t.Add(59 * time.Minute)
	if act, _ := s.ActiveGrant(ctx, orgA); act == nil || act.UUID != short.UUID {
		t.Fatal("grant should still be active before expiry")
	}
	c.t = c.t.Add(time.Minute)
	if act, _ := s.ActiveGrant(ctx, orgA); act != nil {
		t.Fatal("grant active at expires_at")
	}
	if _, err := s.Revoke(ctx, orgA, owner, short.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke of an expired grant = %v", err)
	}
	expired, total, _ := s.List(ctx, orgA, model.ListFilter{Statuses: []string{model.StatusExpired}})
	if total != 1 || expired[0].UUID != short.UUID {
		t.Fatalf("expired list = %+v", expired)
	}
	if _, _, err := s.List(ctx, orgA, model.ListFilter{Statuses: []string{"active", "bogus"}}); err == nil {
		t.Fatal("unknown status accepted")
	}
}

func TestTargetHasSupportAccess(t *testing.T) {
	s, repo, c := newService()
	ctx := context.Background()
	const platformOnly, member, multi int64 = 100, 101, 102
	repo.members[member] = []int64{orgA}
	repo.members[multi] = []int64{orgA, orgB}

	check := func(user int64, want bool) {
		t.Helper()
		got, err := s.TargetHasSupportAccess(ctx, user)
		if err != nil || got != want {
			t.Fatalf("user %d: got %v (%v), want %v", user, got, err, want)
		}
	}
	check(platformOnly, true) // enters no organization
	check(member, false)
	check(multi, false)

	_, _ = s.Open(ctx, orgB, owner, model.OpenInput{Scope: model.ScopeReadOnly, DurationHours: 2})
	check(member, false) // B's grant does not open A
	check(multi, true)

	c.t = c.t.Add(3 * time.Hour)
	check(multi, false) // expired
}
