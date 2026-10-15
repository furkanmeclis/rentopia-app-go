package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
)

// RentalLedger is the part of the finance ledger (TEC-717) the toll module
// posts through: it reconciles the accruals of a rental with its approved
// charges on the given queries (the caller's transaction).
type RentalLedger interface {
	SyncRental(ctx context.Context, q *db.Queries, orgID, rentalID int64) error
}

// Ledger implements model.LedgerPoster on the finance ledger.
//
// A toll / toll_fee charge is an approved rental charge, so it is accrued
// under the charge's own accrual key (rental_charge:<id>): at most one live
// accrual per charge, whatever the number of calls (the ledger's unique
// index and insert trigger refuse a second one). A reversal row (negative
// amount, reversal_of_id) is never accrued itself; it marks the charge it
// reverses as no longer due, so the charge's accrual gets a reversal entry.
// Both run inside the toll module's transaction.
type Ledger struct {
	Finance RentalLedger
}

var errLedgerEntry = errors.New("tolls: ledger: invalid entry")

// PostTollCharge implements model.LedgerPoster.
func (l Ledger) PostTollCharge(ctx context.Context, tx pgx.Tx, e model.LedgerEntry) error {
	if l.Finance == nil {
		return nil
	}
	if tx == nil || e.OrganizationID == 0 || e.RentalID == 0 || e.ChargeID == 0 {
		return errLedgerEntry
	}
	if e.Kind != model.ChargeToll && e.Kind != model.ChargeTollFee {
		return fmt.Errorf("%w: kind %q", errLedgerEntry, e.Kind)
	}
	if e.ReversalOf == nil && !e.Amount.IsPositive() {
		return fmt.Errorf("%w: a charge must be positive", errLedgerEntry)
	}
	if e.ReversalOf != nil && e.Amount.IsPositive() {
		return fmt.Errorf("%w: a reversal must be negative", errLedgerEntry)
	}
	return l.Finance.SyncRental(ctx, db.New(tx), e.OrganizationID, e.RentalID)
}

var _ model.LedgerPoster = Ledger{}
