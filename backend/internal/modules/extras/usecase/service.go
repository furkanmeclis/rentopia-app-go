// Package usecase holds the extras business rules (TEC-702): the catalog,
// effective-dated per-product prices, branch stock of physical equipment,
// timed equipment allocations (ReserveEquipment / ConfirmEquipment /
// ReleaseEquipment) and the resolver that turns selected extras into
// pricing engine lines. docs/design/extras.md.
//
// Every method takes the organization id explicitly (orgctx.Scope in
// handlers, the job in workers). The catalog and prices are organization
// wide; stock rows and allocations belong to a branch and follow the
// member's branch scope (orgctx.BranchScope / CheckBranch).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxListTargets caps exports and bulk "all matching".
const MaxListTargets = 10000

var (
	ErrNotFound  = errors.New("extra not found")
	ErrCodeTaken = errors.New("a live extra already has this code")
	// ErrAllocationNotFound: unknown equipment allocation (or outside the
	// branch scope).
	ErrAllocationNotFound = errors.New("equipment allocation not found")
	// ErrNotActive: the allocation was already released / expired.
	ErrNotActive = errors.New("the allocation is not active")
	// ErrHoldExpired: the hold lapsed before the call.
	ErrHoldExpired = errors.New("the hold has expired")
	// ErrNotHold: the call needs a hold.
	ErrNotHold = errors.New("the allocation is not a hold")
)

// StockError is answered with 409 EQUIPMENT_UNAVAILABLE: the branch does
// not have enough free units in the range (nothing was written).
type StockError struct {
	Stock     int
	Available int
	Requested int
}

func (e *StockError) Error() string {
	return fmt.Sprintf("equipment unavailable: %d requested, %d available of %d", e.Requested, e.Available, e.Stock)
}

// Detail is one field-level validation problem.
type Detail struct {
	Field, Code, Message string
}

// ValidationError carries field details; handlers answer 400.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if e == nil || len(e.Details) == 0 {
		return nil
	}
	return e
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// Service implements the extras use cases.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	now  func() time.Time
	// Tests only: noStockLock reads the stock row without FOR UPDATE and
	// afterCheck runs between the stock check and the insert (they prove
	// the row lock is what prevents overselling).
	noStockLock bool
	afterCheck  func()
}

// New builds the service. pool may be nil in unit tests that never write.
func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// DisableStockLock reads the stock row without locking it (tests only).
func (s *Service) DisableStockLock() { s.noStockLock = true }

// SetAfterCheckHook runs fn between the stock check and the insert of a
// reserve (tests only: widens the race window).
func (s *Service) SetAfterCheckHook(fn func()) { s.afterCheck = fn }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// tx runs fn in one transaction.
func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("extras: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"code": "code", "name": "name", "extra_type": "extra_type", "status": "status",
	"default_price": "default_price", "sort_order": "sort_order", "created_at": "created_at", "updated_at": "updated_at",
}

// ------------------------------------------------------------------ catalog

func extraFrom(id uuid.UUID, code, name, desc, typ, charge string, minQ, maxQ int32, maxTotal pgtype.Numeric,
	channels []string, status string, physical bool, sortOrder int32, created, updated pgtype.Timestamptz) model.Extra {
	e := model.Extra{
		UUID: id, Code: code, Name: name, Description: desc, ExtraType: typ, ChargeType: charge,
		MinQuantity: int(minQ), MaxQuantity: int(maxQ), MaxTotal: moneyPtr(maxTotal), Channels: channelsOut(channels),
		Status: status, IsPhysical: physical, SortOrder: int(sortOrder),
		CreatedAt: created.Time.UTC(), UpdatedAt: updated.Time.UTC(),
	}
	return e
}

func extraFromRow(r db.GetExtraByUUIDRow) model.Extra {
	return extraFrom(r.Uuid, r.Code, r.Name, r.Description, r.ExtraType, r.ChargeType, r.MinQuantity, r.MaxQuantity,
		r.MaxTotal, r.Channels, r.Status, r.IsPhysical, r.SortOrder, r.CreatedAt, r.UpdatedAt)
}

