package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SendInput is one text message.
type SendInput struct {
	// Purpose is "notify.<event>" or "test".
	Purpose string
	Phone   string
	Body    string
	// LinkPreview asks for a link preview (the body carries a link).
	LinkPreview  bool
	DeliveryUUID *uuid.UUID
	CustomerID   int64
	Test         bool
}

// DocumentInput is one document message (a contract or invoice PDF).
type DocumentInput struct {
	Purpose    string
	Phone      string
	FileName   string
	MimeType   string
	Caption    string
	Data       []byte
	CustomerID int64
}

// Sent is an accepted message.
type Sent struct {
	UUID              uuid.UUID
	Provider          string
	ProviderMessageID string
	RecipientMasked   string
}

// Sortable lists the message log sort fields.
var Sortable = []string{"created_at", "status", "purpose"}

// SendText sends a transactional text message through the organization's
// linked session (rate limits, number check, message log).
func (s *Service) SendText(ctx context.Context, orgID int64, in SendInput) (Sent, error) {
	return s.send(ctx, orgID, model.KindText, in.Phone, in.Purpose, in.DeliveryUUID, in.CustomerID, in.Test,
		func(st state, to, id string) (whatsapp.SendResult, error) {
			return st.prov.SendText(ctx, st.acc, whatsapp.TextMessage{To: to, Body: in.Body, ID: id, LinkPreview: in.LinkPreview})
		})
}

// SendDocument sends a document (PDF) with an optional caption.
func (s *Service) SendDocument(ctx context.Context, orgID int64, in DocumentInput) (Sent, error) {
	if len(in.Data) == 0 || strings.TrimSpace(in.FileName) == "" {
		return Sent{}, &ValidationError{Details: []Detail{{Field: "document", Code: "required", Message: "a file name and content are required"}}}
	}
	return s.send(ctx, orgID, model.KindDocument, in.Phone, in.Purpose, nil, in.CustomerID, false,
		func(st state, to, id string) (whatsapp.SendResult, error) {
			return st.prov.SendDocument(ctx, st.acc, whatsapp.DocumentMessage{
				To: to, ID: id, FileName: in.FileName, MimeType: in.MimeType, Caption: in.Caption, Data: in.Data,
			})
		})
}

func (s *Service) send(ctx context.Context, orgID int64, kind, rawPhone, purpose string, delivery *uuid.UUID, customerID int64, test bool,
	call func(st state, to, id string) (whatsapp.SendResult, error)) (Sent, error) {
	st, err := s.current(ctx, orgID)
	if errors.Is(err, ErrNoAccount) {
		return Sent{}, ErrNotConnected
	}
	if err != nil {
		return Sent{}, err
	}
	if st.sess == nil || st.sess.Status != model.StatusConnected || whatsapp.Token(st.acc) == "" {
		return Sent{}, ErrNotConnected
	}
	if st.prov == nil {
		return Sent{}, ErrProviderUnavailable
	}
	phone := pii.Phone(rawPhone)
	if !pii.ValidPhone(phone) {
		return Sent{}, ErrInvalidPhone
	}
	masked, hash := pii.MaskPhone(phone), sha(phone)
	now := s.clock()
	n, err := s.q.CountWhatsAppSessionSince(ctx, db.CountWhatsAppSessionSinceParams{
		OrganizationID: orgID, SessionID: st.sess.ID, Since: ts(now.Add(-time.Minute)),
	})
	if err != nil {
		return Sent{}, err
	}
	if n >= int64(st.sess.RateLimitPerMinute) {
		return Sent{RecipientMasked: masked}, ErrRateLimited
	}
	r, err := s.q.CountWhatsAppRecipientSince(ctx, db.CountWhatsAppRecipientSinceParams{
		OrganizationID: orgID, SessionID: st.sess.ID, RecipientHash: hash, Since: ts(now.Add(-time.Hour)),
	})
	if err != nil {
		return Sent{}, err
	}
	if r >= PerRecipientPerHour {
		return Sent{RecipientMasked: masked}, ErrRateLimited
	}
	ok, err := st.prov.OnWhatsApp(ctx, st.acc, phone)
	if err != nil {
		return Sent{RecipientMasked: masked}, providerErr(err)
	}
	if !ok {
		return Sent{RecipientMasked: masked}, ErrNotOnWhatsApp
	}
	p := db.InsertWhatsAppMessageParams{
		OrganizationID: orgID, SessionID: st.sess.ID, Kind: kind, Purpose: truncate(purpose, 80), RecipientMasked: masked,
		RecipientHash: hash, ProviderMessageID: newMessageID(), Test: test,
	}
	if delivery != nil {
		p.DeliveryUuid = pgtype.UUID{Bytes: *delivery, Valid: true}
	}
	if customerID > 0 {
		p.CustomerID = pgtype.Int8{Int64: customerID, Valid: true}
	}
	row, err := s.q.InsertWhatsAppMessage(ctx, p)
	if err != nil {
		return Sent{}, err
	}
	out := Sent{UUID: row.Uuid, Provider: st.acc.Provider, ProviderMessageID: row.ProviderMessageID, RecipientMasked: masked}
	if _, err := call(st, phone, row.ProviderMessageID); err != nil {
		msg := err.Error()
		var rej *whatsapp.Rejected
		if !errors.As(err, &rej) {
			// The outcome is unknown; a late receipt still marks it.
			msg = "outcome unknown: " + msg
		}
		if _, ferr := s.q.MarkWhatsAppMessageFailed(ctx, db.MarkWhatsAppMessageFailedParams{
			Error: truncate(msg, 500), FailedAt: ts(s.clock()), OrganizationID: orgID, ID: row.ID,
		}); ferr != nil {
			s.log.Warn("whatsapp_mark_failed", "message", row.Uuid, "error", ferr)
		}
		return out, providerErr(err)
	}
	if err := s.q.MarkWhatsAppMessageSent(ctx, db.MarkWhatsAppMessageSentParams{
		SentAt: ts(s.clock()), OrganizationID: orgID, ID: row.ID,
	}); err != nil {
		return out, err
	}
	return out, nil
}

