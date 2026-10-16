package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Corporate agreements (TEC-723, docs/design/corporate-agreements.md): a
// quote and a reservation can be sold under an agreement of a corporate
// customer. Pricing then uses the agreement's tariff (rate plans of scope
// agreement) and its coupon eligibility; the reservation checks the
// agreement's customer, validity and permitted drivers, and its open
// account (confirmation without prepayment within the credit limit).

// Agreement statuses that may sell.
const agreementActive = "active"

// agreement is the sale view of an agreement.
type agreement = db.GetAgreementForSaleRow

// CreditLimitError is answered with 422 CREDIT_LIMIT_EXCEEDED: confirming
// the reservation on the agreement's open account would take the
// customer's exposure above the credit limit.
type CreditLimitError struct {
	Limit    money.Money `json:"credit_limit"`
	Balance  money.Money `json:"balance"`
	Pending  money.Money `json:"pending"`
	Total    money.Money `json:"total"`
	Exposure money.Money `json:"exposure"`
}

func (e *CreditLimitError) Error() string {
	return fmt.Sprintf("the open-account credit limit %s would be exceeded (exposure %s)", e.Limit, e.Exposure)
}

// agreementForSale resolves an agreement uuid of a body: in the
// organization and the caller's branch scope, active and valid on the
// Istanbul day of at.
func (s *Service) agreementForSale(ctx context.Context, orgID int64, id uuid.UUID, at time.Time, field string) (*agreement, error) {
	a, err := s.q.GetAgreementForSale(ctx, db.GetAgreementForSaleParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return nil, validation(field, "not_found", "agreement was not found")
	}
	if err != nil {
		return nil, err
	}
	if a.BranchID.Valid && orgctx.CheckBranch(ctx, a.BranchID.Int64) != nil {
		return nil, validation(field, "not_found", "agreement was not found")
	}
	if a.Status != agreementActive {
		return nil, validation(field, "agreement_inactive", "the agreement is not active")
	}
	day := rentaltime.LocalDate(at)
	if day.Before(rentaltime.DateOf(a.ValidFrom.Time)) || (a.ValidTo.Valid && day.After(rentaltime.DateOf(a.ValidTo.Time))) {
		return nil, validation(field, "agreement_not_valid", "the agreement is not valid on the pick-up day")
	}
	return &a, nil
}

// agreementByID loads the agreement a quote / reservation was sold under
// (no state checks: a sold reservation keeps its agreement).
func (s *Service) agreementByID(ctx context.Context, orgID int64, id pgtype.Int8) (*agreement, error) {
	if !id.Valid {
		return nil, nil
	}
	row, err := s.q.GetAgreementForSaleByID(ctx, db.GetAgreementForSaleByIDParams{OrganizationID: orgID, ID: id.Int64})
	if err != nil {
		return nil, err
	}
	a := agreement(row)
	return &a, nil
}

// reservationAgreement is the agreement of a reservation (changes and
// extensions are priced under it).
func (s *Service) reservationAgreement(ctx context.Context, orgID, reservationID int64) (*agreement, error) {
	id, err := s.q.GetReservationAgreementID(ctx, db.GetReservationAgreementIDParams{OrganizationID: orgID, ID: reservationID})
	if err != nil {
		return nil, err
	}
	return s.agreementByID(ctx, orgID, id)
}

// checkAgreementParties enforces the agreement's customer: the contract
// party or the invoice addressee must be the agreement's corporate
// customer. When the contract party is someone else (an employee) and no
// invoice addressee is given, the agreement's customer becomes it, so the
// rent lands on the corporate account.
func (s *Service) checkAgreementParties(ctx context.Context, orgID int64, a *agreement, customer *db.GetReservationCustomerRow, invoice **db.GetReservationCustomerRow) error {
	if customer.ID == a.CustomerID {
		return nil
	}
	if *invoice != nil {
		if (*invoice).ID == a.CustomerID {
			return nil
		}
		return validation("invoice_customer_uuid", "agreement_customer", "the agreement's customer must be the customer or the invoice addressee")
	}
	id := a.CustomerUuid
	c, err := s.customerOf(ctx, orgID, &id, "invoice_customer_uuid")
	if err != nil {
		return err
	}
	*invoice = c
	return nil
}

// checkAgreementDrivers refuses drivers the agreement does not permit (no
// permitted driver listed = anyone may drive).
func (s *Service) checkAgreementDrivers(ctx context.Context, orgID int64, a *agreement, drivers []checkedDriver) error {
	ve := &ValidationError{}
	for i, d := range drivers {
		rule, err := s.q.AgreementDriverRules(ctx, db.AgreementDriverRulesParams{OrganizationID: orgID, AgreementID: a.ID, DriverID: d.row.ID})
		if err != nil {
			return err
		}
		if rule.Listed > 0 && !rule.Permitted {
			ve.add("drivers."+itoa(i)+".driver_uuid", "not_permitted", "the agreement does not permit this driver")
		}
	}
	return ve.orNil()
}

// checkCredit refuses an open-account confirmation above the credit limit:
// exposure = the customer's ledger balance + the agreement's live
// reservations not accrued yet (excluding exclude) + total.
func (s *Service) checkCredit(ctx context.Context, q *db.Queries, orgID int64, a *agreement, total money.Money, exclude int64) error {
	if q == nil {
		q = s.q
	}
	row, err := q.AgreementCreditExposure(ctx, db.AgreementCreditExposureParams{
		OrganizationID: orgID, CustomerID: int8Of(a.CustomerID), AgreementID: int8Of(a.ID), ExcludeReservationID: int8Of(exclude),
	})
	if err != nil {
		return err
	}
	limit := money.FromNumericOrZero(a.CreditLimit)
	balance, pending := money.FromNumericOrZero(row.Balance), money.FromNumericOrZero(row.Pending)
	exposure := balance.Add(pending).Add(total)
	if exposure.Cmp(limit) > 0 {
		return &CreditLimitError{Limit: limit, Balance: balance, Pending: pending, Total: total, Exposure: exposure}
	}
	return nil
}

// agreementRef is the API reference of a stored agreement link.
func agreementRef(id pgtype.UUID, code, name pgtype.Text) *model.AgreementRef {
	if !id.Valid {
		return nil
	}
	return &model.AgreementRef{UUID: id.Bytes, Code: code.String, Name: name.String}
}
