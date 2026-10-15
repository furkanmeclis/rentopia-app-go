// Package usecase holds the campaign rules (TEC-703): coupon codes and
// repeat-customer campaigns, the eligibility check that feeds the pricing
// engine's single discount (Evaluate), and the redemption API reservations
// (TEC-706) build on (Redeem / Consume / Release), with the atomic usage
// counter. docs/design/campaigns.md.
//
// Campaigns are organization-wide configuration (no branch scope). Every
// method takes the organization id explicitly.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxListTargets caps exports and bulk "all matching".
const MaxListTargets = 10000

var (
	ErrNotFound  = errors.New("campaign not found")
	ErrCodeTaken = errors.New("a live campaign already has this code")
	// ErrAlreadyRedeemed: the reservation already carries a discount (one
	// campaign per reservation).
	ErrAlreadyRedeemed = errors.New("the reservation already has a discount")
	// ErrRedemptionNotFound: the reservation has no live redemption.
	ErrRedemptionNotFound = errors.New("redemption not found")
)

// RejectionError is a campaign that does not apply (Redeem); Reason is one
// of model.ReasonX.
type RejectionError struct {
	Reason string
	Params map[string]string
}

func (e *RejectionError) Error() string { return "campaign rejected: " + e.Reason }

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

// CompletedRentalCounter answers how many completed rentals a customer has
// in the organization (repeat-customer campaigns). Rentals arrive in M3;
// until then ZeroRentalCounter answers 0, so repeat-customer campaigns
// never apply.
type CompletedRentalCounter interface {
	CompletedRentals(ctx context.Context, orgID int64, customer uuid.UUID) (int, error)
}

// ZeroRentalCounter is the M2 counter: nobody has completed a rental yet.
type ZeroRentalCounter struct{}

// CompletedRentals implements CompletedRentalCounter.
func (ZeroRentalCounter) CompletedRentals(context.Context, int64, uuid.UUID) (int, error) {
	return 0, nil
}

// Service implements the campaign use cases.
type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	rentals CompletedRentalCounter
	now     func() time.Time
}

// New builds the service with the zero rental counter.
func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q, rentals: ZeroRentalCounter{}, now: time.Now}
}

// SetRentalCounter plugs the completed-rental counter (M3 rentals).
func (s *Service) SetRentalCounter(c CompletedRentalCounter) { s.rentals = c }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) today() pgtype.Date {
	return pgtype.Date{Time: rentaltime.LocalDate(s.clock()).Time(), Valid: true}
}

// tx runs fn in one transaction.
func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("campaigns: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "code": "code", "start_date": "start_date", "end_date": "end_date",
	"used_count": "used_count", "created_at": "created_at", "updated_at": "updated_at",
}

// ------------------------------------------------------------------ reads

type row = db.GetCampaignByUUIDRow

