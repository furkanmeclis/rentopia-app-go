package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- contacts (corporate customers) ------------------------------------------------

// ListContacts returns the contacts of a customer (masked).
func (s *Service) ListContacts(ctx context.Context, orgID int64, customerUUID uuid.UUID) ([]model.Contact, error) {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListContacts(ctx, orgID, c.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Contact, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapContact(row))
	}
	return out, nil
}

// CreateContact adds a contact to a corporate customer.
func (s *Service) CreateContact(ctx context.Context, orgID int64, customerUUID uuid.UUID, in model.ContactInput) (model.Contact, error) {
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return model.Contact{}, err
	}
	if c.CustomerType != model.TypeCorporate {
		return model.Contact{}, validation("customer_type", "not_corporate", "contacts belong to corporate customers")
	}
	row := db.CustomerContact{}
	if err := applyContact(&row, in, true); err != nil {
		return model.Contact{}, err
	}
	var created db.CustomerContact
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		var err error
		created, err = tx.CreateContact(ctx, orgID, c.ID, row)
		if err != nil {
			return err
		}
		if created.IsPrimary {
			return tx.ClearPrimaryContact(ctx, orgID, c.ID, created.Uuid)
		}
		return nil
	})
	if err != nil {
		return model.Contact{}, mapErr(err)
	}
	return mapContact(created), nil
}

// PatchContact updates the given fields of a contact.
func (s *Service) PatchContact(ctx context.Context, orgID int64, customerUUID, id uuid.UUID, in model.ContactInput) (model.Contact, error) {
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return model.Contact{}, err
	}
	row, err := s.repo.GetContact(ctx, orgID, c.ID, id)
	if err != nil {
		return model.Contact{}, mapNotFound(err, ErrContactNotFound)
	}
	if err := applyContact(&row, in, false); err != nil {
		return model.Contact{}, err
	}
	var updated db.CustomerContact
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		var err error
		updated, err = tx.UpdateContact(ctx, orgID, c.ID, row)
		if err != nil {
			return err
		}
		if updated.IsPrimary {
			return tx.ClearPrimaryContact(ctx, orgID, c.ID, updated.Uuid)
		}
		return nil
	})
	if err != nil {
		return model.Contact{}, mapNotFound(err, ErrContactNotFound)
	}
	return mapContact(updated), nil
}

// DeleteContact removes a contact.
func (s *Service) DeleteContact(ctx context.Context, orgID int64, customerUUID, id uuid.UUID) error {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteContact(ctx, orgID, c.ID, id); err != nil {
		return mapNotFound(err, ErrContactNotFound)
	}
	return nil
}

func applyContact(row *db.CustomerContact, in model.ContactInput, create bool) error {
	ve := &ValidationError{}
	applyText(ve, "full_name", in.FullName, 255, &row.FullName)
	if row.FullName == "" && (create || in.FullName != nil) {
		ve.add("full_name", "required", "full_name is required")
	}
	applyText(ve, "title", in.Title, 128, &row.Title)
	applyPhone(ve, "phone", in.Phone, &row.Phone)
	applyEmail(ve, "email", in.Email, &row.Email)
	if in.IsPrimary != nil {
		row.IsPrimary = *in.IsPrimary
	}
	applyText(ve, "note", in.Note, 2000, &row.Note)
	return ve.orNil()
}

func mapContact(row db.CustomerContact) model.Contact {
	return model.Contact{
		UUID: row.Uuid, FullName: row.FullName, Title: row.Title, PhoneMasked: pii.MaskPhone(row.Phone),
		EmailMasked: pii.MaskEmail(row.Email), IsPrimary: row.IsPrimary, Note: row.Note,
		CreatedAt: row.CreatedAt.Time.UTC(),
	}
}

// --- marketing consents -------------------------------------------------------------

// Consents returns the current state per channel and the full history.
func (s *Service) Consents(ctx context.Context, orgID int64, customerUUID uuid.UUID) (model.ConsentsView, error) {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return model.ConsentsView{}, err
	}
	return s.consentsView(ctx, orgID, c.ID)
}

func (s *Service) consentsView(ctx context.Context, orgID, customerID int64) (model.ConsentsView, error) {
	rows, err := s.repo.ListConsents(ctx, orgID, customerID)
	if err != nil {
		return model.ConsentsView{}, err
	}
	out := model.ConsentsView{Current: []model.ConsentState{}, Items: make([]model.Consent, 0, len(rows))}
	open := map[string]model.Consent{}
	for _, row := range rows {
		item := mapConsent(row)
		out.Items = append(out.Items, item)
		if item.Active {
			open[item.Channel] = item
		}
	}
	for _, ch := range model.ConsentChannels {
		st := model.ConsentState{Channel: ch}
		if item, ok := open[ch]; ok {
			since := item.GrantedAt
			st.Granted, st.Since, st.TextVersion = true, &since, item.TextVersion
		}
		out.Current = append(out.Current, st)
	}
	return out, nil
}