// channelsOut keeps nil (every channel) apart from an explicit list.
func channelsOut(v []string) []string {
	if v == nil {
		return nil
	}
	return append([]string{}, v...)
}

func (s *Service) getExtra(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetExtraByUUIDRow, error) {
	row, err := q.GetExtraByUUID(ctx, db.GetExtraByUUIDParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// List lists the catalog.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Extra, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 25
	}
	if f.SortField == "" {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, err := s.q.ListExtras(ctx, db.ListExtrasParams{
		NowAt: ts(s.clock()), OrganizationID: orgID, Types: strs(f.Types), ChargeTypes: strs(f.ChargeTypes),
		Statuses: strs(f.Statuses), IsPhysical: boolArg(f.IsPhysical), Channel: textArg(f.Channel), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountExtras(ctx, db.CountExtrasParams{
		OrganizationID: orgID, Types: strs(f.Types), ChargeTypes: strs(f.ChargeTypes), Statuses: strs(f.Statuses),
		IsPhysical: boolArg(f.IsPhysical), Channel: textArg(f.Channel), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Extra, 0, len(rows))
	for _, r := range rows {
		e := extraFrom(r.Uuid, r.Code, r.Name, r.Description, r.ExtraType, r.ChargeType, r.MinQuantity, r.MaxQuantity,
			r.MaxTotal, r.Channels, r.Status, r.IsPhysical, r.SortOrder, r.CreatedAt, r.UpdatedAt)
		e.DefaultPrice = moneyPtr(r.DefaultPrice)
		e.StockTotal = r.StockTotal
		e.ProductPriceCount = r.ProductPriceCount
		out = append(out, e)
	}
	return out, total, nil
}

// ExportRows lists every matching extra (exports).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Extra, error) {
	f.Limit, f.Offset = MaxListTargets, 0
	f.SortField, f.SortDesc = "code", false
	rows, _, err := s.List(ctx, orgID, f)
	return rows, err
}

// TargetUUIDs lists the uuids matching a filter (bulk "all matching").
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	return s.q.ListExtraUUIDs(ctx, db.ListExtraUUIDsParams{
		OrganizationID: orgID, Types: strs(f.Types), ChargeTypes: strs(f.ChargeTypes), Statuses: strs(f.Statuses),
		IsPhysical: boolArg(f.IsPhysical), Channel: textArg(f.Channel), Q: textArg(f.Q), LimitCount: limit,
	})
}

// Facets answers the dynamic filter options.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ExtraFacets(ctx, orgID)
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{ExtraType: []model.Facet{}, ChargeType: []model.Facet{}, Status: []model.Facet{}, IsPhysical: []model.Facet{}}
	for _, r := range rows {
		f := model.Facet{Value: r.Value, Count: r.Count}
		switch r.Facet {
		case "extra_type":
			out.ExtraType = append(out.ExtraType, f)
		case "charge_type":
			out.ChargeType = append(out.ChargeType, f)
		case "status":
			out.Status = append(out.Status, f)
		case "is_physical":
			out.IsPhysical = append(out.IsPhysical, f)
		}
	}
	return out, nil
}

// Get loads one extra.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Extra, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return model.Extra{}, err
	}
	e := extraFromRow(row)
	if p, err := s.q.GetEffectiveExtraPrice(ctx, db.GetEffectiveExtraPriceParams{
		OrganizationID: orgID, ExtraID: row.ID, NowAt: ts(s.clock()),
	}); err == nil {
		e.DefaultPrice = moneyPtr(p.Price)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.Extra{}, err
	}
	return e, nil
}

// fields is a validated extra.
type fields struct {
	code, name, desc, typ, charge, status string
	minQ, maxQ, sortOrder                 int32
	maxTotal                              pgtype.Numeric
	channels                              []string
	physical                              bool
}

func fieldsFromRow(r db.GetExtraByUUIDRow) fields {
	return fields{
		code: r.Code, name: r.Name, desc: r.Description, typ: r.ExtraType, charge: r.ChargeType, status: r.Status,
		minQ: r.MinQuantity, maxQ: r.MaxQuantity, sortOrder: r.SortOrder, maxTotal: r.MaxTotal,
		channels: r.Channels, physical: r.IsPhysical,
	}
}

