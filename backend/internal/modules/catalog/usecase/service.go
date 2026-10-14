// Package usecase implements the platform vehicle catalog: TSB imports
// (dry run + apply), catalog curation and the tenant read-only search.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidRequest = errors.New("invalid request")
	ErrConflict       = errors.New("conflict")
	// ErrImportState is returned when an import is not in a state that
	// allows the requested action.
	ErrImportState = errors.New("import is not in a state that allows this action")
)

// Repository is the persistence the service needs.
type Repository interface {
	Queries() *db.Queries
	WithTx(ctx context.Context, fn func(q *db.Queries) error) error
	LoadState(ctx context.Context, period string) (model.State, error)
	Apply(ctx context.Context, plan model.Plan) (model.ApplyResult, error)
	RebuildAutoModels(ctx context.Context) (model.RebuildResult, error)
}

// Enqueuer schedules background jobs.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service is the catalog use case.
type Service struct {
	repo     Repository
	q        *db.Queries
	storage  storage.Driver
	queue    Enqueuer
	activity *activity.Recorder
	log      *slog.Logger
	events   resourceevents.Publisher
}

// New builds the service. A nil queue runs import jobs inline.
func New(repo Repository, store storage.Driver, enq Enqueuer, rec *activity.Recorder, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, q: repo.Queries(), storage: store, queue: enq, activity: rec, log: log}
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, repository.ErrConflict):
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func textPtr(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*p), Valid: true}
}