func rowFromList(r db.ListCampaignsRow) row {
	return row{
		ID: r.ID, Uuid: r.Uuid, Name: r.Name, Description: r.Description, CampaignType: r.CampaignType, Code: r.Code,
		DiscountKind: r.DiscountKind, DiscountValue: r.DiscountValue, MaxDiscount: r.MaxDiscount, MinSubtotal: r.MinSubtotal,
		StartDate: r.StartDate, EndDate: r.EndDate, Channels: r.Channels, ProductScope: r.ProductScope,
		CustomerUuids: r.CustomerUuids, UsageLimit: r.UsageLimit, PerCustomerLimit: r.PerCustomerLimit, UsedCount: r.UsedCount,
		MinCompletedRentals: r.MinCompletedRentals, ReleaseOnCancel: r.ReleaseOnCancel, CorporateEligible: r.CorporateEligible,
		IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// stateOf computes the state on a day (same rules as the SQL CASE).
func stateOf(r row, today rentaltime.Date) string {
	switch {
	case !r.IsActive:
		return model.StateInactive
	case r.EndDate.Valid && rentaltime.DateOf(r.EndDate.Time).Before(today):
		return model.StateExpired
	case r.StartDate.Valid && rentaltime.DateOf(r.StartDate.Time).After(today):
		return model.StateScheduled
	case r.UsageLimit.Valid && r.UsedCount >= r.UsageLimit.Int32:
		return model.StateExhausted
	}
	return model.StateActive
}

func campaignFrom(r row, targets []db.ListCampaignTargetsRow, today rentaltime.Date) model.Campaign {
	c := model.Campaign{
		UUID: r.Uuid, Name: r.Name, Description: r.Description, CampaignType: r.CampaignType,
		DiscountKind: r.DiscountKind, DiscountValue: money.FromNumericOrZero(r.DiscountValue),
		MaxDiscount: moneyPtr(r.MaxDiscount), MinSubtotal: moneyPtr(r.MinSubtotal),
		StartDate: datePtr(r.StartDate), EndDate: datePtr(r.EndDate), Channels: nilOrCopy(r.Channels),
		ProductScope: r.ProductScope, Products: []model.ProductRef{}, Segments: []model.SegmentRef{},
		UsageLimit: int4Ptr(r.UsageLimit), PerCustomerLimit: int4Ptr(r.PerCustomerLimit), UsedCount: int(r.UsedCount),
		MinCompletedRentals: int(r.MinCompletedRentals), ReleaseOnCancel: r.ReleaseOnCancel,
		CorporateEligible: r.CorporateEligible, IsActive: r.IsActive, State: stateOf(r, today),
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.CustomerUuids != nil {
		c.CustomerUUIDs = append([]uuid.UUID{}, r.CustomerUuids...)
	}
	if r.Code.Valid {
		v := r.Code.String
		c.Code = &v
	}
	if c.UsageLimit != nil {
		left := max(0, *c.UsageLimit-c.UsedCount)
		c.Remaining = &left
	}
	for _, t := range targets {
		if t.CampaignID != r.ID {
			continue
		}
		if t.ProductUuid.Valid {
			c.Products = append(c.Products, model.ProductRef{UUID: t.ProductUuid.Bytes, Code: t.ProductCode.String, Title: t.ProductTitle.String})
		}
		if t.SegmentUuid.Valid {
			c.Segments = append(c.Segments, model.SegmentRef{UUID: t.SegmentUuid.Bytes, Code: t.SegmentCode.String, Name: t.SegmentName.String})
		}
	}
	return c
}

func (s *Service) getRow(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (row, error) {
	r, err := q.GetCampaignByUUID(ctx, db.GetCampaignByUUIDParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// Get loads one campaign with its targets.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Campaign, error) {
	r, err := s.getRow(ctx, s.q, orgID, id)
	if err != nil {
		return model.Campaign{}, err
	}
	targets, err := s.q.ListCampaignTargets(ctx, db.ListCampaignTargetsParams{OrganizationID: orgID, CampaignIds: []int64{r.ID}})
	if err != nil {
		return model.Campaign{}, err
	}
	return campaignFrom(r, targets, rentaltime.LocalDate(s.clock())), nil
}

// List lists campaigns.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Campaign, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 25
	}
	if f.SortField == "" {
		f.SortField, f.SortDesc = "created_at", true
	}
	today := s.today()
	list, err := s.q.ListCampaigns(ctx, db.ListCampaignsParams{
		Today: today, OrganizationID: orgID, Types: strs(f.Types), Kinds: strs(f.Kinds), States: strs(f.States),
		Channel: textArg(f.Channel), Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCampaigns(ctx, db.CountCampaignsParams{
		Today: today, OrganizationID: orgID, Types: strs(f.Types), Kinds: strs(f.Kinds), States: strs(f.States),
		Channel: textArg(f.Channel), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(list))
	for _, r := range list {
		ids = append(ids, r.ID)
	}
	targets, err := s.q.ListCampaignTargets(ctx, db.ListCampaignTargetsParams{OrganizationID: orgID, CampaignIds: ids})
	if err != nil {
		return nil, 0, err
	}
	day := rentaltime.LocalDate(s.clock())
	out := make([]model.Campaign, 0, len(list))
	for _, r := range list {
		out = append(out, campaignFrom(rowFromList(r), targets, day))
	}
	return out, total, nil
}

// ExportRows lists every matching campaign (exports).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Campaign, error) {
	f.Limit, f.Offset, f.SortField, f.SortDesc = MaxListTargets, 0, "name", false
	out, _, err := s.List(ctx, orgID, f)
	return out, err
}

// TargetUUIDs lists the uuids matching a filter (bulk "all matching").
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	return s.q.ListCampaignUUIDs(ctx, db.ListCampaignUUIDsParams{
		Today: s.today(), OrganizationID: orgID, Types: strs(f.Types), Kinds: strs(f.Kinds), States: strs(f.States),
		Channel: textArg(f.Channel), Q: textArg(f.Q), LimitCount: limit,
	})
}

// Facets answers the dynamic filter options.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	list, err := s.q.CampaignFacets(ctx, db.CampaignFacetsParams{OrganizationID: orgID, Today: s.today()})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{CampaignType: []model.Facet{}, DiscountKind: []model.Facet{}, State: []model.Facet{}}
	for _, r := range list {
		f := model.Facet{Value: r.Value, Count: r.Count}
		switch r.Facet {
		case "campaign_type":
			out.CampaignType = append(out.CampaignType, f)
		case "discount_kind":
			out.DiscountKind = append(out.DiscountKind, f)
		case "state":
			out.State = append(out.State, f)
		}
	}
	return out, nil
}

// Activity lists the audit events of one campaign.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getRow(ctx, s.q, orgID, id); err != nil {
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

// Redemptions lists the redemptions of one campaign.
func (s *Service) Redemptions(ctx context.Context, orgID int64, id uuid.UUID, statuses []string, limit, offset int32) ([]model.Redemption, int64, error) {
	r, err := s.getRow(ctx, s.q, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	for _, st := range statuses {
		if !slices.Contains(model.RedemptionStatuses, st) {
			return nil, 0, &ValidationError{Details: []Detail{{Field: "status", Code: "invalid", Message: "unknown redemption status"}}}
		}
	}
	list, err := s.q.ListCampaignRedemptions(ctx, db.ListCampaignRedemptionsParams{
		OrganizationID: orgID, CampaignID: r.ID, Statuses: strs(statuses), LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCampaignRedemptions(ctx, db.CountCampaignRedemptionsParams{OrganizationID: orgID, CampaignID: r.ID, Statuses: strs(statuses)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Redemption, 0, len(list))
	for _, x := range list {
		red := model.Redemption{
			UUID: x.Uuid, ReservationUUID: x.ReservationUuid, Subtotal: money.FromNumericOrZero(x.Subtotal),
			DiscountAmount: money.FromNumericOrZero(x.DiscountAmount), Status: x.Status, ReleaseReason: x.ReleaseReason,
			UsageReturned: x.UsageReturned, CreatedBy: actorFrom(x.ActorUuid, x.ActorName, x.ActorSurname, x.ActorEmail),
			CreatedAt: x.CreatedAt.Time.UTC(), ConsumedAt: timePtr(x.ConsumedAt), ReleasedAt: timePtr(x.ReleasedAt),
		}
		if x.CustomerUuid.Valid {
			u := uuid.UUID(x.CustomerUuid.Bytes)
			red.CustomerUUID = &u
		}
		if x.Code.Valid {
			v := x.Code.String
			red.Code = &v
		}
		out = append(out, red)
	}
	return out, total, nil
}

// ----------------------------------------------------------------- writes

// fields is a validated campaign.
type fields struct {
	name, desc, typ, kind, scope string
	code                         pgtype.Text
	value, maxDiscount, minSub   pgtype.Numeric
	start, end                   pgtype.Date
	channels                     []string
	customers                    []uuid.UUID
	usageLimit, perCustomer      pgtype.Int4
	minRentals                   int32
	releaseOnCancel, corporate   bool
	active                       bool
	// targets: product / segment ids (nil = keep the stored targets).
	products, segments []int64
	targetsSet         bool
}

func fieldsFromRow(r row) fields {
	return fields{
		name: r.Name, desc: r.Description, typ: r.CampaignType, kind: r.DiscountKind, scope: r.ProductScope, code: r.Code,
		value: r.DiscountValue, maxDiscount: r.MaxDiscount, minSub: r.MinSubtotal, start: r.StartDate, end: r.EndDate,
		channels: r.Channels, customers: r.CustomerUuids, usageLimit: r.UsageLimit, perCustomer: r.PerCustomerLimit,
		minRentals: r.MinCompletedRentals, releaseOnCancel: r.ReleaseOnCancel, corporate: r.CorporateEligible, active: r.IsActive,
	}
}

var codeRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,31}$`)

func (s *Service) apply(ctx context.Context, orgID int64, ve *ValidationError, f *fields, in model.CampaignInput, create bool, used int32) {
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
	if in.CampaignType != nil || create {
		if in.CampaignType == nil || !slices.Contains(model.Types, *in.CampaignType) {
			ve.add("campaign_type", "invalid", "campaign_type must be coupon or repeat_customer")
		} else {
			f.typ = *in.CampaignType
		}
	}
	if in.Code.Set {
		f.code = pgtype.Text{}
		if in.Code.Value != nil {
			if v := strings.ToUpper(strings.TrimSpace(*in.Code.Value)); v != "" {
				f.code = pgtype.Text{String: v, Valid: true}
			}
		}
	}
	switch f.typ {
	case model.TypeCoupon:
		if !f.code.Valid {
			ve.add("code", "required", "a coupon needs a code")
		} else if !codeRE.MatchString(f.code.String) {
			ve.add("code", "invalid", "code is 3-32 letters, digits, - or _")
		}
	case model.TypeRepeatCustomer:
		f.code = pgtype.Text{}
	}
	if in.DiscountKind != nil || create {
		if in.DiscountKind == nil || !slices.Contains(model.Kinds, *in.DiscountKind) {
			ve.add("discount_kind", "invalid", "discount_kind must be percent or amount")
		} else {
			f.kind = *in.DiscountKind
		}
	}
	if in.DiscountValue != nil || create {
		raw := model.Decimal("")
		if in.DiscountValue != nil {
			raw = *in.DiscountValue
		}
		if m, err := money.Parse(string(raw)); err != nil || !m.IsPositive() {
			ve.add("discount_value", "invalid", "discount_value must be a positive amount with at most 2 decimals")
		} else {
			f.value = m.Numeric()
		}
	}
	if f.kind == model.KindPercent && f.value.Valid {
		if money.FromNumericOrZero(f.value).Cmp(money.FromUnits(100)) > 0 {
			ve.add("discount_value", "out_of_range", "a percentage is at most 100")
		}
	}
	optMoney := func(field string, o model.Optional[model.Decimal], dst *pgtype.Numeric, positive bool) {
		if !o.Set {
			return
		}
		*dst = pgtype.Numeric{}
		if o.Value == nil || *o.Value == "" {
			return
		}
		m, err := money.Parse(string(*o.Value))
		if err != nil || m.IsNegative() || (positive && m.IsZero()) {
			ve.add(field, "invalid", field+" must be a valid amount with at most 2 decimals")
			return
		}
		*dst = m.Numeric()
	}
	optMoney("max_discount", in.MaxDiscount, &f.maxDiscount, true)
	optMoney("min_subtotal", in.MinSubtotal, &f.minSub, false)
	optDate := func(field string, o model.Optional[string], dst *pgtype.Date) {
		if !o.Set {
			return
		}
		*dst = pgtype.Date{}
		if o.Value == nil || strings.TrimSpace(*o.Value) == "" {
			return
		}
		d, err := rentaltime.ParseDate(strings.TrimSpace(*o.Value))
		if err != nil {
			ve.add(field, "invalid", field+" must be a date (YYYY-MM-DD)")
			return
		}
		*dst = pgtype.Date{Time: d.Time(), Valid: true}
	}
	optDate("start_date", in.StartDate, &f.start)
	optDate("end_date", in.EndDate, &f.end)
	if f.start.Valid && f.end.Valid && f.end.Time.Before(f.start.Time) {
		ve.add("end_date", "before_start", "end_date must not be before start_date")
	}
	if in.Channels.Set {
		f.channels = nil
		if in.Channels.Value != nil {
			f.channels = []string{}
			for i, c := range *in.Channels.Value {
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
	if in.ProductScope != nil || create {
		v := model.ScopeAll
		if in.ProductScope != nil {
			v = *in.ProductScope
		}
		if !slices.Contains(model.Scopes, v) {
			ve.add("product_scope", "invalid", "product_scope must be all, segments or products")
		} else if v != f.scope {
			f.scope, f.targetsSet = v, true
		}
	}
	if in.RentalProductUUIDs != nil {
		f.targetsSet, f.products = true, []int64{}
		for i, id := range *in.RentalProductUUIDs {
			p, err := s.q.GetCampaignProductByUUID(ctx, db.GetCampaignProductByUUIDParams{OrganizationID: orgID, Uuid: id})
			if err != nil {
				ve.add(fmt.Sprintf("rental_product_uuids.%d", i), "not_found", "rental product was not found")
				continue
			}
			if !slices.Contains(f.products, p.ID) {
				f.products = append(f.products, p.ID)
			}
		}
	}
	if in.SegmentUUIDs != nil {
		f.targetsSet, f.segments = true, []int64{}
		for i, id := range *in.SegmentUUIDs {
			sg, err := s.q.GetCampaignSegmentByUUID(ctx, db.GetCampaignSegmentByUUIDParams{OrganizationID: orgID, Uuid: id})
			if err != nil {
				ve.add(fmt.Sprintf("segment_uuids.%d", i), "not_found", "segment was not found")
				continue
			}
			if !slices.Contains(f.segments, sg.ID) {
				f.segments = append(f.segments, sg.ID)
			}
		}
	}
	if f.targetsSet {
		switch f.scope {
		case model.ScopeAll:
			f.products, f.segments = nil, nil
		case model.ScopeSegments:
			f.products = nil
			if len(f.segments) == 0 && !ve.has("segment_uuids") {
				ve.add("segment_uuids", "required", "pick at least one segment")
			}
		case model.ScopeProducts:
			f.segments = nil
			if len(f.products) == 0 && !ve.has("rental_product_uuids") {
				ve.add("rental_product_uuids", "required", "pick at least one rental product")
			}
		}
	}
	if in.CustomerUUIDs.Set {
		f.customers = nil
		if in.CustomerUUIDs.Value != nil {
			f.customers = []uuid.UUID{}
			for _, id := range *in.CustomerUUIDs.Value {
				if !slices.Contains(f.customers, id) {
					f.customers = append(f.customers, id)
				}
			}
			if len(f.customers) > 1000 {
				ve.add("customer_uuids", "too_many", "at most 1000 customers")
			}
		}
	}
	optInt := func(field string, o model.Optional[int], dst *pgtype.Int4, lo, hi int) {
		if !o.Set {
			return
		}
		*dst = pgtype.Int4{}
		if o.Value == nil {
			return
		}
		if *o.Value < lo || *o.Value > hi {
			ve.add(field, "out_of_range", fmt.Sprintf("%s must be between %d and %d", field, lo, hi))
			return
		}
		*dst = pgtype.Int4{Int32: int32(*o.Value), Valid: true}
	}
	optInt("usage_limit", in.UsageLimit, &f.usageLimit, 1, 10000000)
	optInt("per_customer_limit", in.PerCustomerLimit, &f.perCustomer, 1, 1000)
	if f.usageLimit.Valid && f.usageLimit.Int32 < used {
		ve.add("usage_limit", "below_used", fmt.Sprintf("usage_limit must not be below the %d uses so far", used))
	}
	if in.MinCompletedRentals != nil {
		if *in.MinCompletedRentals < 1 || *in.MinCompletedRentals > 1000 {
			ve.add("min_completed_rentals", "out_of_range", "min_completed_rentals must be between 1 and 1000")
		}
		f.minRentals = int32(*in.MinCompletedRentals)
	}
	if in.ReleaseOnCancel != nil {
		f.releaseOnCancel = *in.ReleaseOnCancel
	}
	if in.CorporateEligible != nil {
		f.corporate = *in.CorporateEligible
	}
	if in.IsActive != nil {
		f.active = *in.IsActive
	}
}

func (e *ValidationError) has(prefix string) bool {
	for _, d := range e.Details {
		if strings.HasPrefix(d.Field, prefix) {
			return true
		}
	}
	return false
}

func (s *Service) writeTargets(ctx context.Context, q *db.Queries, orgID, campaignID int64, f fields) error {
	if !f.targetsSet {
		return nil
	}
	if err := q.DeleteCampaignTargets(ctx, db.DeleteCampaignTargetsParams{OrganizationID: orgID, CampaignID: campaignID}); err != nil {
		return err
	}
	for _, id := range f.products {
		if err := q.InsertCampaignTarget(ctx, db.InsertCampaignTargetParams{
			OrganizationID: orgID, CampaignID: campaignID, RentalProductID: pgtype.Int8{Int64: id, Valid: true},
		}); err != nil {
			return err
		}
	}
	for _, id := range f.segments {
		if err := q.InsertCampaignTarget(ctx, db.InsertCampaignTargetParams{
			OrganizationID: orgID, CampaignID: campaignID, SegmentID: pgtype.Int8{Int64: id, Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

// Create adds a campaign.
func (s *Service) Create(ctx context.Context, orgID int64, in model.CampaignInput) (model.Campaign, error) {
	ve := &ValidationError{}
	f := fields{scope: "", minRentals: 1, releaseOnCancel: true, active: true}
	s.apply(ctx, orgID, ve, &f, in, true, 0)
	if err := ve.orNil(); err != nil {
		return model.Campaign{}, err
	}
	var id uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		r, err := q.CreateCampaign(ctx, db.CreateCampaignParams{
			OrganizationID: orgID, Name: f.name, Description: f.desc, CampaignType: f.typ, Code: f.code,
			DiscountKind: f.kind, DiscountValue: f.value, MaxDiscount: f.maxDiscount, MinSubtotal: f.minSub,
			StartDate: f.start, EndDate: f.end, Channels: f.channels, ProductScope: f.scope, CustomerUuids: f.customers,
			UsageLimit: f.usageLimit, PerCustomerLimit: f.perCustomer, MinCompletedRentals: f.minRentals,
			ReleaseOnCancel: f.releaseOnCancel, CorporateEligible: f.corporate, IsActive: f.active, CreatedBy: actorArg(ctx),
		})
		if err != nil {
			return mapErr(err)
		}
		id = r.Uuid
		return s.writeTargets(ctx, q, orgID, r.ID, f)
	})
	if err != nil {
		return model.Campaign{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Patch changes a campaign (absent fields unchanged).
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.CampaignInput) (model.Campaign, error) {
	r, err := s.getRow(ctx, s.q, orgID, id)
	if err != nil {
		return model.Campaign{}, err
	}
	ve := &ValidationError{}
	f := fieldsFromRow(r)
	s.apply(ctx, orgID, ve, &f, in, false, r.UsedCount)
	if err := ve.orNil(); err != nil {
		return model.Campaign{}, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.UpdateCampaign(ctx, db.UpdateCampaignParams{
			OrganizationID: orgID, ID: r.ID, Name: f.name, Description: f.desc, CampaignType: f.typ, Code: f.code,
			DiscountKind: f.kind, DiscountValue: f.value, MaxDiscount: f.maxDiscount, MinSubtotal: f.minSub,
			StartDate: f.start, EndDate: f.end, Channels: f.channels, ProductScope: f.scope, CustomerUuids: f.customers,
			UsageLimit: f.usageLimit, PerCustomerLimit: f.perCustomer, MinCompletedRentals: f.minRentals,
			ReleaseOnCancel: f.releaseOnCancel, CorporateEligible: f.corporate, IsActive: f.active,
		})
		if err := rows(n, err); err != nil {
			return err
		}
		return s.writeTargets(ctx, q, orgID, r.ID, f)
	})
	if err != nil {
		return model.Campaign{}, err
	}
	return s.Get(ctx, orgID, id)
}

// SetActive switches a campaign on / off (bulk); returns the previous
// value.
func (s *Service) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	r, err := s.getRow(ctx, s.q, orgID, id)
	if err != nil {
		return false, err
	}
	n, err := s.q.SetCampaignActive(ctx, db.SetCampaignActiveParams{OrganizationID: orgID, ID: r.ID, IsActive: active})
	return r.IsActive, rows(n, err)
}

// Delete soft-deletes a campaign (redemptions stay).
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	r, err := s.getRow(ctx, s.q, orgID, id)
	if err != nil {
		return err
	}
	n, err := s.q.SoftDeleteCampaign(ctx, db.SoftDeleteCampaignParams{OrganizationID: orgID, ID: r.ID})
	return rows(n, err)
}

// --------------------------------------------------------------- helpers

func moneyPtr(n pgtype.Numeric) *money.Money {
	if !n.Valid {
		return nil
	}
	m := money.FromNumericOrZero(n)
	return &m
}

func datePtr(d pgtype.Date) *rentaltime.Date {
	if !d.Valid {
		return nil
	}
	v := rentaltime.DateOf(d.Time)
	return &v
}

func int4Ptr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func nilOrCopy(v []string) []string {
	if v == nil {
		return nil
	}
	return append([]string{}, v...)
}

func textArg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func strs(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

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
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "uq_campaigns_organization_code":
			return ErrCodeTaken
		case "uq_campaign_redemptions_organization_reservation":
			return ErrAlreadyRedeemed
		}
	}
	return err
}
