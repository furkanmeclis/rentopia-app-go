package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var keyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func templateOf(id uuid.UUID, name string, version int32, status string, slots, checklist []byte, created, updated pgtype.Timestamptz) model.Template {
	t := model.Template{
		UUID: id, Name: name, Version: version, Status: status, PhotoSlots: []model.Slot{}, Checklist: []model.Item{},
		CreatedAt: created.Time.UTC(), UpdatedAt: updated.Time.UTC(),
	}
	_ = json.Unmarshal(slots, &t.PhotoSlots)
	_ = json.Unmarshal(checklist, &t.Checklist)
	return t
}

// ActiveTemplate returns the organization's active template, creating the
// default one (six photos, standard equipment) when there is none yet.
func (s *Service) ActiveTemplate(ctx context.Context, orgID int64) (model.Template, int64, error) {
	row, err := s.q.GetActiveInspectionTemplate(ctx, orgID)
	if err == nil {
		return templateOf(row.Uuid, row.Name, row.Version, row.Status, row.PhotoSlots, row.Checklist, row.CreatedAt, row.UpdatedAt), row.ID, nil
	}
	if !notFound(err) {
		return model.Template{}, 0, err
	}
	if _, err := s.saveTemplate(ctx, orgID, model.DefaultTemplate()); err != nil && !uniqueViolation(err, "") {
		return model.Template{}, 0, err
	}
	row, err = s.q.GetActiveInspectionTemplate(ctx, orgID)
	if err != nil {
		return model.Template{}, 0, err
	}
	return templateOf(row.Uuid, row.Name, row.Version, row.Status, row.PhotoSlots, row.Checklist, row.CreatedAt, row.UpdatedAt), row.ID, nil
}

// GetTemplate returns one template version.
func (s *Service) GetTemplate(ctx context.Context, orgID int64, id uuid.UUID) (model.Template, error) {
	row, err := s.q.GetInspectionTemplate(ctx, db.GetInspectionTemplateParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Template{}, ErrTemplateNotFound
	}
	if err != nil {
		return model.Template{}, err
	}
	return templateOf(row.Uuid, row.Name, row.Version, row.Status, row.PhotoSlots, row.Checklist, row.CreatedAt, row.UpdatedAt), nil
}

// ListTemplates returns a page of template versions (the active one is
// created on first use).
func (s *Service) ListTemplates(ctx context.Context, orgID int64, q string, statuses []string, sortField string, desc bool, limit, offset int32) ([]model.Template, int64, error) {
	if _, _, err := s.ActiveTemplate(ctx, orgID); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if sortField != "version" && sortField != "created_at" {
		sortField, desc = "version", true
	}
	q = strings.TrimSpace(q)
	rows, err := s.q.ListInspectionTemplates(ctx, db.ListInspectionTemplatesParams{
		OrganizationID: orgID, Statuses: nilIfEmpty(statuses), Q: pgtype.Text{String: q, Valid: q != ""},
		SortField: sortField, SortDesc: desc, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Template, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		t := templateOf(r.Uuid, r.Name, r.Version, r.Status, r.PhotoSlots, r.Checklist, r.CreatedAt, r.UpdatedAt)
		t.Inspections = r.Inspections
		out = append(out, t)
	}
	return out, total, nil
}

// SaveTemplate stores a new template version and makes it the active one
// (the previous version is archived; forms keep the version they used).
func (s *Service) SaveTemplate(ctx context.Context, orgID int64, in model.TemplateInput) (model.Template, error) {
	in, err := cleanTemplate(in)
	if err != nil {
		return model.Template{}, err
	}
	id, err := s.saveTemplate(ctx, orgID, in)
	if err != nil {
		return model.Template{}, err
	}
	s.publish(ctx, model.ResourceTemplates, "created", id)
	return s.GetTemplate(ctx, orgID, id)
}

func (s *Service) saveTemplate(ctx context.Context, orgID int64, in model.TemplateInput) (uuid.UUID, error) {
	var out uuid.UUID
	err := s.tx(ctx, func(q *db.Queries) error {
		version, err := q.NextInspectionTemplateVersion(ctx, orgID)
		if err != nil {
			return err
		}
		if err := q.ArchiveActiveInspectionTemplate(ctx, orgID); err != nil {
			return err
		}
		row, err := q.InsertInspectionTemplate(ctx, db.InsertInspectionTemplateParams{
			OrganizationID: orgID, Name: in.Name, Version: version, PhotoSlots: mustJSON(in.PhotoSlots),
			Checklist: mustJSON(in.Checklist), CreatedBy: int8Ptr(actorID(ctx)),
		})
		out = row.Uuid
		return err
	})
	return out, err
}

func cleanTemplate(in model.TemplateInput) (model.TemplateInput, error) {
	ve := &ValidationError{}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 2 || n > 160 {
		ve.add("name", "invalid", "name is 2-160 characters")
	}
	if len(in.PhotoSlots) == 0 || len(in.PhotoSlots) > 24 {
		ve.add("photo_slots", "invalid", "a template has 1-24 photo slots")
	}
	if len(in.Checklist) > 60 {
		ve.add("checklist", "invalid", "a template has at most 60 checklist items")
	}
	seen := map[string]bool{model.SlotDamage: true, model.SlotExtra: true}
	for i := range in.PhotoSlots {
		sl := &in.PhotoSlots[i]
		sl.Key, sl.Label = strings.TrimSpace(sl.Key), strings.TrimSpace(sl.Label)
		field := fmt.Sprintf("photo_slots.%d", i)
		if !keyRE.MatchString(sl.Key) || seen[sl.Key] {
			ve.add(field+".key", "invalid", "a unique key of lowercase letters, digits and _")
		}
		seen[sl.Key] = true
		if n := utf8.RuneCountInString(sl.Label); n < 1 || n > 60 {
			ve.add(field+".label", "invalid", "label is 1-60 characters")
		}
	}
	items := map[string]bool{}
	for i := range in.Checklist {
		it := &in.Checklist[i]
		it.Key, it.Label, it.Price = strings.TrimSpace(it.Key), strings.TrimSpace(it.Label), strings.TrimSpace(it.Price)
		field := fmt.Sprintf("checklist.%d", i)
		if !keyRE.MatchString(it.Key) || items[it.Key] {
			ve.add(field+".key", "invalid", "a unique key of lowercase letters, digits and _")
		}
		items[it.Key] = true
		if n := utf8.RuneCountInString(it.Label); n < 1 || n > 80 {
			ve.add(field+".label", "invalid", "label is 1-80 characters")
		}
		if it.Price != "" {
			if m, err := money.Parse(it.Price); err != nil || m.IsNegative() {
				ve.add(field+".price", "invalid", "price must be an amount with at most 2 decimals")
			} else {
				it.Price = m.String()
			}
		}
	}
	return in, ve.orNil()
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}
