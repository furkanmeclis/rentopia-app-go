package orgcache

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/redis/go-redis/v9"
)

// Features caches the enabled plan features and the plan of an
// organization (TEC-674) under the same version counter as the scope
// entries: every InvalidateOrganization (plan, override, status change)
// drops them too. It implements features.AccessPolicy for public and
// background paths and features.Source for /v1/auth/me; tenant routes
// read the features of their cached orgctx.Scope instead. Redis errors
// fail open to the source.
type Features struct {
	c   *CachedResolver
	src features.Source
}

var (
	_ features.AccessPolicy = (*Features)(nil)
	_ features.Source       = (*Features)(nil)
)

// Features returns the feature cache sharing c's keys and versions.
func (c *CachedResolver) Features(src features.Source) *Features {
	return &Features{c: c, src: src}
}

// NewFeatures builds a feature cache without a scope resolver (the worker).
// rdb may be nil (pass-through).
func NewFeatures(src features.Source, rdb *redis.Client, ttl time.Duration, appEnv string) *Features {
	return New(nil, rdb, ttl, appEnv).Features(src)
}

// cached reads key kind of orgID (f = features, p = plan) through the
// versioned cache; load fills a miss.
func cached[T any](ctx context.Context, c *CachedResolver, kind string, orgID int64, load func() (T, error)) (T, error) {
	if c.rdb == nil {
		return load()
	}
	ver, err := c.version(ctx, orgID)
	if err != nil {
		return load()
	}
	key := c.prefix + kind + ":" + strconv.FormatInt(orgID, 10) + ":" + ver
	if raw, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var v T
		if json.Unmarshal(raw, &v) == nil {
			return v, nil
		}
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	if raw, err := json.Marshal(v); err == nil {
		c.rdb.Set(ctx, key, raw, c.ttl)
	}
	return v, nil
}

// EnabledFeatures implements features.Loader with the cache.
func (f *Features) EnabledFeatures(ctx context.Context, organizationID int64) ([]features.Feature, error) {
	return cached(ctx, f.c, "f", organizationID, func() ([]features.Feature, error) {
		keys, err := f.src.EnabledFeatures(ctx, organizationID)
		if keys == nil && err == nil {
			keys = []features.Feature{}
		}
		return keys, err
	})
}

// cachedPlan is the cached shape of a plan (PlanRef hides Assigned from
// JSON).
type cachedPlan struct {
	Code, Name, NameEN string
	PeriodStartsAt     *time.Time
	PeriodEndsAt       *time.Time
	UpdatedAt          *time.Time
	Assigned           bool
}

// Plan implements features.Source with the cache.
func (f *Features) Plan(ctx context.Context, organizationID int64) (features.PlanRef, error) {
	p, err := cached(ctx, f.c, "p", organizationID, func() (cachedPlan, error) {
		ref, err := f.src.Plan(ctx, organizationID)
		return cachedPlan{
			Code: ref.Code, Name: ref.Name, NameEN: ref.NameEN, PeriodStartsAt: ref.PeriodStartsAt,
			PeriodEndsAt: ref.PeriodEndsAt, UpdatedAt: ref.UpdatedAt, Assigned: ref.Assigned,
		}, err
	})
	if err != nil {
		return features.PlanRef{}, err
	}
	return features.PlanRef{
		Code: p.Code, Name: p.Name, NameEN: p.NameEN, PeriodStartsAt: p.PeriodStartsAt,
		PeriodEndsAt: p.PeriodEndsAt, UpdatedAt: p.UpdatedAt, Assigned: p.Assigned,
	}, nil
}

// Enabled implements features.AccessPolicy.
func (f *Features) Enabled(ctx context.Context, organizationID int64, feature features.Feature) (bool, error) {
	keys, err := f.EnabledFeatures(ctx, organizationID)
	if err != nil {
		return false, err
	}
	for _, k := range keys {
		if k == feature {
			return true, nil
		}
	}
	return false, nil
}
