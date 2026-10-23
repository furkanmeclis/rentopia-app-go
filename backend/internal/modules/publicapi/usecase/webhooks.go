package usecase

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Webhook headers. The signature is HMAC-SHA256 (hex) of
// "<timestamp>.<body>" with the endpoint's secret: "v1=<hex>".
const (
	HeaderWebhookID        = "Rentopia-Webhook-Id"
	HeaderWebhookEvent     = "Rentopia-Webhook-Event"
	HeaderWebhookTimestamp = "Rentopia-Webhook-Timestamp"
	HeaderWebhookSignature = "Rentopia-Webhook-Signature"
)

// Delivery tuning.
const (
	deliveryLease      = 2 * time.Minute
	deliveryBatch      = 50
	maxResponseExcerpt = 512
	maxEndpointURL     = 2048
	webhookSecretBytes = 24
)

// retryBackoff is the wait after attempt N (1-based); the last entry
// repeats.
var retryBackoff = []time.Duration{
	time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

// EndpointSortColumns are the sortable endpoint list columns.
var EndpointSortColumns = map[string]string{"name": "name", "created_at": "created_at"}

// SignWebhook is the signature header value of a body sent at ts.
func SignWebhook(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(ts, 10)))
	_, _ = mac.Write([]byte{'.'})
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func secretAAD(orgID int64, id uuid.UUID) string {
	return "public_webhook:" + strconv.FormatInt(orgID, 10) + ":" + id.String()
}

// ------------------------------------------------------------ transport

// privateAddr reports addresses a webhook must never reach.
func privateAddr(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsMulticast() || a.IsUnspecified() || a.IsInterfaceLocalMulticast() ||
		netip.MustParsePrefix("100.64.0.0/10").Contains(a) || netip.MustParsePrefix("fc00::/7").Contains(a)
}

// webhookTransport refuses private targets at dial time (after DNS), so a
// public name that resolves to an internal address is refused too.
func (s *Service) webhookTransport() http.RoundTripper {
	allow := s.allowPrivate
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allow {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			a, err := netip.ParseAddr(host)
			if err != nil || privateAddr(a) {
				return ErrWebhookTarget
			}
			return nil
		},
	}
	return &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 20,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
	}
}

// validateEndpointURL checks the target before it is stored.
func (s *Service) validateEndpointURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || len(raw) > maxEndpointURL || u.Fragment != "" {
		return "", validation("url", "invalid", "the URL is not valid")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !s.allowPrivate {
			return "", validation("url", "https_required", ErrWebhookTarget.Error())
		}
	default:
		return "", validation("url", "invalid", "the URL must start with https://")
	}
	host := strings.ToLower(u.Hostname())
	if !s.allowPrivate {
		if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") ||
			strings.HasSuffix(host, ".local") {
			return "", validation("url", "private_target", ErrWebhookTarget.Error())
		}
		if a, err := netip.ParseAddr(host); err == nil && privateAddr(a) {
			return "", validation("url", "private_target", ErrWebhookTarget.Error())
		}
	}
	return u.String(), nil
}

// ------------------------------------------------------------ endpoints

func endpointView(w db.PublicWebhookEndpoint) model.Endpoint {
	ev := w.Events
	if ev == nil {
		ev = []string{}
	}
	return model.Endpoint{
		UUID: w.Uuid, Name: w.Name, URL: w.Url, SecretLast4: w.SecretLast4, Events: ev, Status: w.Status,
		LastSuccessAt: tsPtr(w.LastSuccessAt), LastFailureAt: tsPtr(w.LastFailureAt),
		ConsecutiveFailures: int(w.ConsecutiveFailures), CreatedAt: w.CreatedAt.Time.UTC(), UpdatedAt: w.UpdatedAt.Time.UTC(),
	}
}

type endpointFields struct {
	name, url, status string
	events            []string
}

