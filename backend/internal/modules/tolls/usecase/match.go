package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// pass is a toll pass locked for update (FOR UPDATE inside the caller's
// transaction): every charge decision of a pass runs under this lock, so a
// pass that arrives twice at once still ends with one live charge.
type pass = db.LockTollPassByUUIDRow

// target is where the matching (or a person) puts a pass.
type target struct {
	status    string
	reason    string
	vehicleID pgtype.Int8
	rentalID  pgtype.Int8
	method    string
	manual    bool
	// decided: a person made this decision (decided_by / decided_at).
	decided bool
}

// decide is the automatic matching: plate (or tag) → vehicle, then the one
// rental whose ACTUAL out → in leg of that vehicle contains the pass time.
// Anything ambiguous names its reason and goes to review.
func (s *Service) decide(ctx context.Context, q *db.Queries, orgID int64, p pass) (target, error) {
	t := target{status: model.StatusNeedsReview}
	at := p.PassedAt
	var plateVehicle, tagVehicle int64
	if key := strings.TrimSpace(p.PlateKey.String); key != "" {
		rows, err := q.FindTollVehiclesByPlate(ctx, db.FindTollVehiclesByPlateParams{OrganizationID: orgID, PlateKey: pgtype.Text{String: key, Valid: true}})
		if err != nil {
			return t, err
		}
		live := 0
		for _, r := range rows {
			if !r.DeletedAt.Valid {
				live++
			}
		}
		if live > 1 {
			t.reason = model.ReasonVehicleConflict
			return t, nil
		}
		if len(rows) > 0 {
			plateVehicle = rows[0].ID
		}
	}
	if key := strings.TrimSpace(p.TagKey.String); key != "" {
		rows, err := q.FindTollVehiclesByTag(ctx, db.FindTollVehiclesByTagParams{OrganizationID: orgID, TagKey: pgtype.Text{String: key, Valid: true}, At: at})
		if err != nil {
			return t, err
		}
		if len(rows) > 1 {
			t.reason = model.ReasonVehicleConflict
			return t, nil
		}
		if len(rows) == 1 {
			tagVehicle = rows[0].VehicleID
		}
	}
	switch {
	case plateVehicle > 0 && tagVehicle > 0 && plateVehicle != tagVehicle:
		t.reason = model.ReasonVehicleConflict
		return t, nil
	case plateVehicle > 0:
		t.vehicleID, t.method = i8(plateVehicle), model.MethodPlate
	case tagVehicle > 0:
		t.vehicleID, t.method = i8(tagVehicle), model.MethodTag
	default:
		t.reason = model.ReasonVehicleUnknown
		return t, nil
	}
	vehicleID := t.vehicleID.Int64
	transfers, err := q.CountTollVehicleTransfersAt(ctx, db.CountTollVehicleTransfersAtParams{OrganizationID: orgID, VehicleID: vehicleID, At: at})
	if err != nil {
		return t, err
	}
	legs, err := q.ListTollRentalLegs(ctx, db.ListTollRentalLegsParams{OrganizationID: orgID, VehicleID: vehicleID, At: at})
	if err != nil {
		return t, err
	}
	rentals := map[int64]bool{}
	for _, l := range legs {
		rentals[l.RentalID] = true
	}
	switch {
	case transfers > 0:
		// A transfer is not a rental: whoever drove, it is not the
		// customer's debt unless a person says so.
		t.reason = model.ReasonInTransfer
	case len(rentals) > 1:
		t.reason = model.ReasonOverlap
	case len(rentals) == 1:
		t.status, t.reason, t.rentalID = model.StatusMatched, "", i8(legs[0].RentalID)
	default:
		planned, err := q.ListTollPlannedRentals(ctx, db.ListTollPlannedRentalsParams{OrganizationID: orgID, VehicleID: vehicleID, At: at})
		if err != nil {
			return t, err
		}
		t.reason = model.ReasonNoRental
		for _, r := range planned {
			if !r.ActualPickupAt.Valid {
				t.reason = model.ReasonMissingActualTimes
				break
			}
			t.reason = model.ReasonOutsideActualWindow
		}
	}
	return t, nil
}

