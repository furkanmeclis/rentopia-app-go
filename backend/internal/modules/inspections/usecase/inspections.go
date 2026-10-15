package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// futureTolerance lets a device time be a little ahead of the server.
const futureTolerance = 5 * time.Minute

// rental loads the rental in the caller's branch scope.
func (s *Service) rental(ctx context.Context, orgID int64, id uuid.UUID) (rentalmodel.Rental, error) {
	r, err := s.rentals.Get(ctx, orgID, id)
	if errors.Is(err, rentalusecase.ErrNotFound) {
		return r, ErrRentalNotFound
	}
	return r, err
}

// row loads an inspection and checks the rental's branch scope.
func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.GetInspectionRow, rentalmodel.Rental, error) {
	row, err := s.q.GetInspection(ctx, db.GetInspectionParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, rentalmodel.Rental{}, ErrNotFound
	}
	if err != nil {
		return row, rentalmodel.Rental{}, err
	}
	r, err := s.rental(ctx, orgID, row.RentalUuid)
	if errors.Is(err, ErrRentalNotFound) {
		return row, r, ErrNotFound
	}
	return row, r, err
}

func (s *Service) draft(ctx context.Context, orgID int64, id uuid.UUID) (db.GetInspectionRow, rentalmodel.Rental, error) {
	row, r, err := s.row(ctx, orgID, id)
	if err != nil {
		return row, r, err
	}
	if row.Status != model.StatusDraft {
		return row, r, stateErr("finalized", "a finalized form cannot change (create a new version with a reason)")
	}
	return row, r, nil
}

// Start opens the handover / return form of a rental, or returns the live
// one (draft or finalized). A handover form needs a rental in preparation,
// a return form a rental under way.
func (s *Service) Start(ctx context.Context, orgID int64, in model.StartInput) (model.Inspection, bool, error) {
	ve := &ValidationError{}
	if in.RentalUUID == nil {
		ve.add("rental_uuid", "required", "rental_uuid is required")
	}
	if !model.Contains(model.Kinds, in.Kind) {
		ve.add("kind", "invalid", "kind is handover or return")
	}
	if err := ve.orNil(); err != nil {
		return model.Inspection{}, false, err
	}
	r, err := s.rental(ctx, orgID, *in.RentalUUID)
	if errors.Is(err, ErrRentalNotFound) {
		return model.Inspection{}, false, validation("rental_uuid", "not_found", "rental was not found")
	}
	if err != nil {
		return model.Inspection{}, false, err
	}
	facts, err := s.q.GetInspectionRentalFacts(ctx, db.GetInspectionRentalFactsParams{OrganizationID: orgID, Uuid: r.UUID})
	if err != nil {
		return model.Inspection{}, false, err
	}
	live, err := s.q.GetLiveInspection(ctx, db.GetLiveInspectionParams{OrganizationID: orgID, RentalID: facts.ID, Kind: in.Kind})
	if err == nil {
		out, err := s.Get(ctx, orgID, live.Uuid)
		return out, false, err
	}
	if !notFound(err) {
		return model.Inspection{}, false, err
	}
	switch {
	case in.Kind == model.KindHandover && r.Status != rentalmodel.StatusPreparation:
		return model.Inspection{}, false, stateErr("invalid_state", "a handover form needs a rental in preparation")
	case in.Kind == model.KindReturn && r.Status != rentalmodel.StatusActive:
		return model.Inspection{}, false, stateErr("invalid_state", "a return form needs a rental under way")
	}
	tpl, tplID, err := s.ActiveTemplate(ctx, orgID)
	if err != nil {
		return model.Inspection{}, false, err
	}
	snap := model.TemplateSnapshot{UUID: tpl.UUID, Name: tpl.Name, Version: tpl.Version, PhotoSlots: tpl.PhotoSlots, Checklist: tpl.Checklist}
	var id uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		row, err := q.InsertInspection(ctx, db.InsertInspectionParams{
			OrganizationID: orgID, RentalID: facts.ID, VehicleID: facts.VehicleID, Kind: in.Kind, Version: 1,
			TemplateID: tplID, TemplateSnapshot: mustJSON(snap), Equipment: []byte("{}"), Verification: []byte("{}"),
			ChargeOverrides: []byte("[]"), CreatedBy: int8Ptr(actorID(ctx)),
		})
		id = row.Uuid
		return err
	})
	if uniqueViolation(err, "uq_inspections_live") {
		live, lerr := s.q.GetLiveInspection(ctx, db.GetLiveInspectionParams{OrganizationID: orgID, RentalID: facts.ID, Kind: in.Kind})
		if lerr != nil {
			return model.Inspection{}, false, lerr
		}
		out, gerr := s.Get(ctx, orgID, live.Uuid)
		return out, false, gerr
	}
	if err != nil {
		return model.Inspection{}, false, err
	}
	s.publish(ctx, model.ResourceInspections, "created", id)
	out, err := s.Get(ctx, orgID, id)
	return out, true, err
}