func (s *Service) validateEndpoint(in model.EndpointInput, cur *endpointFields) (endpointFields, error) {
	ve := &ValidationError{}
	f := endpointFields{status: model.EndpointActive}
	if cur != nil {
		f = *cur
	}
	if in.Name != nil || cur == nil {
		name := ""
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		switch {
		case name == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(name) > 120:
			ve.add("name", "too_long", "name is at most 120 characters")
		default:
			f.name = name
		}
	}
	if in.URL != nil || cur == nil {
		raw := ""
		if in.URL != nil {
			raw = *in.URL
		}
		if strings.TrimSpace(raw) == "" {
			ve.add("url", "required", "url is required")
		} else if u, err := s.validateEndpointURL(raw); err != nil {
			var v *ValidationError
			if errors.As(err, &v) {
				ve.Details = append(ve.Details, v.Details...)
			}
		} else {
			f.url = u
		}
	}
	if in.Events != nil || cur == nil {
		events := model.WebhookEvents
		if in.Events != nil {
			events = *in.Events
		}
		clean := []string{}
		for i, e := range events {
			if !contains(model.WebhookEvents, e) {
				ve.add("events."+itoa(i), "invalid", "unknown event")
				continue
			}
			if !contains(clean, e) {
				clean = append(clean, e)
			}
		}
		if len(clean) == 0 {
			ve.add("events", "required", "subscribe to at least one event")
		}
		f.events = clean
	}
	if in.Status != nil {
		if !contains(model.EndpointStatuses, *in.Status) {
			ve.add("status", "invalid", "status is active or disabled")
		} else {
			f.status = *in.Status
		}
	}
	return f, ve.orNil()
}

func newWebhookSecret() (string, error) {
	t, err := randomToken(webhookSecretBytes)
	return "whsec_" + t, err
}