func optString(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func boolPtr(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func int4Ptr(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func validCategory(c string) bool {
	switch c {
	case tsbcatalog.CategoryPassenger, tsbcatalog.CategoryLightCommercial, tsbcatalog.CategoryOther:
		return true
	}
	return false
}

func validFuel(f string) bool {
	switch f {
	case tsbcatalog.FuelPetrol, tsbcatalog.FuelDiesel, tsbcatalog.FuelHybrid, tsbcatalog.FuelElectric, tsbcatalog.FuelLPG:
		return true
	}
	return false
}

func validTransmission(t string) bool {
	return t == tsbcatalog.TransmissionManual || t == tsbcatalog.TransmissionAutomatic
}

// ---------- brands ----------

func mapBrand(b db.ListCatalogBrandsRow) model.Brand {
	return model.Brand{
		UUID: b.Uuid, TSBCode: b.TsbCode, TSBName: b.TsbName, DisplayName: b.DisplayName,
		Category: b.Category, IsActive: b.IsActive, TypesCount: b.TypesCount,
		ModelsCount: b.ModelsCount, UnresolvedCount: b.UnresolvedCount,
		DisplayBrand: model.DisplayBrandRef{UUID: b.DisplayBrandUuid, Name: b.DisplayBrandName},
		CreatedAt:    b.CreatedAt.Time, UpdatedAt: b.UpdatedAt.Time,
	}
}

func (s *Service) ListBrands(ctx context.Context, f model.BrandFilter) ([]model.Brand, int64, error) {
	if f.Category != "" && !validCategory(f.Category) {
		return nil, 0, fmt.Errorf("%w: unknown category", ErrInvalidRequest)
	}
	did, err := s.displayBrandID(ctx, f.DisplayBrandUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListCatalogBrands(ctx, db.ListCatalogBrandsParams{
		Category: text(f.Category), IsActive: boolPtr(f.IsActive), Q: text(f.Q), DisplayBrandID: did,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCatalogBrands(ctx, db.CountCatalogBrandsParams{
		Category: text(f.Category), IsActive: boolPtr(f.IsActive), Q: text(f.Q), DisplayBrandID: did,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Brand, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapBrand(r))
	}
	return out, total, nil
}

func (s *Service) GetBrand(ctx context.Context, id uuid.UUID) (model.Brand, error) {
	b, err := s.q.GetCatalogBrandByUUID(ctx, id)
	if err != nil {
		return model.Brand{}, mapErr(repository.MapErr(err))
	}
	return mapBrand(db.ListCatalogBrandsRow(b)), nil
}

func (s *Service) PatchBrand(ctx context.Context, id uuid.UUID, in model.PatchBrandInput) (model.Brand, error) {
	if in.DisplayName != nil && strings.TrimSpace(*in.DisplayName) == "" {
		return model.Brand{}, fmt.Errorf("%w: display_name cannot be empty", ErrInvalidRequest)
	}
	if in.DisplayName != nil && in.DisplayBrandUUID != nil {
		return model.Brand{}, fmt.Errorf("%w: send display_name or display_brand_uuid, not both", ErrInvalidRequest)
	}
	if in.Category != nil && !validCategory(*in.Category) {
		return model.Brand{}, fmt.Errorf("%w: unknown category", ErrInvalidRequest)
	}
	var orphan *string
	err := s.repo.WithTx(ctx, func(q *db.Queries) error {
		b, err := q.UpdateCatalogBrand(ctx, db.UpdateCatalogBrandParams{
			Category: textPtr(in.Category), IsActive: boolPtr(in.IsActive), Uuid: id,
		})
		if err != nil {
			return err
		}
		// Re-map to another display brand: by name (created when missing)
		// or by uuid. A display brand left without TSB brands is removed.
		var targetID int64
		switch {
		case in.DisplayName != nil:
			d, err := repository.EnsureDisplayBrand(ctx, q, *in.DisplayName)
			if err != nil {
				return err
			}
			targetID = d.ID
		case in.DisplayBrandUUID != nil:
			d, err := q.GetCatalogDisplayBrandByUUID(ctx, *in.DisplayBrandUUID)
			if err != nil {
				return err
			}
			targetID = d.ID
		default:
			return nil
		}
		if _, err := q.SetCatalogBrandDisplayBrand(ctx, db.SetCatalogBrandDisplayBrandParams{
			BrandID: b.ID, DisplayBrandID: targetID,
		}); err != nil {
			return err
		}
		if b.DisplayBrandID != targetID {
			orphan, err = deleteIfEmpty(ctx, q, b.DisplayBrandID)
		}
		return err
	})
	if err != nil {
		return model.Brand{}, mapErr(repository.MapErr(err))
	}
	s.dropLogo(ctx, orphan)
	s.publish(ctx, LiveBrands, resourceevents.ActionUpdated, id)
	return s.GetBrand(ctx, id)
}

// ---------- models ----------

func (s *Service) brandID(ctx context.Context, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	b, err := s.q.GetCatalogBrandByUUID(ctx, *id)
	if err != nil {
		return pgtype.Int8{}, mapErr(repository.MapErr(err))
	}
	return pgtype.Int8{Int64: b.ID, Valid: true}, nil
}

func (s *Service) modelID(ctx context.Context, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	m, err := s.q.GetCatalogModelByUUID(ctx, *id)
	if err != nil {
		return pgtype.Int8{}, mapErr(repository.MapErr(err))
	}
	return pgtype.Int8{Int64: m.ID, Valid: true}, nil
}

func mapModel(m db.ListCatalogModelsRow) model.Model {
	return model.Model{
		UUID: m.Uuid, Name: m.Name, Source: m.Source,
		Brand: model.BrandRef{
			UUID: m.BrandUuid, DisplayName: m.BrandDisplayName, TSBCode: m.BrandTsbCode, TSBName: m.BrandTsbName,
		},
		DisplayBrand: model.DisplayBrandRef{UUID: m.DisplayBrandUuid, Name: m.DisplayBrandName},
		TypesCount:   m.TypesCount, GroupSize: m.GroupSize,
		CreatedAt: m.CreatedAt.Time, UpdatedAt: m.UpdatedAt.Time,
	}
}

func (s *Service) ListModels(ctx context.Context, f model.ModelFilter) ([]model.Model, int64, error) {
	if f.Source != "" && f.Source != model.SourceAuto && f.Source != model.SourceManual {
		return nil, 0, fmt.Errorf("%w: unknown source", ErrInvalidRequest)
	}
	bid, err := s.brandID(ctx, f.BrandUUID)
	if err != nil {
		return nil, 0, err
	}
	did, err := s.displayBrandID(ctx, f.DisplayBrandUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListCatalogModels(ctx, db.ListCatalogModelsParams{
		BrandID: bid, DisplayBrandID: did, Source: text(f.Source), Q: text(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCatalogModels(ctx, db.CountCatalogModelsParams{
		BrandID: bid, DisplayBrandID: did, Source: text(f.Source), Q: text(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Model, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapModel(r))
	}
	return out, total, nil
}

func (s *Service) GetModel(ctx context.Context, id uuid.UUID) (model.Model, error) {
	m, err := s.q.GetCatalogModelByUUID(ctx, id)
	if err != nil {
		return model.Model{}, mapErr(repository.MapErr(err))
	}
	out := mapModel(db.ListCatalogModelsRow(m))
	rules, err := s.q.ListCatalogModelRulesByModel(ctx, m.ID)
	if err != nil {
		return model.Model{}, err
	}
	out.Rules = make([]model.ModelRule, 0, len(rules))
	for _, r := range rules {
		out.Rules = append(out.Rules, model.ModelRule{Prefix: r.Prefix, CreatedAt: r.CreatedAt.Time})
	}
	return out, nil
}

func (s *Service) CreateModel(ctx context.Context, brandUUID uuid.UUID, name string) (model.Model, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return model.Model{}, fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	bid, err := s.brandID(ctx, &brandUUID)
	if err != nil {
		return model.Model{}, err
	}
	m, err := s.q.CreateCatalogModel(ctx, db.CreateCatalogModelParams{BrandID: bid.Int64, Name: name, Source: model.SourceManual})
	if err != nil {
		return model.Model{}, mapErr(repository.MapErr(err))
	}
	s.publish(ctx, LiveModels, resourceevents.ActionCreated, m.Uuid)
	return s.GetModel(ctx, m.Uuid)
}

// PatchModel renames a model. Any admin edit marks it "manual".
func (s *Service) PatchModel(ctx context.Context, id uuid.UUID, in model.PatchModelInput) (model.Model, error) {
	params := db.UpdateCatalogModelParams{Uuid: id, Source: text(model.SourceManual)}
	if in.Name != nil {
		name := strings.Join(strings.Fields(*in.Name), " ")
		if name == "" {
			return model.Model{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
		}
		params.Name = text(name)
	}
	if in.Source != nil {
		if *in.Source != model.SourceAuto && *in.Source != model.SourceManual {
			return model.Model{}, fmt.Errorf("%w: unknown source", ErrInvalidRequest)
		}
		params.Source = text(*in.Source)
	}
	if _, err := s.q.UpdateCatalogModel(ctx, params); err != nil {
		return model.Model{}, mapErr(repository.MapErr(err))
	}
	s.publish(ctx, LiveModels, resourceevents.ActionUpdated, id)
	return s.GetModel(ctx, id)
}

// MergeModel moves the types and rules of src into dst and deletes src.
// Models of two TSB brands under one display brand cannot share rows (a
// type keeps its TSB brand), so there src is renamed to dst's name: both
// then form one grouped model for tenants. When src's brand already has a
// model of that name, src is merged into it instead.
func (s *Service) MergeModel(ctx context.Context, src, dst uuid.UUID) (model.Model, error) {
	if src == dst {
		return model.Model{}, fmt.Errorf("%w: cannot merge a model into itself", ErrInvalidRequest)
	}
	from, err := s.q.GetCatalogModelByUUID(ctx, src)
	if err != nil {
		return model.Model{}, mapErr(repository.MapErr(err))
	}
	to, err := s.q.GetCatalogModelByUUID(ctx, dst)
	if err != nil {
		return model.Model{}, mapErr(repository.MapErr(err))
	}
	if from.BrandID != to.BrandID {
		if from.DisplayBrandUuid != to.DisplayBrandUuid {
			return model.Model{}, fmt.Errorf("%w: models belong to different display brands", ErrInvalidRequest)
		}
		same, err := s.q.GetCatalogModelByBrandName(ctx, db.GetCatalogModelByBrandNameParams{BrandID: from.BrandID, Name: to.Name})
		switch {
		case err == nil && same.ID != from.ID:
			return s.MergeModel(ctx, src, same.Uuid)
		case err == nil || errors.Is(repository.MapErr(err), repository.ErrNotFound):
			if _, err := s.q.UpdateCatalogModel(ctx, db.UpdateCatalogModelParams{
				Uuid: from.Uuid, Name: text(to.Name), Source: text(model.SourceManual),
			}); err != nil {
				return model.Model{}, mapErr(repository.MapErr(err))
			}
			return s.GetModel(ctx, dst)
		default:
			return model.Model{}, err
		}
	}
	err = s.repo.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.ReassignCatalogTypesModel(ctx, db.ReassignCatalogTypesModelParams{
			ToModelID:   pgtype.Int8{Int64: to.ID, Valid: true},
			FromModelID: pgtype.Int8{Int64: from.ID, Valid: true},
		}); err != nil {
			return err
		}
		if _, err := q.ReassignCatalogModelRules(ctx, db.ReassignCatalogModelRulesParams{ToModelID: to.ID, FromModelID: from.ID}); err != nil {
			return err
		}
		_, err := q.DeleteCatalogModel(ctx, from.ID)
		return err
	})
	if err != nil {
		return model.Model{}, mapErr(repository.MapErr(err))
	}
	s.publish(ctx, LiveModels, resourceevents.ActionDeleted, src)
	s.publish(ctx, LiveModels, resourceevents.ActionUpdated, dst)
	// The types of src now point at dst.
	s.publish(ctx, LiveTypes, resourceevents.ActionBulk)
	return s.GetModel(ctx, dst)
}

// ---------- types ----------

func mapTypeRow(t db.ListCatalogTypesRow) model.Type {
	out := model.Type{
		UUID: t.Uuid, TSBTypeCode: t.TsbTypeCode, RawName: t.RawName, Years: t.Years,
		SuggestedTransmission: optString(t.SuggestedTransmission), SuggestedFuel: optString(t.SuggestedFuel),
		FirstSeenPeriod: t.FirstSeenPeriod, LastSeenPeriod: t.LastSeenPeriod, IsActive: t.IsActive,
		Brand:        model.BrandRef{UUID: t.BrandUuid, DisplayName: t.BrandDisplayName, TSBCode: t.BrandTsbCode},
		DisplayBrand: model.DisplayBrandRef{UUID: t.DisplayBrandUuid, Name: t.DisplayBrandName},
		CreatedAt:    t.CreatedAt.Time, UpdatedAt: t.UpdatedAt.Time,
	}
	if out.Years == nil {
		out.Years = []int32{}
	}
	if t.ModelUuid.Valid {
		out.Model = &model.ModelRef{UUID: uuid.UUID(t.ModelUuid.Bytes), Name: t.ModelName.String, Source: t.ModelSource.String}
	}
	return out
}

func (s *Service) ListTypes(ctx context.Context, f model.TypeFilter) ([]model.Type, int64, error) {
	if f.Category != "" && !validCategory(f.Category) {
		return nil, 0, fmt.Errorf("%w: unknown category", ErrInvalidRequest)
	}
	if f.Fuel != "" && !validFuel(f.Fuel) {
		return nil, 0, fmt.Errorf("%w: unknown fuel", ErrInvalidRequest)
	}
	if f.Transmission != "" && !validTransmission(f.Transmission) {
		return nil, 0, fmt.Errorf("%w: unknown transmission", ErrInvalidRequest)
	}
	bid, err := s.brandID(ctx, f.BrandUUID)
	if err != nil {
		return nil, 0, err
	}
	mid, err := s.modelID(ctx, f.ModelUUID)
	if err != nil {
		return nil, 0, err
	}
	did, err := s.displayBrandID(ctx, f.DisplayBrandUUID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListCatalogTypes(ctx, db.ListCatalogTypesParams{
		BrandID: bid, DisplayBrandID: did, Category: text(f.Category), ModelID: mid, Year: int4Ptr(f.Year), Unresolved: f.Unresolved,
		IsActive: boolPtr(f.IsActive), Fuel: text(f.Fuel), Transmission: text(f.Transmission), Q: text(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCatalogTypes(ctx, db.CountCatalogTypesParams{
		BrandID: bid, DisplayBrandID: did, Category: text(f.Category), ModelID: mid, Year: int4Ptr(f.Year), Unresolved: f.Unresolved,
		IsActive: boolPtr(f.IsActive), Fuel: text(f.Fuel), Transmission: text(f.Transmission), Q: text(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Type, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapTypeRow(r))
	}
	return out, total, nil
}

func (s *Service) GetType(ctx context.Context, id uuid.UUID) (model.TypeDetail, error) {
	t, err := s.q.GetCatalogTypeByUUID(ctx, id)
	if err != nil {
		return model.TypeDetail{}, mapErr(repository.MapErr(err))
	}
	detail := model.TypeDetail{
		Type: mapTypeRow(db.ListCatalogTypesRow{
			ID: t.ID, Uuid: t.Uuid, TsbTypeCode: t.TsbTypeCode, RawName: t.RawName, Years: t.Years,
			SuggestedTransmission: t.SuggestedTransmission, SuggestedFuel: t.SuggestedFuel,
			FirstSeenPeriod: t.FirstSeenPeriod, LastSeenPeriod: t.LastSeenPeriod, IsActive: t.IsActive,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, BrandUuid: t.BrandUuid,
			BrandDisplayName: t.BrandDisplayName, BrandTsbCode: t.BrandTsbCode,
			DisplayBrandUuid: t.DisplayBrandUuid, DisplayBrandName: t.DisplayBrandName,
			ModelUuid: t.ModelUuid, ModelName: t.ModelName, ModelSource: t.ModelSource,
		}),
		BrandTSBName: t.BrandTsbName,
		Valuations:   []model.Valuation{},
		History:      []model.Valuation{},
	}
	vals, err := s.q.ListCatalogTypeValuations(ctx, t.ID)
	if err != nil {
		return model.TypeDetail{}, err
	}
	for _, v := range vals {
		f, _ := repository.NumericToFloat(v.Value)
		val := model.Valuation{ModelYear: v.ModelYear, Period: v.Period, Value: f}
		if detail.LatestPeriod == "" {
			detail.LatestPeriod = v.Period
		}
		if v.Period == detail.LatestPeriod {
			detail.Valuations = append(detail.Valuations, val)
		}
		detail.History = append(detail.History, val)
	}
	return detail, nil
}

// PatchType assigns a model (optionally saving a prefix rule) and/or
// overrides the gearbox / fuel suggestions.
func (s *Service) PatchType(ctx context.Context, id uuid.UUID, in model.PatchTypeInput) (model.TypeDetail, error) {
	t, err := s.q.GetCatalogTypeByUUID(ctx, id)
	if err != nil {
		return model.TypeDetail{}, mapErr(repository.MapErr(err))
	}
	params := db.UpdateCatalogTypeParams{Uuid: id}
	var target db.GetCatalogModelByUUIDRow
	if in.SetModel {
		params.SetModel = true
		if in.ModelUUID != nil {
			target, err = s.q.GetCatalogModelByUUID(ctx, *in.ModelUUID)
			if err != nil {
				return model.TypeDetail{}, mapErr(repository.MapErr(err))
			}
			if target.BrandID != t.BrandID {
				// A model of a sibling TSB brand (same display brand): use
				// the same-name model of the type's own brand, created when
				// missing, so both group into one model.
				if target.DisplayBrandUuid != t.DisplayBrandUuid {
					return model.TypeDetail{}, fmt.Errorf("%w: model belongs to another brand", ErrInvalidRequest)
				}
				own, err := s.sameNameModel(ctx, t.BrandID, target.Name)
				if err != nil {
					return model.TypeDetail{}, err
				}
				target.ID, target.Name, target.BrandID = own.ID, own.Name, own.BrandID
			}
			params.ModelID = pgtype.Int8{Int64: target.ID, Valid: true}
		}
	}
	if in.SaveRule && (!in.SetModel || in.ModelUUID == nil) {
		return model.TypeDetail{}, fmt.Errorf("%w: save_rule needs model_uuid", ErrInvalidRequest)
	}
	prefix := strings.Join(strings.Fields(in.RulePrefix), " ")
	if in.SaveRule {
		if prefix == "" {
			prefix = target.Name
		}
		if !tsbcatalog.HasPrefixToken(t.RawName, prefix) {
			return model.TypeDetail{}, fmt.Errorf("%w: rule_prefix must be a prefix of the type name", ErrInvalidRequest)
		}
	}
	if in.SetTransmission {
		params.SetTransmission = true
		if in.SuggestedTransmission != nil && *in.SuggestedTransmission != "" {
			if !validTransmission(*in.SuggestedTransmission) {
				return model.TypeDetail{}, fmt.Errorf("%w: unknown transmission", ErrInvalidRequest)
			}
			params.SuggestedTransmission = text(*in.SuggestedTransmission)
		}
	}
	if in.SetFuel {
		params.SetFuel = true
		if in.SuggestedFuel != nil && *in.SuggestedFuel != "" {
			if !validFuel(*in.SuggestedFuel) {
				return model.TypeDetail{}, fmt.Errorf("%w: unknown fuel", ErrInvalidRequest)
			}
			params.SuggestedFuel = text(*in.SuggestedFuel)
		}
	}
	err = s.repo.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.UpdateCatalogType(ctx, params); err != nil {
			return err
		}
		if !in.SaveRule {
			return nil
		}
		if _, err := q.UpsertCatalogModelRule(ctx, db.UpsertCatalogModelRuleParams{
			BrandID: t.BrandID, Prefix: prefix, ModelID: target.ID,
		}); err != nil {
			return err
		}
		// Apply the rule to the brand's types that have no model yet or an
		// auto-derived one.
		cands, err := q.ListCatalogTypesForRule(ctx, t.BrandID)
		if err != nil {
			return err
		}
		ids := make([]int64, 0)
		for _, c := range cands {
			if tsbcatalog.HasPrefixToken(c.RawName, prefix) {
				ids = append(ids, c.ID)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		_, err = q.SetCatalogTypesModel(ctx, db.SetCatalogTypesModelParams{
			ModelID: pgtype.Int8{Int64: target.ID, Valid: true}, Ids: ids,
		})
		return err
	})
	if err != nil {
		return model.TypeDetail{}, mapErr(repository.MapErr(err))
	}
	if in.SaveRule {
		// The rule may have reassigned other types of the brand too.
		s.publish(ctx, LiveTypes, resourceevents.ActionBulk)
	} else {
		s.publish(ctx, LiveTypes, resourceevents.ActionUpdated, id)
	}
	return s.GetType(ctx, id)
}

// ---------- shared helpers ----------

func actorRef(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// enqueue schedules a catalog import step, or runs it inline without a queue.
func (s *Service) enqueue(ctx context.Context, importID int64, action string) error {
	if s.queue == nil {
		return s.ProcessImport(ctx, importID, action)
	}
	task, err := queue.NewCatalogImportTask(importID, action)
	if err != nil {
		return err
	}
	_, err = s.queue.Enqueue(task, asynq.Queue(queue.QueueImports), asynq.MaxRetry(1), asynq.Timeout(15*time.Minute))
	return err
}