// Get returns one form with its evidence, readiness and (for a return)
// the handover record it is compared with.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Inspection, error) {
	row, r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	return s.view(ctx, orgID, row, r)
}

func (s *Service) view(ctx context.Context, orgID int64, row db.GetInspectionRow, r rentalmodel.Rental) (model.Inspection, error) {
	out := model.Inspection{
		UUID: row.Uuid, Kind: row.Kind, Version: row.Version, Status: row.Status, RevisionReason: row.RevisionReason,
		OccurredAt: timePtr(row.OccurredAt), Note: row.Note, CustomerStatement: row.CustomerStatement,
		CustomerReviewedAt: timePtr(row.CustomerReviewedAt), SignatureStatus: row.SignatureStatus, SignatureNote: row.SignatureNote,
		Disputed: row.Disputed, DisputeNote: row.DisputeNote, ReviewStatus: row.ReviewStatus, ReviewedAt: timePtr(row.ReviewedAt),
		ReviewNote: row.ReviewNote, FinalizedAt: timePtr(row.FinalizedAt), CreatedAt: row.CreatedAt.Time.UTC(),
		UpdatedAt:   row.UpdatedAt.Time.UTC(),
		ReviewedBy:  actorOf(row.ReviewedByUuid, row.ReviewedByName, row.ReviewedBySurname, row.ReviewedByEmail),
		FinalizedBy: actorOf(row.FinalizedByUuid, row.FinalizedByName, row.FinalizedBySurname, row.FinalizedByEmail),
		CreatedBy:   actorOf(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		Equipment:   map[string]bool{}, ChargeOverrides: []model.ChargeOverride{}, Damages: []model.Damage{}, Photos: []model.Photo{},
		Versions: []model.VersionRef{},
	}
	if row.Km.Valid {
		v := row.Km.Int32
		out.Km = &v
	}
	if row.Fuel.Valid {
		v := row.Fuel.Int16
		out.Fuel = &v
	}
	if row.VehicleStatusAfter.Valid {
		v := row.VehicleStatusAfter.String
		out.VehicleStatusAfter = &v
	}
	if row.PreviousUuid.Valid {
		v := uuid.UUID(row.PreviousUuid.Bytes)
		out.PreviousUUID = &v
	}
	if row.BranchUuid.Valid {
		out.Branch = &model.Ref{UUID: row.BranchUuid.Bytes, Code: row.BranchCode.String, Name: row.BranchName.String}
	}
	if row.PointUuid.Valid {
		out.Point = &model.Ref{UUID: row.PointUuid.Bytes, Name: row.PointName.String}
	}
	_ = json.Unmarshal(row.TemplateSnapshot, &out.Template)
	if out.Template.PhotoSlots == nil {
		out.Template.PhotoSlots = []model.Slot{}
	}
	if out.Template.Checklist == nil {
		out.Template.Checklist = []model.Item{}
	}
	_ = json.Unmarshal(row.Equipment, &out.Equipment)
	_ = json.Unmarshal(row.Verification, &out.Verification)
	_ = json.Unmarshal(row.ChargeOverrides, &out.ChargeOverrides)
	if out.Equipment == nil {
		out.Equipment = map[string]bool{}
	}
	if out.ChargeOverrides == nil {
		out.ChargeOverrides = []model.ChargeOverride{}
	}
	damages, photos, err := s.evidence(ctx, orgID, row.ID)
	if err != nil {
		return out, err
	}
	out.Damages, out.Photos = damages, photos
	sig, err := s.signature(ctx, orgID, row.ID)
	if err != nil {
		return out, err
	}
	out.Signature = sig
	summary, err := s.summary(ctx, orgID, r)
	if err != nil {
		return out, err
	}
	out.Rental = summary
	out.Readiness = readiness(out)
	versions, err := s.q.ListRentalInspections(ctx, db.ListRentalInspectionsParams{OrganizationID: orgID, RentalID: row.RentalID})
	if err != nil {
		return out, err
	}
	for _, v := range versions {
		if v.Kind == row.Kind {
			out.Versions = append(out.Versions, model.VersionRef{UUID: v.Uuid, Version: v.Version, Status: v.Status, FinalizedAt: timePtr(v.FinalizedAt)})
		}
	}
	if row.Kind == model.KindReturn {
		if hid, err := s.q.GetFinalizedHandoverInspection(ctx, db.GetFinalizedHandoverInspectionParams{OrganizationID: orgID, RentalID: row.RentalID}); err == nil {
			if b, err := s.baseline(ctx, orgID, hid); err == nil {
				out.Baseline = b
			}
		}
	}
	return out, nil
}

func (s *Service) baseline(ctx context.Context, orgID int64, id uuid.UUID) (*model.Baseline, error) {
	row, err := s.q.GetInspection(ctx, db.GetInspectionParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return nil, err
	}
	b := &model.Baseline{UUID: row.Uuid, Version: row.Version, OccurredAt: timePtr(row.OccurredAt), Equipment: map[string]bool{}}
	if row.Km.Valid {
		v := row.Km.Int32
		b.Km = &v
	}
	if row.Fuel.Valid {
		v := row.Fuel.Int16
		b.Fuel = &v
	}
	_ = json.Unmarshal(row.Equipment, &b.Equipment)
	b.Damages, b.Photos, err = s.evidence(ctx, orgID, row.ID)
	return b, err
}

func (s *Service) evidence(ctx context.Context, orgID, inspectionID int64) ([]model.Damage, []model.Photo, error) {
	drows, err := s.q.ListInspectionDamages(ctx, db.ListInspectionDamagesParams{OrganizationID: orgID, InspectionID: inspectionID})
	if err != nil {
		return nil, nil, err
	}
	damages := make([]model.Damage, 0, len(drows))
	for _, d := range drows {
		damages = append(damages, damageOf(d.Uuid, d.Zone, d.PosX, d.PosY, d.DamageType, d.Severity, d.Description, d.IsNew, d.Disputed, d.CreatedAt))
	}
	prows, err := s.q.ListInspectionPhotos(ctx, db.ListInspectionPhotosParams{OrganizationID: orgID, InspectionID: inspectionID})
	if err != nil {
		return nil, nil, err
	}
	photos := make([]model.Photo, 0, len(prows))
	for _, p := range prows {
		photos = append(photos, photoOf(p))
	}
	return damages, photos, nil
}

func numPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func numOf(v *float64, scale float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	if err := n.Scan(strconvFloat(math.Round(*v*scale) / scale)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

func damageOf(id uuid.UUID, zone string, x, y pgtype.Numeric, typ, severity, desc string, isNew, disputed bool, created pgtype.Timestamptz) model.Damage {
	return model.Damage{
		UUID: id, Zone: zone, PosX: numPtr(x), PosY: numPtr(y), DamageType: typ, Severity: severity, Description: desc,
		IsNew: isNew, Disputed: disputed, CreatedAt: created.Time.UTC(),
	}
}

func photoOf(p db.ListInspectionPhotosRow) model.Photo {
	out := model.Photo{
		UUID: p.Uuid, Slot: p.Slot, ClientRef: p.ClientRef.String, Status: p.Status, ContentType: p.ContentType,
		DeclaredSizeBytes: p.DeclaredSizeBytes, SizeBytes: p.SizeBytes, SHA256: p.Sha256.String,
		InspectionVersion: p.InspectionVersion, UploadedAt: timePtr(p.UploadedAt), DeviceCapturedAt: timePtr(p.DeviceCapturedAt),
		UploadedBy: actorOf(p.UploadedByUuid, p.UploadedByName, p.UploadedBySurname, p.UploadedByEmail),
		EXIF: model.PhotoEXIF{
			TakenAt: timePtr(p.ExifTakenAt), TakenLocal: p.ExifTakenLocal, GPSLat: numPtr(p.ExifGpsLat), GPSLng: numPtr(p.ExifGpsLng),
		},
		ScanStatus: p.ScanStatus, Copied: p.CopiedFromID.Valid, CreatedAt: p.CreatedAt.Time.UTC(),
	}
	var extra struct {
		Make  string `json:"make"`
		Model string `json:"model"`
	}
	_ = json.Unmarshal(p.Exif, &extra)
	out.EXIF.Make, out.EXIF.Model = extra.Make, extra.Model
	if p.DamageUuid.Valid {
		v := uuid.UUID(p.DamageUuid.Bytes)
		out.DamageUUID = &v
	}
	return out
}

func (s *Service) signature(ctx context.Context, orgID, inspectionID int64) (*model.Signature, error) {
	row, err := s.q.GetInspectionSignature(ctx, db.GetInspectionSignatureParams{OrganizationID: orgID, InspectionID: inspectionID})
	if notFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &model.Signature{
		UUID: row.Uuid, SignerName: row.SignerName, SignerRole: row.SignerRole, SHA256: row.Sha256, SizeBytes: row.SizeBytes,
		PointCount: row.PointCount, SignedAt: row.SignedAt.Time.UTC(), DeviceSignedAt: timePtr(row.DeviceSignedAt),
	}
	if row.ApprovalUuid.Valid {
		out.ContractApproval = &model.ContractApproval{
			ApprovalUUID: row.ApprovalUuid.Bytes, DocumentUUID: row.ApprovalDocumentUuid.Bytes,
			VersionUUID: row.ApprovalVersionUuid.Bytes, ContentHash: row.ApprovalContentHash.String,
		}
	}
	return out, nil
}

func (s *Service) summary(ctx context.Context, orgID int64, r rentalmodel.Rental) (model.RentalSummary, error) {
	out := model.RentalSummary{
		UUID: r.UUID, Reference: r.Reference, Status: r.Status, Customer: r.Customer, Vehicle: r.Vehicle, Product: r.Product,
		Drivers: r.Drivers, PlannedPickupAt: r.PlannedPickupAt, PlannedReturnAt: r.PlannedReturnAt, PlannedPickup: r.PlannedPickup,
		PlannedReturn: r.PlannedReturn, ActualPickupAt: r.ActualPickupAt, ActualReturnAt: r.ActualReturnAt, Contract: r.Contract,
		Legs: r.Legs,
	}
	facts, err := s.q.GetInspectionRentalFacts(ctx, db.GetInspectionRentalFactsParams{OrganizationID: orgID, Uuid: r.UUID})
	if err != nil {
		return out, err
	}
	total, paid := money.FromNumericOrZero(facts.TotalAmount), money.FromNumericOrZero(facts.PaidAmount)
	cond := rentalmodel.ConditionsFromSnapshot(facts.ProductSnapshot)
	out.Amounts = model.Amounts{
		Currency: strings.TrimSpace(facts.Currency), Total: total, Paid: paid, Balance: total.Sub(paid).ClampZero(),
		PaymentStatus: facts.PaymentStatus, DepositAmount: cond.DepositAmount, DepositStatus: "not_tracked",
	}
	return out, nil
}

// readiness computes what still blocks the completion. RT-A17: a photo row
// that is not uploaded and completed, a required slot without a completed
// photo, or a damage without one keeps the form from becoming final.
func readiness(in model.Inspection) model.Readiness {
	out := model.Readiness{RequiredSlots: []string{}, MissingSlots: []string{}, DamagesWithoutPhoto: []uuid.UUID{}, MissingFields: []string{}}
	done := map[string]bool{}
	damageDone := map[uuid.UUID]bool{}
	for _, p := range in.Photos {
		if p.Status != model.PhotoCompleted {
			out.IncompletePhotos++
			continue
		}
		done[p.Slot] = true
		if p.DamageUUID != nil {
			damageDone[*p.DamageUUID] = true
		}
	}
	for _, sl := range in.Template.PhotoSlots {
		if sl.Required {
			out.RequiredSlots = append(out.RequiredSlots, sl.Key)
			if !done[sl.Key] {
				out.MissingSlots = append(out.MissingSlots, sl.Key)
			}
		}
	}
	for _, d := range in.Damages {
		if !damageDone[d.UUID] {
			out.DamagesWithoutPhoto = append(out.DamagesWithoutPhoto, d.UUID)
		}
	}
	if in.Km == nil {
		out.MissingFields = append(out.MissingFields, "km")
	}
	if in.Fuel == nil {
		out.MissingFields = append(out.MissingFields, "fuel")
	}
	for _, it := range in.Template.Checklist {
		if _, ok := in.Equipment[it.Key]; it.Required && !ok {
			out.MissingFields = append(out.MissingFields, "equipment."+it.Key)
		}
	}
	if !in.Verification.PlateConfirmed {
		out.MissingFields = append(out.MissingFields, "verification.plate_confirmed")
	}
	if in.Kind == model.KindHandover {
		if !in.Verification.LicenceChecked {
			out.MissingFields = append(out.MissingFields, "verification.licence_checked")
		}
		if !in.Verification.IdentityChecked {
			out.MissingFields = append(out.MissingFields, "verification.identity_checked")
		}
	}
	if in.CustomerReviewedAt == nil {
		out.MissingFields = append(out.MissingFields, "customer_reviewed")
	}
	if in.Signature == nil {
		out.MissingFields = append(out.MissingFields, "signature")
	}
	out.PhotosReady = out.IncompletePhotos == 0 && len(out.MissingSlots) == 0 && len(out.DamagesWithoutPhoto) == 0
	// A missing signature is decided at the completion (waiver / refusal).
	fieldsReady := true
	for _, f := range out.MissingFields {
		if f != "signature" {
			fieldsReady = false
		}
	}
	out.CanComplete = out.PhotosReady && fieldsReady && in.Status == model.StatusDraft
	return out
}

// List returns a page of forms in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.ListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.Sortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	q := strings.TrimSpace(f.Q)
	var rentals []uuid.UUID
	if len(f.RentalUUIDs) > 0 {
		rentals = f.RentalUUIDs
	}
	rows, err := s.q.ListInspections(ctx, db.ListInspectionsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Kinds: nilIfEmpty(f.Kinds), Statuses: nilIfEmpty(f.Statuses),
		ReviewStatuses: nilIfEmpty(f.ReviewStatuses), RentalUuids: rentals, DisputedOnly: f.DisputedOnly,
		Q: pgtype.Text{String: q, Valid: q != ""}, SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ListItem, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		item := model.ListItem{
			UUID: r.Uuid, Kind: r.Kind, Version: r.Version, Status: r.Status, SignatureStatus: r.SignatureStatus,
			Disputed: r.Disputed, ReviewStatus: r.ReviewStatus, Photos: r.Photos, Damages: r.Damages,
			Rental:  model.RentalRef{UUID: r.RentalUuid, Reference: r.RentalReference, Status: r.RentalStatus},
			Vehicle: rentalmodel.VehicleRef{UUID: r.VehicleUuid, Plate: r.VehiclePlate},
			Customer: rentalmodel.CustomerRef{
				UUID: r.CustomerUuid, CustomerType: r.CustomerType,
				DisplayName: customerName(r.CustomerType, r.CustomerFirstName, r.CustomerLastName, r.CustomerCompanyTitle),
			},
			OccurredAt: timePtr(r.OccurredAt), FinalizedAt: timePtr(r.FinalizedAt), CreatedAt: r.CreatedAt.Time.UTC(),
			UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.Km.Valid {
			v := r.Km.Int32
			item.Km = &v
		}
		if r.Fuel.Valid {
			v := r.Fuel.Int16
			item.Fuel = &v
		}
		out = append(out, item)
	}
	return out, total, nil
}

func customerName(typ, first, last, company string) string {
	if typ == "corporate" && strings.TrimSpace(company) != "" {
		return company
	}
	return strings.TrimSpace(first + " " + last)
}

// Facets returns the facet counts of the list.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.ListInspectionFacets(ctx, db.ListInspectionFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Kind: []model.FacetValue{}, Status: []model.FacetValue{}, ReviewStatus: []model.FacetValue{}}
	for _, r := range rows {
		v := model.FacetValue{Value: r.Value, Count: r.Count}
		switch r.Facet {
		case "kind":
			out.Kind = append(out.Kind, v)
		case "status":
			out.Status = append(out.Status, v)
		case "review_status":
			out.ReviewStatus = append(out.ReviewStatus, v)
		}
	}
	return out, nil
}

// UpdateDraft patches a draft. Changing a computed charge amount needs
// tenant.price.override.
func (s *Service) UpdateDraft(ctx context.Context, orgID int64, id uuid.UUID, in model.DraftInput) (model.Inspection, error) {
	row, r, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	cur, err := s.view(ctx, orgID, row, r)
	if err != nil {
		return model.Inspection{}, err
	}
	ve := &ValidationError{}
	p := db.UpdateInspectionDraftParams{
		OrganizationID: orgID, ID: row.ID, OccurredAt: row.OccurredAt, BranchID: row.BranchID, PointID: row.PointID,
		Km: row.Km, Fuel: row.Fuel, Equipment: row.Equipment, Verification: row.Verification, Note: row.Note,
		CustomerStatement: row.CustomerStatement, CustomerReviewedAt: row.CustomerReviewedAt, ChargeOverrides: row.ChargeOverrides,
		VehicleStatusAfter: row.VehicleStatusAfter, Disputed: row.Disputed, DisputeNote: row.DisputeNote,
	}
	if in.OccurredAt != nil {
		if in.OccurredAt.After(s.clock().Add(futureTolerance)) {
			ve.add("occurred_at", "in_future", "the time cannot be in the future")
		}
		p.OccurredAt = tsPtr(in.OccurredAt)
	}
	if in.BranchUUID != nil {
		b, err := s.q.GetRentalBranch(ctx, db.GetRentalBranchParams{OrganizationID: orgID, Uuid: *in.BranchUUID})
		if notFound(err) {
			ve.add("branch_uuid", "not_found", "branch was not found")
		} else if err != nil {
			return model.Inspection{}, err
		} else {
			p.BranchID = int8Of(b.ID)
			p.PointID = pgtype.Int8{}
		}
	}
	if in.PointUUID != nil {
		pt, err := s.q.GetReservationPointByUUID(ctx, db.GetReservationPointByUUIDParams{OrganizationID: orgID, Uuid: *in.PointUUID})
		switch {
		case notFound(err) || (err == nil && p.BranchID.Valid && pt.BranchID != p.BranchID.Int64):
			ve.add("point_uuid", "not_found", "service point was not found at the branch")
		case err != nil:
			return model.Inspection{}, err
		default:
			p.PointID = int8Of(pt.ID)
			p.BranchID = int8Of(pt.BranchID)
		}
	}
	if in.Km != nil {
		if *in.Km < 0 || *in.Km > 5000000 {
			ve.add("km", "out_of_range", "the odometer reading is out of range")
		}
		p.Km = int4Ptr(in.Km)
	}
	if in.Fuel != nil {
		if *in.Fuel < 0 || *in.Fuel > 100 {
			ve.add("fuel", "out_of_range", "the fuel level is 0-100")
		}
		p.Fuel = int2Ptr(in.Fuel)
	}
	if in.Equipment != nil {
		eq := map[string]bool{}
		for k, v := range cur.Equipment {
			eq[k] = v
		}
		known := map[string]bool{}
		for _, it := range cur.Template.Checklist {
			known[it.Key] = true
		}
		for k, v := range in.Equipment {
			if !known[k] {
				ve.add("equipment."+k, "unknown", "not an item of the template")
				continue
			}
			eq[k] = v
		}
		p.Equipment = mustJSON(eq)
	}
	if in.Verification != nil {
		v := *in.Verification
		if v.DriverUUID != nil {
			found := false
			for _, d := range r.Drivers {
				found = found || (d.UUID != nil && *d.UUID == *v.DriverUUID)
			}
			if !found {
				ve.add("verification.driver_uuid", "not_found", "driver is not a driver of the rental")
			}
		}
		p.Verification = mustJSON(v)
	}
	if in.Note != nil {
		if len(*in.Note) > 2000 {
			ve.add("note", "too_long", "note is at most 2000 characters")
		}
		p.Note = strings.TrimSpace(*in.Note)
	}
	if in.CustomerStatement != nil {
		if len(*in.CustomerStatement) > 4000 {
			ve.add("customer_statement", "too_long", "statement is at most 4000 characters")
		}
		p.CustomerStatement = strings.TrimSpace(*in.CustomerStatement)
	}
	if in.CustomerReviewed != nil {
		if *in.CustomerReviewed {
			p.CustomerReviewedAt = ts(s.clock())
		} else {
			p.CustomerReviewedAt = pgtype.Timestamptz{}
		}
	}
	returnOnly := func(field string) {
		if row.Kind != model.KindReturn {
			ve.add(field, "return_only", "only a return form has this field")
		}
	}
	if in.VehicleStatusAfter != nil {
		returnOnly("vehicle_status_after")
		if !model.Contains(model.VehicleStatusesAfter, *in.VehicleStatusAfter) {
			ve.add("vehicle_status_after", "invalid", "cleaning, available, maintenance or preparing")
		}
		p.VehicleStatusAfter = textPtr(in.VehicleStatusAfter)
	}
	if in.Disputed != nil {
		returnOnly("disputed")
		p.Disputed = *in.Disputed
	}
	if in.DisputeNote != nil {
		if len(*in.DisputeNote) > 2000 {
			ve.add("dispute_note", "too_long", "note is at most 2000 characters")
		}
		p.DisputeNote = strings.TrimSpace(*in.DisputeNote)
	}
	if in.ChargeOverrides != nil {
		returnOnly("charge_overrides")
		list, err := s.checkOverrides(ctx, cur, *in.ChargeOverrides, ve)
		if err != nil {
			return model.Inspection{}, err
		}
		p.ChargeOverrides = mustJSON(list)
	}
	if err := ve.orNil(); err != nil {
		return model.Inspection{}, err
	}
	n, err := s.q.UpdateInspectionDraft(ctx, p)
	if err != nil {
		if immutable(err) {
			return model.Inspection{}, stateErr("finalized", "a finalized form cannot change")
		}
		return model.Inspection{}, err
	}
	if n != 1 {
		return model.Inspection{}, ErrConcurrent
	}
	return s.after(ctx, orgID, id)
}

// checkOverrides validates staff charge amounts: keys of computed charges
// (late_return, mileage, fuel, equipment:<item>) need tenant.price.override
// when the amount differs; damage:<uuid> names a damage of the form.
func (s *Service) checkOverrides(ctx context.Context, cur model.Inspection, list []model.ChargeOverride, ve *ValidationError) ([]model.ChargeOverride, error) {
	damages := map[string]bool{}
	for _, d := range cur.Damages {
		damages[d.UUID.String()] = true
	}
	items := map[string]bool{}
	for _, it := range cur.Template.Checklist {
		items[it.Key] = true
	}
	prev := map[string]model.ChargeOverride{}
	for _, o := range cur.ChargeOverrides {
		prev[o.Key] = o
	}
	out := []model.ChargeOverride{}
	seen := map[string]bool{}
	for i, o := range list {
		field := "charge_overrides." + itoa(i)
		o.Key = strings.TrimSpace(o.Key)
		o.Reason = strings.TrimSpace(o.Reason)
		kind, ref, _ := strings.Cut(o.Key, ":")
		computed := false
		switch {
		case o.Key == "late_return" || o.Key == "mileage" || o.Key == "fuel":
			computed = true
		case kind == "equipment" && items[ref]:
			computed = true
		case kind == "damage" && damages[ref]:
		default:
			ve.add(field+".key", "invalid", "unknown charge key")
			continue
		}
		if seen[o.Key] {
			ve.add(field+".key", "duplicate", "one amount per charge")
			continue
		}
		seen[o.Key] = true
		if o.Amount.IsNegative() {
			ve.add(field+".amount", "invalid", "amount must be zero or positive")
		}
		if computed {
			if p, ok := prev[o.Key]; !ok || !p.Amount.Equal(o.Amount) {
				if !has(ctx, rbac.PermTenantPriceOverride) {
					return nil, ErrForbidden
				}
			}
			if _, err := requireReason(field+".reason", o.Reason); err != nil {
				ve.add(field+".reason", "required", "a reason is required (3-500 characters)")
			}
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Key < out[b].Key })
	return out, nil
}

func (s *Service) after(ctx context.Context, orgID int64, id uuid.UUID) (model.Inspection, error) {
	s.publish(ctx, model.ResourceInspections, "updated", id)
	return s.Get(ctx, orgID, id)
}

// ------------------------------------------------------------- damages

func checkDamage(in model.DamageInput) error {
	ve := &ValidationError{}
	if !model.Contains(model.Zones, in.Zone) {
		ve.add("zone", "invalid", "unknown zone of the vehicle diagram")
	}
	if in.DamageType == "" {
		in.DamageType = "scratch"
	}
	if !model.Contains(model.DamageTypes, in.DamageType) {
		ve.add("damage_type", "invalid", "unknown damage type")
	}
	if in.Severity != "" && !model.Contains(model.Severities, in.Severity) {
		ve.add("severity", "invalid", "minor, moderate or major")
	}
	for f, v := range map[string]*float64{"pos_x": in.PosX, "pos_y": in.PosY} {
		if v != nil && (*v < 0 || *v > 1) {
			ve.add(f, "out_of_range", "a position is 0-1")
		}
	}
	if len(in.Description) > 2000 {
		ve.add("description", "too_long", "description is at most 2000 characters")
	}
	return ve.orNil()
}

func damageDefaults(in model.DamageInput, kind string) (string, string, bool) {
	typ, sev := in.DamageType, in.Severity
	if typ == "" {
		typ = "scratch"
	}
	if sev == "" {
		sev = "minor"
	}
	isNew := kind == model.KindReturn
	if in.IsNew != nil {
		isNew = *in.IsNew
	}
	return typ, sev, isNew
}

// AddDamage records a finding on the vehicle diagram.
func (s *Service) AddDamage(ctx context.Context, orgID int64, id uuid.UUID, in model.DamageInput) (model.Inspection, uuid.UUID, error) {
	row, _, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, uuid.Nil, err
	}
	if err := checkDamage(in); err != nil {
		return model.Inspection{}, uuid.Nil, err
	}
	typ, sev, isNew := damageDefaults(in, row.Kind)
	d, err := s.q.InsertInspectionDamage(ctx, db.InsertInspectionDamageParams{
		OrganizationID: orgID, InspectionID: row.ID, Zone: in.Zone, PosX: numOf(in.PosX, 1e4), PosY: numOf(in.PosY, 1e4),
		DamageType: typ, Severity: sev, Description: strings.TrimSpace(in.Description), IsNew: isNew,
		Disputed: in.Disputed && row.Kind == model.KindReturn, CreatedBy: int8Ptr(actorID(ctx)),
	})
	if err != nil {
		if immutable(err) {
			return model.Inspection{}, uuid.Nil, stateErr("finalized", "a finalized form cannot change")
		}
		return model.Inspection{}, uuid.Nil, err
	}
	out, err := s.after(ctx, orgID, id)
	return out, d.Uuid, err
}

// UpdateDamage replaces a finding.
func (s *Service) UpdateDamage(ctx context.Context, orgID int64, id, damageID uuid.UUID, in model.DamageInput) (model.Inspection, error) {
	row, _, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	d, err := s.q.GetInspectionDamage(ctx, db.GetInspectionDamageParams{OrganizationID: orgID, InspectionID: row.ID, Uuid: damageID})
	if notFound(err) {
		return model.Inspection{}, ErrDamageNotFound
	}
	if err != nil {
		return model.Inspection{}, err
	}
	if err := checkDamage(in); err != nil {
		return model.Inspection{}, err
	}
	typ, sev, isNew := damageDefaults(in, row.Kind)
	if _, err := s.q.UpdateInspectionDamage(ctx, db.UpdateInspectionDamageParams{
		OrganizationID: orgID, InspectionID: row.ID, ID: d.ID, Zone: in.Zone, PosX: numOf(in.PosX, 1e4), PosY: numOf(in.PosY, 1e4),
		DamageType: typ, Severity: sev, Description: strings.TrimSpace(in.Description), IsNew: isNew,
		Disputed: in.Disputed && row.Kind == model.KindReturn,
	}); err != nil {
		if immutable(err) {
			return model.Inspection{}, stateErr("finalized", "a finalized form cannot change")
		}
		return model.Inspection{}, err
	}
	return s.after(ctx, orgID, id)
}

// DeleteDamage removes a finding of a draft (its photos go with it).
func (s *Service) DeleteDamage(ctx context.Context, orgID int64, id, damageID uuid.UUID) (model.Inspection, error) {
	row, _, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	d, err := s.q.GetInspectionDamage(ctx, db.GetInspectionDamageParams{OrganizationID: orgID, InspectionID: row.ID, Uuid: damageID})
	if notFound(err) {
		return model.Inspection{}, ErrDamageNotFound
	}
	if err != nil {
		return model.Inspection{}, err
	}
	photos, err := s.q.ListInspectionPhotos(ctx, db.ListInspectionPhotosParams{OrganizationID: orgID, InspectionID: row.ID})
	if err != nil {
		return model.Inspection{}, err
	}
	if _, err := s.q.DeleteInspectionDamage(ctx, db.DeleteInspectionDamageParams{OrganizationID: orgID, InspectionID: row.ID, ID: d.ID}); err != nil {
		return model.Inspection{}, err
	}
	for _, p := range photos {
		if p.DamageID.Valid && p.DamageID.Int64 == d.ID && !p.CopiedFromID.Valid {
			s.deleteObject(ctx, p.ObjectKey)
		}
	}
	return s.after(ctx, orgID, id)
}
