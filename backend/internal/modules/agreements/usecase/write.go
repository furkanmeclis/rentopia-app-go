package usecase

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fields is the validated, complete field set of an agreement.
type fields struct {
	name                        string
	branchID                    pgtype.Int8
	validFrom                   rentaltime.Date
	validTo                     *rentaltime.Date
	invoicePeriod               string
	term                        int32
	openAccount, couponEligible bool
	creditLimit                 *money.Money
	km, maintDays, maintKm      *int32
	notes                       string
}

func fieldsOf(r db.GetAgreementRow) fields {
	f := fields{
		name: r.Name, branchID: r.BranchID, validFrom: rentaltime.DateOf(r.ValidFrom.Time), validTo: datePtr(r.ValidTo),
		invoicePeriod: r.InvoicePeriod, term: r.PaymentTermDays, openAccount: r.OpenAccount, couponEligible: r.CouponEligible,
		creditLimit: moneyPtr(r.CreditLimit), km: int4Ptr(r.MonthlyKmAllowance), maintDays: int4Ptr(r.MaintenanceIntervalDays),
		maintKm: int4Ptr(r.MaintenanceIntervalKm), notes: r.Notes,
	}
	return f
}

// apply validates in over f (the current values on PATCH, defaults on
// create).
func (s *Service) apply(ctx context.Context, orgID int64, f *fields, in model.Input, ve *ValidationError) {
	if in.Name != nil {
		f.name = strings.TrimSpace(*in.Name)
	}
	if f.name == "" || len([]rune(f.name)) > 255 {
		ve.add("name", "required", "name is required (at most 255 characters)")
	}
	switch {
	case in.ClearBranch:
		if err := orgctx.RequireAllBranches(ctx); err != nil {
			ve.add("branch_uuid", "not_allowed", "a head office agreement needs access to every branch")
		}
		f.branchID = pgtype.Int8{}
	case in.BranchUUID != nil:
		b, err := s.q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: *in.BranchUUID})
		if err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
			ve.add("branch_uuid", "not_found", "branch was not found")
		} else {
			f.branchID = int8Of(b.ID)
		}
	}
	if in.ValidFrom != nil {
		f.validFrom = *in.ValidFrom
	}
	if f.validFrom.IsZero() {
		ve.add("valid_from", "required", "valid_from is required")
	}
	switch {
	case in.ClearValidTo:
		f.validTo = nil
	case in.ValidTo != nil:
		v := *in.ValidTo
		f.validTo = &v
	}
	if f.validTo != nil && !f.validFrom.IsZero() && f.validTo.Before(f.validFrom) {
		ve.add("valid_to", "before_start", "valid_to must not be before valid_from")
	}
	if in.InvoicePeriod != nil {
		f.invoicePeriod = *in.InvoicePeriod
	}
	if !slices.Contains(model.InvoicePeriods, f.invoicePeriod) {
		ve.add("invoice_period", "invalid", "invoice_period must be monthly or per_rental")
	}
	if in.PaymentTermDays != nil {
		f.term = *in.PaymentTermDays
	}
	if f.term < 0 || f.term > 365 {
		ve.add("payment_term_days", "out_of_range", "payment_term_days must be between 0 and 365")
	}
	if in.OpenAccount != nil {
		f.openAccount = *in.OpenAccount
	}
	if in.CreditLimit != nil {
		v := *in.CreditLimit
		f.creditLimit = &v
	}
	if f.creditLimit != nil && f.creditLimit.IsNegative() {
		ve.add("credit_limit", "invalid", "credit_limit must not be negative")
	}
	if f.openAccount && f.creditLimit == nil {
		ve.add("credit_limit", "required", "an open account needs a credit limit")
	}
	if in.CouponEligible != nil {
		f.couponEligible = *in.CouponEligible
	}
	positive := func(field string, v *int32) *int32 {
		if v == nil {
			return nil
		}
		if *v <= 0 {
			// 0 clears the term.
			return nil
		}
		if *v > 1000000 {
			ve.add(field, "out_of_range", field+" is too large")
		}
		return v
	}
	if in.MonthlyKmAllowance != nil {
		f.km = positive("monthly_km_allowance", in.MonthlyKmAllowance)
	}
	if in.MaintenanceIntervalDays != nil {
		f.maintDays = positive("maintenance_interval_days", in.MaintenanceIntervalDays)
	}
	if in.MaintenanceIntervalKm != nil {
		f.maintKm = positive("maintenance_interval_km", in.MaintenanceIntervalKm)
	}
	if in.Notes != nil {
		f.notes = strings.TrimSpace(*in.Notes)
	}
	if len(f.notes) > 5000 {
		ve.add("notes", "too_long", "notes are at most 5000 characters")
	}
}

