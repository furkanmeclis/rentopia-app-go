package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
)

// supportGate grants access to the users in allowed (TEC-678).
type supportGate struct {
	allowed map[int64]bool
	calls   int
}

func (g *supportGate) TargetHasSupportAccess(_ context.Context, userID int64) (bool, error) {
	g.calls++
	return g.allowed[userID], nil
}

func TestImpersonationNeedsSupportGrant(t *testing.T) {
	e := newFlowEnv(t)
	ctx := context.Background()
	staff := e.user(t, "staff@example.com")
	member := e.user(t, "member@example.com")
	gate := &supportGate{allowed: map[int64]bool{}}
	e.uc.SetSupportAccess(gate)

	// No grant: refused, for a super admin too (no bypass).
	_, err := e.uc.ImpersonatePlatformUser(ctx, staff, true, false, member.UUID, model.SessionMeta{})
	if !errors.Is(err, ErrSupportGrantRequired) {
		t.Fatalf("without grant: err = %v, want ErrSupportGrantRequired", err)
	}

	gate.allowed[member.ID] = true
	res, err := e.uc.ImpersonatePlatformUser(ctx, staff, true, false, member.UUID, model.SessionMeta{})
	if err != nil {
		t.Fatalf("with grant: %v", err)
	}
	if res.Session.ImpersonatorUUID == nil || *res.Session.ImpersonatorUUID != staff.UUID || res.AccessToken == "" {
		t.Fatalf("session = %+v", res.Session)
	}

	// Without the gate (tenancy off) impersonation is not gated.
	e.uc.SetSupportAccess(nil)
	gate.allowed[member.ID] = false
	if _, err := e.uc.ImpersonatePlatformUser(ctx, staff, true, false, member.UUID, model.SessionMeta{}); err != nil {
		t.Fatalf("tenancy off: %v", err)
	}
	if gate.calls != 2 {
		t.Fatalf("gate calls = %d, want 2", gate.calls)
	}
}