// settle applies a target to a locked pass: it reverses the live charges
// that no longer fit (another rental, another amount, no rental) and
// creates the ones that are due, then stores the status. It returns the
// rentals whose charges changed.
func (s *Service) settle(ctx context.Context, q *db.Queries, tx pgx.Tx, orgID int64, p *pass, t target, reason string) ([]uuid.UUID, error) {
	amount := money.FromNumericOrZero(p.Amount)
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		currency = "TRY"
	}
	var rental *db.GetTollRentalRow
	if t.rentalID.Valid {
		r, err := q.GetTollRental(ctx, db.GetTollRentalParams{OrganizationID: orgID, ID: t.rentalID.Int64})
		if err != nil {
			return nil, err
		}
		if r.Status == "cancelled" {
			// A rental cancelled after the assignment: back to review.
			t.status, t.reason, t.rentalID, t.manual = model.StatusNeedsReview, model.ReasonRentalCancelled, pgtype.Int8{}, false
		} else {
			rental = &r
		}
	}
	touched := []uuid.UUID{}
	chargeID, feeID := p.ChargeID, p.FeeChargeID
	// Toll amount.
	if chargeID.Valid {
		cur, err := q.GetTollCharge(ctx, db.GetTollChargeParams{OrganizationID: orgID, ID: chargeID.Int64})
		if err != nil {
			return nil, err
		}
		keep := rental != nil && cur.RentalID == rental.ID && amount.IsPositive() &&
			money.FromNumericOrZero(cur.Amount).Equal(amount) && strings.TrimSpace(cur.Currency) == currency && !cur.Reversed
		if !keep {
			r, err := s.reverse(ctx, q, tx, orgID, p, cur, reason)
			if err != nil {
				return nil, err
			}
			touched = append(touched, r)
			chargeID = pgtype.Int8{}
		}
	}
	if rental != nil && amount.IsPositive() && !chargeID.Valid {
		id, err := s.charge(ctx, q, tx, orgID, p, *rental, model.ChargeToll, amount, currency, reason)
		if err != nil {
			return nil, err
		}
		chargeID = i8(id)
		touched = append(touched, rental.Uuid)
	}
	// Processing fee: its own line, only next to a charged toll.
	if feeID.Valid {
		cur, err := q.GetTollCharge(ctx, db.GetTollChargeParams{OrganizationID: orgID, ID: feeID.Int64})
		if err != nil {
			return nil, err
		}
		if rental == nil || cur.RentalID != rental.ID || !chargeID.Valid || cur.Reversed {
			r, err := s.reverse(ctx, q, tx, orgID, p, cur, reason)
			if err != nil {
				return nil, err
			}
			touched = append(touched, r)
			feeID = pgtype.Int8{}
		}
	}
	if rental != nil && chargeID.Valid && !feeID.Valid {
		fee, err := s.processingFee(ctx, q, orgID)
		if err != nil {
			return nil, err
		}
		if fee.IsPositive() {
			id, err := s.charge(ctx, q, tx, orgID, p, *rental, model.ChargeTollFee, fee, currency, reason)
			if err != nil {
				return nil, err
			}
			feeID = i8(id)
		}
	}
	status := t.status
	switch {
	case status == model.StatusIgnored:
	case rental != nil && chargeID.Valid:
		status = model.StatusCharged
	case rental != nil:
		status = model.StatusMatched
	case status == model.StatusMatched || status == model.StatusCharged || status == "":
		status = model.StatusNeedsReview
	}
	reviewReason := t.reason
	if status == model.StatusMatched || status == model.StatusCharged || status == model.StatusIgnored {
		reviewReason = ""
	}
	rentalID := pgtype.Int8{}
	if rental != nil {
		rentalID = i8(rental.ID)
	}
	if err := q.SetTollPassMatch(ctx, db.SetTollPassMatchParams{
		OrganizationID: orgID, ID: p.ID, Status: status, ReviewReason: reviewReason, VehicleID: t.vehicleID,
		RentalID: rentalID, MatchMethod: t.method, Manual: t.manual, ChargeID: chargeID, FeeChargeID: feeID,
		Decided: t.decided, DecidedBy: actorID(ctx),
	}); err != nil {
		return nil, err
	}
	if status != p.Status || reviewReason != p.ReviewReason || rentalID != p.RentalID {
		action := model.EventMatched
		if status == model.StatusNeedsReview || status == model.StatusUnmatched {
			action = model.EventReview
		}
		if !t.decided {
			data := map[string]any{"review_reason": reviewReason, "method": t.method}
			if rental != nil {
				data["rental_uuid"], data["rental_reference"] = rental.Uuid, rental.Reference
			}
			if err := passEvent(ctx, q, orgID, p.ID, action, p.Status, status, "", data); err != nil {
				return nil, err
			}
		}
	}
	p.Status, p.ReviewReason, p.RentalID, p.VehicleID, p.ChargeID, p.FeeChargeID = status, reviewReason, rentalID, t.vehicleID, chargeID, feeID
	p.MatchMethod, p.Manual = t.method, t.manual
	return touched, nil
}

