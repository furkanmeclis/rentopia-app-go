// Package usecase is the notification engine (TEC-733,
// docs/design/notification-engine.md): organization rules route an event
// to an ordered list of channels (SMS, e-mail, WhatsApp, push) with
// fallback; templates render the message (tr / en, {{variables}});
// every attempt is logged per channel.
//
//   - Emit / EmitTx create a dispatch (one event for one recipient) once:
//     the idempotency key (event + reference + discriminator) is unique per
//     organization, so a retried caller or a second sweep never creates a
//     second dispatch. EmitTx writes inside the caller's transaction (the
//     reservation transition and its notification commit together).
//   - Process sends the dispatch on its next channel. Each channel is tried
//     at most once per dispatch (unique dispatch + channel): a retry never
//     sends the same event twice on the same channel. A failed or skipped
//     channel moves to the next one when the rule allows fallback (RT-A30).
//   - Channels that confirm later (SMS delivery reports) answer "accepted"
//     and report through ReportDelivery; a failed report runs the fallback.
//   - Sweep (worker, every minute) reclaims dispatches left in "sending"
//     (ReclaimStuck), creates reminder and sweep-detected dispatches and
//     processes the due ones.
//   - Channels with a send ledger (channel.Ledger, SMS) are asked before
//     every send whether they already took a message for the delivery; a
//     found message is never handed to the provider again (TEC-962).
//
// Marketing consent (İYS) is not consulted: these are transactional
// messages; marketing communication preferences stay with the customer's
// marketing consents and campaigns.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/templates"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dispatch statuses.
const (
	DispatchPending   = "pending"
	DispatchSending   = "sending"
	DispatchDelivered = "delivered"
	DispatchFailed    = "failed"
	DispatchSkipped   = "skipped"
	// DispatchReconciled: still unsent in restored data but sent after the
	// backup; marked by the operator (cmd/messaging-reconcile), never sent.
	DispatchReconciled = "reconciled"
)

// Delivery statuses.
const (
	DeliverySending   = "sending"
	DeliverySent      = "sent"
	DeliveryDelivered = "delivered"
	DeliveryFailed    = "failed"
	DeliverySkipped   = "skipped"
)

// Resource is the activity resource of rule / template changes.
const Resource = "tenant.notification_settings"

var (
	// ErrUnknownEvent: the event code is not in the catalog.
	ErrUnknownEvent = errors.New("unknown notification event")
	// ErrNotFound: unknown record (or another organization's).
	ErrNotFound = errors.New("not found")
)

// Engine is the notification engine.
type Engine struct {
	pool       *pgxpool.Pool
	q          *db.Queries
	channels   map[string]channel.Channel
	activity   *activity.Recorder
	log        *slog.Logger
	now        func() time.Time
	portalBase string
	batch      int32
	// features: the SMS and WhatsApp channels are plan features
	// (TEC-674); outside the plan the channel is skipped and the dispatch
	// moves on to its next channel. E-mail and push are never gated.
	features features.AccessPolicy
}

// SetFeatures gates the SMS / WhatsApp channels by the organization's
// plan (nil: no gate).
func (e *Engine) SetFeatures(p features.AccessPolicy) { e.features = p }

// New builds the engine. frontendURL builds the customer portal link of
// customer messages (empty: no link).
func New(pool *pgxpool.Pool, q *db.Queries, frontendURL string) *Engine {
	return &Engine{
		pool: pool, q: q, channels: map[string]channel.Channel{}, log: slog.Default(), now: time.Now,
		portalBase: strings.TrimRight(strings.TrimSpace(frontendURL), "/"), batch: 200,
	}
}

// Register adds channel adapters (a later adapter of the same name
// replaces the earlier one). Reporting channels get the engine as their
// reporter.
func (e *Engine) Register(chs ...channel.Channel) {
	for _, c := range chs {
		if c == nil || !channel.IsKnown(c.Name()) {
			continue
		}
		e.channels[c.Name()] = c
		if rc, ok := c.(channel.ReportingChannel); ok {
			rc.SetReporter(e)
		}
	}
}

// Available reports whether a channel has an adapter.
func (e *Engine) Available(name string) bool {
	_, ok := e.channels[name]
	return ok
}