func (s *Service) apply(ve *ValidationError, f *fields, in model.ExtraInput, create bool) {
	if in.Code != nil || create {
		v := ""
		if in.Code != nil {
			v = strings.TrimSpace(*in.Code)
		}
		switch {
		case v == "":
			ve.add("code", "required", "code is required")
		case len([]rune(v)) > 32:
			ve.add("code", "too_long", "code is at most 32 characters")
		}
		f.code = strings.ToUpper(v)
	}
	if in.Name != nil || create {
		v := ""
		if in.Name != nil {
			v = strings.TrimSpace(*in.Name)
		}
		switch {
		case v == "":
			ve.add("name", "required", "name is required")
		case len([]rune(v)) > 255:
			ve.add("name", "too_long", "name is at most 255 characters")
		}
		f.name = v
	}
	if in.Description != nil {
		f.desc = strings.TrimSpace(*in.Description)
		if len([]rune(f.desc)) > 2000 {
			ve.add("description", "too_long", "description is at most 2000 characters")
		}
	}
	if in.ExtraType != nil || create {
		if in.ExtraType == nil || !slices.Contains(model.Types, *in.ExtraType) {
			ve.add("extra_type", "invalid", "extra_type is not a known type")
		} else {
			f.typ = *in.ExtraType
		}
	}
	if in.ChargeType != nil || create {
		if in.ChargeType == nil || !slices.Contains(model.ChargeTypes, *in.ChargeType) {
			ve.add("charge_type", "invalid", "charge_type must be per_day, per_unit or per_rental")
		} else {
			f.charge = *in.ChargeType
		}
	}
	if in.MinQuantity != nil {
		f.minQ = int32(*in.MinQuantity)
	}
	if in.MaxQuantity != nil {
		f.maxQ = int32(*in.MaxQuantity)
	}
	if f.charge == model.ChargePerRental {
		f.minQ, f.maxQ = 1, 1
	}
	if f.minQ < 1 || f.minQ > model.MaxQuantity {
		ve.add("min_quantity", "out_of_range", "min_quantity must be between 1 and 100")
	}
	if f.maxQ < 1 || f.maxQ > model.MaxQuantity {
		ve.add("max_quantity", "out_of_range", "max_quantity must be between 1 and 100")
	} else if f.maxQ < f.minQ {
		ve.add("max_quantity", "below_min", "max_quantity must not be below min_quantity")
	}
	if in.MaxTotal.Set {
		if in.MaxTotal.Value == "" {
			f.maxTotal = pgtype.Numeric{}
		} else if m, ok := parsePrice(ve, "max_total", in.MaxTotal.Value); ok {
			f.maxTotal = m.Numeric()
		}
	}
	if in.Channels.Set {
		f.channels = nil
		if in.Channels.Value != nil {
			f.channels = []string{}
			for i, c := range in.Channels.Value {
				if !slices.Contains(osmodel.SalesChannels, c) {
					ve.add(fmt.Sprintf("channels.%d", i), "invalid", "channel is not a sales channel")
					continue
				}
				if !slices.Contains(f.channels, c) {
					f.channels = append(f.channels, c)
				}
			}
		}
	}
	if in.Status != nil {
		if !slices.Contains(model.Statuses, *in.Status) {
			ve.add("status", "invalid", "status must be draft, published or archived")
		} else {
			f.status = *in.Status
		}
	}
	if in.IsPhysical != nil {
		f.physical = *in.IsPhysical
	}
	if f.physical && f.typ == model.TypeAdditionalDriver {
		ve.add("is_physical", "invalid", "an additional driver is not physical equipment")
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > 10000 {
			ve.add("sort_order", "out_of_range", "sort_order must be between 0 and 10000")
		}
		f.sortOrder = int32(*in.SortOrder)
	}
}

