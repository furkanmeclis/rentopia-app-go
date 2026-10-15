// Package logsms is the development SMS provider (TEC-734): it "sends" by
// writing a log line (masked number; OTP bodies redacted, other bodies by
// length only) and reports every message delivered. It is registered only
// when SMS_LOG_PROVIDER is on and never in production; an account of this
// provider makes no network call.
package logsms

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

// ProviderCode is the provider code of the log provider.
const ProviderCode = "log"

// Adapter implements sms.Provider by logging.
type Adapter struct {
	Log *slog.Logger
}

var _ sms.Provider = (*Adapter)(nil)

// New builds the adapter.
func New(log *slog.Logger) *Adapter {
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{Log: log}
}

// Kind implements integrations.Adapter.
func (a *Adapter) Kind() string { return integrations.KindSMS }

// Provider implements integrations.Adapter.
func (a *Adapter) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter: only a sender name.
func (a *Adapter) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "msgheader", LabelKey: "payments.integrations.fields.msgheader", Secret: false, Required: true},
	}
}

// Check implements integrations.Adapter.
func (a *Adapter) Check(context.Context, integrations.Account) error { return nil }

// Send implements sms.Provider.
func (a *Adapter) Send(_ context.Context, acc integrations.Account, msg sms.Message) (sms.SendResult, error) {
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := "log-" + hex.EncodeToString(b[:])
	body := "[" + strconv.Itoa(len([]rune(msg.Body))) + " chars]"
	if msg.Class == sms.ClassOTP {
		body = "[otp redacted]"
	}
	a.Log.Info("sms_log_provider_send", "organization_id", acc.OrganizationID, "to", pii.MaskPhone(msg.To),
		"class", msg.Class, "message_id", id, "body", body)
	return sms.SendResult{MessageID: id, Code: "00"}, nil
}

// Status implements sms.Provider: every logged message counts as delivered.
func (a *Adapter) Status(_ context.Context, _ integrations.Account, messageID string) (sms.Delivery, error) {
	if messageID == "" {
		return sms.Delivery{State: sms.DeliveryNotFound}, nil
	}
	return sms.Delivery{State: sms.DeliveryDelivered, Code: "log"}, nil
}