// SetActivity records rule / template changes.
func (e *Engine) SetActivity(rec *activity.Recorder) { e.activity = rec }

// SetLogger replaces the logger.
func (e *Engine) SetLogger(l *slog.Logger) {
	if l != nil {
		e.log = l
	}
}

// SetClock replaces the clock (tests).
func (e *Engine) SetClock(now func() time.Time) { e.now = now }

func (e *Engine) clock() time.Time { return e.now().UTC().Truncate(time.Microsecond) }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

// Rule is an event's effective rule (organization override or default).
type Rule struct {
	Event    string
	Enabled  bool
	Channels []string
	Fallback bool
	Offsets  []int32
	Custom   bool
	Updated  *time.Time
}

func defaultRule(ev catalog.Event) Rule {
	return Rule{
		Event: ev.Code, Enabled: true, Channels: append([]string(nil), ev.Channels...), Fallback: true,
		Offsets: append([]int32(nil), ev.Offsets...),
	}
}

func ruleOf(ev catalog.Event, row db.NotificationRule) Rule {
	t := row.UpdatedAt.Time
	return Rule{
		Event: ev.Code, Enabled: row.Enabled, Channels: row.Channels, Fallback: row.Fallback, Offsets: row.ReminderOffsets,
		Custom: true, Updated: &t,
	}
}

// RuleFor returns an event's effective rule.
func (e *Engine) RuleFor(ctx context.Context, q *db.Queries, orgID int64, code string) (Rule, error) {
	ev, ok := catalog.Get(code)
	if !ok {
		return Rule{}, ErrUnknownEvent
	}
	row, err := q.GetNotificationRule(ctx, db.GetNotificationRuleParams{OrganizationID: orgID, EventCode: code})
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultRule(ev), nil
	}
	if err != nil {
		return Rule{}, err
	}
	return ruleOf(ev, row), nil
}

// EmitInput is one event for one recipient.
type EmitInput struct {
	Event      string
	CustomerID int64
	UserID     int64
	RefType    string
	RefUUID    *uuid.UUID
	// Key tells apart several dispatches of one event and reference (a
	// reminder offset, a status); with the event and reference it forms
	// the idempotency key.
	Key  string
	Vars map[string]string
	// Link overrides the default link (customer portal).
	Link string
	// Channels overrides the rule's channels (test sends).
	Channels    []string
	ScheduledAt time.Time
	Test        bool
}

// Emitted is the result of an emit.
type Emitted struct {
	Dispatch db.NotificationDispatch
	// Created is false when the event was already emitted (same key) or
	// the rule is disabled (Dispatch is then zero).
	Created bool
	// Disabled: the organization turned the event off.
	Disabled bool
}

func idempotencyKey(in EmitInput) string {
	ref := in.RefType
	if in.RefUUID != nil {
		ref += ":" + in.RefUUID.String()
	}
	k := in.Event + "|" + ref + "|" + in.Key
	if len(k) > 200 {
		k = k[:200]
	}
	return k
}

// Emit creates the dispatch (once) and sends it now when it is due.
func (e *Engine) Emit(ctx context.Context, orgID int64, in EmitInput) (Emitted, error) {
	out, err := e.EmitTx(ctx, e.q, orgID, in)
	if err != nil || !out.Created {
		return out, err
	}
	if out.Dispatch.Status == DispatchPending && !out.Dispatch.ScheduledAt.Time.After(e.clock()) {
		if err := e.Process(ctx, orgID, out.Dispatch.ID); err != nil {
			e.log.Warn("notify_process_failed", "dispatch", out.Dispatch.Uuid, "error", err)
		}
		if d, err := e.q.GetNotificationDispatch(ctx, db.GetNotificationDispatchParams{OrganizationID: orgID, ID: out.Dispatch.ID}); err == nil {
			out.Dispatch = d
		}
	}
	return out, nil
}