// ListEndpoints returns a page of webhook endpoints.
func (s *Service) ListEndpoints(ctx context.Context, orgID int64, f model.EndpointFilter) ([]model.Endpoint, int64, error) {
	params := db.ListPublicWebhookEndpointsParams{
		OrganizationID: orgID, Q: textArg(f.Q), Statuses: nilIfEmpty(f.Statuses), SortField: EndpointSortColumns[f.SortField],
		SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	rows, err := s.q.ListPublicWebhookEndpoints(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountPublicWebhookEndpoints(ctx, db.CountPublicWebhookEndpointsParams{
		OrganizationID: orgID, Q: params.Q, Statuses: params.Statuses,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Endpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, endpointView(r))
	}
	return out, total, nil
}

func (s *Service) endpoint(ctx context.Context, orgID int64, id uuid.UUID) (db.PublicWebhookEndpoint, error) {
	w, err := s.q.GetPublicWebhookEndpoint(ctx, db.GetPublicWebhookEndpointParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// GetEndpoint returns one endpoint.
func (s *Service) GetEndpoint(ctx context.Context, orgID int64, id uuid.UUID) (model.Endpoint, error) {
	w, err := s.endpoint(ctx, orgID, id)
	if err != nil {
		return model.Endpoint{}, err
	}
	return endpointView(w), nil
}

// CreateEndpoint adds an endpoint; its signing secret is returned once.
func (s *Service) CreateEndpoint(ctx context.Context, orgID int64, in model.EndpointInput) (model.IssuedEndpoint, error) {
	f, err := s.validateEndpoint(in, nil)
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	id := uuid.New()
	enc, err := s.secrets.Seal(secret, secretAAD(orgID, id))
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	w, err := s.q.CreatePublicWebhookEndpoint(ctx, db.CreatePublicWebhookEndpointParams{
		OrganizationID: orgID, Uuid: id, Name: f.name, Url: f.url, SecretEnc: enc, SecretLast4: last4(secret),
		Events: f.events, CreatedBy: actorArg(ctx),
	})
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	s.publish(ctx, model.ResourceWebhooks, "created", id)
	return model.IssuedEndpoint{Endpoint: endpointView(w), Secret: secret}, nil
}

// PatchEndpoint changes an endpoint (re-enabling resets its failure count).
func (s *Service) PatchEndpoint(ctx context.Context, orgID int64, id uuid.UUID, in model.EndpointInput) (model.Endpoint, error) {
	w, err := s.endpoint(ctx, orgID, id)
	if err != nil {
		return model.Endpoint{}, err
	}
	f, err := s.validateEndpoint(in, &endpointFields{name: w.Name, url: w.Url, status: w.Status, events: w.Events})
	if err != nil {
		return model.Endpoint{}, err
	}
	if _, err := s.q.UpdatePublicWebhookEndpoint(ctx, db.UpdatePublicWebhookEndpointParams{
		OrganizationID: orgID, Uuid: id, Name: f.name, Url: f.url, Events: f.events, Status: f.status,
	}); err != nil {
		return model.Endpoint{}, err
	}
	s.publish(ctx, model.ResourceWebhooks, "updated", id)
	return s.GetEndpoint(ctx, orgID, id)
}

// RotateEndpointSecret replaces the signing secret (returned once).
func (s *Service) RotateEndpointSecret(ctx context.Context, orgID int64, id uuid.UUID) (model.IssuedEndpoint, error) {
	if _, err := s.endpoint(ctx, orgID, id); err != nil {
		return model.IssuedEndpoint{}, err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	enc, err := s.secrets.Seal(secret, secretAAD(orgID, id))
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	if _, err := s.q.RotatePublicWebhookSecret(ctx, db.RotatePublicWebhookSecretParams{
		OrganizationID: orgID, Uuid: id, SecretEnc: enc, SecretLast4: last4(secret),
	}); err != nil {
		return model.IssuedEndpoint{}, err
	}
	ep, err := s.GetEndpoint(ctx, orgID, id)
	if err != nil {
		return model.IssuedEndpoint{}, err
	}
	s.publish(ctx, model.ResourceWebhooks, "updated", id)
	return model.IssuedEndpoint{Endpoint: ep, Secret: secret}, nil
}

// DeleteEndpoint removes an endpoint and its deliveries.
func (s *Service) DeleteEndpoint(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.q.DeletePublicWebhookEndpoint(ctx, db.DeletePublicWebhookEndpointParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	s.publish(ctx, model.ResourceWebhooks, "deleted", id)
	return nil
}

// ------------------------------------------------------------ deliveries

func deliveryView(d db.ListPublicWebhookDeliveriesRow) model.Delivery {
	out := model.Delivery{
		UUID: d.Uuid, Endpoint: model.EndpointRef{UUID: d.EndpointUuid, Name: d.EndpointName, URL: d.EndpointUrl},
		EventType: d.EventType, EventUUID: d.EventUuid, Status: d.Status, Attempts: int(d.Attempts),
		LastError: d.LastError, DeliveredAt: tsPtr(d.DeliveredAt), CreatedAt: d.CreatedAt.Time.UTC(), UpdatedAt: d.UpdatedAt.Time.UTC(),
	}
	if d.Status == model.DeliveryPending {
		out.NextAttemptAt = tsPtr(d.NextAttemptAt)
	}
	if d.LastStatusCode.Valid {
		v := int(d.LastStatusCode.Int32)
		out.LastStatusCode = &v
	}
	return out
}

// ListDeliveries returns a page of deliveries.
func (s *Service) ListDeliveries(ctx context.Context, orgID int64, f model.DeliveryFilter) ([]model.Delivery, int64, error) {
	var endpoints []uuid.UUID
	if len(f.EndpointUUIDs) > 0 {
		endpoints = f.EndpointUUIDs
	}
	rows, err := s.q.ListPublicWebhookDeliveries(ctx, db.ListPublicWebhookDeliveriesParams{
		OrganizationID: orgID, EndpointUuids: endpoints, Statuses: nilIfEmpty(f.Statuses), EventTypes: nilIfEmpty(f.EventTypes),
		SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountPublicWebhookDeliveries(ctx, db.CountPublicWebhookDeliveriesParams{
		OrganizationID: orgID, EndpointUuids: endpoints, Statuses: nilIfEmpty(f.Statuses), EventTypes: nilIfEmpty(f.EventTypes),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Delivery, 0, len(rows))
	for _, r := range rows {
		out = append(out, deliveryView(r))
	}
	return out, total, nil
}

// GetDelivery returns one delivery with its payload.
func (s *Service) GetDelivery(ctx context.Context, orgID int64, id uuid.UUID) (model.Delivery, error) {
	d, err := s.q.GetPublicWebhookDelivery(ctx, db.GetPublicWebhookDeliveryParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Delivery{}, ErrNotFound
	}
	if err != nil {
		return model.Delivery{}, err
	}
	out := deliveryView(db.ListPublicWebhookDeliveriesRow{
		ID: d.ID, Uuid: d.Uuid, EventType: d.EventType, EventUuid: d.EventUuid, Status: d.Status, Attempts: d.Attempts,
		NextAttemptAt: d.NextAttemptAt, LastStatusCode: d.LastStatusCode, LastError: d.LastError, DeliveredAt: d.DeliveredAt,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, EndpointUuid: d.EndpointUuid, EndpointName: d.EndpointName, EndpointUrl: d.EndpointUrl,
	})
	out.Payload = json.RawMessage(d.Payload)
	return out, nil
}

// DeliveryStats counts failed / pending deliveries of the last 7 days.
func (s *Service) DeliveryStats(ctx context.Context, orgID int64) (model.DeliveryStats, error) {
	r, err := s.q.CountPublicWebhookDeliveryStats(ctx, db.CountPublicWebhookDeliveryStatsParams{
		OrganizationID: orgID, Since: ts(s.clock().Add(-7 * 24 * time.Hour)),
	})
	if err != nil {
		return model.DeliveryStats{}, err
	}
	return model.DeliveryStats{Failed: r.Failed, Pending: r.Pending}, nil
}

// RetryDelivery puts a failed delivery back in the queue and tries it now.
func (s *Service) RetryDelivery(ctx context.Context, orgID int64, id uuid.UUID) (model.Delivery, error) {
	d, err := s.q.GetPublicWebhookDelivery(ctx, db.GetPublicWebhookDeliveryParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Delivery{}, ErrNotFound
	}
	if err != nil {
		return model.Delivery{}, err
	}
	n, err := s.q.RequeuePublicWebhookDelivery(ctx, db.RequeuePublicWebhookDeliveryParams{OrganizationID: orgID, Uuid: id, Now: ts(s.clock())})
	if err != nil {
		return model.Delivery{}, err
	}
	if n == 0 {
		return model.Delivery{}, validation("status", "not_failed", "only a failed delivery can be retried")
	}
	s.attemptNow(ctx, orgID, d.ID)
	s.publish(ctx, model.ResourceWebhooks, "updated", id)
	return s.GetDelivery(ctx, orgID, id)
}

// TestEndpoint sends a signed ping to an endpoint now and returns the
// delivery.
func (s *Service) TestEndpoint(ctx context.Context, orgID int64, id uuid.UUID) (model.Delivery, error) {
	w, err := s.endpoint(ctx, orgID, id)
	if err != nil {
		return model.Delivery{}, err
	}
	org, _ := orgctx.From(ctx)
	event := uuid.New()
	payload, err := json.Marshal(model.WebhookEvent{
		ID: event, Type: model.EventPing, CreatedAt: s.clock(), Organization: org.Slug, Data: map[string]any{"message": "ping"},
	})
	if err != nil {
		return model.Delivery{}, err
	}
	d, err := s.q.InsertPublicWebhookDelivery(ctx, db.InsertPublicWebhookDeliveryParams{
		OrganizationID: orgID, EndpointID: w.ID, EventType: model.EventPing, EventUuid: event, Payload: payload,
		NextAttemptAt: ts(s.clock()),
	})
	if err != nil {
		return model.Delivery{}, err
	}
	s.attemptNow(ctx, orgID, d.ID)
	s.publish(ctx, model.ResourceWebhooks, "updated", id)
	return s.GetDelivery(ctx, orgID, d.Uuid)
}

// ------------------------------------------------------------ events

// ReservationChanged implements the reservations LifecycleListener: the
// lifecycle events of reservations the public API made become webhook
// events.
func (s *Service) ReservationChanged(ctx context.Context, orgID int64, reservation uuid.UUID, event string) {
	switch event {
	case resusecase.LifecycleConfirmed:
		s.Emit(ctx, orgID, model.EventReservationConfirmed, reservation)
	case resusecase.LifecycleChanged, resusecase.LifecycleDriversChanged:
		s.Emit(ctx, orgID, model.EventReservationChanged, reservation)
	case resusecase.LifecycleTerminated:
		s.Emit(ctx, orgID, model.EventReservationCancelled, reservation)
	case resusecase.LifecyclePlateFixed:
		s.Emit(ctx, orgID, model.EventReservationPlateFixed, reservation)
	}
}

// Emit queues a reservation event for every active endpoint subscribed to
// it, when the reservation was made through the public API. Failures are
// logged: the booking itself never fails on a webhook.
func (s *Service) Emit(ctx context.Context, orgID int64, eventType string, reservation uuid.UUID) {
	ctx = context.WithoutCancel(ctx)
	if err := s.emit(ctx, orgID, eventType, reservation); err != nil {
		s.log.Warn("public_webhook_emit_failed", "event", eventType, "error", err)
	}
}

func (s *Service) emit(ctx context.Context, orgID int64, eventType string, reservation uuid.UUID) error {
	pr, err := s.q.GetPublicReservationByUUID(ctx, db.GetPublicReservationByUUIDParams{OrganizationID: orgID, ReservationUuid: reservation})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Webhooks are part of the public_api plan feature (TEC-674): outside
	// the plan no event is queued (endpoints stay).
	if err := features.Require(ctx, s.features, orgID, features.PublicAPI); err != nil {
		if _, off := features.FeatureOf(err); off {
			return nil
		}
		return err
	}
	endpoints, err := s.q.ListWebhookEndpointsForEvent(ctx, db.ListWebhookEndpointsForEventParams{OrganizationID: orgID, EventType: eventType})
	if err != nil || len(endpoints) == 0 {
		return err
	}
	octx := orgctx.WithScope(ctx, orgctx.Scope{ID: orgID})
	r, err := s.reservations.Get(octx, orgID, reservation)
	if err != nil {
		return err
	}
	org := pr.OrganizationSlug
	ext := ""
	if r.ExternalReference != nil {
		ext = *r.ExternalReference
	}
	product := r.Plan.SoldProduct
	payload, err := json.Marshal(model.WebhookEvent{
		ID: uuid.New(), Type: eventType, CreatedAt: s.clock(), Organization: org,
		Data: map[string]any{"reservation": model.WebhookReservation{
			Reference: r.Reference, Status: r.Status, PaymentStatus: r.PaymentStatus, StartsAt: r.StartsAt, EndsAt: r.EndsAt,
			PickupBranch: model.Place{UUID: r.PickupBranch.UUID, Name: r.PickupBranch.Name},
			ReturnBranch: model.Place{UUID: r.ReturnBranch.UUID, Name: r.ReturnBranch.Name},
			Subgroup:     model.SubgroupRef{UUID: product.UUID, Code: product.Code, Title: product.Title},
			Channel:      pr.Channel, Currency: r.Price.Currency, Total: r.Price.Total, Prepayment: r.Price.Prepayment,
			Paid: r.Price.Paid, External: ext,
		}},
	})
	if err != nil {
		return err
	}
	var event struct {
		ID uuid.UUID `json:"id"`
	}
	_ = json.Unmarshal(payload, &event)
	ids := make([]int64, 0, len(endpoints))
	for _, ep := range endpoints {
		d, err := s.q.InsertPublicWebhookDelivery(ctx, db.InsertPublicWebhookDeliveryParams{
			OrganizationID: orgID, EndpointID: ep, EventType: eventType, EventUuid: event.ID, Payload: payload,
			NextAttemptAt: ts(s.clock()),
		})
		if err != nil {
			return err
		}
		ids = append(ids, d.ID)
	}
	if s.immediate {
		go func() {
			for _, id := range ids {
				s.attemptNow(ctx, orgID, id)
			}
		}()
	}
	return nil
}

// SetImmediateDelivery tries new deliveries right away in the background
// (the worker sweep retries them either way).
func (s *Service) SetImmediateDelivery(on bool) { s.immediate = on }

// attemptNow delivers one queued delivery unless another attempt holds it.
func (s *Service) attemptNow(ctx context.Context, orgID, id int64) {
	now := s.clock()
	d, err := s.q.ClaimWebhookDelivery(ctx, db.ClaimWebhookDeliveryParams{
		OrganizationID: orgID, ID: id, Now: ts(now), LeaseUntil: ts(now.Add(deliveryLease)),
	})
	if err != nil {
		return
	}
	s.deliver(ctx, db.ClaimDueWebhookDeliveriesRow(d))
}

// DeliverDue sends every due delivery (worker sweep, every minute) and
// purges expired idempotency records. It returns the number of attempts.
func (s *Service) DeliverDue(ctx context.Context) (int, error) {
	if _, err := s.PurgeIdempotency(ctx); err != nil {
		s.log.Warn("public_api_idempotency_purge_failed", "error", err)
	}
	total := 0
	for range 20 {
		now := s.clock()
		rows, err := s.q.ClaimDueWebhookDeliveries(ctx, db.ClaimDueWebhookDeliveriesParams{
			Now: ts(now), LeaseUntil: ts(now.Add(deliveryLease)), RowLimit: deliveryBatch,
		})
		if err != nil {
			return total, err
		}
		for _, d := range rows {
			s.deliver(ctx, d)
		}
		total += len(rows)
		if len(rows) < deliveryBatch {
			break
		}
	}
	return total, nil
}

// deliver POSTs one claimed delivery and records the outcome.
func (s *Service) deliver(ctx context.Context, d db.ClaimDueWebhookDeliveriesRow) {
	w, err := s.q.GetPublicWebhookEndpointByID(ctx, db.GetPublicWebhookEndpointByIDParams{OrganizationID: d.OrganizationID, ID: d.EndpointID})
	if err != nil {
		return
	}
	status, code, errText := s.post(ctx, w, d)
	now := s.clock()
	attempts := int(d.Attempts) + 1
	params := db.FinishWebhookDeliveryParams{
		OrganizationID: d.OrganizationID, ID: d.ID, Status: status, NextAttemptAt: ts(now), LastError: errText,
	}
	if code > 0 {
		params.LastStatusCode = pgtype.Int4{Int32: int32(code), Valid: true}
	}
	switch {
	case status == model.DeliverySucceeded:
		params.DeliveredAt = ts(now)
	case w.Status != model.EndpointActive && d.EventType != model.EventPing:
		params.Status, params.LastError = model.DeliveryFailed, "endpoint disabled"
	case attempts >= s.limits.WebhookMaxAttempts || d.EventType == model.EventPing:
		params.Status = model.DeliveryFailed
	default:
		params.Status = model.DeliveryPending
		params.NextAttemptAt = ts(now.Add(retryBackoff[min(attempts-1, len(retryBackoff)-1)]))
	}
	if err := s.q.FinishWebhookDelivery(ctx, params); err != nil {
		s.log.Warn("public_webhook_finish_failed", "error", err)
	}
	if err := s.q.RecordWebhookEndpointResult(ctx, db.RecordWebhookEndpointResultParams{
		OrganizationID: d.OrganizationID, ID: w.ID, Ok: status == model.DeliverySucceeded, Now: ts(now),
	}); err != nil {
		s.log.Warn("public_webhook_health_failed", "error", err)
	}
}

// post sends the signed request: (succeeded | failed, HTTP status, error).
func (s *Service) post(ctx context.Context, w db.PublicWebhookEndpoint, d db.ClaimDueWebhookDeliveriesRow) (string, int, string) {
	if w.Status != model.EndpointActive && d.EventType != model.EventPing {
		return model.DeliveryFailed, 0, "endpoint disabled"
	}
	secret, err := s.secrets.Open(w.SecretEnc, secretAAD(d.OrganizationID, w.Uuid))
	if err != nil {
		return model.DeliveryFailed, 0, "signing secret unavailable"
	}
	if _, err := s.validateEndpointURL(w.Url); err != nil {
		return model.DeliveryFailed, 0, ErrWebhookTarget.Error()
	}
	cctx, cancel := context.WithTimeout(ctx, s.limits.WebhookTimeout)
	defer cancel()
	stamp := s.clock().Unix()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, w.Url, bytes.NewReader(d.Payload))
	if err != nil {
		return model.DeliveryFailed, 0, "invalid request"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Rentopia-Webhooks/1")
	req.Header.Set(HeaderWebhookID, d.Uuid.String())
	req.Header.Set(HeaderWebhookEvent, d.EventType)
	req.Header.Set(HeaderWebhookTimestamp, strconv.FormatInt(stamp, 10))
	req.Header.Set(HeaderWebhookSignature, SignWebhook(secret, stamp, d.Payload))
	res, err := s.client.Do(req)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, ErrWebhookTarget) {
			msg = ErrWebhookTarget.Error()
		}
		return model.DeliveryFailed, 0, truncate(msg, maxResponseExcerpt)
	}
	defer func() { _ = res.Body.Close() }()
	excerpt, _ := io.ReadAll(io.LimitReader(res.Body, maxResponseExcerpt))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return model.DeliverySucceeded, res.StatusCode, ""
	}
	return model.DeliveryFailed, res.StatusCode, truncate(fmt.Sprintf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(excerpt))), maxResponseExcerpt)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
