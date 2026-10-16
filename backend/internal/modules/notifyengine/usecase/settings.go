package usecase

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/templates"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Detail is a field validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// Template limits.
const (
	maxSubject = 200
	maxBody    = 2000
)

// Channels lists the known channels and whether an adapter is installed.
func (e *Engine) Channels() []model.ChannelInfo {
	out := make([]model.ChannelInfo, 0, len(channel.Known))
	for _, c := range channel.Known {
		out = append(out, model.ChannelInfo{Name: c, Available: e.Available(c)})
	}
	return out
}

func ruleView(r Rule) model.RuleView {
	ch := r.Channels
	if ch == nil {
		ch = []string{}
	}
	off := r.Offsets
	if off == nil {
		off = []int32{}
	}
	return model.RuleView{Enabled: r.Enabled, Channels: ch, Fallback: r.Fallback, ReminderOffsets: off, Custom: r.Custom}
}

func eventInfo(ev catalog.Event, r Rule) model.EventInfo {
	return model.EventInfo{
		Code: ev.Code, Group: ev.Group, Audience: ev.Audience, Instant: ev.Instant, Reminder: ev.Reminder,
		Variables: append([]string{}, ev.Vars...), Rule: ruleView(r), Default: ruleView(defaultRule(ev)), UpdatedAt: r.Updated,
	}
}

// ListEvents returns the catalog with the organization's effective rules.
func (e *Engine) ListEvents(ctx context.Context, orgID int64) ([]model.EventInfo, error) {
	rows, err := e.q.ListNotificationRules(ctx, orgID)
	if err != nil {
		return nil, err
	}
	custom := map[string]db.NotificationRule{}
	for _, r := range rows {
		custom[r.EventCode] = r
	}
	out := []model.EventInfo{}
	for _, ev := range catalog.All() {
		r := defaultRule(ev)
		if row, ok := custom[ev.Code]; ok {
			r = ruleOf(ev, row)
		}
		out = append(out, eventInfo(ev, r))
	}
	return out, nil
}

// PutRule replaces an event's rule.
func (e *Engine) PutRule(ctx context.Context, orgID int64, code string, in model.RuleInput) (model.EventInfo, error) {
	ev, ok := catalog.Get(code)
	if !ok {
		return model.EventInfo{}, ErrNotFound
	}
	ve := &ValidationError{}
	seen := map[string]bool{}
	channels := []string{}
	for i, c := range in.Channels {
		c = strings.TrimSpace(c)
		switch {
		case !channel.IsKnown(c):
			ve.add("channels."+itoa(i), "invalid", "unknown channel")
		case seen[c]:
			ve.add("channels."+itoa(i), "duplicate", "a channel appears once")
		default:
			seen[c] = true
			channels = append(channels, c)
		}
	}
	if in.Enabled && len(in.Channels) == 0 {
		ve.add("channels", "required", "choose at least one channel")
	}
	offsets := []int32{}
	if !ev.Reminder && len(in.ReminderOffsets) > 0 {
		ve.add("reminder_offsets", "not_allowed", "this event has no reminders")
	}
	if ev.Reminder {
		if len(in.ReminderOffsets) > 5 {
			ve.add("reminder_offsets", "too_many", "at most 5 reminders")
		}
		if in.Enabled && len(in.ReminderOffsets) == 0 && !ev.Instant {
			ve.add("reminder_offsets", "required", "add at least one reminder time")
		}
		dup := map[int32]bool{}
		for i, o := range in.ReminderOffsets {
			switch {
			case o < 0 || o > catalog.MaxOffsetMinutes:
				ve.add("reminder_offsets."+itoa(i), "out_of_range", "between 0 minutes and 30 days")
			case dup[o]:
				ve.add("reminder_offsets."+itoa(i), "duplicate", "a reminder time appears once")
			default:
				dup[o] = true
				offsets = append(offsets, o)
			}
		}
		sort.Slice(offsets, func(i, j int) bool { return offsets[i] > offsets[j] })
	}
	if err := ve.orNil(); err != nil {
		return model.EventInfo{}, err
	}
	row, err := e.q.UpsertNotificationRule(ctx, db.UpsertNotificationRuleParams{
		OrganizationID: orgID, EventCode: code, Enabled: in.Enabled, Channels: channels, Fallback: in.Fallback,
		ReminderOffsets: offsets, UpdatedBy: actorID(ctx),
	})
	if err != nil {
		return model.EventInfo{}, err
	}
	e.audit(ctx, orgID, "notification_rules.updated", row.Uuid, map[string]any{
		"event": code, "enabled": in.Enabled, "channels": channels, "fallback": in.Fallback, "reminder_offsets": offsets,
	})
	return eventInfo(ev, ruleOf(ev, row)), nil
}

