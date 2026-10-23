// Package adapters holds the notification engine's channel adapters
// (TEC-733): SMS through the tenant SMS module (TEC-734), e-mail through
// the SMTP sender, web push for members (notification center) and
// customers (customer_push_subscriptions). A WhatsApp adapter (TEC-735)
// implements channel.Channel the same way and is registered next to these.
package adapters

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RefTypeDelivery is the sms_messages.ref_type of engine messages (the
// ref uuid is the notification delivery).
const RefTypeDelivery = "notification_delivery"

// DeliveryKey is the sms_messages.idempotency_key of the engine's message
// for one delivery attempt (unique per organization, TEC-962): the
// delivery is one dispatch (event + recipient) on one channel, so a
// replayed or resumed attempt finds its message instead of sending again.
func DeliveryKey(delivery uuid.UUID) string { return "notify:" + delivery.String() }

// SMSSender is the part of the SMS service the channel needs.
type SMSSender interface {
	Send(ctx context.Context, orgID int64, in smsusecase.SendInput) (smsusecase.Sent, error)
}

// SMS sends transactional SMS through the organization's SMS account and
// turns the send job's final state into a delivery report.
type SMS struct {
	sender   SMSSender
	q        *db.Queries
	reporter channel.Reporter
}

// NewSMS builds the channel. Register it as a runner listener
// (runner.AddListener) so delivery reports reach the engine.
func NewSMS(sender SMSSender, q *db.Queries) *SMS { return &SMS{sender: sender, q: q} }

// Name implements channel.Channel.
func (*SMS) Name() string { return channel.SMS }

// Supports implements channel.Channel: customers only (members are
// reached by push / e-mail).
func (*SMS) Supports(audience string) bool { return audience == channel.AudienceCustomer }

// SetReporter implements channel.ReportingChannel.
func (s *SMS) SetReporter(r channel.Reporter) { s.reporter = r }

// Send implements channel.Channel.
func (s *SMS) Send(ctx context.Context, orgID int64, m channel.Message) (channel.Result, error) {
	phone := pii.Phone(m.Recipient.Phone)
	if !pii.ValidPhone(phone) {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "no valid mobile number"}, nil
	}
	masked := pii.MaskPhone(phone)
	ref := m.DeliveryUUID
	in := smsusecase.SendInput{
		Class: sms.ClassTransactional, Purpose: "notify." + m.Event, Phone: phone, Body: m.Body,
		RedactedBody: m.RedactedBody, RefType: RefTypeDelivery, RefUUID: &ref, IdempotencyKey: DeliveryKey(ref),
	}
	if m.Recipient.CustomerID > 0 {
		id := m.Recipient.CustomerID
		in.CustomerID = &id
	}
	sent, err := s.sender.Send(ctx, orgID, in)
	var ve *smsusecase.ValidationError
	switch {
	case errors.Is(err, smsusecase.ErrNoAccount):
		return channel.Result{Outcome: channel.OutcomeSkipped, RecipientMasked: masked, Error: "no SMS provider account"}, nil
	case errors.Is(err, smsusecase.ErrRateLimited), errors.Is(err, smsusecase.ErrNoConsent), errors.As(err, &ve):
		return channel.Result{Outcome: channel.OutcomeFailed, RecipientMasked: masked, Error: err.Error()}, nil
	case err != nil:
		return channel.Result{Outcome: channel.OutcomeFailed, RecipientMasked: masked, Error: err.Error()}, nil
	}
	// Without a worker queue the first step ran inline: a final state is
	// known already.
	return smsResult(sent.MessageUUID, masked, sent.Status, ""), nil
}

// smsResult maps an SMS message's status to the channel result.
func smsResult(id uuid.UUID, masked, status, errText string) channel.Result {
	res := channel.Result{Outcome: channel.OutcomeAccepted, Provider: "sms", ExternalUUID: &id, RecipientMasked: masked}
	switch status {
	case smsusecase.StatusDelivered, smsusecase.StatusReconciled:
		res.Outcome = channel.OutcomeDelivered
	case smsusecase.StatusFailed, smsusecase.StatusUndelivered, smsusecase.StatusBlocked:
		res.Outcome, res.Error = channel.OutcomeFailed, "SMS "+status
		if errText != "" {
			res.Error += ": " + errText
		}
	}
	return res
}

// Lookup implements channel.Ledger: the SMS already created for the
// delivery (sms_messages.idempotency_key), whatever its state.
func (s *SMS) Lookup(ctx context.Context, orgID int64, delivery uuid.UUID) (channel.Result, bool, error) {
	if s.q == nil {
		return channel.Result{}, false, nil
	}
	m, err := s.q.GetSMSMessageByIdempotencyKey(ctx, db.GetSMSMessageByIdempotencyKeyParams{
		OrganizationID: orgID, IdempotencyKey: pgtype.Text{String: DeliveryKey(delivery), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return channel.Result{}, false, nil
	}
	if err != nil {
		return channel.Result{}, false, err
	}
	res := smsResult(m.Uuid, m.PhoneMasked, m.Status, m.Error)
	res.ProviderRef = m.ProviderMessageID
	return res, true, nil
}

// IntegrationJobChanged implements the runner listener: the final state
// of an SMS send job that carries an engine message.
func (s *SMS) IntegrationJobChanged(ctx context.Context, orgID int64, job jobsusecase.State, _ integrations.Result) {
	if s.reporter == nil || job.Kind != smsusecase.JobKind || orgID <= 0 {
		return
	}
	var delivered bool
	switch job.Status {
	case jobsusecase.StatusSucceeded:
		delivered = true
	case jobsusecase.StatusFailed:
	default:
		return
	}
	var p struct {
		Message string `json:"sms_message_uuid"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return
	}
	msgID, err := uuid.Parse(p.Message)
	if err != nil {
		return
	}
	ref, err := s.q.GetNotifySMSRef(ctx, db.GetNotifySMSRefParams{OrganizationID: orgID, Uuid: msgID})
	if err != nil || ref.RefType != RefTypeDelivery || !ref.RefUuid.Valid {
		return
	}
	deliveryID := uuid.UUID(ref.RefUuid.Bytes)
	r := channel.Report{
		Channel: channel.SMS, DeliveryUUID: &deliveryID, ExternalUUID: msgID, Delivered: delivered, ProviderRef: job.ProviderRef,
	}
	if !delivered {
		r.Error = job.LastError
		if r.Error == "" {
			r.Error = "SMS not delivered"
		}
	}
	s.reporter.ReportDelivery(ctx, orgID, r)
}

var (
	_ channel.ReportingChannel = (*SMS)(nil)
	_ channel.Ledger           = (*SMS)(nil)
)
