// Package usecase validates and stores organization-scoped site leads.
package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	notify "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("site lead not found")

type ValidationError struct{ Details []response.Detail }

func (e *ValidationError) Error() string { return "invalid site lead" }

type Service struct {
	q        *db.Queries
	salt     []byte
	notifier *notify.Engine
}

// New uses the application's existing private encryption key as the IP hash salt.
func New(q *db.Queries, salt string) *Service   { return &Service{q: q, salt: []byte(salt)} }
func (s *Service) SetNotifier(n *notify.Engine) { s.notifier = n }
func (s *Service) IPHash(org int64, ip string) string {
	h := hmac.New(sha256.New, s.salt)
	_, _ = fmt.Fprintf(h, "site-leads:%d:%s", org, ip)
	return hex.EncodeToString(h.Sum(nil))
}

func Validate(in *model.Input) error {
	ve := &ValidationError{Details: []response.Detail{}}
	add := func(field, code string) {
		ve.Details = append(ve.Details, response.Detail{Field: field, Code: code, Message: field + " is " + code})
	}
	fields := []struct {
		key   string
		value *string
		limit int
	}{
		{"name", &in.Name, 160}, {"email", &in.Email, 254}, {"phone", &in.Phone, 40}, {"company", &in.Company, 200},
		{"city", &in.City, 100}, {"message", &in.Message, 5000},
	}
	for _, f := range fields {
		*f.value = strings.TrimSpace(*f.value)
		if utf8.RuneCountInString(*f.value) > f.limit {
			add(f.key, "too_long")
		}
	}
	in.Email = strings.ToLower(in.Email)
	addr, err := mail.ParseAddress(in.Email)
	if err != nil || addr.Address != in.Email {
		add("email", "invalid")
	}
	if !slices.Contains(model.Kinds, in.Kind) {
		add("kind", "invalid")
	}
	required := map[string]string{}
	switch in.Kind {
	case "contact":
		required = map[string]string{"name": in.Name, "message": in.Message}
	case "corporate":
		required = map[string]string{"company": in.Company, "name": in.Name, "phone": in.Phone}
	case "franchise":
		required = map[string]string{"name": in.Name, "phone": in.Phone, "city": in.City}
	}
	for field, value := range required {
		if value == "" {
			add(field, "required")
		}
	}
	if in.Locale == "" {
		in.Locale = "tr"
	}
	if in.Locale != "tr" && in.Locale != "en" {
		add("locale", "invalid")
	}
	if !in.ConsentKVKK {
		add("consent_kvkk", "required")
	}
	if in.FleetSize != nil && (in.Kind != "corporate" || *in.FleetSize < 0 || *in.FleetSize > 100000) {
		add("fleet_size", "invalid")
	}
	if len(ve.Details) > 0 {
		return ve
	}
	return nil
}

// Submit never returns a lead identifier, including duplicate subscriptions and bot submissions.
func (s *Service) Submit(ctx context.Context, org int64, in model.Input, origin, ip, agent string) error {
	if strings.TrimSpace(in.Honeypot) != "" {
		return nil
	}
	if err := Validate(&in); err != nil {
		return err
	}
	if len(s.salt) == 0 {
		return errors.New("site lead hashing unavailable")
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return errors.New("site lead origin unavailable")
	}
	params := db.InsertSiteLeadParams{OrganizationID: org, Kind: in.Kind, Name: in.Name, Email: in.Email, Phone: in.Phone,
		Company: in.Company, City: in.City, Message: in.Message, Locale: in.Locale, ConsentKvkk: in.ConsentKVKK,
		ConsentMarketing: in.ConsentMarketing, SourceHost: u.Hostname(), IpHash: s.IPHash(org, ip), UserAgent: truncate(agent, 300)}
	if in.FleetSize != nil {
		params.FleetSize = pgtype.Int4{Int32: *in.FleetSize, Valid: true}
	}
	row, err := s.q.InsertSiteLead(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	s.notify(ctx, org, row)
	return nil
}

func (s *Service) notify(ctx context.Context, org int64, row db.SiteLead) {
	if s.notifier == nil {
		return
	}
	recipients, err := s.q.SiteLeadRecipients(ctx, org)
	if err != nil {
		slog.Warn("site_lead_recipients_failed", "error", err)
		return
	}
	for _, id := range recipients {
		_, err := s.notifier.Emit(ctx, org, notify.EmitInput{Event: catalog.SiteLeadCreated, UserID: id, Channels: []string{channel.Push},
			RefType: model.Resource, RefUUID: &row.Uuid, Key: fmt.Sprintf("user:%d", id), Vars: map[string]string{"kind": row.Kind}})
		if err != nil {
			slog.Warn("site_lead_notification_failed", "error", err)
		}
	}
}

func truncate(value string, n int) string {
	r := []rune(value)
	if len(r) > n {
		return string(r[:n])
	}
	return value
}
func record(r db.SiteLead) model.Lead {
	out := model.Lead{UUID: r.Uuid, Kind: r.Kind, Name: r.Name, Email: r.Email, Phone: r.Phone, Company: r.Company, City: r.City,
		Message: r.Message, Locale: r.Locale, ConsentKVKK: r.ConsentKvkk, ConsentMarketing: r.ConsentMarketing,
		SourceHost: r.SourceHost, Status: r.Status, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, Note: r.Note}
	if r.FleetSize.Valid {
		out.FleetSize = &r.FleetSize.Int32
	}
	if r.HandledBy.Valid {
		out.HandledBy = &r.HandledBy.Int64
	}
	if r.HandledAt.Valid {
		out.HandledAt = &r.HandledAt.Time
	}
	return out
}
func mapped(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (model.Lead, error) {
	r, err := s.q.GetSiteLead(ctx, db.GetSiteLeadParams{OrganizationID: org, Uuid: id})
	return record(r), mapped(err)
}
func (s *Service) Patch(ctx context.Context, org int64, id uuid.UUID, user int64, in model.Patch) (model.Lead, error) {
	ve := &ValidationError{Details: []response.Detail{}}
	p := db.PatchSiteLeadParams{OrganizationID: org, Uuid: id, HandledBy: pgtype.Int8{Int64: user, Valid: user > 0}}
	if in.Status != nil {
		if !slices.Contains(model.Statuses, *in.Status) {
			ve.Details = append(ve.Details, response.Detail{Field: "status", Code: "invalid", Message: "invalid status"})
		}
		p.Status = pgtype.Text{String: *in.Status, Valid: true}
	}
	if in.Note != nil {
		if utf8.RuneCountInString(*in.Note) > 5000 {
			ve.Details = append(ve.Details, response.Detail{Field: "note", Code: "too_long", Message: "at most 5000 characters"})
		}
		p.Note = pgtype.Text{String: strings.TrimSpace(*in.Note), Valid: true}
	}
	if len(ve.Details) > 0 {
		return model.Lead{}, ve
	}
	r, err := s.q.PatchSiteLead(ctx, p)
	return record(r), mapped(err)
}
func (s *Service) List(ctx context.Context, p db.ListSiteLeadsParams) ([]model.Lead, int64, error) {
	rows, err := s.q.ListSiteLeads(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountSiteLeads(ctx, db.CountSiteLeadsParams{OrganizationID: p.OrganizationID, Kind: p.Kind, Status: p.Status, DateFrom: p.DateFrom, DateTo: p.DateTo, Q: p.Q})
	out := []model.Lead{}
	for _, r := range rows {
		out = append(out, record(r))
	}
	return out, total, err
}