func creditArg(v *money.Money) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	return v.Numeric()
}

// applyTerms writes the payment term onto the customer's ledger account.
func (s *Service) applyTerms(ctx context.Context, q *db.Queries, orgID, customerID int64, branchID pgtype.Int8, term int32) error {
	if s.terms == nil {
		return nil
	}
	_, err := s.terms.ApplyCustomerTerms(ctx, q, orgID, customerID, branchID.Int64, term)
	return err
}

// Create opens an agreement (draft, or active with Activate) with a
// corporate customer. The payment term goes onto the customer's account.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input) (model.Agreement, error) {
	ve := &ValidationError{}
	var customer db.GetCustomerForAgreementRow
	if in.CustomerUUID == nil {
		ve.add("customer_uuid", "required", "customer_uuid is required")
	} else {
		c, err := s.q.GetCustomerForAgreement(ctx, db.GetCustomerForAgreementParams{OrganizationID: orgID, Uuid: *in.CustomerUUID})
		switch {
		case err != nil:
			ve.add("customer_uuid", "not_found", "customer was not found")
		case c.CustomerType != "corporate":
			ve.add("customer_uuid", "not_corporate", "an agreement is made with a corporate customer")
		case c.AnonymizedAt.Valid:
			ve.add("customer_uuid", "anonymized", "the customer was anonymized")
		default:
			customer = c
		}
	}
	f := fields{invoicePeriod: model.InvoicePerRental}
	if in.BranchUUID == nil && !in.ClearBranch && orgctx.BranchScope(ctx) != nil {
		// A branch-scoped member opens agreements of a branch.
		ve.add("branch_uuid", "required", "branch_uuid is required for a member limited to some branches")
	}
	s.apply(ctx, orgID, &f, in, ve)
	if err := ve.orNil(); err != nil {
		return model.Agreement{}, err
	}
	status := model.StatusDraft
	if in.Activate {
		status = model.StatusActive
	}
	var id uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		if err := q.LockAgreementCodes(ctx, orgID); err != nil {
			return err
		}
		n, err := q.NextAgreementNumber(ctx, orgID)
		if err != nil {
			return err
		}
		row, err := q.InsertAgreement(ctx, db.InsertAgreementParams{
			OrganizationID: orgID, Code: fmt.Sprintf("KA-%06d", n), Name: f.name, CustomerID: customer.ID, BranchID: f.branchID,
			Status: status, ValidFrom: dateArg(&f.validFrom), ValidTo: dateArg(f.validTo), InvoicePeriod: f.invoicePeriod,
			PaymentTermDays: f.term, OpenAccount: f.openAccount, CreditLimit: creditArg(f.creditLimit), CouponEligible: f.couponEligible,
			MonthlyKmAllowance: int4Arg(f.km), MaintenanceIntervalDays: int4Arg(f.maintDays), MaintenanceIntervalKm: int4Arg(f.maintKm),
			Currency: string(money.TRY), Notes: f.notes, CreatedBy: actorID(ctx),
		})
		if err != nil {
			if uniqueViolation(err) {
				return &StateError{Code: "code_taken", Message: "the agreement code is taken; retry"}
			}
			return err
		}
		id = row.Uuid
		return s.applyTerms(ctx, q, orgID, customer.ID, f.branchID, f.term)
	})
	if err != nil {
		return model.Agreement{}, err
	}
	s.publish(ctx, "created", id)
	return s.Get(ctx, orgID, id)
}

// Update edits an agreement (not a terminated one). A changed payment term
// goes onto the customer's account; a changed invoice period applies to
// rentals created from then on (a rental keeps the billing mode it was
// created with).
func (s *Service) Update(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Agreement, []string, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, nil, err
	}
	if r.Status == model.StatusTerminated {
		return model.Agreement{}, nil, &StateError{Code: "terminated", Message: "a terminated agreement cannot change"}
	}
	if in.CustomerUUID != nil && *in.CustomerUUID != r.CustomerUuid {
		return model.Agreement{}, nil, validation("customer_uuid", "immutable", "the customer of an agreement never changes")
	}
	ve := &ValidationError{}
	f := fieldsOf(r)
	before := f
	s.apply(ctx, orgID, &f, in, ve)
	if err := ve.orNil(); err != nil {
		return model.Agreement{}, nil, err
	}
	changed := changedFields(before, f)
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAgreement(ctx, db.LockAgreementParams{OrganizationID: orgID, ID: r.ID}); err != nil {
			return err
		}
		n, err := q.UpdateAgreement(ctx, db.UpdateAgreementParams{
			OrganizationID: orgID, ID: r.ID, Name: f.name, BranchID: f.branchID, ValidFrom: dateArg(&f.validFrom),
			ValidTo: dateArg(f.validTo), InvoicePeriod: f.invoicePeriod, PaymentTermDays: f.term, OpenAccount: f.openAccount,
			CreditLimit: creditArg(f.creditLimit), CouponEligible: f.couponEligible, MonthlyKmAllowance: int4Arg(f.km),
			MaintenanceIntervalDays: int4Arg(f.maintDays), MaintenanceIntervalKm: int4Arg(f.maintKm), Notes: f.notes,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
		if before.term != f.term || !r.AccountUuid.Valid {
			return s.applyTerms(ctx, q, orgID, r.CustomerID, f.branchID, f.term)
		}
		return nil
	})
	if err != nil {
		return model.Agreement{}, nil, err
	}
	s.publish(ctx, "updated", id)
	out, err := s.Get(ctx, orgID, id)
	return out, changed, err
}

