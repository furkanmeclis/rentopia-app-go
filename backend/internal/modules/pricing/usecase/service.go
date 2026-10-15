// Package usecase holds the pricing business rules: rate plans with
// duration bands (TEC-700), named date rules (TEC-701), the product × band
// matrix, the price list import and the preview, which feeds the pure
// engine (internal/platform/pricing). Every method takes the organization
// id of the caller explicitly (orgctx.Scope in handlers, the job in
// workers).
//
// Versioning (docs/design/pricing.md): writes only touch the single draft
// version of a plan / rule (created from the active version on the first
// edit); activation validates and freezes the draft, the previous active
// version becomes superseded. Activation needs tenant.pricing.activate.
//
// Branch scope: a plan bound to a branch (scope branch / branch_channel) is
// visible to members whose scope covers the branch; common and channel
// plans and date rules are organization-wide configuration.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxListTargets caps exports.
const MaxListTargets = 10000

var (
	ErrPlanNotFound = errors.New("rate plan not found")
	ErrRuleNotFound = errors.New("date rule not found")
	ErrNameTaken    = errors.New("a live date rule already has this name")
	// ErrState is answered with 409: the action does not fit the record's
	// state (discarding the only draft, deactivating a draft …).
	ErrState = errors.New("invalid state")
	// ErrConcurrent is answered with 409: someone else changed the record.
	ErrConcurrent = errors.New("the record was changed by someone else; reload and retry")
)

// StateError is ErrState with a stable code.
type StateError struct {
	Code    string
	Message string
}

func (e *StateError) Error() string { return e.Message }
func (e *StateError) Unwrap() error { return ErrState }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
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

func (e *ValidationError) has(field string) bool {
	for _, d := range e.Details {
		if d.Field == field {
			return true
		}
	}
	return false
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// ActivationError lists why activation was refused (handlers answer 422).
type ActivationError struct {
	Issues []model.Issue
}

func (e *ActivationError) Error() string {
	if len(e.Issues) == 0 {
		return "activation refused"
	}
	return e.Issues[0].Field + ": " + e.Issues[0].Message
}

// SettingsReader reads the organization settings (VAT rate, prepayment,
// sales channels, max online rental length).
type SettingsReader interface {
	Current(ctx context.Context, orgID int64) (osmodel.Version, error)
}

// OneWayResolver answers the one-way fee of a route (fleetops). allowed
// false = the return branch is not allowed for the product.
type OneWayResolver interface {
	ResolveOneWayFee(ctx context.Context, orgID int64, from, to, product uuid.UUID) (allowed bool, fee money.Money, err error)
}

// ExtrasResolver turns selected extras into engine lines (TEC-702,
// extras module): the price of the product effective at the quote
// instant, the charge type, the quantity bounds and the line cap. Problems
// with a selection come back as details on "extras.N.*".
type ExtrasResolver interface {
	ResolveExtras(ctx context.Context, orgID, productID int64, channel string, items []model.PreviewExtraInput, at time.Time) ([]pricing.Extra, []Detail, error)
}

// CouponQuery is what a campaign is checked against.
type CouponQuery struct {
	Code                 string
	ProductID, SegmentID int64
	Channel              string
	Subtotal             money.Money
	CustomerUUID         *uuid.UUID
	At                   time.Time
}

// CouponEvaluator checks a coupon code / automatic campaign (TEC-703,
// campaigns module) without consuming it. discount nil = nothing to apply
// (result says why when a code was given).
type CouponEvaluator interface {
	EvaluateCoupon(ctx context.Context, orgID int64, q CouponQuery) (*pricing.Discount, *model.CouponResult, error)
}

// Service implements the pricing use cases.
type Service struct {
	repo     *repository.Postgres
	settings SettingsReader
	oneWay   OneWayResolver
	extras   ExtrasResolver
	coupons  CouponEvaluator
	now      func() time.Time
}

// SetExtrasResolver plugs the extras catalog into the preview / quote.
func (s *Service) SetExtrasResolver(r ExtrasResolver) { s.extras = r }

// SetCouponEvaluator plugs the campaigns into the preview / quote.
func (s *Service) SetCouponEvaluator(c CouponEvaluator) { s.coupons = c }

// New builds the service. settings may be nil (defaults: no VAT split,
// no prepayment, every channel inactive except branch).
func New(repo *repository.Postgres, settings SettingsReader) *Service {
	return &Service{repo: repo, settings: settings, now: time.Now}
}

// SetOneWayResolver plugs the one-way fee lookup into the preview.
func (s *Service) SetOneWayResolver(r OneWayResolver) { s.oneWay = r }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// orgPricing is what the settings contribute to pricing.
type orgPricing struct {
	VAT        *money.Rate
	Prepayment pricing.Prepayment
	FullAllow  bool
	Active     map[string]bool
	MaxOnline  int
}

// anyOnline reports whether an online channel is switched on.
func (p orgPricing) anyOnline() bool {
	for code, on := range p.Active {
		if on && osmodel.IsOnlineChannel(code) {
			return true
		}
	}
	return false
}

func (s *Service) orgPricing(ctx context.Context, orgID int64) (orgPricing, []string, error) {
	cfg := osmodel.Defaults()
	if s.settings != nil {
		v, err := s.settings.Current(ctx, orgID)
		if err != nil {
			return orgPricing{}, nil, err
		}
		cfg = v.Settings
	}
	out := orgPricing{Active: map[string]bool{}, MaxOnline: cfg.Reservation.MaxOnlineDays(), FullAllow: cfg.Payment.FullPaymentAllowed}
	warnings := []string{}
	for _, c := range cfg.SalesChannels {
		out.Active[c.Code] = c.IsActive
	}
	if raw := strings.TrimSpace(cfg.Tax.DefaultVATRate); raw != "" {
		if r, err := money.ParseRate(raw); err == nil {
			out.VAT = &r
		}
	}
	if out.VAT == nil {
		warnings = append(warnings, "vat_rate_missing")
	}
	switch cfg.Payment.PrepaymentRule {
	case osmodel.PrepaymentFixed:
		if m, err := money.Parse(cfg.Payment.PrepaymentValue); err == nil {
			out.Prepayment = pricing.Prepayment{Rule: pricing.PrepaymentFixed, Amount: m}
		}
	case osmodel.PrepaymentPercent:
		if r, err := money.ParseRate(cfg.Payment.PrepaymentValue); err == nil {
			out.Prepayment = pricing.Prepayment{Rule: pricing.PrepaymentPercent, Percent: r}
		}
	default:
		out.Prepayment = pricing.Prepayment{Rule: pricing.PrepaymentNone}
	}
	return out, warnings, nil
}

// validChannel reports whether code is a settings sales channel.
func validChannel(code string) bool {
	return slices.Contains(osmodel.SalesChannels, code)
}

// --------------------------------------------------------------- helpers

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

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	if errors.Is(err, repository.ErrConcurrentDraft) || errors.Is(err, repository.ErrFrozen) {
		return ErrConcurrent
	}
	return err
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func int8Arg(v int64, ok bool) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: ok} }

