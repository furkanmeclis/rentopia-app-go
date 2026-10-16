package usecase

// Settlements without fresh money (TEC-719 × TEC-718): a deposit deduction
// and the settling leg of an offset are credits that pay open items. Each
// gets a collection document (one per ledger entry) and collection
// allocations, so open items and allocation statuses see them as paid;
// nothing settles an item without allocation rows. Deposit holds and
// returns never get a document: they are a liability, never customer
// credit, never allocatable.

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// creditDoc describes the document of an existing credit entry.
type creditDoc struct {
	accountID     int64
	currency      string
	entryID       int64
	amount        money.Money
	method        string
	provider      string
	date          pgtype.Date
	branchID      int64
	cashAccountID pgtype.Int8
	rentalID      int64
	reservationID int64
	ref, note     string
	// chargeID: the rental charge paid first (its live accrual).
	chargeID int64
	// onlyPurpose: items of this purpose (rent / charge) are paid before
	// the rest of the rental's items.
	onlyPurpose string
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// creditDocument writes the document and allocates it: the named charge
// first, then items of the purpose, then the rental's other items oldest
// first (the whole account without a rental); what is left stays credit.
// The caller holds the account lock in the same transaction.
func (s *Service) creditDocument(ctx context.Context, q *db.Queries, orgID int64, d creditDoc) (uuid.UUID, error) {
	if err := q.LockOrganizationCollectionCodes(ctx, orgID); err != nil {
		return uuid.Nil, err
	}
	code, err := q.NextCollectionCode(ctx, orgID)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	coll, err := q.InsertCollection(ctx, db.InsertCollectionParams{
		Uuid: id, OrganizationID: orgID, Code: code, AccountID: d.accountID, Method: d.method, Provider: d.provider,
		Purpose: model.PurposeGeneral, Currency: d.currency, Amount: d.amount.Numeric(), GrossAmount: d.amount.Numeric(),
		CommissionAmount: money.Zero().Numeric(), NetAmount: d.amount.Numeric(), CollectionDate: d.date,
		ReferenceNo: truncate(d.ref, 64), Note: truncate(d.note, 1000), BranchID: int8Of(d.branchID),
		CashAccountID: d.cashAccountID, RentalID: int8Of(d.rentalID), ReservationID: int8Of(d.reservationID),
		LedgerEntryID: d.entryID, SettlementStatus: model.SettlementNone, CreatedBy: actorID(ctx),
	})
	if err != nil {
		return uuid.Nil, err
	}
	left := d.amount
	pay := func(entryID int64, source string) error {
		rows, err := q.ListOpenItems(ctx, db.ListOpenItemsParams{OrganizationID: orgID, AccountID: d.accountID, EntryIds: []int64{entryID}})
		if err != nil || len(rows) == 0 {
			return err
		}
		amt := m(rows[0].Amount).Sub(m(rows[0].Allocated)).Min(left)
		if !amt.IsPositive() {
			return nil
		}
		if _, err := q.InsertCollectionAllocation(ctx, db.InsertCollectionAllocationParams{
			OrganizationID: orgID, CollectionID: coll.ID, EntryID: entryID, Kind: model.AllocKindAllocate,
			Amount: amt.Numeric(), Source: source, CreatedBy: actorID(ctx),
		}); err != nil {
			return allocationConflict(err)
		}
		left = left.Sub(amt)
		return nil
	}
	if d.chargeID != 0 && d.rentalID != 0 {
		live, err := q.ListLiveRentalAccruals(ctx, db.ListLiveRentalAccrualsParams{OrganizationID: orgID, RentalID: int8Of(d.rentalID)})
		if err != nil {
			return uuid.Nil, err
		}
		for _, l := range live {
			if l.RentalChargeID.Valid && l.RentalChargeID.Int64 == d.chargeID && l.AccountID == d.accountID {
				if err := pay(l.ID, model.AllocSourceManual); err != nil {
					return uuid.Nil, err
				}
			}
		}
	}
	if left.IsPositive() && (d.onlyPurpose == model.PurposeRent || d.onlyPurpose == model.PurposeCharge) {
		p := db.ListOpenItemsParams{OrganizationID: orgID, AccountID: d.accountID, OpenOnly: true}
		if d.rentalID != 0 {
			p.RentalID = int8Of(d.rentalID)
		}
		items, err := q.ListOpenItems(ctx, p)
		if err != nil {
			return uuid.Nil, err
		}
		for _, it := range items {
			if !left.IsPositive() {
				break
			}
			if it.Purpose == d.onlyPurpose {
				if err := pay(it.ID, model.AllocSourceAuto); err != nil {
					return uuid.Nil, err
				}
			}
		}
	}
	if left.IsPositive() {
		if _, err := s.allocate(ctx, q, orgID, allocation{
			collectionID: coll.ID, accountID: d.accountID, mode: model.AllocateAuto, rentalID: d.rentalID,
			source: model.AllocSourceAuto, field: "allocations",
		}); err != nil {
			return uuid.Nil, err
		}
	}
	s.publish(ctx, model.ResourceCollections, "created", id)
	return id, nil
}

// settlementOnly refuses refunds and voids of a document that brought no
// money (a deposit deduction or an offset leg): they follow their deposit /
// pair; a correction is an adjustment.
func (s *Service) settlementOnly(ctx context.Context, orgID int64, collection uuid.UUID) error {
	row, err := s.q.GetCollectionRow(ctx, db.GetCollectionRowParams{OrganizationID: orgID, Uuid: collection})
	if err != nil {
		if notFound(err) {
			return nil
		}
		return err
	}
	e, err := s.q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: row.LedgerEntryUuid})
	if err != nil {
		return err
	}
	if e.Kind == model.KindOffset || e.SourceType == model.SourceSecurityDeposit {
		return stateErr("deposit_managed", "a deposit deduction or offset follows its deposit; post an adjustment instead")
	}
	return nil
}
