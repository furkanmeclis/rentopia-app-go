package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/google/uuid"
)

// ApplyCustomerTerms writes a payment term onto a customer's ledger account,
// opening the account (at branchID, 0 = none) on first use. Corporate
// agreements (TEC-723) call it, on q (the caller's transaction), whenever
// an agreement's payment term is set: the account's due dates follow the
// agreement from then on. Returns the account uuid.
func (s *Service) ApplyCustomerTerms(ctx context.Context, q *db.Queries, orgID, customerID, branchID int64, termDays int32) (uuid.UUID, error) {
	if q == nil {
		q = s.q
	}
	acc, err := s.ensureCustomerAccount(ctx, q, orgID, customerID, branchID, "")
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
		return uuid.Nil, err
	}
	if acc.Term != termDays {
		if _, err := q.SetAccountTermsForCustomer(ctx, db.SetAccountTermsForCustomerParams{
			OrganizationID: orgID, CustomerID: int8Of(customerID), PaymentTermDays: termDays,
		}); err != nil {
			return uuid.Nil, err
		}
		s.publish(ctx, model.ResourceAccounts, "updated", acc.UUID)
	}
	return acc.UUID, nil
}
