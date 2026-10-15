package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxBodyBytes caps a template body (sanitized HTML).
const MaxBodyBytes = 512 << 10

func actorOf(id pgtype.UUID, name, surname, email pgtype.Text) *model.Actor {
	if !id.Valid {
		return nil
	}
	return &model.Actor{UUID: id.Bytes, Name: name.String, Surname: surname.String, Email: email.String}
}

func templateView(row db.GetContractTemplateRow) model.Template {
	t := model.Template{
		UUID: row.Uuid, Kind: row.Kind, Name: row.Name, Version: row.Version, Status: row.Status,
		Content: json.RawMessage(row.Content), BodyHTML: row.BodyHtml, Placeholders: row.Placeholders, UsedBy: row.UsedBy,
		ActivatedAt: timePtr(row.ActivatedAt), ArchivedAt: timePtr(row.ArchivedAt),
		CreatedBy:   actorOf(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		ActivatedBy: actorOf(row.ActivatedByUuid, row.ActivatedByName, row.ActivatedBySurname, row.ActivatedByEmail),
		CreatedAt:   row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if t.Placeholders == nil {
		t.Placeholders = []string{}
	}
	return t
}

func (s *Service) templateRow(ctx context.Context, orgID int64, id uuid.UUID) (db.GetContractTemplateRow, error) {
	row, err := s.q.GetContractTemplate(ctx, db.GetContractTemplateParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrTemplateNotFound
	}
	return row, err
}

// GetTemplate returns one template version.
func (s *Service) GetTemplate(ctx context.Context, orgID int64, id uuid.UUID) (model.Template, error) {
	row, err := s.templateRow(ctx, orgID, id)
	if err != nil {
		return model.Template{}, err
	}
	return templateView(row), nil
}

// ListTemplates returns a page of template versions.
func (s *Service) ListTemplates(ctx context.Context, orgID int64, f model.TemplateFilter) ([]model.Template, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if _, ok := model.TemplateSortable[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	q := strings.TrimSpace(f.Q)
	rows, err := s.q.ListContractTemplates(ctx, db.ListContractTemplatesParams{
		OrganizationID: orgID, Kinds: nilIfEmpty(f.Kinds), Statuses: nilIfEmpty(f.Statuses), Q: textOf(q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Template, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		t := model.Template{
			UUID: r.Uuid, Kind: r.Kind, Name: r.Name, Version: r.Version, Status: r.Status, Placeholders: r.Placeholders,
			ActivatedAt: timePtr(r.ActivatedAt), ArchivedAt: timePtr(r.ArchivedAt),
			CreatedBy: actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
			CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
		}
		if t.Placeholders == nil {
			t.Placeholders = []string{}
		}
		out = append(out, t)
	}
	return out, total, nil
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// checkBody sanitizes a body and validates its placeholders.
func checkBody(ve *ValidationError, body string) (string, []string) {
	clean := Sanitize(body)
	if len(clean) > MaxBodyBytes {
		ve.add("body_html", "too_large", "the template body is at most 512 KiB")
	}
	known, unknown := Tokens(clean)
	if len(unknown) > 0 {
		ve.add("body_html", "unknown_placeholder", "unknown placeholders: "+strings.Join(unknown, ", "))
	}
	return clean, known
}

func checkName(ve *ValidationError, name string) string {
	n := strings.TrimSpace(name)
	if utf8.RuneCountInString(n) < 2 || utf8.RuneCountInString(n) > 160 {
		ve.add("name", "invalid", "name is 2-160 characters")
	}
	return n
}

func contentOf(raw json.RawMessage) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	return raw
}

// CreateTemplate creates the next draft version of a kind.
func (s *Service) CreateTemplate(ctx context.Context, orgID int64, in model.TemplateInput) (model.Template, error) {
	ve := &ValidationError{}
	if !model.Contains(model.Kinds, in.Kind) {
		ve.add("kind", "invalid", "unknown template kind")
	}
	name := ""
	if in.Name == nil {
		ve.add("name", "required", "name is required")
	} else {
		name = checkName(ve, *in.Name)
	}
	body := ""
	if in.BodyHTML != nil {
		body = *in.BodyHTML
	}
	clean, keys := checkBody(ve, body)
	if len(in.Content) > 0 && !json.Valid(in.Content) {
		ve.add("content", "invalid", "content must be JSON")
	}
	if err := ve.orNil(); err != nil {
		return model.Template{}, err
	}
	return s.insertTemplate(ctx, orgID, in.Kind, name, contentOf(in.Content), clean, keys)
}

func (s *Service) insertTemplate(ctx context.Context, orgID int64, kind, name string, content []byte, body string, keys []string) (model.Template, error) {
	var row db.InsertContractTemplateRow
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		row, err = s.q.InsertContractTemplate(ctx, db.InsertContractTemplateParams{
			OrganizationID: orgID, Kind: kind, Name: name, Content: content, BodyHtml: body, Placeholders: keys,
			CreatedBy: int8Ptr(actorID(ctx)),
		})
		if !uniqueViolation(err, "uq_contract_templates_organization_kind_version") {
			break
		}
	}
	if err != nil {
		return model.Template{}, err
	}
	s.publish(ctx, model.ResourceTemplates, "created", row.Uuid)
	return s.GetTemplate(ctx, orgID, row.Uuid)
}

// UpdateTemplate edits a draft (an active or archived version is
// immutable: create the next version).
func (s *Service) UpdateTemplate(ctx context.Context, orgID int64, id uuid.UUID, in model.TemplateInput) (model.Template, error) {
	row, err := s.templateRow(ctx, orgID, id)
	if err != nil {
		return model.Template{}, err
	}
	if row.Status != model.TemplateDraft {
		return model.Template{}, stateErr("not_draft", "only a draft can be edited; create a new version")
	}
	ve := &ValidationError{}
	if in.Kind != "" && in.Kind != row.Kind {
		ve.add("kind", "immutable", "the kind of a template cannot change")
	}
	name := row.Name
	if in.Name != nil {
		name = checkName(ve, *in.Name)
	}
	body, keys := row.BodyHtml, row.Placeholders
	if in.BodyHTML != nil {
		body, keys = checkBody(ve, *in.BodyHTML)
	}
	content := row.Content
	if in.Content != nil {
		if !json.Valid(in.Content) {
			ve.add("content", "invalid", "content must be JSON")
		}
		content = contentOf(in.Content)
	}
	if err := ve.orNil(); err != nil {
		return model.Template{}, err
	}
	n, err := s.q.UpdateContractTemplate(ctx, db.UpdateContractTemplateParams{
		OrganizationID: orgID, ID: row.ID, Name: name, Content: content, BodyHtml: body, Placeholders: keys,
	})
	if err != nil {
		return model.Template{}, err
	}
	if n != 1 {
		return model.Template{}, stateErr("not_draft", "only a draft can be edited; create a new version")
	}
	s.publish(ctx, model.ResourceTemplates, "updated", id)
	return s.GetTemplate(ctx, orgID, id)
}

// DeleteTemplate removes a draft.
func (s *Service) DeleteTemplate(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.templateRow(ctx, orgID, id)
	if err != nil {
		return err
	}
	if row.Status != model.TemplateDraft {
		return stateErr("not_draft", "only a draft can be deleted")
	}
	if _, err := s.q.DeleteContractTemplate(ctx, db.DeleteContractTemplateParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return err
	}
	s.publish(ctx, model.ResourceTemplates, "deleted", id)
	return nil
}

// NewVersion copies a template version into the next draft of its kind.
func (s *Service) NewVersion(ctx context.Context, orgID int64, id uuid.UUID) (model.Template, error) {
	row, err := s.templateRow(ctx, orgID, id)
	if err != nil {
		return model.Template{}, err
	}
	return s.insertTemplate(ctx, orgID, row.Kind, row.Name, row.Content, row.BodyHtml, row.Placeholders)
}

// ActivateTemplate makes a draft the active version of its kind (the
// previous active one is archived). Documents rendered later use it; the
// existing versions keep the template they were rendered with.
func (s *Service) ActivateTemplate(ctx context.Context, orgID int64, id uuid.UUID) (model.Template, error) {
	row, err := s.templateRow(ctx, orgID, id)
	if err != nil {
		return model.Template{}, err
	}
	if row.Status != model.TemplateDraft {
		return model.Template{}, stateErr("not_draft", "only a draft can be activated")
	}
	if strings.TrimSpace(row.BodyHtml) == "" {
		return model.Template{}, validation("body_html", "required", "the template body is empty")
	}
	now := s.clock()
	err = s.tx(ctx, func(q *db.Queries) error {
		if err := q.ArchiveActiveContractTemplate(ctx, db.ArchiveActiveContractTemplateParams{
			OrganizationID: orgID, Kind: row.Kind, ArchivedAt: ts(now),
		}); err != nil {
			return err
		}
		n, err := q.ActivateContractTemplate(ctx, db.ActivateContractTemplateParams{
			OrganizationID: orgID, ID: row.ID, ActivatedAt: ts(now), ActivatedBy: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return stateErr("not_draft", "only a draft can be activated")
		}
		return nil
	})
	if uniqueViolation(err, "uq_contract_templates_active_kind") {
		return model.Template{}, stateErr("concurrent_activation", "another version was activated at the same time")
	}
	if err != nil {
		return model.Template{}, err
	}
	s.publish(ctx, model.ResourceTemplates, "updated", id)
	return s.GetTemplate(ctx, orgID, id)
}

// Preview renders a body (saved or edited) as a document: with sample data,
// or with the data of a reservation in the caller's scope.
func (s *Service) Preview(ctx context.Context, orgID int64, in model.PreviewInput) (model.Preview, error) {
	kind := in.Kind
	if kind == "" {
		kind = model.KindRentalContract
	}
	if !model.Contains(model.Kinds, kind) {
		return model.Preview{}, validation("kind", "invalid", "unknown template kind")
	}
	if len(in.BodyHTML) > MaxBodyBytes {
		return model.Preview{}, validation("body_html", "too_large", "the template body is at most 512 KiB")
	}
	clean := Sanitize(in.BodyHTML)
	_, unknown := Tokens(clean)
	snap := sampleSnapshot()
	reference := snap.Values["reservation.reference"]
	if in.ReservationUUID != nil {
		res, err := s.q.GetDocumentReservationRef(ctx, db.GetDocumentReservationRefParams{OrganizationID: orgID, Uuid: *in.ReservationUUID})
		if notFound(err) || (err == nil && !inScope(ctx, res.PickupBranchID, res.ReturnBranchID)) {
			return model.Preview{}, validation("reservation_uuid", "not_found", "reservation was not found")
		}
		if err != nil {
			return model.Preview{}, err
		}
		snap, err = s.snapshot(ctx, orgID, res.ID, res.Uuid)
		if err != nil {
			return model.Preview{}, err
		}
		snap.Values["document.version"] = "1"
		snap.Values["document.date"] = formatDate(s.clock())
		reference = res.Reference
	}
	body, empty := fill(clean, snap.Values)
	doc := documentHTML(kind, snap.Values["company.legal_name"], reference, 1, s.clock(), body)
	return model.Preview{HTML: doc, Unknown: unknown, Empty: empty}, nil
}
