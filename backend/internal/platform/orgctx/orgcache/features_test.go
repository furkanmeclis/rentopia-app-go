package orgcache

import (
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/redis/go-redis/v9"
)

type fakeLoader struct {
	mu        sync.Mutex
	calls     int
	planCalls int
	keys      map[int64][]features.Feature
	plans     map[int64]string
}

func (f *fakeLoader) Plan(_ context.Context, orgID int64) (features.PlanRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.planCalls++
	return features.PlanRef{Code: f.plans[orgID], Name: f.plans[orgID], Assigned: true}, nil
}

func (f *fakeLoader) EnabledFeatures(_ context.Context, orgID int64) ([]features.Feature, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return append([]features.Feature(nil), f.keys[orgID]...), nil
}

// Features are cached per organization and dropped with the organization's
// version (a plan or override change), so no request after the first
// queries the database until something changes.
func TestFeaturesCacheAndInvalidation(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	loader := &fakeLoader{
		keys:  map[int64][]features.Feature{1: {features.Site}, 2: {}},
		plans: map[int64]string{1: features.PlanComplete},
	}
	c := New(nil, rdb, 0, "test")
	f := c.Features(loader)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		on, err := f.Enabled(ctx, 1, features.Site)
		if err != nil || !on {
			t.Fatalf("site on org 1: %v %v", on, err)
		}
	}
	if loader.calls != 1 {
		t.Fatalf("loader calls = %d, want 1 (cached)", loader.calls)
	}
	if on, _ := f.Enabled(ctx, 2, features.Site); on {
		t.Fatal("org 2 has no features")
	}
	if on, _ := f.Enabled(ctx, 2, features.Site); on || loader.calls != 2 {
		t.Fatalf("an empty set is cached too (calls %d)", loader.calls)
	}

	// The plan (for /v1/auth/me) is cached next to the features.
	for i := 0; i < 3; i++ {
		if p, err := f.Plan(ctx, 1); err != nil || p.Code != features.PlanComplete || !p.Assigned {
			t.Fatalf("plan = %+v %v", p, err)
		}
	}
	if loader.planCalls != 1 {
		t.Fatalf("plan loads = %d, want 1 (cached)", loader.planCalls)
	}

	// Plan change: the organizations module invalidates the organization.
	loader.mu.Lock()
	loader.keys[1] = []features.Feature{features.PublicAPI}
	loader.plans[1] = features.PlanManagement
	loader.mu.Unlock()
	if err := c.InvalidateOrganization(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if on, _ := f.Enabled(ctx, 1, features.PublicAPI); !on {
		t.Fatal("invalidated entry still served")
	}
	if loader.calls != 3 {
		t.Fatalf("loader calls = %d, want 3", loader.calls)
	}
	if p, _ := f.Plan(ctx, 1); p.Code != features.PlanManagement || loader.planCalls != 2 {
		t.Fatalf("invalidated plan = %+v (loads %d)", p, loader.planCalls)
	}

	// Without Redis the loader answers every time.
	nf := NewFeatures(loader, nil, 0, "test")
	before := loader.calls
	_, _ = nf.Enabled(ctx, 1, features.Site)
	_, _ = nf.Enabled(ctx, 1, features.Site)
	if loader.calls != before+2 {
		t.Fatal("pass-through without Redis")
	}
}
