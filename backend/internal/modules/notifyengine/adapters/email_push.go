package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
)

// Email sends plain-text e-mail through the SMTP sender.
type Email struct {
	mail mail.Sender
}

// NewEmail builds the channel.
func NewEmail(m mail.Sender) *Email { return &Email{mail: m} }

// Name implements channel.Channel.
func (*Email) Name() string { return channel.Email }

// Supports implements channel.Channel.
func (*Email) Supports(string) bool { return true }

func validEmail(v string) bool {
	local, domain, ok := strings.Cut(v, "@")
	return ok && local != "" && strings.Contains(domain, ".") && !strings.ContainsAny(v, " \r\n,;<>\"") && len(v) <= 254
}

// Send implements channel.Channel.
func (e *Email) Send(ctx context.Context, _ int64, m channel.Message) (channel.Result, error) {
	to := strings.TrimSpace(m.Recipient.Email)
	if !validEmail(to) {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "no valid e-mail address"}, nil
	}
	masked := channel.MaskEmail(to)
	if e.mail == nil {
		return channel.Result{Outcome: channel.OutcomeSkipped, RecipientMasked: masked, Error: "no mail sender"}, nil
	}
	if err := e.mail.Send(ctx, mail.Message{To: []string{to}, Subject: m.Subject, Body: m.Body}); err != nil {
		return channel.Result{Outcome: channel.OutcomeFailed, Provider: "smtp", RecipientMasked: masked, Error: err.Error()}, nil
	}
	return channel.Result{Outcome: channel.OutcomeSent, Provider: "smtp", RecipientMasked: masked}, nil
}

// Notifier is the notification center (member inbox, realtime, web push
// and mobile push of members).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// VAPID is the web push key pair (shared with the member push).
type VAPID struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// Push reaches members through the notification center (their inbox,
// mirrored to their web push subscriptions and mobile devices) and
// customers through their own web push subscriptions
// (customer_push_subscriptions; members' and customers' subscriptions are
// never mixed).
type Push struct {
	notifier Notifier
	q        *db.Queries
	vapid    VAPID
	send     func(payload []byte, sub *webpush.Subscription, opts *webpush.Options) (*http.Response, error)
}

// NewPush builds the channel (notifier may be nil: no member push).
func NewPush(n Notifier, q *db.Queries, v VAPID) *Push {
	return &Push{notifier: n, q: q, vapid: v, send: webpush.SendNotification}
}

// Name implements channel.Channel.
func (*Push) Name() string { return channel.Push }

// Supports implements channel.Channel.
func (*Push) Supports(string) bool { return true }

// Send implements channel.Channel.
func (p *Push) Send(ctx context.Context, orgID int64, m channel.Message) (channel.Result, error) {
	if m.Recipient.Audience == channel.AudienceStaff {
		return p.sendStaff(ctx, orgID, m)
	}
	return p.sendCustomer(ctx, orgID, m)
}

func (p *Push) sendStaff(ctx context.Context, orgID int64, m channel.Message) (channel.Result, error) {
	if p.notifier == nil || m.Recipient.UserID <= 0 {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "no notification center"}, nil
	}
	org, user := orgID, m.Recipient.UserID
	in := notifmodel.EnqueueInput{
		OrganizationID: &org, UserID: &user, Channels: []string{notifmodel.ChannelInapp},
		Priority: notifmodel.PriorityNormal, Title: m.Subject, Body: m.Body, SourceEvent: "notify." + m.Event,
		Payload: map[string]any{"notification_delivery_uuid": m.DeliveryUUID.String()},
	}
	if m.Link != "" {
		link := m.Link
		in.ActionURL = &link
	}
	rows, err := p.notifier.Enqueue(ctx, in)
	if err != nil {
		return channel.Result{Outcome: channel.OutcomeFailed, Provider: "notification_center", Error: err.Error()}, nil
	}
	if len(rows) == 0 {
		return channel.Result{Outcome: channel.OutcomeSkipped, Provider: "notification_center", Error: "turned off in the member's preferences"}, nil
	}
	return channel.Result{
		Outcome: channel.OutcomeSent, Provider: "notification_center", ProviderRef: rows[0].UUID.String(),
		RecipientMasked: m.Recipient.Name,
	}, nil
}

func (p *Push) sendCustomer(ctx context.Context, orgID int64, m channel.Message) (channel.Result, error) {
	if p.vapid.PublicKey == "" || p.vapid.PrivateKey == "" {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "web push is not configured"}, nil
	}
	subs, err := p.q.ListCustomerPushSubscriptions(ctx, db.ListCustomerPushSubscriptionsParams{
		OrganizationID: orgID, CustomerID: m.Recipient.CustomerID,
	})
	if err != nil {
		return channel.Result{Outcome: channel.OutcomeFailed, Error: err.Error()}, nil
	}
	if len(subs) == 0 {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "no push subscription"}, nil
	}
	data := map[string]any{"notification_delivery_uuid": m.DeliveryUUID.String()}
	if m.Link != "" {
		data["action_url"] = m.Link
	}
	payload, _ := json.Marshal(map[string]any{"title": m.Subject, "body": m.Body, "data": data})
	ok, lastErr := 0, ""
	for _, s := range subs {
		resp, err := p.send(payload, &webpush.Subscription{
			Endpoint: s.Endpoint, Keys: webpush.Keys{P256dh: s.KeyP256dh, Auth: s.KeyAuth},
		}, &webpush.Options{
			VAPIDPublicKey: p.vapid.PublicKey, VAPIDPrivateKey: p.vapid.PrivateKey, Subscriber: p.vapid.Subject, TTL: 3600,
		})
		if err != nil {
			lastErr = err.Error()
			continue
		}
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
			// The browser dropped the subscription.
			_ = p.q.DeleteCustomerPushSubscriptionByID(ctx, db.DeleteCustomerPushSubscriptionByIDParams{OrganizationID: orgID, ID: s.ID})
			lastErr = fmt.Sprintf("subscription expired (%d)", resp.StatusCode)
		case resp.StatusCode >= 300:
			lastErr = fmt.Sprintf("push service answered %d", resp.StatusCode)
		default:
			ok++
		}
	}
	masked := fmt.Sprintf("%d device(s)", len(subs))
	if ok == 0 {
		return channel.Result{Outcome: channel.OutcomeFailed, Provider: "webpush", RecipientMasked: masked, Error: lastErr}, nil
	}
	return channel.Result{Outcome: channel.OutcomeSent, Provider: "webpush", RecipientMasked: masked}, nil
}

// SetSender replaces the web push sender (tests).
func (p *Push) SetSender(fn func(payload []byte, sub *webpush.Subscription, opts *webpush.Options) (*http.Response, error)) {
	p.send = fn
}