// TestSend sends a fixed test message to a number.
func (s *Service) TestSend(ctx context.Context, orgID int64, in model.TestInput) (model.Message, error) {
	if !pii.ValidPhone(pii.Phone(in.Phone)) {
		return model.Message{}, &ValidationError{Details: []Detail{{Field: "phone", Code: "invalid", Message: "a valid mobile number is required"}}}
	}
	sent, err := s.SendText(ctx, orgID, SendInput{
		Purpose: "test", Phone: in.Phone, Test: true,
		Body: "Rentopia: WhatsApp bağlantınız çalışıyor. / Your WhatsApp connection works.",
	})
	if err != nil {
		return model.Message{}, err
	}
	st, _ := s.current(ctx, orgID)
	s.audit(ctx, orgID, ActionTestSent, st.sess, map[string]any{"recipient": sent.RecipientMasked})
	return s.message(ctx, orgID, sent.ProviderMessageID)
}

func (s *Service) message(ctx context.Context, orgID int64, providerID string) (model.Message, error) {
	m, err := s.q.GetWhatsAppMessageByProviderID(ctx, db.GetWhatsAppMessageByProviderIDParams{
		OrganizationID: orgID, ProviderMessageID: providerID,
	})
	if err != nil {
		return model.Message{}, err
	}
	out := model.Message{
		UUID: m.Uuid, Kind: m.Kind, Purpose: m.Purpose, RecipientMasked: m.RecipientMasked,
		ProviderMessageID: m.ProviderMessageID, Status: m.Status, Error: m.Error, Test: m.Test, SentAt: timePtr(m.SentAt),
		DeliveredAt: timePtr(m.DeliveredAt), ReadAt: timePtr(m.ReadAt), FailedAt: timePtr(m.FailedAt),
		CreatedAt: m.CreatedAt.Time.UTC(),
	}
	if m.DeliveryUuid.Valid {
		id := uuid.UUID(m.DeliveryUuid.Bytes)
		out.DeliveryUUID = &id
	}
	return out, nil
}

// ApplyReceipt records a delivery / read receipt and reports the first
// delivery of an engine message to the notification engine.
func (s *Service) ApplyReceipt(ctx context.Context, orgID int64, providerID, receipt string, at time.Time) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return
	}
	m, err := s.q.GetWhatsAppMessageByProviderID(ctx, db.GetWhatsAppMessageByProviderIDParams{
		OrganizationID: orgID, ProviderMessageID: providerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return // a message sent outside Rentopia
	}
	if err != nil {
		s.log.Warn("whatsapp_receipt_lookup_failed", "organization_id", orgID, "error", err)
		return
	}
	if at.IsZero() {
		at = s.clock()
	}
	firstDelivery := !m.DeliveredAt.Valid
	switch receipt {
	case whatsapp.ReceiptDelivered:
		_, err = s.q.MarkWhatsAppMessageDelivered(ctx, db.MarkWhatsAppMessageDeliveredParams{At: ts(at), OrganizationID: orgID, ID: m.ID})
	case whatsapp.ReceiptRead:
		_, err = s.q.MarkWhatsAppMessageRead(ctx, db.MarkWhatsAppMessageReadParams{At: ts(at), OrganizationID: orgID, ID: m.ID})
	default:
		return
	}
	if err != nil {
		s.log.Warn("whatsapp_receipt_failed", "message", m.Uuid, "error", err)
		return
	}
	if firstDelivery && m.DeliveryUuid.Valid && s.reporter != nil {
		id := uuid.UUID(m.DeliveryUuid.Bytes)
		s.reporter.ReportDelivery(ctx, orgID, channel.Report{
			Channel: channel.WhatsApp, DeliveryUUID: &id, ExternalUUID: m.Uuid, Delivered: true, ProviderRef: m.ProviderMessageID,
		})
	}
}