func int4Ptr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func int4Arg(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func textArg(v string) pgtype.Text {
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

func dateArg(d *rentaltime.Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func dateOf(d pgtype.Date) rentaltime.Date {
	return rentaltime.DateOf(d.Time)
}

func strs(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func uuids(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

func checkPriority(ve *ValidationError, field string, v *int) int {
	if v == nil {
		return 0
	}
	if *v < 0 || *v > model.MaxPriority {
		ve.add(field, "out_of_range", "priority must be between 0 and 1000")
		return 0
	}
	return *v
}

func checkText(ve *ValidationError, field, v string, max int) string {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > max {
		ve.add(field, "too_long", field+" is too long")
	}
	return v
}

// parsePrice parses a non-negative amount with at most 2 decimals.
func parsePrice(ve *ValidationError, field string, d model.Decimal) (money.Money, bool) {
	m, err := money.Parse(string(d))
	if err != nil || m.IsNegative() {
		ve.add(field, "invalid", field+" must be a non-negative amount with at most 2 decimals")
		return money.Money{}, false
	}
	return m, true
}

func activityEntries(rows []db.ListOrganizationResourceActivityRow) []model.ActivityEntry {
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		entry := model.ActivityEntry{
			UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload),
			CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if len(entry.Payload) == 0 {
			entry.Payload = json.RawMessage("{}")
		}
		entry.Actor = actorFrom(row.ActorUuid, row.ActorName, row.ActorSurname, row.ActorEmail)
		out = append(out, entry)
	}
	return out
}

func issuesFrom(in []pricing.Issue, prefix string) []model.Issue {
	out := make([]model.Issue, 0, len(in))
	for _, i := range in {
		field := i.Field
		if prefix != "" {
			field = prefix + "." + field
		}
		out = append(out, model.Issue{Field: field, Code: i.Code, Message: i.Message, Params: i.Params})
	}
	return out
}