func changedFields(a, b fields) []string {
	out := []string{}
	add := func(name string, diff bool) {
		if diff {
			out = append(out, name)
		}
	}
	add("name", a.name != b.name)
	add("branch", a.branchID != b.branchID)
	add("valid_from", a.validFrom != b.validFrom)
	add("valid_to", fmt.Sprint(a.validTo) != fmt.Sprint(b.validTo))
	add("invoice_period", a.invoicePeriod != b.invoicePeriod)
	add("payment_term_days", a.term != b.term)
	add("open_account", a.openAccount != b.openAccount)
	add("credit_limit", fmt.Sprint(a.creditLimit) != fmt.Sprint(b.creditLimit))
	add("coupon_eligible", a.couponEligible != b.couponEligible)
	add("monthly_km_allowance", fmt.Sprint(a.km) != fmt.Sprint(b.km))
	add("maintenance_interval_days", fmt.Sprint(a.maintDays) != fmt.Sprint(b.maintDays))
	add("maintenance_interval_km", fmt.Sprint(a.maintKm) != fmt.Sprint(b.maintKm))
	add("notes", a.notes != b.notes)
	return out
}

// SetStatus moves an agreement: draft → active, active ↔ suspended,
// anything but terminated → terminated (with a reason). Only an active
// agreement sells; reservations already sold keep their agreement.
func (s *Service) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, in model.StatusInput) (model.Agreement, string, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, "", err
	}
	if !slices.Contains(model.Transitions[r.Status], in.Status) {
		return model.Agreement{}, "", &StateError{Code: "invalid_transition", Message: "the agreement cannot move from " + r.Status + " to " + in.Status}
	}
	reason := strings.TrimSpace(in.Reason)
	var terminated pgtype.Timestamptz
	if in.Status == model.StatusTerminated {
		if len(reason) < 3 || len(reason) > 500 {
			return model.Agreement{}, "", validation("reason", "required", "a termination needs a reason (3-500 characters)")
		}
		terminated = pgtype.Timestamptz{Time: s.clock(), Valid: true}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.LockAgreement(ctx, db.LockAgreementParams{OrganizationID: orgID, ID: r.ID}); err != nil {
			return err
		}
		_, err := q.SetAgreementStatus(ctx, db.SetAgreementStatusParams{
			OrganizationID: orgID, ID: r.ID, Status: in.Status, TerminatedAt: terminated, TerminationReason: reason,
		})
		return err
	})
	if err != nil {
		return model.Agreement{}, "", err
	}
	s.publish(ctx, "updated", id)
	out, err := s.Get(ctx, orgID, id)
	return out, r.Status, err
}

// Delete removes a draft agreement that never sold anything.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return err
	}
	if r.Status != model.StatusDraft {
		return &StateError{Code: "not_draft", Message: "only a draft agreement can be deleted (terminate it instead)"}
	}
	res, err := s.q.ListAgreementReservations(ctx, db.ListAgreementReservationsParams{OrganizationID: orgID, AgreementID: int8Of(r.ID)})
	if err != nil {
		return err
	}
	if len(res) > 0 {
		return &StateError{Code: "has_sales", Message: "the agreement has reservations"}
	}
	if _, err := s.q.SoftDeleteAgreement(ctx, db.SoftDeleteAgreementParams{OrganizationID: orgID, ID: r.ID}); err != nil {
		return err
	}
	s.publish(ctx, "deleted", id)
	return nil
}

