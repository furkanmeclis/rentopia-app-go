package supportaccess

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// GrantLookup returns the organization's active support grant (nil when
// there is none). usecase.Service implements it.
type GrantLookup interface {
	ActiveGrant(ctx context.Context, orgID int64) (*orgctx.SupportGrant, error)
}

// GuardResolver wraps the tenant resolver (outside the cache) with the
// support access rule of TEC-678: a request of an impersonated session
// (JWT imp claim) resolves only while the organization has an active,
// unexpired, unrevoked grant; otherwise orgctx.ErrSupportGrantRequired
// (403 SUPPORT_GRANT_REQUIRED). The grant is put on the scope, where
// RequireOrganization enforces read_only (403 SUPPORT_READ_ONLY on every
// non-GET request) and the activity recorder stamps its uuid. Regular
// sessions pass through untouched. No super admin bypass.
func GuardResolver(inner orgctx.Resolver, grants GrantLookup) orgctx.Resolver {
	if inner == nil || grants == nil {
		return inner
	}
	return &guard{inner: inner, grants: grants}
}

type guard struct {
	inner  orgctx.Resolver
	grants GrantLookup
}

func (g *guard) Resolve(ctx context.Context, ref string, userID int64) (orgctx.Scope, error) {
	scope, err := g.inner.Resolve(ctx, ref, userID)
	if err != nil {
		return scope, err
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok || p.ImpersonatorUserID == nil {
		return scope, nil
	}
	grant, err := g.grants.ActiveGrant(ctx, scope.ID)
	if err != nil {
		return orgctx.Scope{}, err
	}
	if grant == nil {
		return orgctx.Scope{}, orgctx.ErrSupportGrantRequired
	}
	scope.SupportGrant = grant
	return scope, nil
}
