package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Equipment stock (docs/design/extras.md §3).
//
// Invariant: for every (extra, branch) and every instant t, the sum of the
// quantities of the active, unexpired allocations covering t is at most the
// branch stock. Every writer that adds consumption first locks the stock
// row (SELECT … FOR UPDATE), then reads the overlapping allocations and
// computes the peak with a sweep line, then inserts. Two writers of the
// same extra at the same branch therefore run one after the other; the
// second sees the first's row (READ COMMITTED re-reads after the lock) and
// is refused when the last unit is gone. Releases only lower consumption
// and need no lock.

// interval is one allocation inside a window.
type interval struct {
	start, end time.Time
	qty        int
	hold       bool
}

// peak is the highest concurrent quantity of the intervals inside
// [from, to) (sweep line over the clipped starts and ends; ranges are
// half-open, so an end and a start at the same instant do not overlap).
func peak(list []interval, from, to time.Time) int {
	type event struct {
		at    time.Time
		delta int
	}
	events := make([]event, 0, 2*len(list))
	for _, iv := range list {
		s, e := iv.start, iv.end
		if s.Before(from) {
			s = from
		}
		if e.After(to) {
			e = to
		}
		if !s.Before(e) {
			continue
		}
		events = append(events, event{s, iv.qty}, event{e, -iv.qty})
	}
	sort.Slice(events, func(i, j int) bool {
		if !events[i].at.Equal(events[j].at) {
			return events[i].at.Before(events[j].at)
		}
		return events[i].delta < events[j].delta // ends first
	})
	cur, best := 0, 0
	for _, ev := range events {
		cur += ev.delta
		if cur > best {
			best = cur
		}
	}
	return best
}

func intervals(rows []db.ListOverlappingEquipmentRow, extraID int64) []interval {
	out := []interval{}
	for _, r := range rows {
		if r.ExtraID != extraID {
			continue
		}
		out = append(out, interval{start: r.StartsAt.Time, end: r.EndsAt.Time, qty: int(r.Quantity), hold: r.Kind == model.KindHold})
	}
	return out
}

func (s *Service) overlapping(ctx context.Context, q *db.Queries, orgID int64, extraIDs []int64, branchID int64, from, to, now time.Time) ([]db.ListOverlappingEquipmentRow, error) {
	return q.ListOverlappingEquipment(ctx, db.ListOverlappingEquipmentParams{
		OrganizationID: orgID, ExtraIds: extraIDs, BranchID: branchID, NowAt: ts(now), FromAt: ts(from), ToAt: ts(to),
	})
}

// ------------------------------------------------------------------- stock

// horizon is how far ahead the committed peak of a stock row is computed.
const horizon = 366 * 24 * time.Hour

// Stock lists the stock of a physical extra at every branch in the
// caller's branch scope, with the units in use now and the committed peak.
func (s *Service) Stock(ctx context.Context, orgID int64, id uuid.UUID) ([]model.StockRow, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return nil, err
	}
	return s.stockRows(ctx, s.q, orgID, row.ID)
}