func (s *Service) processingFee(ctx context.Context, q *db.Queries, orgID int64) (money.Money, error) {
	st, err := q.GetTollSettings(ctx, orgID)
	if notFound(err) {
		return money.Zero(), nil
	}
	if err != nil {
		return money.Zero(), err
	}
	return money.FromNumericOrZero(st.ProcessingFee), nil
}

func passFacts(p pass) map[string]any {
	return map[string]any{
		"external_id": p.ExternalID, "provider": p.Provider, "passed_at": p.PassedAt.Time.UTC(), "plate": p.Plate,
		"tag_number": p.TagNumber, "station": p.Station, "exit_station": p.ExitStation,
		"amount": money.FromNumericOrZero(p.Amount).String(), "currency": strings.TrimSpace(p.Currency),
	}
}

// charge records a toll / fee charge on the rental (approved: a reported
// fact) with the rental timeline entry and the ledger posting.
func (s *Service) charge(ctx context.Context, q *db.Queries, tx pgx.Tx, orgID int64, p *pass, rental db.GetTollRentalRow,
	kind string, amount money.Money, currency, reason string) (int64, error) {
	calc := passFacts(*p)
	calc["toll_pass_uuid"] = p.Uuid
	calc["revision"] = p.Revision
	if kind == model.ChargeTollFee {
		calc = map[string]any{"toll_pass_uuid": p.Uuid, "external_id": p.ExternalID, "processing_fee": amount.String()}
	}
	note := "toll pass " + p.ExternalID
	row, err := q.InsertTollCharge(ctx, db.InsertTollChargeParams{
		OrganizationID: orgID, RentalID: rental.ID, Kind: kind, Currency: currency, Amount: amount.Numeric(),
		Calculation: mustJSON(calc), Reason: reason, TollPassID: i8(p.ID), CreatedBy: actorID(ctx), DecisionNote: note,
	})
	if err != nil {
		return 0, err
	}
	if err := q.InsertRentalEvent(ctx, db.InsertRentalEventParams{
		OrganizationID: orgID, RentalID: rental.ID, Action: model.RentalEventTollCharged, Reason: reason,
		Data: mustJSON(map[string]any{
			"charge_uuid": row.Uuid, "kind": kind, "amount": amount.String(), "toll_pass_uuid": p.Uuid,
			"external_id": p.ExternalID, "station": p.Station, "passed_at": p.PassedAt.Time.UTC(),
		}),
		ActorID: actorID(ctx),
	}); err != nil {
		return 0, err
	}
	if err := passEvent(ctx, q, orgID, p.ID, model.EventChargeCreated, "", "", reason, map[string]any{
		"charge_uuid": row.Uuid, "kind": kind, "amount": amount.String(), "rental_uuid": rental.Uuid,
		"rental_reference": rental.Reference,
	}); err != nil {
		return 0, err
	}
	if err := s.ledger.PostTollCharge(ctx, tx, model.LedgerEntry{
		OrganizationID: orgID, RentalID: rental.ID, RentalUUID: rental.Uuid, CustomerID: rental.CustomerID,
		ChargeID: row.ID, ChargeUUID: row.Uuid, Kind: kind, Amount: amount, Currency: currency,
		TollPassUUID: p.Uuid, PassedAt: p.PassedAt.Time.UTC(),
	}); err != nil {
		return 0, fmt.Errorf("tolls: ledger: %w", err)
	}
	return row.ID, nil
}