// EmitTx creates the dispatch inside the caller's transaction (q); it is
// sent by the next sweep (or Process after the commit).
func (e *Engine) EmitTx(ctx context.Context, q *db.Queries, orgID int64, in EmitInput) (Emitted, error) {
	ev, ok := catalog.Get(in.Event)
	if !ok {
		return Emitted{}, ErrUnknownEvent
	}
	rule, err := e.RuleFor(ctx, q, orgID, in.Event)
	if err != nil {
		return Emitted{}, err
	}
	if !rule.Enabled && !in.Test {
		return Emitted{Disabled: true}, nil
	}
	org, err := q.GetNotificationOrganization(ctx, orgID)
	if err != nil {
		return Emitted{}, fmt.Errorf("notify organization: %w", err)
	}
	p := db.InsertNotificationDispatchParams{
		OrganizationID: orgID, EventCode: in.Event, Audience: ev.Audience, RefType: in.RefType,
		IdempotencyKey: idempotencyKey(in), Language: "tr", Fallback: rule.Fallback, Status: DispatchPending,
		ScheduledAt: ts(e.clock()), Test: in.Test, CreatedBy: actorID(ctx),
	}
	if !in.ScheduledAt.IsZero() {
		p.ScheduledAt = ts(in.ScheduledAt.UTC())
	}
	if in.RefUUID != nil {
		p.RefUuid = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
	}
	vars := map[string]string{}
	for k, v := range in.Vars {
		vars[k] = v
	}
	vars["organization_name"] = org.Name
	switch ev.Audience {
	case channel.AudienceCustomer:
		if in.CustomerID <= 0 {
			return Emitted{}, errors.New("notify: customer recipient required")
		}
		c, err := q.GetNotificationCustomer(ctx, db.GetNotificationCustomerParams{OrganizationID: orgID, ID: in.CustomerID})
		if err != nil {
			return Emitted{}, fmt.Errorf("notify customer: %w", err)
		}
		p.CustomerID = pgtype.Int8{Int64: c.ID, Valid: true}
		p.Language = customerLanguage(c.Nationality)
		if _, ok := vars["customer_name"]; !ok {
			vars["customer_name"] = customerName(c)
		}
		if _, ok := vars["link"]; !ok && e.portalBase != "" {
			vars["link"] = e.portalBase + "/r/" + org.Slug
		}
	case channel.AudienceStaff:
		if in.UserID <= 0 {
			return Emitted{}, errors.New("notify: staff recipient required")
		}
		u, err := q.GetNotificationStaff(ctx, db.GetNotificationStaffParams{OrganizationID: orgID, UserID: in.UserID})
		if err != nil {
			return Emitted{}, fmt.Errorf("notify staff: %w", err)
		}
		p.UserID = pgtype.Int8{Int64: u.ID, Valid: true}
		if u.Locale == "en" {
			p.Language = "en"
		}
		if _, ok := vars["staff_name"]; !ok {
			vars["staff_name"] = strings.TrimSpace(u.Name)
		}
	}
	if in.Link != "" {
		vars["link"] = in.Link
	}
	if _, ok := vars["link"]; !ok {
		vars["link"] = ""
	}
	channels := rule.Channels
	if len(in.Channels) > 0 {
		channels = in.Channels
	}
	p.Channels = filterChannels(channels)
	if len(p.Channels) == 0 {
		p.Status, p.LastError, p.CompletedAt = DispatchSkipped, "no channel configured", ts(e.clock())
	}
	p.Vars, _ = json.Marshal(vars)
	row, err := q.InsertNotificationDispatch(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := q.GetNotificationDispatchByKey(ctx, db.GetNotificationDispatchByKeyParams{
			OrganizationID: orgID, IdempotencyKey: p.IdempotencyKey,
		})
		if gerr != nil {
			return Emitted{}, gerr
		}
		return Emitted{Dispatch: existing}, nil
	}
	if err != nil {
		return Emitted{}, err
	}
	return Emitted{Dispatch: row, Created: true}, nil
}

