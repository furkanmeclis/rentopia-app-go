package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Complete finalizes a draft. RT-A17: a form with a photo that is not
// uploaded and completed, a required slot without a completed photo or a
// damage without a photo cannot become final (409 INSPECTION_INCOMPLETE
// with the readiness).
//
// Handover: the rentals Handover use case records the actual time, place,
// km and fuel, makes the rental active and finalizes the form in the same
// transaction. Without a signature the handover needs a waiver
// (tenant.signature.waive). The KABİS hook runs after the commit.
//
// Return: the rentals Return use case records the return (late fee from
// the policy snapshot) and, in the same transaction, the form's charges
// (km, fuel, equipment, damages; disputed when the customer disputes or
// did not sign) are proposed, the form is finalized and the deposit task
// opens. A signed, undisputed return then finishes the return check (the
// vehicle goes to cleaning / maintenance / preparation through operation
// tasks); otherwise the form waits in the review queue.
func (s *Service) Complete(ctx context.Context, orgID int64, id uuid.UUID, in model.CompleteInput) (model.Inspection, error) {
	row, r, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	cur, err := s.view(ctx, orgID, row, r)
	if err != nil {
		return model.Inspection{}, err
	}
	rd := cur.Readiness
	fieldsMissing := false
	for _, f := range rd.MissingFields {
		fieldsMissing = fieldsMissing || f != "signature"
	}
	if !rd.PhotosReady || fieldsMissing {
		return model.Inspection{}, &StateError{Code: "inspection_incomplete", Message: "the form is not complete: every photo must be uploaded and completed", Data: rd}
	}
	sigStatus, sigNote := model.SignatureSigned, ""
	if cur.Signature == nil {
		reason, err := requireReason("no_signature_reason", in.NoSignatureReason)
		if err != nil {
			return model.Inspection{}, &StateError{Code: "signature_required", Message: "the customer's signature is required (or a reason without it)", Data: rd}
		}
		sigNote = reason
		if row.Kind == model.KindHandover {
			if !has(ctx, rbac.PermTenantSignatureWaive) {
				return model.Inspection{}, ErrForbidden
			}
			sigStatus = model.SignatureWaived
		} else {
			sigStatus = model.SignatureRefused
		}
	}
	at := s.clock()
	if cur.OccurredAt != nil {
		at = *cur.OccurredAt
	}
	revision := row.PreviousID.Valid
	if row.Kind == model.KindHandover {
		return s.completeHandover(ctx, orgID, row, r, cur, in, at, sigStatus, sigNote, revision)
	}
	return s.completeReturn(ctx, orgID, row, r, cur, at, sigStatus, sigNote, revision)
}