// ResetRule drops the organization's rule (the default applies again).
func (e *Engine) ResetRule(ctx context.Context, orgID int64, code string) (model.EventInfo, error) {
	ev, ok := catalog.Get(code)
	if !ok {
		return model.EventInfo{}, ErrNotFound
	}
	n, err := e.q.DeleteNotificationRule(ctx, db.DeleteNotificationRuleParams{OrganizationID: orgID, EventCode: code})
	if err != nil {
		return model.EventInfo{}, err
	}
	if n > 0 {
		e.audit(ctx, orgID, "notification_rules.reset", uuid.Nil, map[string]any{"event": code})
	}
	return eventInfo(ev, defaultRule(ev)), nil
}

// ListTemplates returns every event / channel / language template: the
// organization's own or the default.
func (e *Engine) ListTemplates(ctx context.Context, orgID int64, event, ch, lang string) ([]model.Template, error) {
	rows, err := e.q.ListNotificationMessageTemplates(ctx, orgID)
	if err != nil {
		return nil, err
	}
	custom := map[string]db.NotificationMessageTemplate{}
	for _, r := range rows {
		custom[r.EventCode+"|"+r.Channel+"|"+r.Language] = r
	}
	out := []model.Template{}
	for _, ev := range catalog.All() {
		if event != "" && ev.Code != event {
			continue
		}
		for _, c := range channel.Known {
			if ch != "" && c != ch {
				continue
			}
			for _, l := range catalog.Languages {
				if lang != "" && l != lang {
					continue
				}
				out = append(out, templateView(ev, c, l, custom))
			}
		}
	}
	return out, nil
}

func templateView(ev catalog.Event, ch, lang string, custom map[string]db.NotificationMessageTemplate) model.Template {
	t := model.Template{Event: ev.Code, Channel: ch, Language: lang, Variables: append([]string{}, ev.Vars...)}
	if row, ok := custom[ev.Code+"|"+ch+"|"+lang]; ok {
		u := row.UpdatedAt.Time
		t.Subject, t.Body, t.Custom, t.UpdatedAt = row.Subject, row.Body, true, &u
		return t
	}
	d := ev.Templates[lang]
	t.Subject, t.Body = d.Subject, d.Body
	return t
}