func (s *Service) stockRows(ctx context.Context, q *db.Queries, orgID, extraID int64) ([]model.StockRow, error) {
	list, err := q.ListExtraStock(ctx, db.ListExtraStockParams{OrganizationID: orgID, ExtraID: extraID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return nil, err
	}
	now := s.clock()
	out := make([]model.StockRow, 0, len(list))
	for _, r := range list {
		item := model.StockRow{
			Branch:       model.BranchRef{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
			BranchActive: r.BranchActive, Quantity: int(r.Quantity), UpdatedAt: timePtr(r.UpdatedAt),
		}
		allocs, err := s.overlapping(ctx, q, orgID, []int64{extraID}, r.BranchID, now, now.Add(horizon), now)
		if err != nil {
			return nil, err
		}
		ivs := intervals(allocs, extraID)
		item.PeakCommitted = peak(ivs, now, now.Add(horizon))
		for _, iv := range ivs {
			if !iv.start.After(now) && iv.end.After(now) {
				item.InUseNow += iv.qty
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// SetStock writes the counts of the listed branches. A count below what is
// already committed is recorded (an operational fact: a seat broke) and
// reported as a warning; new sales are refused until stock is back.
func (s *Service) SetStock(ctx context.Context, orgID int64, id uuid.UUID, in model.StockInput) (model.StockResult, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return model.StockResult{}, err
	}
	ve := &ValidationError{}
	if !row.IsPhysical {
		ve.add("items", "not_physical", "only physical extras have a stock")
	}
	if len(in.Items) == 0 {
		ve.add("items", "required", "nothing to change")
	}
	type change struct {
		branch db.Branch
		qty    int32
	}
	changes := []change{}
	seen := map[int64]bool{}
	for i, item := range in.Items {
		field := fmt.Sprintf("items.%d", i)
		if item.BranchUUID == nil {
			ve.add(field+".branch_uuid", "required", "branch_uuid is required")
			continue
		}
		b, ok := s.branchInScope(ctx, s.q, orgID, *item.BranchUUID)
		if !ok {
			ve.add(field+".branch_uuid", "not_found", "branch was not found")
			continue
		}
		if seen[b.ID] {
			ve.add(field+".branch_uuid", "duplicate", "the branch is listed twice")
			continue
		}
		seen[b.ID] = true
		if item.Quantity == nil || *item.Quantity < 0 || *item.Quantity > model.MaxStock {
			ve.add(field+".quantity", "out_of_range", "quantity must be between 0 and 100000")
			continue
		}
		changes = append(changes, change{branch: b, qty: int32(*item.Quantity)})
	}
	if err := ve.orNil(); err != nil {
		return model.StockResult{}, err
	}
	out := model.StockResult{Warnings: []model.StockWarning{}}
	now := s.clock()
	err = s.tx(ctx, func(q *db.Queries) error {
		for _, c := range changes {
			// The upsert locks the stock row like a reserve does.
			if err := q.UpsertExtraStock(ctx, db.UpsertExtraStockParams{
				OrganizationID: orgID, ExtraID: row.ID, BranchID: c.branch.ID, Quantity: c.qty,
			}); err != nil {
				return err
			}
			allocs, err := s.overlapping(ctx, q, orgID, []int64{row.ID}, c.branch.ID, now, now.Add(horizon), now)
			if err != nil {
				return err
			}
			if p := peak(intervals(allocs, row.ID), now, now.Add(horizon)); p > int(c.qty) {
				out.Warnings = append(out.Warnings, model.StockWarning{
					BranchUUID: c.branch.Uuid, Code: "below_committed", Quantity: int(c.qty), PeakCommitted: p,
				})
			}
		}
		return nil
	})
	if err != nil {
		return model.StockResult{}, err
	}
	out.Items, err = s.stockRows(ctx, s.q, orgID, row.ID)
	return out, err
}

// ------------------------------------------------------------ availability

// Availability answers, for the physical extras (all published ones, or
// the listed uuids) at a branch, how many units are free for the whole
// range (read-only).
func (s *Service) Availability(ctx context.Context, orgID int64, in model.AvailabilityInput) (model.AvailabilityResult, error) {
	ve := &ValidationError{}
	var branch db.Branch
	if in.BranchUUID == nil {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	} else if b, ok := s.branchInScope(ctx, s.q, orgID, *in.BranchUUID); !ok {
		ve.add("branch_uuid", "not_found", "branch was not found")
	} else {
		branch = b
	}
	from, to := checkRange(ve, in.StartsAt, in.EndsAt, true)
	var extras []db.GetExtrasByUUIDsRow
	if len(in.ExtraUUIDs) > 0 {
		list, err := s.q.GetExtrasByUUIDs(ctx, db.GetExtrasByUUIDsParams{OrganizationID: orgID, Uuids: in.ExtraUUIDs})
		if err != nil {
			return model.AvailabilityResult{}, err
		}
		for i, id := range in.ExtraUUIDs {
			idx := slices.IndexFunc(list, func(r db.GetExtrasByUUIDsRow) bool { return r.Uuid == id })
			switch {
			case idx < 0:
				ve.add(fmt.Sprintf("extra_uuids.%d", i), "not_found", "extra was not found")
			case !list[idx].IsPhysical:
				ve.add(fmt.Sprintf("extra_uuids.%d", i), "not_physical", "the extra is not physical equipment")
			}
		}
		extras = list
	}
	if err := ve.orNil(); err != nil {
		return model.AvailabilityResult{}, err
	}
	if len(in.ExtraUUIDs) == 0 {
		physical := true
		list, _, err := s.List(ctx, orgID, model.ListFilter{Limit: MaxListTargets, IsPhysical: &physical, Statuses: []string{model.StatusPublished}, SortField: "sort_order"})
		if err != nil {
			return model.AvailabilityResult{}, err
		}
		ids := make([]uuid.UUID, 0, len(list))
		for _, e := range list {
			ids = append(ids, e.UUID)
		}
		if extras, err = s.q.GetExtrasByUUIDs(ctx, db.GetExtrasByUUIDsParams{OrganizationID: orgID, Uuids: ids}); err != nil {
			return model.AvailabilityResult{}, err
		}
	}
	sort.Slice(extras, func(i, j int) bool {
		if extras[i].SortOrder != extras[j].SortOrder {
			return extras[i].SortOrder < extras[j].SortOrder
		}
		return extras[i].Code < extras[j].Code
	})
	out := model.AvailabilityResult{
		Branch: model.BranchRef{UUID: branch.Uuid, Code: branch.Code, Name: branch.Name},
		Items:  []model.EquipmentAvailability{},
	}
	if len(extras) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(extras))
	for _, e := range extras {
		ids = append(ids, e.ID)
	}
	stock, err := s.q.ListBranchStockForExtras(ctx, db.ListBranchStockForExtrasParams{OrganizationID: orgID, BranchID: branch.ID, ExtraIds: ids})
	if err != nil {
		return model.AvailabilityResult{}, err
	}
	allocs, err := s.overlapping(ctx, s.q, orgID, ids, branch.ID, from, to, s.clock())
	if err != nil {
		return model.AvailabilityResult{}, err
	}
	for _, e := range extras {
		item := model.EquipmentAvailability{Extra: model.ExtraRef{UUID: e.Uuid, Code: e.Code, Name: e.Name}}
		for _, st := range stock {
			if st.ExtraID == e.ID {
				item.Stock = int(st.Quantity)
			}
		}
		all := intervals(allocs, e.ID)
		confirmed := []interval{}
		for _, iv := range all {
			if !iv.hold {
				confirmed = append(confirmed, iv)
			}
		}
		used := peak(all, from, to)
		item.Reserved = peak(confirmed, from, to)
		item.Held = used - item.Reserved
		item.Available = max(0, item.Stock-used)
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// checkRange validates a [start, end) range (required: both must be set).
func checkRange(ve *ValidationError, start, end *time.Time, required bool) (time.Time, time.Time) {
	switch {
	case start == nil && end == nil && !required:
		return time.Time{}, time.Time{}
	case start == nil:
		ve.add("starts_at", "required", "starts_at is required")
	case end == nil:
		ve.add("ends_at", "required", "ends_at is required")
	case !end.After(*start):
		ve.add("ends_at", "before_start", "ends_at must be after starts_at")
	case end.Sub(*start) > model.MaxRangeDays*24*time.Hour:
		ve.add("ends_at", "too_long", "the range covers at most 365 days")
	default:
		return start.UTC().Truncate(time.Microsecond), end.UTC().Truncate(time.Microsecond)
	}
	return time.Time{}, time.Time{}
}

// ------------------------------------------------------------- allocations

// Reserve is ReserveEquipment: it allocates units of a physical extra at a
// branch for [starts_at, ends_at) as a hold (expires) or a reservation,
// refusing with *StockError when the branch has fewer free units for the
// whole range. With VehicleAllocationUUID the equipment follows that
// availability allocation (kind, expiry, defaults for branch and range;
// released / confirmed / extended together by the database trigger).
func (s *Service) Reserve(ctx context.Context, orgID int64, in model.ReserveInput, actorID *int64) (model.Allocation, error) {
	ve := &ValidationError{}
	var extra db.GetExtraByUUIDRow
	if in.ExtraUUID == nil {
		ve.add("extra_uuid", "required", "extra_uuid is required")
	} else if e, err := s.getExtra(ctx, s.q, orgID, *in.ExtraUUID); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return model.Allocation{}, err
		}
		ve.add("extra_uuid", "not_found", "extra was not found")
	} else if !e.IsPhysical {
		ve.add("extra_uuid", "not_physical", "the extra is not physical equipment")
	} else if e.Status == model.StatusArchived {
		ve.add("extra_uuid", "archived", "the extra is archived")
	} else {
		extra = e
	}
	qty := 1
	if in.Quantity != nil {
		qty = *in.Quantity
	}
	if qty < 1 || qty > model.MaxQuantity || (extra.ID != 0 && qty > int(extra.MaxQuantity)) {
		ve.add("quantity", "out_of_range", "quantity is outside the extra's limits")
	}
	kind := in.Kind
	if kind == "" {
		kind = model.KindHold
	}
	if in.VehicleAllocationUUID == nil && kind != model.KindHold && kind != model.KindReservation {
		ve.add("kind", "invalid", "kind must be hold or reservation")
	}
	minutes := model.DefaultHoldMinutes
	if in.HoldMinutes != nil {
		if *in.HoldMinutes < 1 || *in.HoldMinutes > model.MaxHoldMinutes {
			ve.add("hold_minutes", "out_of_range", "hold_minutes must be between 1 and 1440")
		}
		minutes = *in.HoldMinutes
	}
	if len(in.RefType) > 32 {
		ve.add("ref_type", "too_long", "ref_type is at most 32 characters")
	}
	var branch *db.Branch
	if in.BranchUUID != nil {
		if b, ok := s.branchInScope(ctx, s.q, orgID, *in.BranchUUID); ok {
			branch = &b
		} else {
			ve.add("branch_uuid", "not_found", "branch was not found")
		}
	} else if in.VehicleAllocationUUID == nil {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	}
	from, to := checkRange(ve, in.StartsAt, in.EndsAt, in.VehicleAllocationUUID == nil)
	if err := ve.orNil(); err != nil {
		return model.Allocation{}, err
	}

	var created uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		now := s.clock()
		params := db.InsertEquipmentAllocationParams{
			OrganizationID: orgID, ExtraID: extra.ID, Quantity: int32(qty), Kind: kind, Note: in.Note,
			RefType: textArg(in.RefType), CreatedBy: int8Ptr(actorID),
		}
		if in.RefUUID != nil {
			params.RefUuid = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
		}
		if kind == model.KindHold {
			params.ExpiresAt = ts(now.Add(time.Duration(minutes) * time.Minute))
		}
		branchID := int64(0)
		if branch != nil {
			branchID = branch.ID
		}
		if in.VehicleAllocationUUID != nil {
			// FOR SHARE: a concurrent release of the vehicle allocation
			// waits for this link (and then releases it too).
			va, err := q.GetVehicleAllocationForEquipment(ctx, db.GetVehicleAllocationForEquipmentParams{OrganizationID: orgID, Uuid: *in.VehicleAllocationUUID})
			if errors.Is(err, pgx.ErrNoRows) {
				return validation("vehicle_allocation_uuid", "not_found", "availability allocation was not found")
			}
			if err != nil {
				return err
			}
			if orgctx.CheckBranch(ctx, va.PickupBranchID) != nil && orgctx.CheckBranch(ctx, va.ReturnBranchID) != nil {
				return validation("vehicle_allocation_uuid", "not_found", "availability allocation was not found")
			}
			if va.Status != model.AllocActive || (va.Kind == model.KindHold && !va.ExpiresAt.Time.After(now)) {
				return validation("vehicle_allocation_uuid", "not_active", "the availability allocation is no longer active")
			}
			params.VehicleAllocationID = pgtype.Int8{Int64: va.ID, Valid: true}
			params.Kind = va.Kind
			params.ExpiresAt = pgtype.Timestamptz{}
			if va.Kind == model.KindHold {
				params.ExpiresAt = va.ExpiresAt
			}
			if !params.RefUuid.Valid && va.RefUuid.Valid {
				params.RefType, params.RefUuid = va.RefType, va.RefUuid
			}
			if branchID == 0 {
				branchID = va.PickupBranchID
				if orgctx.CheckBranch(ctx, branchID) != nil {
					return validation("branch_uuid", "required", "branch_uuid is required (the pick-up branch is outside your branch scope)")
				}
			}
			if from.IsZero() {
				from, to = va.StartsAt.Time.UTC(), va.EndsAt.Time.UTC()
			}
		}
		params.BranchID, params.StartsAt, params.EndsAt = branchID, ts(from), ts(to)

		// The serialisation point: the stock row of (extra, branch).
		var stock int32
		var err error
		if s.noStockLock {
			stock, err = q.GetExtraStock(ctx, db.GetExtraStockParams{OrganizationID: orgID, ExtraID: extra.ID, BranchID: branchID})
		} else {
			stock, err = q.LockExtraStock(ctx, db.LockExtraStockParams{OrganizationID: orgID, ExtraID: extra.ID, BranchID: branchID})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return &StockError{Stock: 0, Available: 0, Requested: qty}
		}
		if err != nil {
			return err
		}
		allocs, err := s.overlapping(ctx, q, orgID, []int64{extra.ID}, branchID, from, to, now)
		if err != nil {
			return err
		}
		used := peak(intervals(allocs, extra.ID), from, to)
		if used+qty > int(stock) {
			return &StockError{Stock: int(stock), Available: max(0, int(stock)-used), Requested: qty}
		}
		if s.afterCheck != nil {
			s.afterCheck()
		}
		row, err := q.InsertEquipmentAllocation(ctx, params)
		if err != nil {
			return err
		}
		created = row.Uuid
		return nil
	})
	if err != nil {
		return model.Allocation{}, err
	}
	return s.GetAllocation(ctx, orgID, created)
}

func (s *Service) allocationRef(ctx context.Context, orgID int64, id uuid.UUID) (db.GetEquipmentAllocationRow, error) {
	row, err := s.q.GetEquipmentAllocation(ctx, db.GetEquipmentAllocationParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrAllocationNotFound
	}
	if err != nil {
		return row, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return row, ErrAllocationNotFound
	}
	return row, nil
}

func (s *Service) whyNot(row db.GetEquipmentAllocationRow) error {
	switch {
	case row.Status != model.AllocActive:
		return ErrNotActive
	case row.Kind != model.KindHold:
		return ErrNotHold
	default:
		return ErrHoldExpired
	}
}

// Confirm is ConfirmEquipment: an active, unexpired hold becomes a
// reservation on the same row (stock is not consumed twice).
func (s *Service) Confirm(ctx context.Context, orgID int64, id uuid.UUID, in model.ConfirmInput) (model.Allocation, error) {
	row, err := s.allocationRef(ctx, orgID, id)
	if err != nil {
		return model.Allocation{}, err
	}
	if len(in.RefType) > 32 {
		return model.Allocation{}, validation("ref_type", "too_long", "ref_type is at most 32 characters")
	}
	var ref pgtype.UUID
	if in.RefUUID != nil {
		ref = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
	}
	n, err := s.q.ConfirmEquipmentAllocation(ctx, db.ConfirmEquipmentAllocationParams{
		OrganizationID: orgID, ID: row.ID, RefType: textArg(in.RefType), RefUuid: ref, NowAt: ts(s.clock()),
	})
	if err != nil {
		return model.Allocation{}, err
	}
	if n != 1 {
		return model.Allocation{}, s.whyNot(row)
	}
	return s.GetAllocation(ctx, orgID, id)
}

// Release is ReleaseEquipment: frees the units exactly once (ErrNotActive
// afterwards).
func (s *Service) Release(ctx context.Context, orgID int64, id uuid.UUID, reason string, actorID *int64) (model.Allocation, error) {
	row, err := s.allocationRef(ctx, orgID, id)
	if err != nil {
		return model.Allocation{}, err
	}
	if reason == "" {
		reason = "released"
	}
	if len(reason) > 48 {
		reason = reason[:48]
	}
	n, err := s.q.ReleaseEquipmentAllocation(ctx, db.ReleaseEquipmentAllocationParams{
		OrganizationID: orgID, ID: row.ID, Status: model.AllocReleased, ReleasedAt: ts(s.clock()),
		ReleasedBy: int8Ptr(actorID), ReleaseReason: reason,
	})
	if err != nil {
		return model.Allocation{}, err
	}
	if n != 1 {
		return model.Allocation{}, ErrNotActive
	}
	return s.GetAllocation(ctx, orgID, id)
}

// ExpireDue is the sweep of lapsed standalone equipment holds (the worker
// runs it with the availability sweep). Holds tied to a vehicle allocation
// expire with it (trigger); lapsed holds stop counting at expires_at even
// before the sweep.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	now := s.clock()
	orgs, err := s.q.ListDueEquipmentOrganizations(ctx, ts(now))
	if err != nil {
		return 0, err
	}
	total := 0
	for _, orgID := range orgs {
		n, err := s.q.ExpireDueEquipment(ctx, db.ExpireDueEquipmentParams{OrganizationID: orgID, NowAt: ts(now)})
		if err != nil {
			return total, err
		}
		total += int(n)
	}
	return total, nil
}

// GetAllocation loads one allocation (branch scope: 404 outside).
func (s *Service) GetAllocation(ctx context.Context, orgID int64, id uuid.UUID) (model.Allocation, error) {
	row, err := s.allocationRef(ctx, orgID, id)
	if err != nil {
		return model.Allocation{}, err
	}
	return s.allocation(db.ListEquipmentAllocationsRow(row)), nil
}

func (s *Service) allocation(r db.ListEquipmentAllocationsRow) model.Allocation {
	a := model.Allocation{
		UUID:     r.Uuid,
		Extra:    model.ExtraRef{UUID: r.ExtraUuid, Code: r.ExtraCode, Name: r.ExtraName},
		Branch:   model.BranchRef{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
		Quantity: int(r.Quantity), Kind: r.Kind, Status: r.Status,
		StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), ExpiresAt: timePtr(r.ExpiresAt),
		Note: r.Note, ReleasedAt: timePtr(r.ReleasedAt), ReleaseReason: r.ReleaseReason,
		CreatedBy: actorFrom(r.ActorUuid, r.ActorName, r.ActorSurname, r.ActorEmail),
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.VehicleAllocationUuid.Valid {
		u := uuid.UUID(r.VehicleAllocationUuid.Bytes)
		a.VehicleAllocationUUID = &u
	}
	if r.RefType.Valid {
		v := r.RefType.String
		a.RefType = &v
	}
	if r.RefUuid.Valid {
		u := uuid.UUID(r.RefUuid.Bytes)
		a.RefUUID = &u
	}
	a.Lapsed = a.Status == model.AllocActive && a.Kind == model.KindHold && a.ExpiresAt != nil && !a.ExpiresAt.After(s.clock())
	return a
}

// AllocationSortColumns are the sortable allocation list fields.
var AllocationSortColumns = map[string]string{
	"starts_at": "starts_at", "ends_at": "ends_at", "quantity": "quantity", "created_at": "created_at",
}

// ListAllocations lists allocations inside the caller's branch scope.
func (s *Service) ListAllocations(ctx context.Context, orgID int64, f model.AllocationFilter) ([]model.Allocation, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 25
	}
	if f.SortField == "" {
		f.SortField, f.SortDesc = "starts_at", true
	}
	var extraID pgtype.Int8
	if f.ExtraUUID != nil {
		row, err := s.getExtra(ctx, s.q, orgID, *f.ExtraUUID)
		if errors.Is(err, ErrNotFound) {
			return nil, 0, validation("extra", "not_found", "extra was not found")
		}
		if err != nil {
			return nil, 0, err
		}
		extraID = pgtype.Int8{Int64: row.ID, Valid: true}
	}
	var branchUUIDs []uuid.UUID
	if len(f.BranchUUIDs) > 0 {
		branchUUIDs = f.BranchUUIDs
	}
	scope := orgctx.BranchScope(ctx)
	list, err := s.q.ListEquipmentAllocations(ctx, db.ListEquipmentAllocationsParams{
		OrganizationID: orgID, BranchIds: scope, ExtraID: extraID, BranchUuids: branchUUIDs,
		Statuses: strs(f.Statuses), Kinds: strs(f.Kinds), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountEquipmentAllocations(ctx, db.CountEquipmentAllocationsParams{
		OrganizationID: orgID, BranchIds: scope, ExtraID: extraID, BranchUuids: branchUUIDs,
		Statuses: strs(f.Statuses), Kinds: strs(f.Kinds), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Allocation, 0, len(list))
	for _, r := range list {
		out = append(out, s.allocation(r))
	}
	return out, total, nil
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