// Create adds an extra (and its default price when given).
func (s *Service) Create(ctx context.Context, orgID int64, in model.ExtraInput) (model.Extra, error) {
	ve := &ValidationError{}
	f := fields{status: model.StatusDraft, minQ: 1, maxQ: 1}
	s.apply(ve, &f, in, true)
	var price *money.Money
	if in.DefaultPrice != nil && *in.DefaultPrice != "" {
		if m, ok := parsePrice(ve, "default_price", *in.DefaultPrice); ok {
			price = &m
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Extra{}, err
	}
	var id uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		row, err := q.CreateExtra(ctx, db.CreateExtraParams{
			OrganizationID: orgID, Code: f.code, Name: f.name, Description: f.desc, ExtraType: f.typ,
			ChargeType: f.charge, MinQuantity: f.minQ, MaxQuantity: f.maxQ, MaxTotal: f.maxTotal,
			Channels: f.channels, Status: f.status, IsPhysical: f.physical, SortOrder: f.sortOrder,
		})
		if err != nil {
			return mapErr(err)
		}
		id = row.Uuid
		if price != nil {
			if _, err := q.InsertExtraPrice(ctx, db.InsertExtraPriceParams{
				OrganizationID: orgID, ExtraID: row.ID, Price: price.Numeric(), ValidFrom: ts(s.clock()), CreatedBy: actorArg(ctx),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Extra{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Patch changes an extra (absent fields unchanged).
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.ExtraInput) (model.Extra, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return model.Extra{}, err
	}
	ve := &ValidationError{}
	f := fieldsFromRow(row)
	s.apply(ve, &f, in, false)
	if in.DefaultPrice != nil {
		ve.add("default_price", "not_allowed", "change prices through PUT /prices")
	}
	if err := ve.orNil(); err != nil {
		return model.Extra{}, err
	}
	n, err := s.q.UpdateExtra(ctx, db.UpdateExtraParams{
		OrganizationID: orgID, ID: row.ID, Code: f.code, Name: f.name, Description: f.desc, ExtraType: f.typ,
		ChargeType: f.charge, MinQuantity: f.minQ, MaxQuantity: f.maxQ, MaxTotal: f.maxTotal,
		Channels: f.channels, Status: f.status, IsPhysical: f.physical, SortOrder: f.sortOrder,
	})
	if err := rows(n, err); err != nil {
		return model.Extra{}, err
	}
	return s.Get(ctx, orgID, id)
}

// SetStatus changes the publish status (bulk); returns the previous one.
func (s *Service) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, status string) (string, error) {
	if !slices.Contains(model.Statuses, status) {
		return "", validation("status", "invalid", "status must be draft, published or archived")
	}
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return "", err
	}
	n, err := s.q.SetExtraStatus(ctx, db.SetExtraStatusParams{OrganizationID: orgID, ID: row.ID, Status: status})
	return row.Status, rows(n, err)
}

// Delete soft-deletes an extra (prices and allocations stay for history).
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return err
	}
	n, err := s.q.SoftDeleteExtra(ctx, db.SoftDeleteExtraParams{OrganizationID: orgID, ID: row.ID})
	return rows(n, err)
}

// Activity lists the audit events of one extra.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getExtra(ctx, s.q, orgID, id); err != nil {
		return nil, 0, err
	}
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ru := pgtype.UUID{Bytes: id, Valid: true}
	list, err := s.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(list))
	for _, r := range list {
		e := model.ActivityEntry{UUID: r.Uuid, Action: r.Action, Payload: json.RawMessage(r.Payload), CreatedAt: r.CreatedAt.Time.UTC()}
		if len(e.Payload) == 0 {
			e.Payload = json.RawMessage("{}")
		}
		e.Actor = actorFrom(r.ActorUuid, r.ActorName, r.ActorSurname, r.ActorEmail)
		out = append(out, e)
	}
	return out, total, nil
}

// ------------------------------------------------------------------ prices