var placeholderRE = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// unknownVars lists placeholders that are not variables of the event.
func unknownVars(ev catalog.Event, texts ...string) []string {
	allowed := map[string]bool{}
	for _, v := range ev.Vars {
		allowed[v] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, t := range texts {
		for _, m := range placeholderRE.FindAllStringSubmatch(t, -1) {
			if !allowed[m[1]] && !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

func validTemplateKey(ch, lang string) error {
	ve := &ValidationError{}
	if !channel.IsKnown(ch) {
		ve.add("channel", "invalid", "unknown channel")
	}
	if lang != "tr" && lang != "en" {
		ve.add("language", "invalid", "language is tr or en")
	}
	return ve.orNil()
}

// PutTemplate replaces an organization template.
func (e *Engine) PutTemplate(ctx context.Context, orgID int64, code, ch, lang string, in model.TemplateInput) (model.Template, error) {
	ev, ok := catalog.Get(code)
	if !ok {
		return model.Template{}, ErrNotFound
	}
	if err := validTemplateKey(ch, lang); err != nil {
		return model.Template{}, err
	}
	subject, body := strings.TrimSpace(in.Subject), strings.TrimSpace(in.Body)
	ve := &ValidationError{}
	if len([]rune(subject)) > maxSubject {
		ve.add("subject", "too_long", "at most 200 characters")
	}
	if ch == channel.Email && subject == "" {
		ve.add("subject", "required", "an e-mail needs a subject")
	}
	if body == "" {
		ve.add("body", "required", "the message is required")
	} else if len([]rune(body)) > maxBody {
		ve.add("body", "too_long", "at most 2000 characters")
	}
	if ch == channel.SMS && len([]rune(body)) > 900 {
		ve.add("body", "too_long", "an SMS is at most 900 characters")
	}
	for _, u := range unknownVars(ev, subject, body) {
		field := "body"
		if strings.Contains(subject, u) {
			field = "subject"
		}
		ve.add(field, "unknown_variable", "unknown variable {{"+u+"}}")
	}
	if err := ve.orNil(); err != nil {
		return model.Template{}, err
	}
	row, err := e.q.UpsertNotificationMessageTemplate(ctx, db.UpsertNotificationMessageTemplateParams{
		OrganizationID: orgID, EventCode: code, Channel: ch, Language: lang, Subject: subject, Body: body,
		UpdatedBy: actorID(ctx),
	})
	if err != nil {
		return model.Template{}, err
	}
	e.audit(ctx, orgID, "notification_templates.updated", row.Uuid, map[string]any{"event": code, "channel": ch, "language": lang})
	return templateView(ev, ch, lang, map[string]db.NotificationMessageTemplate{code + "|" + ch + "|" + lang: row}), nil
}

// ResetTemplate drops the organization template (the default applies).
func (e *Engine) ResetTemplate(ctx context.Context, orgID int64, code, ch, lang string) (model.Template, error) {
	ev, ok := catalog.Get(code)
	if !ok {
		return model.Template{}, ErrNotFound
	}
	if err := validTemplateKey(ch, lang); err != nil {
		return model.Template{}, err
	}
	n, err := e.q.DeleteNotificationMessageTemplate(ctx, db.DeleteNotificationMessageTemplateParams{
		OrganizationID: orgID, EventCode: code, Channel: ch, Language: lang,
	})
	if err != nil {
		return model.Template{}, err
	}
	if n > 0 {
		e.audit(ctx, orgID, "notification_templates.reset", uuid.Nil, map[string]any{"event": code, "channel": ch, "language": lang})
	}
	return templateView(ev, ch, lang, nil), nil
}

// Preview renders a template (the given text, else the effective one)
// with sample values.
func (e *Engine) Preview(ctx context.Context, orgID int64, in model.PreviewInput) (model.Preview, error) {
	ev, ok := catalog.Get(in.Event)
	if !ok {
		return model.Preview{}, &ValidationError{Details: []Detail{{Field: "event", Code: "invalid", Message: "unknown event"}}}
	}
	lang := in.Language
	if lang == "" {
		lang = "tr"
	}
	ch := in.Channel
	if ch == "" {
		ch = channel.SMS
	}
	if err := validTemplateKey(ch, lang); err != nil {
		return model.Preview{}, err
	}
	subject, body := in.Subject, in.Body
	if strings.TrimSpace(body) == "" {
		subject, body = e.templateFor(ctx, e.q, orgID, ev.Code, ch, lang)
	}
	vars := catalog.SampleVars(lang)
	return model.Preview{
		Subject: templates.Render(subject, vars), Body: templates.Render(body, vars), Unknown: unknownVars(ev, subject, body),
	}, nil
}

func optTimeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func optUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func optPgUUID(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := uuid.UUID(id.Bytes)
	return &v
}

func textArray(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// Sortable fields of the delivery history.
var Sortable = []string{"created_at", "channel", "status", "event_code"}

// ListDeliveries returns the delivery history page.
func (e *Engine) ListDeliveries(ctx context.Context, orgID int64, f model.DeliveryFilter) ([]model.Delivery, int64, error) {
	if f.SortField == "" {
		f.SortField, f.SortDesc = "created_at", true
	}
	p := db.ListNotificationDeliveriesParams{
		OrganizationID: orgID, Channels: textArray(f.Channels), Statuses: textArray(f.Statuses), Events: textArray(f.Events),
		Audiences: textArray(f.Audiences), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.Q != "" {
		p.Q = pgtype.Text{String: f.Q, Valid: true}
	}
	if f.CreatedFrom != nil {
		p.CreatedFrom = ts(*f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		p.CreatedTo = ts(*f.CreatedTo)
	}
	rows, err := e.q.ListNotificationDeliveries(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Delivery, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		name := r.CustomerName
		if r.Audience == channel.AudienceStaff {
			name = r.UserName
		}
		out = append(out, model.Delivery{
			UUID: r.Uuid, DispatchUUID: r.DispatchUuid, Event: r.EventCode, Audience: r.Audience, Channel: r.Channel,
			AttemptNo: r.AttemptNo, Status: r.Status, DispatchStatus: r.DispatchStatus, Provider: r.Provider,
			ProviderRef: r.ProviderRef, RecipientMasked: r.RecipientMasked, RecipientName: name,
			CustomerUUID: optUUID(r.CustomerUuid), UserUUID: optUUID(r.UserUuid), Subject: r.Subject, Error: r.Error,
			RefType: r.RefType, RefUUID: optPgUUID(r.RefUuid), Test: r.Test, SentAt: optTimeOf(r.SentAt),
			FinishedAt: optTimeOf(r.FinishedAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	if len(rows) == 0 && f.Offset > 0 {
		// Past the last page: count without paging.
		p.LimitCount, p.OffsetCount = 1, 0
		if all, err := e.q.ListNotificationDeliveries(ctx, p); err == nil && len(all) > 0 {
			total = all[0].TotalCount
		}
	}
	return out, total, nil
}

// Facets returns the history filter options.
func (e *Engine) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	rows, err := e.q.NotificationDeliveryFacets(ctx, orgID)
	if err != nil {
		return model.Facets{}, err
	}
	out := model.Facets{Channel: []model.FacetValue{}, Status: []model.FacetValue{}, Event: []model.FacetValue{}}
	for _, r := range rows {
		v := model.FacetValue{Value: r.Value, Count: r.Count}
		switch r.Facet {
		case "channel":
			out.Channel = append(out.Channel, v)
		case "status":
			out.Status = append(out.Status, v)
		case "event_code":
			out.Event = append(out.Event, v)
		}
	}
	return out, nil
}

// GetDispatch returns a dispatch with its channel attempts.
func (e *Engine) GetDispatch(ctx context.Context, orgID int64, id uuid.UUID) (model.Dispatch, error) {
	d, err := e.q.GetNotificationDispatchView(ctx, db.GetNotificationDispatchViewParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Dispatch{}, ErrNotFound
	}
	if err != nil {
		return model.Dispatch{}, err
	}
	name := d.CustomerName
	if d.Audience == channel.AudienceStaff {
		name = d.UserName
	}
	out := model.Dispatch{
		UUID: d.Uuid, Event: d.EventCode, Audience: d.Audience, Status: d.Status, Channels: d.Channels, Fallback: d.Fallback,
		Language: d.Language, RecipientName: name, CustomerUUID: optUUID(d.CustomerUuid), UserUUID: optUUID(d.UserUuid),
		RefType: d.RefType, RefUUID: optPgUUID(d.RefUuid), LastError: d.LastError, Test: d.Test,
		ScheduledAt: d.ScheduledAt.Time, CompletedAt: optTimeOf(d.CompletedAt), CreatedAt: d.CreatedAt.Time,
		Attempts: []model.Attempt{},
	}
	if out.Channels == nil {
		out.Channels = []string{}
	}
	rows, err := e.q.ListNotificationDeliveriesForDispatch(ctx, db.ListNotificationDeliveriesForDispatchParams{OrganizationID: orgID, DispatchID: d.ID})
	if err != nil {
		return model.Dispatch{}, err
	}
	for _, r := range rows {
		out.Attempts = append(out.Attempts, model.Attempt{
			UUID: r.Uuid, Channel: r.Channel, AttemptNo: r.AttemptNo, Status: r.Status, Provider: r.Provider,
			ProviderRef: r.ProviderRef, RecipientMasked: r.RecipientMasked, Subject: r.Subject, Body: r.Body, Error: r.Error,
			SentAt: optTimeOf(r.SentAt), FinishedAt: optTimeOf(r.FinishedAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// Test sends an event with sample values: customer events to the given
// customer, staff events to the caller. It is logged like any send
// (marked test) and runs the organization's rule (fallback included)
// unless one channel is chosen.
func (e *Engine) Test(ctx context.Context, orgID int64, in model.TestInput) (model.Dispatch, error) {
	ev, ok := catalog.Get(in.Event)
	ve := &ValidationError{}
	if !ok {
		ve.add("event", "invalid", "unknown event")
	}
	if in.Channel != "" && !channel.IsKnown(in.Channel) {
		ve.add("channel", "invalid", "unknown channel")
	}
	if err := ve.orNil(); err != nil {
		return model.Dispatch{}, err
	}
	emit := EmitInput{Event: ev.Code, RefType: "test", Key: "test:" + uuid.NewString(), Test: true}
	if in.Channel != "" {
		emit.Channels = []string{in.Channel}
	}
	vars := catalog.SampleVars("tr")
	delete(vars, "customer_name")
	delete(vars, "staff_name")
	delete(vars, "organization_name")
	delete(vars, "link")
	emit.Vars = vars
	switch ev.Audience {
	case channel.AudienceCustomer:
		if in.CustomerUUID == nil {
			return model.Dispatch{}, &ValidationError{Details: []Detail{{Field: "customer_uuid", Code: "required", Message: "choose a customer"}}}
		}
		id, err := e.q.GetNotificationCustomerByUUID(ctx, db.GetNotificationCustomerByUUIDParams{OrganizationID: orgID, Uuid: *in.CustomerUUID})
		if err != nil {
			return model.Dispatch{}, &ValidationError{Details: []Detail{{Field: "customer_uuid", Code: "not_found", Message: "customer not found"}}}
		}
		emit.CustomerID = id
	default:
		p, ok := authctx.PrincipalFrom(ctx)
		if !ok || p.UserInternal <= 0 {
			return model.Dispatch{}, &ValidationError{Details: []Detail{{Field: "event", Code: "invalid", Message: "no member to send to"}}}
		}
		emit.UserID = p.UserInternal
	}
	out, err := e.Emit(ctx, orgID, emit)
	if err != nil {
		return model.Dispatch{}, err
	}
	e.audit(ctx, orgID, "notifications.test_sent", out.Dispatch.Uuid, map[string]any{"event": ev.Code, "channel": in.Channel})
	return e.GetDispatch(ctx, orgID, out.Dispatch.Uuid)
}

func (e *Engine) audit(ctx context.Context, orgID int64, action string, id uuid.UUID, payload map[string]any) {
	if e.activity == nil {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	var actor *int64
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		v := p.UserInternal
		actor = &v
	}
	var ref *uuid.UUID
	if id != uuid.Nil {
		ref = &id
	}
	e.activity.Record(ctx, actor, action, Resource, ref, payload, nil)
}

func itoa(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return digits[i : i+1]
	}
	return itoa(i/10) + digits[i%10:i%10+1]
}