func filterChannels(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, c := range in {
		if channel.IsKnown(c) && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func customerLanguage(nationality string) string {
	n := strings.ToUpper(strings.TrimSpace(nationality))
	if n != "" && n != "TR" {
		return "en"
	}
	return "tr"
}

func customerName(c db.GetNotificationCustomerRow) string {
	if c.CustomerType == "corporate" && strings.TrimSpace(c.CompanyTitle) != "" {
		return strings.TrimSpace(c.CompanyTitle)
	}
	return strings.TrimSpace(c.FirstName + " " + c.LastName)
}

// FormatTime renders a moment in Istanbul time for a message.
func FormatTime(t time.Time) string { return rentaltime.Local(t).Format("02.01.2006 15:04") }

// FormatDate renders a calendar date for a message.
func FormatDate(t time.Time) string { return t.Format("02.01.2006") }

// step is one claimed channel attempt.
type step struct {
	dispatch db.NotificationDispatch
	delivery db.NotificationDelivery
	channel  string
	// planSkip: the channel is outside the organization's plan; the
	// dispatch moves on even when the rule has no fallback.
	planSkip bool
}

// Process sends a dispatch on its next channel, moving on to the next one
// while channels fail and fallback is on. It returns when the dispatch is
// delivered, waits for a delivery report, or has no channel left.
func (e *Engine) Process(ctx context.Context, orgID, dispatchID int64) error {
	for i := 0; i < len(channel.Known)+1; i++ {
		st, err := e.claim(ctx, orgID, dispatchID)
		if err != nil || st == nil {
			return err
		}
		res, err := e.send(ctx, orgID, st)
		if err != nil {
			// The channel's ledger could not tell whether the message was
			// sent: the attempt stays "sending" and ReclaimStuck asks again.
			return err
		}
		next, err := e.record(ctx, orgID, st, res)
		if err != nil || !next {
			return err
		}
	}
	return nil
}

func (e *Engine) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if e.pool == nil {
		return fn(e.q)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(e.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// claim locks the dispatch and inserts the attempt row of its next
// channel (nil when there is nothing to send).
func (e *Engine) claim(ctx context.Context, orgID, dispatchID int64) (*step, error) {
	var out *step
	err := e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockNotificationDispatch(ctx, db.LockNotificationDispatchParams{OrganizationID: orgID, ID: dispatchID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // locked by another worker or gone
		}
		if err != nil {
			return err
		}
		if d.Status != DispatchPending {
			return nil
		}
		idx := d.NextChannelIndex
		for int(idx) < len(d.Channels) {
			ch := d.Channels[idx]
			subject, body := e.render(ctx, q, orgID, d, ch)
			dl, err := q.InsertNotificationDelivery(ctx, db.InsertNotificationDeliveryParams{
				OrganizationID: orgID, DispatchID: d.ID, Channel: ch, AttemptNo: idx + 1, Status: DeliverySending,
				Subject: truncate(subject, 200), Body: body,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				// This channel was already tried for this dispatch. An
				// attempt interrupted before anything was recorded (released
				// by ReclaimStuck) resumes on its own row; the channel's
				// ledger still stops a second message.
				prev, perr := q.GetNotificationDeliveryForChannel(ctx, db.GetNotificationDeliveryForChannelParams{
					OrganizationID: orgID, DispatchID: d.ID, Channel: ch,
				})
				if perr != nil || !interrupted(&prev) {
					idx++
					continue
				}
				dl, err = prev, nil
			}
			if err != nil {
				return err
			}
			if err := q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchSending, NextChannelIndex: idx, LastError: d.LastError,
			}); err != nil {
				return err
			}
			d.Status, d.NextChannelIndex = DispatchSending, idx
			out = &step{dispatch: d, delivery: dl, channel: ch}
			return nil
		}
		status := DispatchFailed
		if d.LastError == "" {
			d.LastError = "no channel left"
		}
		return q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
			OrganizationID: orgID, ID: d.ID, Status: status, NextChannelIndex: idx, LastError: d.LastError,
			CompletedAt: ts(e.clock()),
		})
	})
	return out, err
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// render picks the organization template of the channel and language,
// else the catalog default, and fills the variables.
func (e *Engine) render(ctx context.Context, q *db.Queries, orgID int64, d db.NotificationDispatch, ch string) (string, string) {
	vars := map[string]string{}
	_ = json.Unmarshal(d.Vars, &vars)
	subject, body := e.templateFor(ctx, q, orgID, d.EventCode, ch, d.Language)
	return templates.Render(subject, vars), templates.Render(body, vars)
}

func (e *Engine) templateFor(ctx context.Context, q *db.Queries, orgID int64, code, ch, lang string) (string, string) {
	row, err := q.GetNotificationMessageTemplate(ctx, db.GetNotificationMessageTemplateParams{
		OrganizationID: orgID, EventCode: code, Channel: ch, Language: lang,
	})
	if err == nil {
		return row.Subject, row.Body
	}
	ev, _ := catalog.Get(code)
	t, ok := ev.Templates[lang]
	if !ok {
		t = ev.Templates["tr"]
	}
	return t.Subject, t.Body
}

// channelInPlan reports whether the organization's plan has the channel's
// feature. A failed lookup fails open (the message goes out, as before
// plans existed) and is logged.
func (e *Engine) channelInPlan(ctx context.Context, orgID int64, ch string) (features.Feature, bool) {
	f, gated := features.ForNotifyChannel(ch)
	if !gated {
		return "", true
	}
	err := features.Require(ctx, e.features, orgID, f)
	if err == nil {
		return f, true
	}
	if _, off := features.FeatureOf(err); off {
		return f, false
	}
	e.log.Warn("notify_feature_check_failed", "organization_id", orgID, "channel", ch, "error", err)
	return f, true
}

// send hands the attempt to its channel. An error leaves the attempt
// "sending" (the ledger could not be read).
func (e *Engine) send(ctx context.Context, orgID int64, st *step) (channel.Result, error) {
	if f, ok := e.channelInPlan(ctx, orgID, st.channel); !ok {
		st.planSkip = true
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: string(f) + " is not included in the organization's plan"}, nil
	}
	c, ok := e.channels[st.channel]
	if !ok {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "channel not available"}, nil
	}
	if !c.Supports(st.dispatch.Audience) {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: "channel does not serve this audience"}, nil
	}
	// Send ledger: a message the channel already took for this delivery is
	// never handed to the provider again.
	if res, found, err := e.lookup(ctx, orgID, st.channel, st.delivery.Uuid); err != nil {
		return channel.Result{}, fmt.Errorf("notify ledger %s: %w", st.channel, err)
	} else if found {
		return res, nil
	}
	rcpt, err := e.recipient(ctx, orgID, st.dispatch)
	if err != nil {
		return channel.Result{Outcome: channel.OutcomeSkipped, Error: err.Error()}, nil
	}
	vars := map[string]string{}
	_ = json.Unmarshal(st.dispatch.Vars, &vars)
	msg := channel.Message{
		DeliveryUUID: st.delivery.Uuid, Event: st.dispatch.EventCode, Recipient: rcpt,
		Subject: st.delivery.Subject, Body: st.delivery.Body, Link: vars["link"], RefType: st.dispatch.RefType,
		Test: st.dispatch.Test,
	}
	if msg.Link != "" && strings.Contains(msg.Body, msg.Link) {
		msg.RedactedBody = strings.ReplaceAll(msg.Body, msg.Link, "[link]")
	}
	if st.dispatch.RefUuid.Valid {
		id := uuid.UUID(st.dispatch.RefUuid.Bytes)
		msg.RefUUID = &id
	}
	res, err := c.Send(ctx, orgID, msg)
	if err != nil {
		res.Outcome = channel.OutcomeFailed
		if res.Error == "" {
			res.Error = err.Error()
		}
	}
	if res.Outcome == "" {
		res.Outcome = channel.OutcomeFailed
	}
	return res, nil
}

