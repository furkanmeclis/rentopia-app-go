// Package usecase holds the corporate agreement rules (TEC-723,
// docs/design/corporate-agreements.md): agreements of corporate customers
// with validity, invoice period, payment term (written onto the customer's
// ledger account), open account and credit limit, coupon eligibility and
// long-term terms; authorised contacts and permitted drivers; the views
// over what was sold under an agreement (reservations and rentals, the
// billing plans of periodically billed rentals, the statement).
//
// Pricing (agreement tariffs), reservations (agreement sale checks, open
// account) and finance (periodic accrual) read the agreement tables
// directly; this package owns writing them.
//
// Branch scope: an agreement with a managing branch is visible inside that
// branch's scope; a head office agreement (no branch) to every member.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	pricingmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxExportRows caps exports.
const MaxExportRows = 10000

var (
	ErrNotFound        = errors.New("agreement not found")
	ErrContactNotFound = errors.New("agreement contact not found")
	ErrDriverNotFound  = errors.New("agreement driver not found")
)

// StateError answers 409 with Code.
type StateError struct{ Code, Message string }

func (e *StateError) Error() string { return e.Message }

// Detail is a field validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
type ValidationError struct{ Details []Detail }

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

// Terms writes the agreement's payment term onto the customer's ledger
// account (finance), inside the caller's transaction.
type Terms interface {
	ApplyCustomerTerms(ctx context.Context, q *db.Queries, orgID, customerID, branchID int64, termDays int32) (uuid.UUID, error)
}

// Tariffs lists the agreement's rate plans (pricing).
type Tariffs interface {
	ListPlans(ctx context.Context, orgID int64, f pricingmodel.RatePlanFilter) ([]pricingmodel.RatePlan, int64, error)
}

// Service implements the agreement use cases.
type Service struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	terms   Terms
	tariffs Tariffs
	events  resourceevents.Publisher
	now     func() time.Time
}

// New builds the service. terms / tariffs may be nil (no account terms,
// no tariff list).
func New(pool *pgxpool.Pool, q *db.Queries, terms Terms, tariffs Tariffs) *Service {
	return &Service{pool: pool, q: q, terms: terms, tariffs: tariffs, events: resourceevents.Nop{}, now: time.Now}
}

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		s.events = p
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) today() rentaltime.Date { return rentaltime.LocalDate(s.clock()) }

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("agreements: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) publish(ctx context.Context, action string, ids ...uuid.UUID) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	s.events.Publish(ctx, model.Resource, action, out...)
}

// ---------------------------------------------------------------- reads

// validity is the computed validity state of an agreement on day.
func validity(from pgtype.Date, to pgtype.Date, day rentaltime.Date) string {
	switch {
	case day.Before(rentaltime.DateOf(from.Time)):
		return model.ValidityUpcoming
	case to.Valid && day.After(rentaltime.DateOf(to.Time)):
		return model.ValidityExpired
	}
	return model.ValidityCurrent
}

func displayName(customerType, company, first, last string) string {
	if customerType == "corporate" {
		return company
	}
	return strings.TrimSpace(first + " " + last)
}

func datePtr(d pgtype.Date) *rentaltime.Date {
	if !d.Valid {
		return nil
	}
	v := rentaltime.DateOf(d.Time)
	return &v
}