// ListMessages returns a page of the message log.
func (s *Service) ListMessages(ctx context.Context, orgID int64, f model.MessageFilter) ([]model.Message, int64, error) {
	p := db.ListWhatsAppMessagesParams{
		OrganizationID: orgID, SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.SortField == "" {
		p.SortField, p.SortDesc = "created_at", true
	}
	if p.LimitCount <= 0 {
		p.LimitCount = 20
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	if len(f.Kinds) > 0 {
		p.Kinds = f.Kinds
	}
	if f.CreatedFrom != nil {
		p.CreatedFrom = ts(*f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		p.CreatedTo = ts(*f.CreatedTo)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListWhatsAppMessages(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Message, 0, len(rows))
	var total int64
	for _, m := range rows {
		total = m.TotalCount
		v := model.Message{
			UUID: m.Uuid, Kind: m.Kind, Purpose: m.Purpose, RecipientMasked: m.RecipientMasked,
			ProviderMessageID: m.ProviderMessageID, Status: m.Status, Error: m.Error, Test: m.Test,
			SentAt: timePtr(m.SentAt), DeliveredAt: timePtr(m.DeliveredAt), ReadAt: timePtr(m.ReadAt),
			FailedAt: timePtr(m.FailedAt), CreatedAt: m.CreatedAt.Time.UTC(), CustomerName: m.CustomerName,
		}
		if m.DeliveryUuid.Valid {
			id := uuid.UUID(m.DeliveryUuid.Bytes)
			v.DeliveryUUID = &id
		}
		if m.CustomerUuid != uuid.Nil {
			id := m.CustomerUuid
			v.CustomerUUID = &id
		}
		out = append(out, v)
	}
	return out, total, nil
}

// Watch is the health sweep: it reads the state of linked (and dropped)
// sessions, tries to reconnect dropped ones, and settles messages whose
// delivery receipt never came. It returns the sessions checked.
func (s *Service) Watch(ctx context.Context) (int, error) {
	rows, err := s.q.ListWhatsAppSessionsToWatch(ctx)
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		octx := orgctx.WithOrganizationID(ctx, r.OrganizationID)
		acc, err := s.accounts.AccountByID(octx, r.OrganizationID, r.IntegrationAccountID)
		if err != nil {
			continue
		}
		st, err := s.stateOf(octx, r.OrganizationID, acc)
		if err != nil || st.sess == nil || st.prov == nil || whatsapp.Token(acc) == "" {
			continue
		}
		if st.sess.Status == model.StatusDisconnected {
			_ = st.prov.Connect(octx, acc)
		}
		if _, err := s.sync(octx, r.OrganizationID, st, ""); err != nil {
			s.log.Warn("whatsapp_watch_failed", "organization_id", r.OrganizationID, "error", err)
		}
	}
	if err := s.expireReceipts(ctx); err != nil {
		return len(rows), err
	}
	return len(rows), nil
}

// expireReceipts fails sent messages without a delivery receipt after
// ReceiptTimeout and reports them (the engine runs the next channel).
func (s *Service) expireReceipts(ctx context.Context) error {
	before := ts(s.clock().Add(-ReceiptTimeout))
	orgs, err := s.q.ListWhatsAppOrganizationsAwaitingReceipt(ctx, before)
	if err != nil {
		return err
	}
	for _, orgID := range orgs {
		octx := orgctx.WithOrganizationID(ctx, orgID)
		msgs, err := s.q.ListWhatsAppMessagesAwaitingReceipt(octx, db.ListWhatsAppMessagesAwaitingReceiptParams{
			OrganizationID: orgID, Before: before,
		})
		if err != nil {
			return err
		}
		for _, m := range msgs {
			const reason = "no delivery receipt within 60 minutes"
			n, err := s.q.MarkWhatsAppMessageFailed(octx, db.MarkWhatsAppMessageFailedParams{
				Error: reason, FailedAt: ts(s.clock()), OrganizationID: orgID, ID: m.ID,
			})
			if err != nil {
				return err
			}
			if n == 1 && m.DeliveryUuid.Valid && s.reporter != nil {
				id := uuid.UUID(m.DeliveryUuid.Bytes)
				s.reporter.ReportDelivery(octx, orgID, channel.Report{
					Channel: channel.WhatsApp, DeliveryUUID: &id, ExternalUUID: m.Uuid, Delivered: false,
					ProviderRef: m.ProviderMessageID, Error: reason,
				})
			}
		}
	}
	return nil
}
