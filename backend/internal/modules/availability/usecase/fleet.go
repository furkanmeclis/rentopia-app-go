package usecase

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/eligibility"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// The eligibility graph: which vehicles keep the promise of which product
// version, and its connected components (vehicles linked through any
// version that accepts both). It changes only when vehicles, products,
// versions or rental groups change, so it is cached per organization and
// rebuilt when GetAvailabilityFingerprint changes (or after fleetCacheTTL,
// which also covers platform catalog edits). A stale graph is safe: the
// engine re-reads locations, blocks, transfers and allocations inside the
// lock, the guard trigger refuses blocked / inactive vehicles and the
// EXCLUDE constraint refuses double booking; staleness can only cost a
// retry or a missed reassignment.

const fleetCacheTTL = 2 * time.Minute

type vehicleInfo struct {
	ID      int64
	UUID    uuid.UUID
	Plate   string
	Pool    string
	Service engine.Interval
}

type versionInfo struct {
	ID             int64
	ProductID      int64
	ProductUUID    uuid.UUID
	Code, Title    string
	Version        int32
	GuaranteeType  string
	Status         string
	IsActive, Live bool
	Current        bool
}

// onSale: the current version of a live, active, published product.
func (v *versionInfo) onSale() bool {
	return v.Current && v.Live && v.IsActive && v.Status == "published"
}

type fleetSnapshot struct {
	fingerprint string
	loadedAt    time.Time
	vehicles    map[int64]*vehicleInfo
	versions    map[string]*versionInfo // key → version
	eligible    map[string]map[int64]bool
	component   map[int64]int64   // vehicle → component id (lowest vehicle id)
	members     map[int64][]int64 // component → vehicles
	products    []*versionInfo    // on-sale versions, code order
}

func versionKey(productID int64, version int32) string {
	return "p" + strconv.FormatInt(productID, 10) + "v" + strconv.FormatInt(int64(version), 10)
}

// componentOf returns the component of a guarantee's eligible vehicles
// (they are all in one component, linked by that guarantee); 0 = none.
func (f *fleetSnapshot) componentOf(key string) int64 {
	for id := range f.eligible[key] {
		return f.component[id]
	}
	return 0
}

type fleetCache struct {
	mu   sync.Mutex
	orgs map[int64]*fleetSnapshot
}

func newFleetCache() *fleetCache { return &fleetCache{orgs: map[int64]*fleetSnapshot{}} }

func (c *fleetCache) invalidate(orgID int64) {
	c.mu.Lock()
	delete(c.orgs, orgID)
	c.mu.Unlock()
}

// snapshot returns the organization's eligibility graph, rebuilding it when
// the fingerprint changed.
func (s *Service) snapshot(ctx context.Context, orgID int64) (*fleetSnapshot, error) {
	fp, err := s.q.GetAvailabilityFingerprint(ctx, orgID)
	if err != nil {
		return nil, err
	}
	s.cache.mu.Lock()
	cur := s.cache.orgs[orgID]
	s.cache.mu.Unlock()
	if cur != nil && cur.fingerprint == fp && time.Since(cur.loadedAt) < fleetCacheTTL {
		return cur, nil
	}
	snap, err := s.buildSnapshot(ctx, orgID, fp)
	if err != nil {
		return nil, err
	}
	s.cache.mu.Lock()
	s.cache.orgs[orgID] = snap
	s.cache.mu.Unlock()
	return snap, nil
}

var turkey = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*3600)
}()

// serviceWindow maps the in / out of service dates (Türkiye calendar days,
// both inclusive) to an interval.
func serviceWindow(in, out pgtype.Date) engine.Interval {
	w := engine.Interval{Start: time.Time{}, End: engine.Forever}
	if in.Valid {
		d := in.Time
		w.Start = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, turkey).UTC()
	}
	if out.Valid {
		d := out.Time.AddDate(0, 0, 1)
		w.End = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, turkey).UTC()
	}
	return w
}