// Prices answers the price matrix of an extra: default, every product with
// its own price (or the fallback) and the scheduled changes.
func (s *Service) Prices(ctx context.Context, orgID int64, id uuid.UUID) (model.PriceMatrix, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return model.PriceMatrix{}, err
	}
	now := ts(s.clock())
	out := model.PriceMatrix{Items: []model.ProductPrice{}, Scheduled: []model.ScheduledPrice{}}
	if p, err := s.q.GetEffectiveExtraPrice(ctx, db.GetEffectiveExtraPriceParams{OrganizationID: orgID, ExtraID: row.ID, NowAt: now}); err == nil {
		out.Default = &model.PriceRow{UUID: p.Uuid, Price: money.FromNumericOrZero(p.Price), ValidFrom: p.ValidFrom.Time.UTC(), ValidTo: timePtr(p.ValidTo)}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.PriceMatrix{}, err
	}
	matrix, err := s.q.ListExtraPriceMatrix(ctx, db.ListExtraPriceMatrixParams{OrganizationID: orgID, ExtraID: row.ID, NowAt: now})
	if err != nil {
		return model.PriceMatrix{}, err
	}
	for _, m := range matrix {
		item := model.ProductPrice{
			Product: model.ProductRef{
				UUID: m.ProductUuid, Code: m.ProductCode, Title: m.ProductTitle, Status: m.ProductStatus,
				SegmentUUID: m.SegmentUuid, SegmentName: m.SegmentName,
			},
			Source: "none",
		}
		switch {
		case m.PriceUuid.Valid:
			p := money.FromNumericOrZero(m.Price)
			item.Price = &model.PriceRow{UUID: m.PriceUuid.Bytes, Price: p, ValidFrom: m.ValidFrom.Time.UTC(), ValidTo: timePtr(m.ValidTo)}
			item.Effective, item.Source = &p, "product"
		case out.Default != nil:
			p := out.Default.Price
			item.Effective, item.Source = &p, "default"
		}
		out.Items = append(out.Items, item)
	}
	scheduled, err := s.q.ListScheduledExtraPrices(ctx, db.ListScheduledExtraPricesParams{OrganizationID: orgID, ExtraID: row.ID, NowAt: now})
	if err != nil {
		return model.PriceMatrix{}, err
	}
	for _, sp := range scheduled {
		item := model.ScheduledPrice{UUID: sp.Uuid, Price: money.FromNumericOrZero(sp.Price), ValidFrom: sp.ValidFrom.Time.UTC(), ValidTo: timePtr(sp.ValidTo)}
		if sp.ProductUuid.Valid {
			u := uuid.UUID(sp.ProductUuid.Bytes)
			item.ProductUUID = &u
		}
		out.Scheduled = append(out.Scheduled, item)
	}
	return out, nil
}

// priceChange is one validated price write (product 0 = default; price nil
// = remove).
type priceChange struct {
	productID int64
	price     *money.Money
}

