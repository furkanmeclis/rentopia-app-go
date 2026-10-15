package usecase

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultChannel is the channel of panel requests that name none.
const DefaultChannel = "branch"

// pastTolerance lets a request start slightly in the past (clock skew,
// "now" pick-ups typed a minute ago).
const pastTolerance = 5 * time.Minute

var poolRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)

// resolved is a request with its places, times and channel checked.
type resolved struct {
	pickup, ret           db.GetAvailabilityBranchRow
	pickupPoint, retPoint *db.GetAvailabilityServicePointRow
	start, end            time.Time
	prep                  int
	channel               string
	pool                  string
	settings              orgsettingsmodel.Settings
}

func (r resolved) period() engine.Interval {
	return engine.Interval{Start: r.start, End: r.readyAt()}
}

func (r resolved) readyAt() time.Time {
	return r.end.Add(time.Duration(r.prep) * time.Minute)
}

func (r resolved) request(key string) engine.Request {
	return engine.Request{Key: key, Pool: r.pool, Period: r.period(), End: r.end, Pickup: r.pickup.ID, Return: r.ret.ID}
}

func (r resolved) echo() model.ResolvedRequest {
	out := model.ResolvedRequest{
		PickupBranch: model.BranchRef{UUID: r.pickup.Uuid, Code: r.pickup.Code, Name: r.pickup.Name},
		ReturnBranch: model.BranchRef{UUID: r.ret.Uuid, Code: r.ret.Code, Name: r.ret.Name},
		StartsAt:     r.start, EndsAt: r.end, ReadyAt: r.readyAt(), PrepMinutes: r.prep,
		Channel: r.channel, FleetPool: r.pool,
	}
	if r.pickupPoint != nil {
		out.PickupPoint = &model.PointRef{UUID: r.pickupPoint.Uuid, Name: r.pickupPoint.Name}
	}
	if r.retPoint != nil {
		out.ReturnPoint = &model.PointRef{UUID: r.retPoint.Uuid, Name: r.retPoint.Name}
	}
	return out
}

func pointID(p *db.GetAvailabilityServicePointRow) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: p.ID, Valid: true}
}

// resolve checks the places (organization, branch scope: pick-up OR return
// branch in the caller's scope), the range and the channel.
func (s *Service) resolve(ctx context.Context, orgID int64, in model.Where) (resolved, error) {
	ve := &ValidationError{}
	r := resolved{}
	var ok bool
	r.pickup, r.pickupPoint, ok = s.place(ctx, orgID, ve, "pickup", in.PickupBranchUUID, in.PickupPointUUID, true)
	if ok {
		retBranch, retPoint := in.ReturnBranchUUID, in.ReturnPointUUID
		if retBranch == nil && retPoint == nil {
			r.ret, r.retPoint = r.pickup, r.pickupPoint
		} else {
			r.ret, r.retPoint, _ = s.place(ctx, orgID, ve, "return", retBranch, retPoint, false)
		}
	}
	if ok && r.ret.ID != 0 && orgctx.CheckBranch(ctx, r.pickup.ID) != nil && orgctx.CheckBranch(ctx, r.ret.ID) != nil {
		ve.add("pickup_branch_uuid", "not_found", "branch was not found")
	}
	r.start, r.end = in.StartsAt.UTC().Truncate(time.Microsecond), in.EndsAt.UTC().Truncate(time.Microsecond)
	now := s.clock()
	switch {
	case in.StartsAt.IsZero():
		ve.add("starts_at", "required", "starts_at is required (RFC 3339)")
	case r.start.Before(now.Add(-pastTolerance)):
		ve.add("starts_at", "in_past", "starts_at is in the past")
	}
	switch {
	case in.EndsAt.IsZero():
		ve.add("ends_at", "required", "ends_at is required (RFC 3339)")
	case !in.StartsAt.IsZero() && !r.end.After(r.start):
		ve.add("ends_at", "before_start", "ends_at must be after starts_at")
	case !in.StartsAt.IsZero() && r.end.Sub(r.start) > model.MaxRangeDays*24*time.Hour:
		ve.add("ends_at", "range_too_large", "the range may span at most 365 days")
	}
	r.pool = strings.ToUpper(strings.TrimSpace(in.FleetPool))
	if r.pool != "" && !poolRE.MatchString(r.pool) {
		ve.add("fleet_pool", "invalid", "fleet pool is invalid")
	}
	r.channel = strings.TrimSpace(in.Channel)
	if r.channel == "" {
		r.channel = DefaultChannel
	}
	settings, err := s.settings.Current(ctx, orgID)
	if err != nil {
		return r, err
	}
	r.settings = settings.Settings
	known, active := false, false
	for _, c := range settings.Settings.SalesChannels {
		if c.Code == r.channel {
			known, active = true, c.IsActive
		}
	}
	switch {
	case !known:
		ve.add("channel", "invalid", "unknown sales channel")
	case !active:
		ve.add("channel", "inactive", "the sales channel is not active")
	}
	if err := ve.orNil(); err != nil {
		return r, err
	}
	r.prep = int(r.ret.PrepMinutes)
	return r, nil
}