func moneyPtr(n pgtype.Numeric) *money.Money {
	if !n.Valid {
		return nil
	}
	v := money.FromNumericOrZero(n)
	return &v
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func int4Arg(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func dateArg(d *rentaltime.Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func textArg(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func uniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nilUUIDs(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

// row loads an agreement in the caller's branch scope.
func (s *Service) row(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetAgreementRow, error) {
	if q == nil {
		q = s.q
	}
	r, err := q.GetAgreement(ctx, db.GetAgreementParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if r.BranchID.Valid && orgctx.CheckBranch(ctx, r.BranchID.Int64) != nil {
		return r, ErrNotFound
	}
	return r, nil
}

// Get returns one agreement with its contacts, drivers, account and the
// open-account exposure.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Agreement, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, err
	}
	out := model.Agreement{
		UUID: r.Uuid, Code: r.Code, Name: r.Name, Status: r.Status, Validity: validity(r.ValidFrom, r.ValidTo, s.today()),
		Customer: model.CustomerRef{
			UUID: r.CustomerUuid, CustomerType: r.CustomerType, IdentityMasked: r.CustomerIdentityHint,
			DisplayName: displayName(r.CustomerType, r.CustomerCompanyTitle, r.CustomerFirstName, r.CustomerLastName),
		},
		ValidFrom: rentaltime.DateOf(r.ValidFrom.Time), ValidTo: datePtr(r.ValidTo), InvoicePeriod: r.InvoicePeriod,
		PaymentTermDays: r.PaymentTermDays, OpenAccount: r.OpenAccount, CreditLimit: moneyPtr(r.CreditLimit),
		CouponEligible: r.CouponEligible, MonthlyKmAllowance: int4Ptr(r.MonthlyKmAllowance),
		MaintenanceIntervalDays: int4Ptr(r.MaintenanceIntervalDays), MaintenanceIntervalKm: int4Ptr(r.MaintenanceIntervalKm),
		Currency: r.Currency, Notes: r.Notes, TerminatedAt: timePtr(r.TerminatedAt), TerminationReason: r.TerminationReason,
		Contacts: []model.Contact{}, Drivers: []model.Driver{}, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.BranchUuid.Valid {
		out.Branch = &model.Ref{UUID: r.BranchUuid.Bytes, Code: r.BranchCode.String, Name: r.BranchName.String}
	}
	if r.AccountUuid.Valid {
		out.Account = &model.AccountRef{UUID: r.AccountUuid.Bytes, Code: r.AccountCode.String, PaymentTermDays: r.AccountPaymentTermDays.Int32}
	}
	if r.CreatedByUuid.Valid {
		out.CreatedBy = &model.Actor{UUID: r.CreatedByUuid.Bytes, Name: r.CreatedByName.String, Surname: r.CreatedBySurname.String, Email: r.CreatedByEmail.String}
	}
	if r.OpenAccount && r.CreditLimit.Valid {
		e, err := s.q.AgreementCreditExposure(ctx, db.AgreementCreditExposureParams{
			OrganizationID: orgID, CustomerID: int8Of(r.CustomerID), AgreementID: int8Of(r.ID),
		})
		if err != nil {
			return model.Agreement{}, err
		}
		limit := money.FromNumericOrZero(r.CreditLimit)
		balance, pending := money.FromNumericOrZero(e.Balance), money.FromNumericOrZero(e.Pending)
		out.Exposure = &model.Exposure{CreditLimit: limit, Balance: balance, Pending: pending, Available: limit.Sub(balance).Sub(pending)}
	}
	contacts, err := s.q.ListAgreementContacts(ctx, db.ListAgreementContactsParams{OrganizationID: orgID, AgreementID: r.ID})
	if err != nil {
		return model.Agreement{}, err
	}
	for _, c := range contacts {
		out.Contacts = append(out.Contacts, model.Contact{
			UUID: c.Uuid, ContactUUID: c.ContactUuid, FullName: c.FullName, Title: c.Title, PhoneMasked: pii.MaskPhone(c.Phone),
			EmailMasked: pii.MaskEmail(c.Email), IsPrimary: c.IsPrimary, Role: c.Role, Note: c.Note, CreatedAt: c.CreatedAt.Time.UTC(),
		})
	}
	drivers, err := s.q.ListAgreementDrivers(ctx, db.ListAgreementDriversParams{OrganizationID: orgID, AgreementID: r.ID})
	if err != nil {
		return model.Agreement{}, err
	}
	for _, d := range drivers {
		item := model.Driver{UUID: d.Uuid, Note: d.Note, CreatedAt: d.CreatedAt.Time.UTC()}
		if d.DriverID.Valid {
			item.Kind = "driver"
			item.DriverUUID = uuidPtr(d.DriverUuid)
			item.Name = strings.TrimSpace(d.DriverFirstName.String + " " + d.DriverLastName.String)
			item.Owner = displayName(d.DriverCustomerType.String, d.DriverCustomerCompanyTitle.String, d.DriverCustomerFirstName.String, d.DriverCustomerLastName.String)
			item.LicenceMasked = d.DriverLicenceHint.String
			item.LicenceExpires = datePtr(d.DriverLicenceExpiresOn)
		} else {
			item.Kind = "customer"
			item.CustomerUUID = uuidPtr(d.CustomerUuid)
			item.Name = strings.TrimSpace(d.CustomerFirstName.String + " " + d.CustomerLastName.String)
			item.LicenceMasked = d.CustomerIdentityHint.String
		}
		out.Drivers = append(out.Drivers, item)
	}
	return out, nil
}

// List returns a page of agreements in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.Filter) ([]model.ListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.Sortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	today := s.today()
	rows, err := s.q.ListAgreements(ctx, db.ListAgreementsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		CustomerUuids: nilUUIDs(f.CustomerUUIDs), BranchUuids: nilUUIDs(f.BranchUUIDs), InvoicePeriods: nilIfEmpty(f.InvoicePeriods),
		Validity: nilIfEmpty(f.Validities), Today: pgtype.Date{Time: today.Time(), Valid: true},
		ValidOnFrom: dateArg(f.ValidOnFrom), ValidOnTo: dateArg(f.ValidOnTo), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ListItem, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		it := model.ListItem{
			UUID: r.Uuid, Code: r.Code, Name: r.Name, Status: r.Status, Validity: validity(r.ValidFrom, r.ValidTo, today),
			Customer: model.CustomerRef{
				UUID: r.CustomerUuid, CustomerType: r.CustomerType,
				DisplayName: displayName(r.CustomerType, r.CustomerCompanyTitle, r.CustomerFirstName, r.CustomerLastName),
			},
			ValidFrom: rentaltime.DateOf(r.ValidFrom.Time), ValidTo: datePtr(r.ValidTo), InvoicePeriod: r.InvoicePeriod,
			PaymentTermDays: r.PaymentTermDays, OpenAccount: r.OpenAccount, CreditLimit: moneyPtr(r.CreditLimit),
			CouponEligible: r.CouponEligible, Currency: r.Currency, ReservationCount: r.ReservationCount, TariffCount: r.TariffCount,
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.BranchUuid.Valid {
			it.Branch = &model.Ref{UUID: r.BranchUuid.Bytes, Code: r.BranchCode.String, Name: r.BranchName.String}
		}
		out = append(out, it)
	}
	return out, total, nil
}

// Facets returns the dynamic filter options in the caller's scope.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListAgreementFacets(ctx, db.ListAgreementFacetsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Today: pgtype.Date{Time: s.today().Time(), Valid: true},
	})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Status: []model.Facet{}, Customer: []model.Facet{}, Branch: []model.Facet{}, InvoicePeriod: []model.Facet{}, Validity: []model.Facet{}}
	for _, r := range rows {
		f := model.Facet{Value: r.Value, Label: r.Label, Count: r.Count}
		switch r.Facet {
		case "status":
			out.Status = append(out.Status, f)
		case "customer":
			out.Customer = append(out.Customer, f)
		case "branch":
			out.Branch = append(out.Branch, f)
		case "invoice_period":
			out.InvoicePeriod = append(out.InvoicePeriod, f)
		case "validity":
			out.Validity = append(out.Validity, f)
		}
	}
	return out, nil
}

// ExportRow is one agreement of an export.
type ExportRow = db.ListAgreementsForExportRow

// ExportRows returns the agreements an export covers.
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.Filter) ([]ExportRow, error) {
	return s.q.ListAgreementsForExport(ctx, db.ListAgreementsForExportParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Statuses: nilIfEmpty(f.Statuses),
		CustomerUuids: nilUUIDs(f.CustomerUUIDs), Validity: nilIfEmpty(f.Validities),
		Today: pgtype.Date{Time: s.today().Time(), Valid: true}, Q: textArg(f.Q), LimitCount: MaxExportRows,
	})
}

// Activity lists the audit events of an agreement.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ref := pgtype.UUID{Bytes: r.Uuid, Valid: true}
	rows, err := s.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ref, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{OrganizationID: org, Resource: model.Resource, ResourceUuid: ref})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		e := model.ActivityEntry{UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time.UTC()}
		if len(e.Payload) == 0 {
			e.Payload = json.RawMessage("{}")
		}
		if row.ActorUuid.Valid {
			e.Actor = &model.Actor{UUID: row.ActorUuid.Bytes, Name: row.ActorName.String, Surname: row.ActorSurname.String, Email: row.ActorEmail.String}
		}
		out = append(out, e)
	}
	return out, total, nil
}
