package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
)

// Channel is the notification engine's WhatsApp channel
// (notifyengine/channel.ReportingChannel). Registered with
// Engine.Register, it sends customer messages through the organization's
// linked session; delivery receipts settle the attempt (a receipt that
// never comes fails it after ReceiptTimeout, so the rule's next channel
// runs).
//
// Only transactional notices go out here: the engine's catalog has no
// marketing events, and a "marketing." event is refused (marketing needs
// its own consent flow).
type Channel struct{ svc *Service }

// NewChannel builds the channel on the service.
func NewChannel(svc *Service) *Channel { return &Channel{svc: svc} }

// Name implements channel.Channel.
func (*Channel) Name() string { return channel.WhatsApp }

// Supports implements channel.Channel: customers only (members are
// reached by push / e-mail).
func (*Channel) Supports(audience string) bool { return audience == channel.AudienceCustomer }

// SetReporter implements channel.ReportingChannel.
func (c *Channel) SetReporter(r channel.Reporter) { c.svc.reporter = r }

// Send implements channel.Channel.
func (c *Channel) Send(ctx context.Context, orgID int64, m channel.Message) (channel.Result, error) {
	if strings.HasPrefix(m.Event, "marketing.") {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "marketing messages are not sent on WhatsApp"}, nil
	}
	phone := pii.Phone(m.Recipient.Phone)
	if !pii.ValidPhone(phone) {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: ErrInvalidPhone.Error()}, nil
	}
	masked := pii.MaskPhone(phone)
	id := m.DeliveryUUID
	sent, err := c.svc.SendText(ctx, orgID, SendInput{
		Purpose: "notify." + m.Event, Phone: phone, Body: m.Body, LinkPreview: m.Link != "",
		DeliveryUUID: &id, CustomerID: m.Recipient.CustomerID, Test: m.Test,
	})
	switch {
	case errors.Is(err, ErrNotConnected), errors.Is(err, ErrNotOnWhatsApp), errors.Is(err, ErrProviderUnavailable),
		errors.Is(err, ErrInvalidPhone):
		return channel.Result{Outcome: channel.OutcomeSkipped, RecipientMasked: masked, Error: err.Error()}, nil
	case err != nil:
		return channel.Result{
			Outcome: channel.OutcomeFailed, Provider: sent.Provider, ProviderRef: sent.ProviderMessageID,
			RecipientMasked: masked, Error: err.Error(),
		}, nil
	}
	ext := sent.UUID
	return channel.Result{
		Outcome: channel.OutcomeAccepted, Provider: sent.Provider, ProviderRef: sent.ProviderMessageID,
		ExternalUUID: &ext, RecipientMasked: masked,
	}, nil
}

var _ channel.ReportingChannel = (*Channel)(nil)