// reverse records the reversal of a charge (negative amount on the same
// rental, reversal_of_id; a charge is reversed once — unique index).
func (s *Service) reverse(ctx context.Context, q *db.Queries, tx pgx.Tx, orgID int64, p *pass, cur db.GetTollChargeRow, reason string) (uuid.UUID, error) {
	rental, err := q.GetTollRental(ctx, db.GetTollRentalParams{OrganizationID: orgID, ID: cur.RentalID})
	if err != nil {
		return uuid.Nil, err
	}
	if cur.Reversed {
		return rental.Uuid, nil
	}
	amount := money.FromNumericOrZero(cur.Amount)
	currency := strings.TrimSpace(cur.Currency)
	if reason == "" {
		reason = "toll pass changed"
	}
	row, err := q.InsertTollCharge(ctx, db.InsertTollChargeParams{
		OrganizationID: orgID, RentalID: cur.RentalID, Kind: cur.Kind, Currency: currency, Amount: amount.Neg().Numeric(),
		Calculation: mustJSON(map[string]any{"reverses": cur.Uuid, "toll_pass_uuid": p.Uuid, "external_id": p.ExternalID}),
		Reason:      reason, TollPassID: i8(p.ID), ReversalOfID: i8(cur.ID), CreatedBy: actorID(ctx),
		DecisionNote: "reversal of " + cur.Uuid.String(),
	})
	if err != nil {
		return uuid.Nil, err
	}
	if err := q.InsertRentalEvent(ctx, db.InsertRentalEventParams{
		OrganizationID: orgID, RentalID: cur.RentalID, Action: model.RentalEventTollReversed, Reason: reason,
		Data: mustJSON(map[string]any{
			"charge_uuid": row.Uuid, "reverses": cur.Uuid, "kind": cur.Kind, "amount": amount.Neg().String(),
			"toll_pass_uuid": p.Uuid, "external_id": p.ExternalID,
		}),
		ActorID: actorID(ctx),
	}); err != nil {
		return uuid.Nil, err
	}
	if err := passEvent(ctx, q, orgID, p.ID, model.EventChargeReversed, "", "", reason, map[string]any{
		"charge_uuid": row.Uuid, "reverses": cur.Uuid, "kind": cur.Kind, "amount": amount.Neg().String(),
		"rental_uuid": rental.Uuid, "rental_reference": rental.Reference,
	}); err != nil {
		return uuid.Nil, err
	}
	reversed := cur.Uuid
	if err := s.ledger.PostTollCharge(ctx, tx, model.LedgerEntry{
		OrganizationID: orgID, RentalID: cur.RentalID, RentalUUID: rental.Uuid, CustomerID: rental.CustomerID,
		ChargeID: row.ID, ChargeUUID: row.Uuid, Kind: cur.Kind, Amount: amount.Neg(), Currency: currency,
		ReversalOf: &reversed, TollPassUUID: p.Uuid, PassedAt: p.PassedAt.Time.UTC(),
	}); err != nil {
		return uuid.Nil, fmt.Errorf("tolls: ledger: %w", err)
	}
	return rental.Uuid, nil
}

func passEvent(ctx context.Context, q *db.Queries, orgID, passID int64, action, from, to, reason string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	return q.InsertTollPassEvent(ctx, db.InsertTollPassEventParams{
		OrganizationID: orgID, TollPassID: passID, Action: action, FromStatus: textOf(from), ToStatus: textOf(to),
		Reason: reason, Data: mustJSON(data), ActorID: actorID(ctx),
	})
}

// automatic re-runs the matching of a pass nobody decided on (ignored or
// manually assigned passes keep their decision; a manual assignment to a
// rental that was cancelled since goes back to review).
func (s *Service) automatic(ctx context.Context, q *db.Queries, tx pgx.Tx, orgID int64, p *pass, reason string) ([]uuid.UUID, error) {
	switch {
	case p.Status == model.StatusIgnored:
		return s.settle(ctx, q, tx, orgID, p, target{status: model.StatusIgnored, vehicleID: p.VehicleID, method: p.MatchMethod, manual: p.Manual}, reason)
	case p.Manual && p.RentalID.Valid:
		return s.settle(ctx, q, tx, orgID, p, target{
			status: model.StatusMatched, vehicleID: p.VehicleID, rentalID: p.RentalID, method: model.MethodManual, manual: true,
		}, reason)
	case p.Manual:
		return s.settle(ctx, q, tx, orgID, p, target{
			status: model.StatusUnmatched, reason: model.ReasonUnassigned, vehicleID: p.VehicleID, method: p.MatchMethod, manual: true,
		}, reason)
	}
	t, err := s.decide(ctx, q, orgID, *p)
	if err != nil {
		return nil, err
	}
	return s.settle(ctx, q, tx, orgID, p, t, reason)
}