// lookup asks the channel's send ledger about a delivery (found false for
// channels without one).
func (e *Engine) lookup(ctx context.Context, orgID int64, ch string, delivery uuid.UUID) (channel.Result, bool, error) {
	c, ok := e.channels[ch]
	if !ok {
		return channel.Result{}, false, nil
	}
	l, ok := c.(channel.Ledger)
	if !ok {
		return channel.Result{}, false, nil
	}
	return l.Lookup(ctx, orgID, delivery)
}

func (e *Engine) recipient(ctx context.Context, orgID int64, d db.NotificationDispatch) (channel.Recipient, error) {
	r := channel.Recipient{Audience: d.Audience, Language: d.Language}
	switch d.Audience {
	case channel.AudienceCustomer:
		c, err := e.q.GetNotificationCustomer(ctx, db.GetNotificationCustomerParams{OrganizationID: orgID, ID: d.CustomerID.Int64})
		if err != nil {
			return r, errors.New("customer not found")
		}
		if c.Inactive {
			return r, errors.New("customer is anonymized or deleted")
		}
		r.CustomerID, r.Name, r.Phone, r.Email = c.ID, customerName(c), c.Phone, c.Email
	case channel.AudienceStaff:
		u, err := e.q.GetNotificationStaff(ctx, db.GetNotificationStaffParams{OrganizationID: orgID, UserID: d.UserID.Int64})
		if err != nil {
			return r, errors.New("member not found")
		}
		r.UserID, r.Name, r.Email = u.ID, strings.TrimSpace(u.Name+" "+u.Surname), u.Email
	}
	return r, nil
}