// place resolves a branch or a service point (the point's branch). Both
// given: the point must belong to the branch.
func (s *Service) place(ctx context.Context, orgID int64, ve *ValidationError, prefix string, branchUUID, pointUUID *uuid.UUID, required bool) (db.GetAvailabilityBranchRow, *db.GetAvailabilityServicePointRow, bool) {
	var point *db.GetAvailabilityServicePointRow
	var branch db.GetAvailabilityBranchRow
	if pointUUID != nil {
		p, err := s.q.GetAvailabilityServicePoint(ctx, db.GetAvailabilityServicePointParams{OrganizationID: orgID, Uuid: *pointUUID})
		if err != nil {
			ve.add(prefix+"_point_uuid", "not_found", "service point was not found")
			return branch, nil, false
		}
		if !p.IsActive {
			ve.add(prefix+"_point_uuid", "inactive", "service point is not active")
			return branch, nil, false
		}
		point = &p
	}
	switch {
	case branchUUID != nil:
		b, err := s.q.GetAvailabilityBranch(ctx, db.GetAvailabilityBranchParams{OrganizationID: orgID, Uuid: *branchUUID})
		if err != nil {
			ve.add(prefix+"_branch_uuid", "not_found", "branch was not found")
			return branch, nil, false
		}
		if point != nil && point.BranchID != b.ID {
			ve.add(prefix+"_point_uuid", "branch_mismatch", "the service point belongs to another branch")
			return branch, nil, false
		}
		branch = b
	case point != nil:
		var err error
		branch, err = s.branchByID(ctx, orgID, point.BranchID)
		if err != nil {
			ve.add(prefix+"_point_uuid", "not_found", "service point was not found")
			return branch, nil, false
		}
	default:
		if required {
			ve.add(prefix+"_branch_uuid", "required", "a branch or a service point is required")
		}
		return branch, nil, false
	}
	if !branch.IsActive {
		ve.add(prefix+"_branch_uuid", "inactive", "branch is not active")
		return branch, nil, false
	}
	return branch, point, true
}

func (s *Service) branchByID(ctx context.Context, orgID, id int64) (db.GetAvailabilityBranchRow, error) {
	rows, err := s.q.ListAvailabilityBranchesByIDs(ctx, db.ListAvailabilityBranchesByIDsParams{OrganizationID: orgID, Ids: []int64{id}})
	if err != nil {
		return db.GetAvailabilityBranchRow{}, err
	}
	if len(rows) == 0 {
		return db.GetAvailabilityBranchRow{}, pgx.ErrNoRows
	}
	r := rows[0]
	return db.GetAvailabilityBranchRow{ID: r.ID, Uuid: r.Uuid, Code: r.Code, Name: r.Name, PrepMinutes: r.PrepMinutes, IsActive: r.IsActive}, nil
}

// oneWayAllowed applies the one-way rules (fleetops) to a product.
func (s *Service) oneWayAllowed(ctx context.Context, q *db.Queries, orgID int64, r resolved, productID int64) (bool, error) {
	if r.pickup.ID == r.ret.ID {
		return true, nil
	}
	rows, err := q.ListOneWayRulesForRoute(ctx, db.ListOneWayRulesForRouteParams{
		OrganizationID: orgID, FromBranchID: r.pickup.ID, ToBranchID: r.ret.ID,
		RentalProductID: pgtype.Int8{Int64: productID, Valid: true},
	})
	if err != nil {
		return false, err
	}
	return fleetopsusecase.QuoteFromRules(rows).Allowed, nil
}

// holdMinutes is the option time of a new hold.
func holdMinutes(r resolved, requested *int) (int, error) {
	if requested != nil {
		if *requested < 1 || *requested > model.MaxHoldMinutes {
			return 0, validation("hold_minutes", "out_of_range", "hold_minutes must be between 1 and 1440")
		}
		return *requested, nil
	}
	if m := r.settings.Reservation.PaymentHoldMinutes; m != nil && *m > 0 && *m <= model.MaxHoldMinutes {
		return *m, nil
	}
	return model.DefaultHoldMinutes, nil
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