// SetPrices writes the listed price rows from EffectiveFrom (default now):
// the row effective then is closed, rows that would start later are
// dropped (never quoted), and the new row starts at EffectiveFrom. Rows
// never change in place, so quotes keep the price row uuid they used.
func (s *Service) SetPrices(ctx context.Context, orgID int64, id uuid.UUID, in model.PricesInput) (model.PriceMatrix, int, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return model.PriceMatrix{}, 0, err
	}
	ve := &ValidationError{}
	now := s.clock()
	from := now
	if in.EffectiveFrom != nil {
		from = in.EffectiveFrom.UTC().Truncate(time.Microsecond)
		if from.Before(now) {
			ve.add("effective_from", "in_past", "effective_from must not be in the past")
		}
	}
	changes := []priceChange{}
	if in.DefaultPrice.Set {
		c := priceChange{}
		if in.DefaultPrice.Value != "" {
			if m, ok := parsePrice(ve, "default_price", in.DefaultPrice.Value); ok {
				c.price = &m
			}
		}
		changes = append(changes, c)
	}
	seen := map[int64]bool{}
	for i, item := range in.Items {
		field := fmt.Sprintf("items.%d", i)
		if item.RentalProductUUID == nil {
			ve.add(field+".rental_product_uuid", "required", "rental_product_uuid is required")
			continue
		}
		p, err := s.q.GetPricingProductByUUID(ctx, db.GetPricingProductByUUIDParams{OrganizationID: orgID, Uuid: *item.RentalProductUUID})
		if err != nil {
			ve.add(field+".rental_product_uuid", "not_found", "rental product was not found")
			continue
		}
		if seen[p.ID] {
			ve.add(field+".rental_product_uuid", "duplicate", "the product is listed twice")
			continue
		}
		seen[p.ID] = true
		c := priceChange{productID: p.ID}
		if item.Price != nil && *item.Price != "" {
			if m, ok := parsePrice(ve, field+".price", *item.Price); ok {
				c.price = &m
			}
		}
		changes = append(changes, c)
	}
	if len(changes) == 0 {
		ve.add("items", "required", "nothing to change")
	}
	if err := ve.orNil(); err != nil {
		return model.PriceMatrix{}, 0, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		for _, c := range changes {
			product := pgtype.Int8{Int64: c.productID, Valid: c.productID != 0}
			if _, err := q.DeleteFutureExtraPrices(ctx, db.DeleteFutureExtraPricesParams{
				OrganizationID: orgID, ExtraID: row.ID, RentalProductID: product, FromAt: ts(from),
			}); err != nil {
				return err
			}
			if _, err := q.CloseExtraPrice(ctx, db.CloseExtraPriceParams{
				OrganizationID: orgID, ExtraID: row.ID, RentalProductID: product, FromAt: ts(from),
			}); err != nil {
				return mapErr(err)
			}
			if c.price == nil {
				continue
			}
			if _, err := q.InsertExtraPrice(ctx, db.InsertExtraPriceParams{
				OrganizationID: orgID, ExtraID: row.ID, RentalProductID: product, Price: c.price.Numeric(),
				ValidFrom: ts(from), CreatedBy: actorArg(ctx),
			}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
	if err != nil {
		return model.PriceMatrix{}, 0, err
	}
	m, err := s.Prices(ctx, orgID, id)
	return m, len(changes), err
}

// PriceHistory lists every price row of an extra, newest first.
func (s *Service) PriceHistory(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.PriceHistoryEntry, int64, error) {
	row, err := s.getExtra(ctx, s.q, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	list, err := s.q.ListExtraPriceHistory(ctx, db.ListExtraPriceHistoryParams{OrganizationID: orgID, ExtraID: row.ID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountExtraPriceHistory(ctx, db.CountExtraPriceHistoryParams{OrganizationID: orgID, ExtraID: row.ID})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.PriceHistoryEntry, 0, len(list))
	for _, r := range list {
		e := model.PriceHistoryEntry{
			UUID: r.Uuid, Price: money.FromNumericOrZero(r.Price), ValidFrom: r.ValidFrom.Time.UTC(), ValidTo: timePtr(r.ValidTo),
			CreatedAt: r.CreatedAt.Time.UTC(), Actor: actorFrom(r.ActorUuid, r.ActorName, r.ActorSurname, r.ActorEmail),
		}
		if r.ProductUuid.Valid {
			e.Product = &model.ProductRef{UUID: r.ProductUuid.Bytes, Code: r.ProductCode.String, Title: r.ProductTitle.String}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// --------------------------------------------------------------- helpers

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func moneyPtr(n pgtype.Numeric) *money.Money {
	if !n.Valid {
		return nil
	}
	m := money.FromNumericOrZero(n)
	return &m
}

func textArg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func strs(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// actorArg is the signed-in user of a request context (null in workers).
func actorArg(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func actorFrom(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

// parsePrice parses a non-negative amount with at most 2 decimals.
func parsePrice(ve *ValidationError, field string, d model.Decimal) (money.Money, bool) {
	m, err := money.Parse(string(d))
	if err != nil || m.IsNegative() || m.Cmp(money.MustParse("999999999999.99")) > 0 {
		ve.add(field, "invalid", field+" must be a non-negative amount with at most 2 decimals")
		return money.Money{}, false
	}
	return m, true
}

func rows(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_extras_organization_code" {
		return ErrCodeTaken
	}
	return err
}

// branchInScope resolves a branch uuid inside the organization and the
// caller's branch scope ("" field code: not found).
func (s *Service) branchInScope(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.Branch, bool) {
	b, err := q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
		return db.Branch{}, false
	}
	return b, true
}