// record writes the attempt's result and reports whether the next channel
// should run now. The provider's acceptance (reference, sent_at) and the
// dispatch's status change commit together, under the dispatch lock (the
// same order as ReportDelivery).
func (e *Engine) record(ctx context.Context, orgID int64, st *step, res channel.Result) (bool, error) {
	now := e.clock()
	p := db.UpdateNotificationDeliveryParams{
		OrganizationID: orgID, ID: st.delivery.ID, Provider: truncate(res.Provider, 32),
		ProviderRef: truncate(res.ProviderRef, 128), RecipientMasked: truncate(res.RecipientMasked, 255), Error: res.Error,
	}
	if res.ExternalUUID != nil {
		p.ExternalUuid = pgtype.UUID{Bytes: *res.ExternalUUID, Valid: true}
	}
	next := false
	err := e.tx(ctx, func(q *db.Queries) error {
		if e.pool != nil {
			if _, err := q.LockNotificationDispatchWait(ctx, db.LockNotificationDispatchWaitParams{
				OrganizationID: orgID, ID: st.dispatch.ID,
			}); err != nil {
				return err
			}
		}
		switch res.Outcome {
		case channel.OutcomeAccepted:
			p.Status, p.SentAt = DeliverySent, ts(now)
			// The dispatch stays "sending" until the report; a report that
			// arrived first already finished the delivery (no rows here).
			_, err := q.UpdateNotificationDelivery(ctx, p)
			return err
		case channel.OutcomeDelivered, channel.OutcomeSent:
			p.Status, p.SentAt, p.FinishedAt = DeliverySent, ts(now), ts(now)
			if res.Outcome == channel.OutcomeDelivered {
				p.Status = DeliveryDelivered
			}
			// No row: a delivery report already settled this attempt.
			if n, err := q.UpdateNotificationDelivery(ctx, p); err != nil || n == 0 {
				return err
			}
			return q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
				OrganizationID: orgID, ID: st.dispatch.ID, Status: DispatchDelivered, NextChannelIndex: st.dispatch.NextChannelIndex,
				CompletedAt: ts(now),
			})
		default:
			p.Status, p.FinishedAt = DeliveryFailed, ts(now)
			if res.Outcome == channel.OutcomeSkipped {
				p.Status = DeliverySkipped
			}
			if n, err := q.UpdateNotificationDelivery(ctx, p); err != nil || n == 0 {
				return err
			}
			var err error
			next, err = e.advance(ctx, q, orgID, st.dispatch, st.channel, res.Error, st.planSkip)
			return err
		}
	})
	if err != nil {
		return false, err
	}
	return next, nil
}

// advance moves a dispatch past a failed channel: to the next channel
// (pending) when fallback allows or the channel was outside the plan
// (always), else failed (skipped when every attempt was skipped).
func (e *Engine) advance(ctx context.Context, q *db.Queries, orgID int64, d db.NotificationDispatch, ch, reason string, always bool) (bool, error) {
	now := e.clock()
	lastErr := ch + ": " + reason
	next := d.NextChannelIndex + 1
	if (d.Fallback || always) && int(next) < len(d.Channels) {
		return true, q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
			OrganizationID: orgID, ID: d.ID, Status: DispatchPending, NextChannelIndex: next, LastError: lastErr,
		})
	}
	status := DispatchFailed
	if rows, err := q.ListNotificationDeliveriesForDispatch(ctx, db.ListNotificationDeliveriesForDispatchParams{
		OrganizationID: orgID, DispatchID: d.ID,
	}); err == nil {
		all := len(rows) > 0
		for _, r := range rows {
			if r.Status != DeliverySkipped {
				all = false
			}
		}
		if all {
			status = DispatchSkipped
		}
	}
	return false, q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
		OrganizationID: orgID, ID: d.ID, Status: status, NextChannelIndex: d.NextChannelIndex, LastError: lastErr,
		CompletedAt: ts(now),
	})
}

