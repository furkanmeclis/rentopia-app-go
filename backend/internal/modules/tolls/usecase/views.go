package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxExport caps exports.
const MaxExport = 10000

// List returns a page of passes in the caller's branch scope.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Pass, int64, error) {
	p := db.ListTollPassesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.SortField == "" {
		p.SortField, p.SortDesc = "passed_at", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 25
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	if len(f.Sources) > 0 {
		p.Sources = f.Sources
	}
	if len(f.Providers) > 0 {
		p.Providers = f.Providers
	}
	if len(f.ReviewReasons) > 0 {
		p.ReviewReasons = f.ReviewReasons
	}
	if len(f.VehicleUUIDs) > 0 {
		p.VehicleUuids = f.VehicleUUIDs
	}
	if len(f.RentalUUIDs) > 0 {
		p.RentalUuids = f.RentalUUIDs
	}
	if f.HasRental != nil {
		p.HasRental = pgtype.Bool{Bool: *f.HasRental, Valid: true}
	}
	if f.From != nil {
		p.PassedFrom = ts(*f.From)
	}
	if f.To != nil {
		p.PassedTo = ts(*f.To)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListTollPasses(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Pass, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		item := model.Pass{
			UUID: r.Uuid, Provider: r.Provider, ExternalID: r.ExternalID, Source: r.Source, PassedAt: r.PassedAt.Time.UTC(),
			Plate: r.Plate, TagNumber: r.TagNumber, Station: r.Station, ExitStation: r.ExitStation,
			Currency: strings.TrimSpace(r.Currency), Amount: money.FromNumericOrZero(r.Amount), Status: r.Status,
			ReviewReason: r.ReviewReason, MatchMethod: r.MatchMethod, Manual: r.Manual, Revision: r.Revision,
			LastFetchedAt: r.LastFetchedAt.Time.UTC(), CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if r.VehicleUuid.Valid {
			item.Vehicle = &model.Ref{UUID: r.VehicleUuid.Bytes, Plate: r.VehiclePlate.String}
		}
		if r.RentalUuid.Valid {
			item.Rental = &model.RentalRef{UUID: r.RentalUuid.Bytes, Reference: r.RentalReference.String, Status: r.RentalStatus.String}
		}
		if r.FeeAmount.Valid {
			fee := money.FromNumericOrZero(r.FeeAmount)
			item.FeeAmount = &fee
		}
		out = append(out, item)
	}
	return out, total, nil
}

// Facets returns the dynamic filter options and the review queue count.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := s.q.TollPassFacets(ctx, db.TollPassFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Status: []model.Facet{}, Source: []model.Facet{}, Provider: []model.Facet{}, ReviewReason: []model.Facet{}}
	for _, r := range rows {
		f := model.Facet{Value: r.Value, Count: r.Count}
		switch r.Facet {
		case "status":
			out.Status = append(out.Status, f)
			if model.Contains(model.ReviewStatuses, r.Value) {
				out.Review += r.Count
			}
		case "source":
			out.Source = append(out.Source, f)
		case "provider":
			out.Provider = append(out.Provider, f)
		case "review_reason":
			out.ReviewReason = append(out.ReviewReason, f)
		}
	}
	return out, nil
}

// Get returns a pass with its revisions, timeline and charges.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.PassDetail, error) {
	r, err := s.q.GetTollPass(ctx, db.GetTollPassParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.PassDetail{}, ErrNotFound
	}
	if err != nil {
		return model.PassDetail{}, err
	}
	ids := []pgtype.Int8{r.VehicleHomeBranchID, r.VehicleCurrentBranchID, r.RentalPickupBranchID, r.RentalReturnBranchID, r.RentalActualReturnBranchID}
	scoped := false
	for _, b := range ids {
		scoped = scoped || b.Valid
	}
	if !scoped {
		ids = []pgtype.Int8{{}}
	}
	if !visible(ctx, ids...) {
		return model.PassDetail{}, ErrNotFound
	}
	out := model.PassDetail{
		Pass: model.Pass{
			UUID: r.Uuid, Provider: r.Provider, ExternalID: r.ExternalID, Source: r.Source, PassedAt: r.PassedAt.Time.UTC(),
			Plate: r.Plate, TagNumber: r.TagNumber, Station: r.Station, ExitStation: r.ExitStation,
			Currency: strings.TrimSpace(r.Currency), Amount: money.FromNumericOrZero(r.Amount), Status: r.Status,
			ReviewReason: r.ReviewReason, MatchMethod: r.MatchMethod, Manual: r.Manual, Revision: r.Revision,
			LastFetchedAt: r.LastFetchedAt.Time.UTC(), CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		},
		FirstFetchedAt: r.FirstFetchedAt.Time.UTC(), Raw: json.RawMessage(r.Raw), DecidedAt: timePtr(r.DecidedAt),
		DecidedBy: actorOf(r.DecidedByUuid, r.DecidedByName, r.DecidedBySurname, r.DecidedByEmail),
		Revisions: []model.Revision{}, Events: []model.Event{}, Charges: []model.Charge{},
	}
	if r.VehicleUuid.Valid {
		out.Vehicle = &model.Ref{UUID: r.VehicleUuid.Bytes, Plate: r.VehiclePlate.String}
	}
	if r.RentalUuid.Valid {
		out.Rental = &model.RentalRef{UUID: r.RentalUuid.Bytes, Reference: r.RentalReference.String, Status: r.RentalStatus.String}
	}
	if r.AccountUuid.Valid {
		out.Account = &model.AccountRef{UUID: r.AccountUuid.Bytes, DisplayName: r.AccountName.String}
	}
	if r.FeeAmount.Valid {
		fee := money.FromNumericOrZero(r.FeeAmount)
		out.FeeAmount = &fee
	}
	revs, err := s.q.ListTollPassRevisions(ctx, db.ListTollPassRevisionsParams{OrganizationID: orgID, TollPassID: r.ID})
	if err != nil {
		return model.PassDetail{}, err
	}
	for _, v := range revs {
		out.Revisions = append(out.Revisions, model.Revision{
			UUID: v.Uuid, Revision: v.Revision, Source: v.Source, ChangedFields: v.ChangedFields,
			Previous: json.RawMessage(v.Previous), Current: json.RawMessage(v.Current),
			AmountBefore: money.FromNumericOrZero(v.AmountBefore), AmountAfter: money.FromNumericOrZero(v.AmountAfter),
			CreatedAt: v.CreatedAt.Time.UTC(),
		})
	}
	events, err := s.q.ListTollPassEvents(ctx, db.ListTollPassEventsParams{OrganizationID: orgID, TollPassID: r.ID})
	if err != nil {
		return model.PassDetail{}, err
	}
	for _, e := range events {
		out.Events = append(out.Events, model.Event{
			UUID: e.Uuid, Action: e.Action, FromStatus: e.FromStatus.String, ToStatus: e.ToStatus.String, Reason: e.Reason,
			Data: json.RawMessage(e.Data), Actor: actorOf(e.ActorUuid, e.ActorName, e.ActorSurname, e.ActorEmail),
			CreatedAt: e.CreatedAt.Time.UTC(),
		})
	}
	charges, err := s.q.ListTollPassCharges(ctx, db.ListTollPassChargesParams{OrganizationID: orgID, TollPassID: pgtype.Int8{Int64: r.ID, Valid: true}})
	if err != nil {
		return model.PassDetail{}, err
	}
	for _, c := range charges {
		item := model.Charge{
			UUID: c.Uuid, Kind: c.Kind, Status: c.Status, Currency: strings.TrimSpace(c.Currency),
			Amount: money.FromNumericOrZero(c.Amount), Reason: c.Reason, CreatedAt: c.CreatedAt.Time.UTC(),
			Rental: model.RentalRef{UUID: c.RentalUuid, Reference: c.RentalReference},
		}
		if c.ReversalOfUuid.Valid {
			u := uuid.UUID(c.ReversalOfUuid.Bytes)
			item.ReversalOfUUID = &u
		}
		out.Charges = append(out.Charges, item)
	}
	return out, nil
}