func (s *Service) buildSnapshot(ctx context.Context, orgID int64, fp string) (*fleetSnapshot, error) {
	rows, err := s.vehicles.AvailabilityFleet(ctx, orgID)
	if err != nil {
		return nil, err
	}
	versionRows, err := s.q.ListAvailabilityProductVersions(ctx, orgID)
	if err != nil {
		return nil, err
	}
	versions := make([]db.RentalProductVersion, 0, len(versionRows))
	for _, v := range versionRows {
		versions = append(versions, db.RentalProductVersion{
			ID: v.ID, Uuid: v.Uuid, OrganizationID: v.OrganizationID, ProductID: v.ProductID, Version: v.Version,
			GuaranteeType: v.GuaranteeType, VehicleID: v.VehicleID, VariantLevel: v.VariantLevel,
			CatalogModelID: v.CatalogModelID, CatalogTypeID: v.CatalogTypeID, RentalGroupID: v.RentalGroupID,
			GuaranteeLabel: v.GuaranteeLabel,
		})
	}
	guarantees, err := s.vehicles.Guarantees(ctx, orgID, versions)
	if err != nil {
		return nil, err
	}
	snap := &fleetSnapshot{
		fingerprint: fp, loadedAt: time.Now(),
		vehicles: map[int64]*vehicleInfo{}, versions: map[string]*versionInfo{},
		eligible: map[string]map[int64]bool{}, component: map[int64]int64{}, members: map[int64][]int64{},
	}
	attrs := make(map[int64]eligibility.VehicleAttributes, len(rows))
	for _, row := range rows {
		v := row.Vehicle
		snap.vehicles[v.ID] = &vehicleInfo{
			ID: v.ID, UUID: v.Uuid, Plate: v.Plate, Pool: v.FleetPool,
			Service: serviceWindow(v.InServiceDate, v.OutOfServiceDate),
		}
		attrs[v.ID] = vehicleusecase.Attributes(row)
	}
	// Union-find over vehicles: a guarantee links all the vehicles it accepts.
	parent := map[int64]int64{}
	find := func(x int64) int64 {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for id := range snap.vehicles {
		parent[id] = id
	}
	for _, v := range versionRows {
		key := versionKey(v.ProductID, v.Version)
		info := &versionInfo{
			ID: v.ID, ProductID: v.ProductID, ProductUUID: v.ProductUuid, Code: v.ProductCode, Title: v.ProductTitle,
			Version: v.Version, GuaranteeType: v.GuaranteeType, Status: v.ProductStatus,
			IsActive: v.ProductIsActive, Live: v.ProductLive, Current: v.Version == v.CurrentVersion,
		}
		snap.versions[key] = info
		set := map[int64]bool{}
		g := guarantees[v.ID]
		var first int64
		for id, a := range attrs {
			if ok, _ := eligibility.SatisfiesGuarantee(a, g); ok {
				set[id] = true
				if first == 0 {
					first = id
				} else if ra, rb := find(first), find(id); ra != rb {
					if ra < rb {
						parent[rb] = ra
					} else {
						parent[ra] = rb
					}
				}
			}
		}
		snap.eligible[key] = set
		if info.onSale() {
			snap.products = append(snap.products, info)
		}
	}
	// Component id = the lowest vehicle id of the component (union by
	// smaller root keeps it the root).
	for id := range snap.vehicles {
		root := find(id)
		snap.component[id] = root
		snap.members[root] = append(snap.members[root], id)
	}
	for _, list := range snap.members {
		sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	}
	sort.Slice(snap.products, func(i, j int) bool { return snap.products[i].Code < snap.products[j].Code })
	return snap, nil
}

// componentState is a component loaded for planning.
type componentState struct {
	st     *engine.State
	allocs map[int64]db.ListAvailabilityAllocationsRow
	// due are active holds past their expiry (still in st; the writer
	// expires them before planning, the reader drops them).
	due []int64
	// segments maps a segment ref to its row (affected reporting).
	segments map[string]db.ListAvailabilitySegmentsRow
}

// loadComponent reads the component's vehicles, blocks, transfers and
// allocations (inside the lock for writers).
func (s *Service) loadComponent(ctx context.Context, q *db.Queries, orgID int64, snap *fleetSnapshot, component int64, now time.Time) (*componentState, error) {
	ids := snap.members[component]
	cs := &componentState{
		st: engine.NewState(now), allocs: map[int64]db.ListAvailabilityAllocationsRow{},
		segments: map[string]db.ListAvailabilitySegmentsRow{},
	}
	if len(ids) == 0 {
		return cs, nil
	}
	states, err := q.ListAvailabilityVehicleStates(ctx, db.ListAvailabilityVehicleStatesParams{OrganizationID: orgID, VehicleIds: ids})
	if err != nil {
		return nil, err
	}
	for _, row := range states {
		info := snap.vehicles[row.ID]
		if info == nil || !row.Usable {
			continue
		}
		cs.st.Vehicles[row.ID] = &engine.Vehicle{ID: row.ID, Pool: info.Pool, Branch: row.CurrentBranchID, Service: info.Service}
	}
	segments, err := q.ListAvailabilitySegments(ctx, db.ListAvailabilitySegmentsParams{
		OrganizationID: orgID, VehicleIds: ids, NowAt: ts(now),
	})
	if err != nil {
		return nil, err
	}
	for _, row := range segments {
		v := cs.st.Vehicles[row.VehicleID]
		if v == nil || !row.StartsAt.Valid {
			continue
		}
		seg := engine.Segment{Kind: row.Kind, Ref: row.RefUuid.String(), Period: engine.Interval{Start: row.StartsAt.Time, End: engine.Forever}}
		if row.EndsAt.Valid {
			seg.Period.End = row.EndsAt.Time
		}
		if row.Kind == engine.SegmentTransfer {
			seg.To = row.ToBranchID
			if row.TransferStatus == "planned" {
				seg.From = row.FromBranchID
			}
		}
		v.Segments = append(v.Segments, seg)
		cs.segments[seg.Ref] = row
	}
	for key, set := range snap.eligible {
		sub := map[int64]bool{}
		for _, id := range ids {
			if set[id] && cs.st.Vehicles[id] != nil {
				sub[id] = true
			}
		}
		if len(sub) > 0 {
			cs.st.Eligible[key] = sub
		}
	}
	cs.st.FlexKeys = map[string]bool{}
	for _, p := range snap.products {
		cs.st.FlexKeys[versionKey(p.ProductID, p.Version)] = true
	}
	allocs, err := q.ListAvailabilityAllocations(ctx, db.ListAvailabilityAllocationsParams{
		OrganizationID: orgID, VehicleIds: ids, NowAt: ts(now),
	})
	if err != nil {
		return nil, err
	}
	for _, row := range allocs {
		if cs.st.Vehicles[row.VehicleID] == nil {
			continue // the vehicle left the fleet: nothing to plan around
		}
		cs.allocs[row.ID] = row
		cs.st.Commitments[row.ID] = &engine.Commitment{
			ID: row.ID, Key: versionKey(row.RentalProductID, row.ProductVersion), Pool: row.FleetPool,
			Period: engine.Interval{Start: row.PeriodStart.Time, End: row.PeriodEnd.Time}, End: row.EndsAt.Time,
			Pickup: row.PickupBranchID, Return: row.ReturnBranchID, Vehicle: row.VehicleID,
			Fixed: row.PlateFixed, Hold: row.Kind == "hold",
		}
		if row.Due {
			cs.due = append(cs.due, row.ID)
		}
	}
	cs.st.Invalidate()
	return cs, nil
}

// dropDue removes due holds from a read-only state (they no longer count).
func (cs *componentState) dropDue() {
	for _, id := range cs.due {
		cs.st.Remove(id)
	}
	cs.due = nil
}

// writer tracks the writes of one planning transaction.
type writer struct {
	ctx      context.Context
	q        *db.Queries
	tx       pgx.Tx
	orgID    int64
	deferred bool
	cs       *componentState
}

func (w *writer) deferConstraint() error {
	if w.deferred {
		return nil
	}
	w.deferred = true
	return deferExclusion(w.ctx, w.tx)
}

// applyMoves writes reassignments (optimistic: a row that moved since it
// was read makes the plan stale).
func (w *writer) applyMoves(moves []engine.Move) error {
	if len(moves) == 0 {
		return nil
	}
	if err := w.deferConstraint(); err != nil {
		return err
	}
	for _, m := range moves {
		n, err := w.q.MoveAllocation(w.ctx, db.MoveAllocationParams{
			OrganizationID: w.orgID, ID: m.Commitment, FromVehicleID: m.From, ToVehicleID: m.To,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return errStale
		}
	}
	return nil
}

// expireDue expires the component's due holds (each exactly once) and
// re-plans what depended on them. Returns the items left affected.
func (w *writer) expireDue(now time.Time, opts engine.Options) ([]string, error) {
	affected := []string{}
	ids := append([]int64(nil), w.cs.due...)
	sort.Slice(ids, func(i, j int) bool {
		return w.cs.allocs[ids[i]].PeriodStart.Time.Before(w.cs.allocs[ids[j]].PeriodStart.Time)
	})
	for _, id := range ids {
		if w.cs.st.Commitments[id] == nil {
			continue
		}
		res := w.cs.st.Release(id, opts)
		if err := w.applyMoves(res.Moves); err != nil {
			return nil, err
		}
		n, err := w.q.ReleaseAllocation(w.ctx, db.ReleaseAllocationParams{
			OrganizationID: w.orgID, ID: id, Status: "expired", ReleasedAt: ts(now), ReleaseReason: "expired",
		})
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, errStale
		}
		affected = append(affected, res.Affected...)
	}
	w.cs.due = nil
	return affected, nil
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