// ConsentChange is the outcome of RecordConsent.
type ConsentChange struct {
	CustomerUUID uuid.UUID
	Channel      string
	Action       string
	View         model.ConsentsView
}

// RecordConsent grants or withdraws the marketing consent of a channel.
// Granting an already granted channel and withdrawing a channel without a
// grant are validation errors (the history stays exact).
func (s *Service) RecordConsent(ctx context.Context, orgID int64, customerUUID uuid.UUID, in model.ConsentInput) (ConsentChange, error) {
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return ConsentChange{}, err
	}
	ve := &ValidationError{}
	channel := strings.TrimSpace(in.Channel)
	if !model.Contains(model.ConsentChannels, channel) {
		ve.add("channel", "invalid", "channel must be one of "+strings.Join(model.ConsentChannels, ", "))
	}
	action := strings.TrimSpace(in.Action)
	if action != "grant" && action != "withdraw" {
		ve.add("action", "invalid", "action must be grant or withdraw")
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "counter"
	}
	if !model.Contains(model.ConsentSources, source) {
		ve.add("source", "invalid", "source must be one of "+strings.Join(model.ConsentSources, ", "))
	}
	textVersion := strings.TrimSpace(in.TextVersion)
	if len(textVersion) > 64 {
		ve.add("text_version", "too_long", "text_version must be at most 64 characters")
	}
	if action == "grant" && textVersion == "" {
		ve.add("text_version", "required", "text_version is required to grant a consent")
	}
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > 2000 {
		ve.add("note", "too_long", "note must be at most 2000 characters")
	}
	at := s.now().UTC()
	if in.At != nil && strings.TrimSpace(*in.At) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.At))
		switch {
		case err != nil:
			ve.add("at", "invalid", "at must be an RFC 3339 time")
		case t.After(at.Add(5 * time.Minute)):
			ve.add("at", "in_future", "at must not be in the future")
		default:
			at = t.UTC()
		}
	}
	if err := ve.orNil(); err != nil {
		return ConsentChange{}, err
	}
	actor := actorArg(ctx)
	switch action {
	case "grant":
		_, err = s.repo.GrantConsent(ctx, db.GrantCustomerConsentParams{
			OrganizationID: orgID, CustomerID: c.ID, Channel: channel, GrantedAt: tstz(at), Source: source,
			TextVersion: textVersion, Note: note, RecordedBy: actor,
		})
		if errors.Is(err, repository.ErrConsentOpen) {
			return ConsentChange{}, validation("channel", "already_granted", "the channel already has a consent")
		}
	case "withdraw":
		var ok bool
		ok, err = s.repo.WithdrawConsent(ctx, db.WithdrawCustomerConsentParams{
			OrganizationID: orgID, CustomerID: c.ID, Channel: channel, WithdrawnAt: tstz(at),
			WithdrawalSource: pgtype.Text{String: source, Valid: true}, WithdrawnBy: actor,
		})
		if err == nil && !ok {
			return ConsentChange{}, validation("channel", "not_granted", "the channel has no consent to withdraw")
		}
	}
	if err != nil {
		return ConsentChange{}, mapErr(err)
	}
	view, err := s.consentsView(ctx, orgID, c.ID)
	if err != nil {
		return ConsentChange{}, err
	}
	return ConsentChange{CustomerUUID: c.Uuid, Channel: channel, Action: action, View: view}, nil
}

func mapConsent(row repository.ConsentRow) model.Consent {
	out := model.Consent{
		UUID: row.Uuid, Channel: row.Channel, Active: !row.WithdrawnAt.Valid, GrantedAt: row.GrantedAt.Time.UTC(),
		Source: row.Source, WithdrawalSource: textPtr(row.WithdrawalSource), TextVersion: row.TextVersion,
		Note: row.Note,
	}
	if row.WithdrawnAt.Valid {
		t := row.WithdrawnAt.Time.UTC()
		out.WithdrawnAt = &t
	}
	if row.RecordedByUuid.Valid {
		out.RecordedBy = &model.Actor{UUID: row.RecordedByUuid.Bytes, Name: row.RecordedByName.String, Surname: row.RecordedBySurname.String}
	}
	if row.WithdrawnByUuid.Valid {
		out.WithdrawnBy = &model.Actor{UUID: row.WithdrawnByUuid.Bytes, Name: row.WithdrawnByName.String, Surname: row.WithdrawnBySurname.String}
	}
	return out
}

func tstz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
