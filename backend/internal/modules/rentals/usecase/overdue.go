package usecase

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SweepOverdue is the minute sweep (worker, after the hold expiry): a rental
// under way whose planned return + the policy's tolerance passed and whose
// engine allocation ended is overdue. The engine keeps the vehicle busy
// OverdueHorizon longer (moving flexible later commitments to other
// vehicles first); the reservations it still runs into are clipped and get
// a late_return resolution task each. Returns the number of rentals
// handled.
func (s *Service) SweepOverdue(ctx context.Context) (int, error) {
	now := s.clock()
	orgs, err := s.q.ListOverdueRentalOrganizations(ctx, ts(now))
	if err != nil {
		return 0, err
	}
	total := 0
	for _, orgID := range orgs {
		octx := orgctx.WithOrganizationID(ctx, orgID)
		ids, err := s.q.ListOverdueRentals(octx, db.ListOverdueRentalsParams{OrganizationID: orgID, NowAt: ts(now)})
		if err != nil {
			return total, err
		}
		for _, id := range ids {
			if s.markOverdue(octx, orgID, id) {
				total++
			}
		}
	}
	return total, nil
}

// MarkOverdue runs the overdue handling of one rental now (tests, manual
// trigger); false when nothing changed.
func (s *Service) MarkOverdue(ctx context.Context, orgID int64, id uuid.UUID) bool {
	return s.markOverdue(ctx, orgID, id)
}

func (s *Service) markOverdue(ctx context.Context, orgID int64, id uuid.UUID) bool {
	c, err := s.q.GetRentalCore(ctx, db.GetRentalCoreParams{OrganizationID: orgID, Uuid: id})
	if err != nil || c.Status != model.StatusActive || !c.AllocationUuid.Valid || c.AllocationKind.String != availmodel.KindRental {
		return false
	}
	now := s.clock()
	until := now.Add(model.OverdueHorizon)
	first := !c.OverdueAt.Valid
	oldEnd := c.AllocationEndsAt.Time
	var touched []uuid.UUID
	res, err := s.avail.SetRentalEnd(ctx, orgID, c.AllocationUuid.Bytes, availmodel.RentalEnd{
		EndsAt: until, Force: true,
		Hook: s.hookOf(func(hctx context.Context, q *db.Queries, tx pgx.Tx, r availmodel.HookResult) error {
			if _, err := q.SetRentalOverdue(hctx, db.SetRentalOverdueParams{OrganizationID: orgID, ID: c.ID, OverdueAt: ts(now)}); err != nil {
				return err
			}
			if first {
				if err := event(hctx, q, orgID, c.ID, model.EventOverdue, "", "", "", map[string]any{
					"planned_return_at": c.PlannedReturnAt.Time.UTC(), "grace_minutes": model.PolicyFromSnapshot(c.PolicySnapshot).Grace(),
				}); err != nil {
					return err
				}
			}
			t, err := s.reservations.RecordPlanEffects(hctx, tx, orgID, r, "rental", c.Uuid)
			touched = t
			return err
		}),
	})
	if err != nil {
		slog.Default().Warn("rental_overdue_failed", "rental", id, "error", err)
		return false
	}
	end := res.Allocation.EndsAt
	if end.After(oldEnd) || first {
		_ = s.tx(ctx, func(q *db.Queries) error {
			return event(ctx, q, orgID, c.ID, model.EventOverdueExtended, "", "", "", map[string]any{
				"expected_until": end, "clipped": res.Clipped, "moved": len(touched),
			})
		})
	}
	s.lateReturnTasks(ctx, orgID, c, res.Affected, map[string]any{"cause": "overdue", "expected_until": until})
	s.publish(ctx, model.ResourceRentals, "updated", id)
	if len(touched) > 0 {
		s.publish(ctx, "tenant.reservations", "updated", touched...)
	}
	return end.After(oldEnd) || first || len(res.Affected) > 0
}