// ReportDelivery implements channel.Reporter: the final result of an
// accepted message. A failure runs the next channel (fallback).
func (e *Engine) ReportDelivery(ctx context.Context, orgID int64, r channel.Report) {
	var (
		dl  db.NotificationDelivery
		err error
	)
	if r.DeliveryUUID != nil {
		dl, err = e.q.GetNotificationDeliveryByUUID(ctx, db.GetNotificationDeliveryByUUIDParams{OrganizationID: orgID, Uuid: *r.DeliveryUUID})
	} else {
		dl, err = e.q.GetNotificationDeliveryByExternal(ctx, db.GetNotificationDeliveryByExternalParams{
			OrganizationID: orgID, Channel: r.Channel, ExternalUuid: pgtype.UUID{Bytes: r.ExternalUUID, Valid: true},
		})
	}
	if err != nil || dl.Channel != r.Channel {
		return
	}
	status := DeliveryDelivered
	if !r.Delivered {
		status = DeliveryFailed
	}
	now := e.clock()
	next := false
	err = e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockNotificationDispatchWait(ctx, db.LockNotificationDispatchWaitParams{OrganizationID: orgID, ID: dl.DispatchID})
		if err != nil {
			return err
		}
		n, err := q.FinishNotificationDelivery(ctx, db.FinishNotificationDeliveryParams{
			OrganizationID: orgID, ID: dl.ID, Status: status, Error: r.Error, ProviderRef: truncate(r.ProviderRef, 128),
			FinishedAt: ts(now),
		})
		if err != nil || n == 0 {
			return err
		}
		// Only the attempt the dispatch is waiting for moves it.
		if d.Status != DispatchSending || int(d.NextChannelIndex) >= len(d.Channels) || d.Channels[d.NextChannelIndex] != dl.Channel {
			return nil
		}
		if r.Delivered {
			return q.UpdateNotificationDispatchState(ctx, db.UpdateNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchDelivered, NextChannelIndex: d.NextChannelIndex,
				LastError: d.LastError, CompletedAt: ts(now),
			})
		}
		reason := r.Error
		if reason == "" {
			reason = "not delivered"
		}
		next, err = e.advance(ctx, q, orgID, d, dl.Channel, reason, false)
		return err
	})
	if err != nil {
		e.log.Warn("notify_report_failed", "delivery", dl.Uuid, "error", err)
		return
	}
	if next {
		if err := e.Process(ctx, orgID, dl.DispatchID); err != nil {
			e.log.Warn("notify_fallback_failed", "delivery", dl.Uuid, "error", err)
		}
	}
}

// ProcessDue sends every due dispatch (sweep) and returns how many were
// processed.
func (e *Engine) ProcessDue(ctx context.Context) (int, error) {
	now := e.clock()
	orgs, err := e.q.ListDueNotificationDispatchOrganizations(ctx, ts(now))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, orgID := range orgs {
		m, err := e.ProcessDueOrganization(ctx, orgID)
		if err != nil {
			e.log.Warn("notify_due_list_failed", "organization", orgID, "error", err)
		}
		n += m
	}
	return n, nil
}

// ProcessDueOrganization sends one organization's due dispatches.
func (e *Engine) ProcessDueOrganization(ctx context.Context, orgID int64) (int, error) {
	ids, err := e.q.ListDueNotificationDispatches(ctx, db.ListDueNotificationDispatchesParams{
		OrganizationID: orgID, NowAt: ts(e.clock()), LimitCount: e.batch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := e.Process(ctx, orgID, id); err != nil {
			e.log.Warn("notify_process_failed", "organization", orgID, "dispatch", id, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// Sweep creates the reminder / sweep-detected dispatches and sends the due
// ones (worker, every minute).
func (e *Engine) Sweep(ctx context.Context) (int, error) {
	if _, err := e.ReclaimStuck(ctx, DefaultStuckSending); err != nil {
		e.log.Warn("notify_reclaim_failed", "error", err)
	}
	if err := e.Reminders(ctx); err != nil {
		e.log.Warn("notify_reminders_failed", "error", err)
	}
	return e.ProcessDue(ctx)
}