func (s *Service) finalize(ctx context.Context, q *db.Queries, orgID int64, row db.GetInspectionRow, at time.Time, leg pgtype.Int8, sigStatus, sigNote, review string) error {
	n, err := q.FinalizeInspection(ctx, db.FinalizeInspectionParams{
		OrganizationID: orgID, ID: row.ID, FinalizedAt: ts(s.clock()), FinalizedBy: int8Ptr(actorID(ctx)), OccurredAt: ts(at),
		RentalVehicleID: leg, SignatureStatus: sigStatus, SignatureNote: sigNote, ReviewStatus: review,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConcurrent
	}
	return rentalEvent(ctx, q, orgID, row.RentalID, rentalmodel.EventInspection, sigNote, map[string]any{
		"inspection_uuid": row.Uuid, "kind": row.Kind, "version": row.Version, "signature_status": sigStatus,
		"review_status": review, "at": at,
	})
}

func placeOf(cur model.Inspection) (*uuid.UUID, *uuid.UUID) {
	var b, p *uuid.UUID
	if cur.Branch != nil {
		v := cur.Branch.UUID
		b = &v
	}
	if cur.Point != nil {
		v := cur.Point.UUID
		p = &v
	}
	return b, p
}

func (s *Service) completeHandover(ctx context.Context, orgID int64, row db.GetInspectionRow, r rentalmodel.Rental, cur model.Inspection, in model.CompleteInput, at time.Time, sigStatus, sigNote string, revision bool) (model.Inspection, error) {
	if revision || r.Status != rentalmodel.StatusPreparation {
		if !revision {
			return model.Inspection{}, stateErr("invalid_state", "the rental is no longer in preparation")
		}
		err := s.tx(ctx, func(q *db.Queries) error {
			leg := pgtype.Int8{}
			if l, err := q.GetRentalLegForInspection(ctx, db.GetRentalLegForInspectionParams{OrganizationID: orgID, RentalID: row.RentalID, VehicleID: row.VehicleID}); err == nil {
				leg = int8Of(l)
			}
			return s.finalize(ctx, q, orgID, row, at, leg, sigStatus, sigNote, model.ReviewNone)
		})
		if err != nil {
			return model.Inspection{}, s.mapFinalizeErr(err)
		}
		return s.after(ctx, orgID, row.Uuid)
	}
	branch, point := placeOf(cur)
	_, err := s.rentals.Handover(ctx, orgID, r.UUID, rentalmodel.HandoverInput{
		At: &at, BranchUUID: branch, PointUUID: point, Km: cur.Km, Fuel: cur.Fuel, Note: cur.Note,
		WaiverReason: strings.TrimSpace(in.ContractWaiverReason),
		Finalize: func(hctx context.Context, tx pgx.Tx, legID int64) error {
			return s.finalize(hctx, s.q.WithTx(tx), orgID, row, at, int8Of(legID), sigStatus, sigNote, model.ReviewNone)
		},
	})
	if err != nil {
		return model.Inspection{}, s.mapFinalizeErr(err)
	}
	s.listener.HandedOver(ctx, orgID, r.UUID, row.Uuid, at)
	return s.after(ctx, orgID, row.Uuid)
}

func (s *Service) completeReturn(ctx context.Context, orgID int64, row db.GetInspectionRow, r rentalmodel.Rental, cur model.Inspection, at time.Time, sigStatus, sigNote string, revision bool) (model.Inspection, error) {
	review := model.ReviewNone
	if cur.Disputed || sigStatus != model.SignatureSigned {
		review = model.ReviewPending
	}
	for _, d := range cur.Damages {
		if d.Disputed {
			review = model.ReviewPending
		}
	}
	if revision || r.Status != rentalmodel.StatusActive {
		if !revision {
			return model.Inspection{}, stateErr("invalid_state", "the rental is not under way")
		}
		err := s.tx(ctx, func(q *db.Queries) error {
			leg := pgtype.Int8{}
			if l, err := q.GetRentalLegForInspection(ctx, db.GetRentalLegForInspectionParams{OrganizationID: orgID, RentalID: row.RentalID, VehicleID: row.VehicleID}); err == nil {
				leg = int8Of(l)
			}
			return s.finalize(ctx, q, orgID, row, at, leg, sigStatus, sigNote, review)
		})
		if err != nil {
			return model.Inspection{}, s.mapFinalizeErr(err)
		}
		return s.after(ctx, orgID, row.Uuid)
	}
	preview, err := s.preview(ctx, orgID, row, cur)
	if err != nil {
		return model.Inspection{}, err
	}
	// Overrides of computed amounts are re-checked at the completion.
	for _, l := range preview.Lines {
		if l.Overridden && !has(ctx, rbac.PermTenantPriceOverride) {
			return model.Inspection{}, ErrForbidden
		}
	}
	facts, err := s.q.GetInspectionRentalFacts(ctx, db.GetInspectionRentalFactsParams{OrganizationID: orgID, Uuid: r.UUID})
	if err != nil {
		return model.Inspection{}, err
	}
	branchID := facts.ReturnBranchID
	if cur.Branch != nil {
		if b, err := s.q.GetRentalBranch(ctx, db.GetRentalBranchParams{OrganizationID: orgID, Uuid: cur.Branch.UUID}); err == nil {
			branchID = b.ID
		}
	}
	unsigned := sigStatus != model.SignatureSigned
	damageNotes := []string{}
	for _, d := range cur.Damages {
		if d.IsNew {
			damageNotes = append(damageNotes, d.Zone+": "+d.DamageType)
		}
	}
	damageNote := strings.Join(damageNotes, "; ")
	if len(damageNote) > 2000 {
		damageNote = damageNote[:2000]
	}
	var depositTask uuid.UUID
	branch, point := placeOf(cur)
	_, err = s.rentals.Return(ctx, orgID, r.UUID, rentalmodel.ReturnInput{
		At: &at, BranchUUID: branch, PointUUID: point, Km: cur.Km, Fuel: cur.Fuel, Note: cur.Note, DamageNote: damageNote,
		Finalize: func(hctx context.Context, tx pgx.Tx, legID int64) error {
			q := s.q.WithTx(tx)
			if err := s.persistCharges(hctx, q, orgID, row, preview, unsigned); err != nil {
				return err
			}
			if err := s.finalize(hctx, q, orgID, row, at, int8Of(legID), sigStatus, sigNote, review); err != nil {
				return err
			}
			note := ""
			if review == model.ReviewPending {
				note = "hold: the return waits for the authorized review"
			}
			tid, err := q.InsertOperationTask(hctx, db.InsertOperationTaskParams{
				OrganizationID: orgID, TaskType: rentalmodel.TaskDeposit, BranchID: branchID, RentalID: int8Of(row.RentalID),
				VehicleID: int8Of(row.VehicleID), DueAt: ts(s.clock().Add(24 * time.Hour)), Note: note, CreatedBy: int8Ptr(actorID(ctx)),
			})
			if err != nil && !notFound(err) {
				return err
			}
			depositTask = tid
			return nil
		},
	})
	if err != nil {
		return model.Inspection{}, s.mapFinalizeErr(err)
	}
	if depositTask != uuid.Nil {
		s.publish(ctx, rentalmodel.ResourceOperationTasks, "created", depositTask)
	}
	if review == model.ReviewNone {
		status := "cleaning"
		if cur.VehicleStatusAfter != nil {
			status = *cur.VehicleStatusAfter
		}
		if _, err := s.rentals.CompleteReturn(ctx, orgID, r.UUID, rentalmodel.CompleteReturnInput{VehicleStatus: status}); err != nil {
			// The return is recorded; the check can be finished on the
			// rental page.
			s.log.Warn("inspection_complete_return_failed", "inspection", row.Uuid.String(), "error", err)
		}
	}
	return s.after(ctx, orgID, row.Uuid)
}

// persistCharges stores the form's charge proposal: the policy late fee
// the rentals Return just proposed is linked (and its staff amount
// applied), the other computed charges replace earlier proposals, and the
// damage amounts staff entered become manual proposals. Every charge stays
// proposed; disputed ones need the damage approver.
func (s *Service) persistCharges(ctx context.Context, q *db.Queries, orgID int64, row db.GetInspectionRow, p model.ChargePreview, unsigned bool) error {
	if err := q.DeleteProposedInspectionPolicyCharges(ctx, db.DeleteProposedInspectionPolicyChargesParams{
		OrganizationID: orgID, RentalID: row.RentalID,
		Kinds: []string{rentalmodel.ChargeMileage, rentalmodel.ChargeFuel, rentalmodel.ChargeEquipment},
	}); err != nil {
		return err
	}
	for _, l := range p.Lines {
		disputed := l.Disputed || unsigned
		if l.Kind == rentalmodel.ChargeLateReturn {
			late, err := q.LinkLateChargeToInspection(ctx, db.LinkLateChargeToInspectionParams{
				OrganizationID: orgID, RentalID: row.RentalID, InspectionID: int8Of(row.ID), Disputed: disputed,
			})
			if notFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if computed := money.FromNumericOrZero(late.Amount); !computed.Equal(l.Amount) {
				if _, err := q.OverrideLateCharge(ctx, db.OverrideLateChargeParams{
					OrganizationID: orgID, ID: late.ID, Amount: l.Amount.Numeric(), OverrideReason: l.Reason,
					OverriddenBy: int8Ptr(actorID(ctx)), OverriddenAt: ts(s.clock()),
				}); err != nil {
					return err
				}
			}
			continue
		}
		if !l.Amount.IsPositive() {
			continue
		}
		var original pgtype.Numeric
		var overriddenBy pgtype.Int8
		var overriddenAt pgtype.Timestamptz
		reason := l.Reason
		overrideReason := ""
		if l.Overridden && l.Computed != nil {
			original = l.Computed.Numeric()
			overriddenBy, overriddenAt = int8Ptr(actorID(ctx)), ts(s.clock())
			overrideReason = l.Reason
		}
		chargeID, err := q.InsertInspectionCharge(ctx, db.InsertInspectionChargeParams{
			OrganizationID: orgID, RentalID: row.RentalID, Kind: l.Kind, Source: l.Source, Currency: p.Currency,
			Amount: l.Amount.Numeric(), SettingsVersion: int32(p.SettingsVersion), Calculation: l.Calculation,
			Reason: reason, CreatedBy: int8Ptr(actorID(ctx)), InspectionID: int8Of(row.ID), Disputed: disputed,
			OriginalAmount: original, OverrideReason: overrideReason, OverriddenBy: overriddenBy, OverriddenAt: overriddenAt,
		})
		if err != nil {
			return err
		}
		if err := rentalEvent(ctx, q, orgID, row.RentalID, rentalmodel.EventChargeProposed, "", map[string]any{
			"charge_uuid": chargeID, "kind": l.Kind, "amount": l.Amount.String(), "source": l.Source, "disputed": disputed,
			"inspection_uuid": row.Uuid,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) mapFinalizeErr(err error) error {
	if immutable(err) {
		return stateErr("finalized", "the form is already finalized")
	}
	return err
}

// Revise opens a new version of a finalized form with a reason (the
// finalized version becomes superseded and stays immutable). The draft
// copies the facts, damages and photos (same objects and hashes); the
// customer signs again.
func (s *Service) Revise(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Inspection, error) {
	row, _, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	why, err := requireReason("reason", reason)
	if err != nil {
		return model.Inspection{}, err
	}
	if row.Status != model.StatusFinalized {
		return model.Inspection{}, stateErr("not_finalized", "only a finalized form gets a new version")
	}
	var next uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.SupersedeInspection(ctx, db.SupersedeInspectionParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		version, err := q.NextInspectionVersion(ctx, db.NextInspectionVersionParams{OrganizationID: orgID, RentalID: row.RentalID, Kind: row.Kind})
		if err != nil {
			return err
		}
		created, err := q.InsertInspection(ctx, db.InsertInspectionParams{
			OrganizationID: orgID, RentalID: row.RentalID, VehicleID: row.VehicleID, Kind: row.Kind, Version: version,
			PreviousID: int8Of(row.ID), RevisionReason: why, TemplateID: row.TemplateID, TemplateSnapshot: row.TemplateSnapshot,
			OccurredAt: row.OccurredAt, BranchID: row.BranchID, PointID: row.PointID, Km: row.Km, Fuel: row.Fuel,
			Equipment: row.Equipment, Verification: row.Verification, Note: row.Note, CustomerStatement: row.CustomerStatement,
			ChargeOverrides: row.ChargeOverrides, VehicleStatusAfter: row.VehicleStatusAfter, CreatedBy: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		next = created.Uuid
		damages, err := q.ListInspectionDamages(ctx, db.ListInspectionDamagesParams{OrganizationID: orgID, InspectionID: row.ID})
		if err != nil {
			return err
		}
		mapped := map[int64]int64{}
		for _, d := range damages {
			nd, err := q.InsertInspectionDamage(ctx, db.InsertInspectionDamageParams{
				OrganizationID: orgID, InspectionID: created.ID, Zone: d.Zone, PosX: d.PosX, PosY: d.PosY, DamageType: d.DamageType,
				Severity: d.Severity, Description: d.Description, IsNew: d.IsNew, Disputed: d.Disputed, CopiedFromID: int8Of(d.ID),
				CreatedBy: int8Ptr(actorID(ctx)),
			})
			if err != nil {
				return err
			}
			mapped[d.ID] = nd.ID
		}
		photos, err := q.ListInspectionPhotos(ctx, db.ListInspectionPhotosParams{OrganizationID: orgID, InspectionID: row.ID})
		if err != nil {
			return err
		}
		for _, p := range photos {
			if p.Status != model.PhotoCompleted {
				continue
			}
			var damage pgtype.Int8
			if p.DamageID.Valid {
				damage = int8Of(mapped[p.DamageID.Int64])
			}
			if _, err := q.InsertInspectionPhoto(ctx, db.InsertInspectionPhotoParams{
				OrganizationID: orgID, InspectionID: created.ID, RentalID: row.RentalID, InspectionVersion: version, Slot: p.Slot,
				DamageID: damage, Status: model.PhotoCompleted, ObjectKey: p.ObjectKey, ContentType: p.ContentType,
				DeclaredSizeBytes: p.DeclaredSizeBytes, SizeBytes: p.SizeBytes, Sha256: p.Sha256, UploadedAt: p.UploadedAt,
				UploadedBy: p.UploadedBy, DeviceCapturedAt: p.DeviceCapturedAt, ExifTakenAt: p.ExifTakenAt,
				ExifTakenLocal: p.ExifTakenLocal, ExifGpsLat: p.ExifGpsLat, ExifGpsLng: p.ExifGpsLng, Exif: p.Exif,
				ScanStatus: p.ScanStatus, CopiedFromID: int8Of(p.ID), CreatedBy: int8Ptr(actorID(ctx)),
			}); err != nil {
				return err
			}
		}
		return rentalEvent(ctx, q, orgID, row.RentalID, rentalmodel.EventInspectionRevised, why, map[string]any{
			"inspection_uuid": created.Uuid, "previous_uuid": row.Uuid, "kind": row.Kind, "version": version,
		})
	})
	if err != nil {
		return model.Inspection{}, s.mapFinalizeErr(err)
	}
	s.publish(ctx, model.ResourceInspections, "updated", row.Uuid)
	s.publish(ctx, model.ResourceInspections, "created", next)
	return s.Get(ctx, orgID, next)
}

// Review resolves the review of a disputed or unsigned return (the
// authorized review queue). Accepting a return without the customer's
// signature needs tenant.signature.waive. Resolving the review never
// approves a charge: disputed charges stay proposed until a member with
// tenant.damage.approve decides them one by one. The return check then
// finishes (vehicle to cleaning / maintenance / preparation).
func (s *Service) Review(ctx context.Context, orgID int64, id uuid.UUID, in model.ReviewInput) (model.Inspection, error) {
	row, r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	note, err := requireReason("note", in.Note)
	if err != nil {
		return model.Inspection{}, err
	}
	if in.VehicleStatus != "" && !model.Contains(model.VehicleStatusesAfter, in.VehicleStatus) {
		return model.Inspection{}, validation("vehicle_status", "invalid", "cleaning, available, maintenance or preparing")
	}
	if row.Status != model.StatusFinalized || row.ReviewStatus != model.ReviewPending {
		return model.Inspection{}, stateErr("no_review", "the form does not wait for a review")
	}
	if row.SignatureStatus != model.SignatureSigned && !has(ctx, rbac.PermTenantSignatureWaive) {
		return model.Inspection{}, ErrForbidden
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.ResolveInspectionReview(ctx, db.ResolveInspectionReviewParams{
			OrganizationID: orgID, ID: row.ID, ReviewedBy: int8Ptr(actorID(ctx)), ReviewedAt: ts(s.clock()), ReviewNote: note,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		return rentalEvent(ctx, q, orgID, row.RentalID, rentalmodel.EventInspectionReviewed, note, map[string]any{
			"inspection_uuid": row.Uuid, "signature_status": row.SignatureStatus, "disputed": row.Disputed,
		})
	})
	if err != nil {
		return model.Inspection{}, err
	}
	if r.Status == rentalmodel.StatusReturnCheck {
		status := in.VehicleStatus
		if status == "" && row.VehicleStatusAfter.Valid {
			status = row.VehicleStatusAfter.String
		}
		if _, err := s.rentals.CompleteReturn(ctx, orgID, r.UUID, rentalmodel.CompleteReturnInput{VehicleStatus: status, Note: note}); err != nil {
			return model.Inspection{}, err
		}
	}
	return s.after(ctx, orgID, id)
}
