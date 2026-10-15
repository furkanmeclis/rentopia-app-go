package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConflictError is answered with 409 (a tag number already on a vehicle
// for an overlapping period).
type ConflictError struct{ Field, Code, Message string }

func (e *ConflictError) Error() string { return e.Message }

// vehicle resolves a vehicle in the caller's scope (home or current
// branch).
func (s *Service) vehicle(ctx context.Context, orgID int64, id uuid.UUID) (db.GetTollVehicleRow, error) {
	v, err := s.q.GetTollVehicle(ctx, db.GetTollVehicleParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return v, ErrVehicleNotFound
	}
	if err != nil {
		return v, err
	}
	if v.DeletedAt.Valid || !visible(ctx, i8(v.HomeBranchID), i8(v.CurrentBranchID)) {
		return v, ErrVehicleNotFound
	}
	return v, nil
}

func tagOf(r db.ListVehicleTollTagsRow, now time.Time) model.Tag {
	t := model.Tag{
		UUID: r.Uuid, Provider: r.Provider, Network: r.Network, TagNumber: r.TagNumber, ValidFrom: r.ValidFrom.Time.UTC(),
		ValidTo: timePtr(r.ValidTo), Note: r.Note, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	t.Active = !t.ValidFrom.After(now) && (t.ValidTo == nil || t.ValidTo.After(now))
	return t
}

// Tags lists the toll tags of a vehicle (current first).
func (s *Service) Tags(ctx context.Context, orgID int64, vehicleID uuid.UUID) ([]model.Tag, error) {
	v, err := s.vehicle(ctx, orgID, vehicleID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListVehicleTollTags(ctx, db.ListVehicleTollTagsParams{OrganizationID: orgID, VehicleID: v.ID})
	if err != nil {
		return nil, err
	}
	now := s.clock()
	out := make([]model.Tag, 0, len(rows))
	for _, r := range rows {
		out = append(out, tagOf(r, now))
	}
	return out, nil
}

type tagValues struct {
	provider, network, number, note string
	from                            time.Time
	to                              *time.Time
}

func validateTag(v tagValues) error {
	ve := &ValidationError{}
	if !providerPattern.MatchString(v.provider) {
		ve.add("provider", "invalid", "provider must be a lower-case code (2-32 letters, digits, _)")
	}
	if !model.Contains(model.Networks, v.network) {
		ve.add("network", "invalid", "network must be hgs or ogs")
	}
	key := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, v.number)
	if len(key) < 4 || len(v.number) > 40 {
		ve.add("tag_number", "invalid", "tag number must have 4-40 characters")
	}
	if v.from.IsZero() {
		ve.add("valid_from", "required", "valid_from is required")
	}
	if v.to != nil && !v.to.After(v.from) {
		ve.add("valid_to", "invalid", "valid_to must be after valid_from")
	}
	if len([]rune(v.note)) > 500 {
		ve.add("note", "too_long", "note is at most 500 characters")
	}
	return ve.orNil()
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// CreateTag links a tag to a vehicle for a period. A tag number is on one
// vehicle at a time (409 when the period overlaps another link).
func (s *Service) CreateTag(ctx context.Context, orgID int64, vehicleID uuid.UUID, in model.TagInput) (model.Tag, error) {
	v, err := s.vehicle(ctx, orgID, vehicleID)
	if err != nil {
		return model.Tag{}, err
	}
	vals := tagValues{
		provider: strings.ToLower(str(in.Provider)), network: strings.ToLower(str(in.Network)), number: str(in.TagNumber),
		note: str(in.Note), to: in.ValidTo,
	}
	if vals.network == "" {
		vals.network = "hgs"
	}
	if in.ValidFrom != nil {
		vals.from = in.ValidFrom.UTC()
	}
	if err := validateTag(vals); err != nil {
		return model.Tag{}, err
	}
	id, err := s.q.InsertVehicleTollTag(ctx, db.InsertVehicleTollTagParams{
		OrganizationID: orgID, VehicleID: v.ID, Provider: vals.provider, Network: vals.network, TagNumber: vals.number,
		ValidFrom: ts(vals.from), ValidTo: tsPtr(vals.to), Note: vals.note, CreatedBy: actorID(ctx),
	})
	if exclusionViolation(err) {
		return model.Tag{}, &ConflictError{Field: "tag_number", Code: "in_use", Message: "this tag is linked to a vehicle for an overlapping period"}
	}
	if err != nil {
		return model.Tag{}, err
	}
	s.rematchTag(ctx, orgID, vals.number)
	return s.tag(ctx, orgID, v.ID, id)
}

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(t.UTC())
}

func (s *Service) tag(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) (model.Tag, error) {
	r, err := s.q.GetVehicleTollTag(ctx, db.GetVehicleTollTagParams{OrganizationID: orgID, VehicleID: vehicleID, Uuid: id})
	if notFound(err) {
		return model.Tag{}, ErrTagNotFound
	}
	if err != nil {
		return model.Tag{}, err
	}
	return tagOf(db.ListVehicleTollTagsRow{
		ID: r.ID, Uuid: r.Uuid, Provider: r.Provider, Network: r.Network, TagNumber: r.TagNumber, ValidFrom: r.ValidFrom,
		ValidTo: r.ValidTo, Note: r.Note, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, s.clock()), nil
}

// UpdateTag changes a tag link (typically ends it: valid_to).
func (s *Service) UpdateTag(ctx context.Context, orgID int64, vehicleID, tagID uuid.UUID, in model.TagInput) (model.Tag, error) {
	v, err := s.vehicle(ctx, orgID, vehicleID)
	if err != nil {
		return model.Tag{}, err
	}
	r, err := s.q.GetVehicleTollTag(ctx, db.GetVehicleTollTagParams{OrganizationID: orgID, VehicleID: v.ID, Uuid: tagID})
	if notFound(err) {
		return model.Tag{}, ErrTagNotFound
	}
	if err != nil {
		return model.Tag{}, err
	}
	vals := tagValues{
		provider: r.Provider, network: r.Network, number: r.TagNumber, note: r.Note, from: r.ValidFrom.Time.UTC(),
		to: timePtr(r.ValidTo),
	}
	if in.Provider != nil {
		vals.provider = strings.ToLower(str(in.Provider))
	}
	if in.Network != nil {
		vals.network = strings.ToLower(str(in.Network))
	}
	if in.TagNumber != nil {
		vals.number = str(in.TagNumber)
	}
	if in.Note != nil {
		vals.note = str(in.Note)
	}
	if in.ValidFrom != nil {
		vals.from = in.ValidFrom.UTC()
	}
	if in.ValidTo != nil {
		t := in.ValidTo.UTC()
		vals.to = &t
	}
	if in.ClearValidTo {
		vals.to = nil
	}
	if err := validateTag(vals); err != nil {
		return model.Tag{}, err
	}
	_, err = s.q.UpdateVehicleTollTag(ctx, db.UpdateVehicleTollTagParams{
		OrganizationID: orgID, ID: r.ID, Provider: vals.provider, Network: vals.network, TagNumber: vals.number,
		ValidFrom: ts(vals.from), ValidTo: tsPtr(vals.to), Note: vals.note,
	})
	if exclusionViolation(err) {
		return model.Tag{}, &ConflictError{Field: "tag_number", Code: "in_use", Message: "this tag is linked to a vehicle for an overlapping period"}
	}
	if err != nil {
		return model.Tag{}, err
	}
	s.rematchTag(ctx, orgID, vals.number)
	return s.tag(ctx, orgID, v.ID, tagID)
}

// DeleteTag removes a mistaken tag link (an ended tag is kept with
// valid_to instead). Matched passes keep their rental.
func (s *Service) DeleteTag(ctx context.Context, orgID int64, vehicleID, tagID uuid.UUID) (model.Tag, error) {
	v, err := s.vehicle(ctx, orgID, vehicleID)
	if err != nil {
		return model.Tag{}, err
	}
	t, err := s.tag(ctx, orgID, v.ID, tagID)
	if err != nil {
		return model.Tag{}, err
	}
	r, err := s.q.GetVehicleTollTag(ctx, db.GetVehicleTollTagParams{OrganizationID: orgID, VehicleID: v.ID, Uuid: tagID})
	if err != nil {
		return model.Tag{}, err
	}
	if _, err := s.q.DeleteVehicleTollTag(ctx, db.DeleteVehicleTollTagParams{OrganizationID: orgID, ID: r.ID}); err != nil {
		return model.Tag{}, err
	}
	return t, nil
}

// rematchTag re-runs the matching of review passes of the tag (best
// effort; the periodic re-match covers failures).
func (s *Service) rematchTag(ctx context.Context, orgID int64, number string) {
	key := strings.ToUpper(strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, number))
	ids, err := s.q.ListTollReviewPassesByTag(ctx, db.ListTollReviewPassesByTagParams{OrganizationID: orgID, TagKey: pgtype.Text{String: key, Valid: true}})
	if err != nil {
		s.log.Warn("toll_tag_rematch_failed", "organization_id", orgID, "error", err)
		return
	}
	changed := []uuid.UUID{}
	for _, id := range ids {
		moved, err := s.rematchOne(ctx, orgID, id)
		if err != nil {
			s.log.Warn("toll_tag_rematch_failed", "organization_id", orgID, "pass", id, "error", err)
			continue
		}
		if moved {
			changed = append(changed, id)
		}
	}
	if len(changed) > 0 {
		s.publish(ctx, orgID, "bulk", changed, nil)
	}
}