// AddContact authorises a contact of the corporate customer.
func (s *Service) AddContact(ctx context.Context, orgID int64, id uuid.UUID, in model.ContactInput) (model.Agreement, uuid.UUID, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, uuid.Nil, err
	}
	ve := &ValidationError{}
	role := in.Role
	if role == "" {
		role = "operations"
	}
	if !slices.Contains(model.ContactRoles, role) {
		ve.add("role", "invalid", "role must be signatory, billing, operations or other")
	}
	var contactID int64
	if in.ContactUUID == nil {
		ve.add("contact_uuid", "required", "contact_uuid is required")
	} else if c, err := s.q.GetCustomerContactForAgreement(ctx, db.GetCustomerContactForAgreementParams{
		OrganizationID: orgID, Uuid: *in.ContactUUID, CustomerID: r.CustomerID,
	}); err != nil {
		ve.add("contact_uuid", "not_found", "contact was not found among the customer's contacts")
	} else {
		contactID = c.ID
	}
	if len(in.Note) > 500 {
		ve.add("note", "too_long", "note is at most 500 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.Agreement{}, uuid.Nil, err
	}
	row, err := s.q.InsertAgreementContact(ctx, db.InsertAgreementContactParams{
		OrganizationID: orgID, AgreementID: r.ID, CustomerContactID: contactID, Role: role, Note: strings.TrimSpace(in.Note),
	})
	if err != nil {
		if uniqueViolation(err) {
			return model.Agreement{}, uuid.Nil, validation("contact_uuid", "duplicate", "the contact is already authorised")
		}
		return model.Agreement{}, uuid.Nil, err
	}
	s.publish(ctx, "updated", id)
	out, err := s.Get(ctx, orgID, id)
	return out, row.Uuid, err
}

// RemoveContact withdraws an authorised contact.
func (s *Service) RemoveContact(ctx context.Context, orgID int64, id, contact uuid.UUID) (model.Agreement, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, err
	}
	n, err := s.q.DeleteAgreementContact(ctx, db.DeleteAgreementContactParams{OrganizationID: orgID, AgreementID: r.ID, Uuid: contact})
	if err != nil {
		return model.Agreement{}, err
	}
	if n == 0 {
		return model.Agreement{}, ErrContactNotFound
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}

// AddDriver permits a driver: a driver record (of any customer of the
// organization) or an individual customer (every driver record of that
// customer).
func (s *Service) AddDriver(ctx context.Context, orgID int64, id uuid.UUID, in model.DriverInput) (model.Agreement, uuid.UUID, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, uuid.Nil, err
	}
	ve := &ValidationError{}
	var driverID, customerID pgtype.Int8
	switch {
	case (in.DriverUUID == nil) == (in.CustomerUUID == nil):
		ve.add("driver_uuid", "one_required", "send driver_uuid or customer_uuid")
	case in.DriverUUID != nil:
		d, err := s.q.GetDriverForAgreement(ctx, db.GetDriverForAgreementParams{OrganizationID: orgID, Uuid: *in.DriverUUID})
		if err != nil {
			ve.add("driver_uuid", "not_found", "driver was not found")
		} else {
			driverID = int8Of(d.ID)
		}
	default:
		c, err := s.q.GetCustomerForAgreement(ctx, db.GetCustomerForAgreementParams{OrganizationID: orgID, Uuid: *in.CustomerUUID})
		switch {
		case err != nil:
			ve.add("customer_uuid", "not_found", "customer was not found")
		case c.CustomerType != "individual":
			ve.add("customer_uuid", "not_individual", "a permitted customer is an individual (add its driver records for a company)")
		default:
			customerID = int8Of(c.ID)
		}
	}
	if len(in.Note) > 500 {
		ve.add("note", "too_long", "note is at most 500 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.Agreement{}, uuid.Nil, err
	}
	row, err := s.q.InsertAgreementDriver(ctx, db.InsertAgreementDriverParams{
		OrganizationID: orgID, AgreementID: r.ID, DriverID: driverID, CustomerID: customerID, Note: strings.TrimSpace(in.Note),
	})
	if err != nil {
		if uniqueViolation(err) {
			return model.Agreement{}, uuid.Nil, validation("driver_uuid", "duplicate", "the driver is already permitted")
		}
		return model.Agreement{}, uuid.Nil, err
	}
	s.publish(ctx, "updated", id)
	out, err := s.Get(ctx, orgID, id)
	return out, row.Uuid, err
}

// RemoveDriver withdraws a permitted driver.
func (s *Service) RemoveDriver(ctx context.Context, orgID int64, id, driver uuid.UUID) (model.Agreement, error) {
	r, err := s.row(ctx, nil, orgID, id)
	if err != nil {
		return model.Agreement{}, err
	}
	n, err := s.q.DeleteAgreementDriver(ctx, db.DeleteAgreementDriverParams{OrganizationID: orgID, AgreementID: r.ID, Uuid: driver})
	if err != nil {
		return model.Agreement{}, err
	}
	if n == 0 {
		return model.Agreement{}, ErrDriverNotFound
	}
	s.publish(ctx, "updated", id)
	return s.Get(ctx, orgID, id)
}
