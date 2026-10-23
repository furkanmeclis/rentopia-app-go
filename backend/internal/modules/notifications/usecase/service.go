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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/actionlink"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/templates"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

// Querier is the persistence surface used by the notification service.
type Querier interface {
	CreateNotification(ctx context.Context, arg db.CreateNotificationParams) (db.Notification, error)
	GetNotificationByUUID(ctx context.Context, arg uuid.UUID) (db.Notification, error)
	GetNotificationByID(ctx context.Context, id int64) (db.Notification, error)
	ListNotificationsForUser(ctx context.Context, arg db.ListNotificationsForUserParams) ([]db.Notification, error)
	CountNotificationsForUser(ctx context.Context, arg db.CountNotificationsForUserParams) (int64, error)
	CountUnreadInappForUser(ctx context.Context, arg db.CountUnreadInappForUserParams) (int64, error)
	ListPlatformNotifications(ctx context.Context, arg db.ListPlatformNotificationsParams) ([]db.Notification, error)
	CountPlatformNotifications(ctx context.Context, arg db.CountPlatformNotificationsParams) (int64, error)
	MarkNotificationProcessing(ctx context.Context, id int64) (db.Notification, error)
	ListStuckProcessingNotificationIDs(ctx context.Context, staleMinutes int32) ([]int64, error)
	ReclaimProcessingNotification(ctx context.Context, arg db.ReclaimProcessingNotificationParams) (db.Notification, error)
	ListStaleQueuedNotifications(ctx context.Context, staleMinutes int32) ([]db.ListStaleQueuedNotificationsRow, error)
	GetNotificationByIdempotencyKey(ctx context.Context, idempotencyKey pgtype.Text) (db.Notification, error)
	MarkNotificationSent(ctx context.Context, arg db.MarkNotificationSentParams) (db.Notification, error)
	MarkNotificationFailed(ctx context.Context, arg db.MarkNotificationFailedParams) (db.Notification, error)
	MarkNotificationRead(ctx context.Context, arg db.MarkNotificationReadParams) (db.Notification, error)
	MarkAllNotificationsReadForUser(ctx context.Context, arg db.MarkAllNotificationsReadForUserParams) (int64, error)
	InsertNotificationHistory(ctx context.Context, arg db.InsertNotificationHistoryParams) (db.NotificationHistory, error)
	GetTemplateByCodeChannelLang(ctx context.Context, arg db.GetTemplateByCodeChannelLangParams) (db.NotificationTemplate, error)
	GetNotificationPreferences(ctx context.Context, userID int64) (db.NotificationPreference, error)
	UpsertNotificationPreferences(ctx context.Context, arg db.UpsertNotificationPreferencesParams) (db.NotificationPreference, error)
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetUserByUUID(ctx context.Context, id uuid.UUID) (db.User, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
}

// Enqueuer schedules background delivery tasks.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service coordinates enqueue + delivery.
type Service struct {
	q            Querier
	queue        Enqueuer
	providers    map[string]providers.Provider
	log          *slog.Logger
	syncMode     bool // when queue is nil, deliver inline
	vapid        *VAPIDConfig
	expo         *expoClient
	pushQueue    Enqueuer
	actionSecret []byte
	actionTTL    time.Duration
	// redeliver re-enqueues stale queued rows (ReclaimStuck); nil delivers
	// them inline.
	redeliver Enqueuer
}

// New creates a notification service.
func New(q Querier, enq Enqueuer, provs []providers.Provider, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	m := make(map[string]providers.Provider, len(provs))
	for _, p := range provs {
		m[p.Channel()] = p
	}
	return &Service{
		q: q, queue: enq, providers: m, log: log, syncMode: enq == nil,
		actionTTL: actionlink.DefaultTTL,
	}
}

// WithRedeliverQueue makes ReclaimStuck hand stale queued rows back to the
// delivery queue (the worker, whose own deliveries run inline) instead of
// delivering them inline.
func (s *Service) WithRedeliverQueue(enq Enqueuer) *Service {
	s.redeliver = enq
	return s
}

// WithActionSigner enables HMAC-signed action URLs (marks read on redeem).
func (s *Service) WithActionSigner(secret string, ttl time.Duration) *Service {
	if secret != "" {
		s.actionSecret = []byte(secret)
	}
	if ttl > 0 {
		s.actionTTL = ttl
	}
	return s
}

// Enqueue creates queued notification rows and schedules delivery.
func (s *Service) Enqueue(ctx context.Context, in model.EnqueueInput) ([]model.Notification, error) {
	if len(in.Channels) == 0 {
		return nil, fmt.Errorf("%w: channels required", ErrInvalidRequest)
	}
	lang := in.Language
	if lang == "" && in.UserID != nil {
		if u, err := s.q.GetUserByID(ctx, *in.UserID); err == nil && u.Locale != "" {
			lang = u.Locale
		}
	}
	if lang == "" {
		lang = "tr"
	}
	priority := in.Priority
	if priority == "" {
		priority = model.PriorityNormal
	}

	prefs := model.Preferences{EmailEnabled: true, InappEnabled: true, RealtimeEnabled: true, PushEnabled: false}
	if in.UserID != nil {
		if p, err := s.q.GetNotificationPreferences(ctx, *in.UserID); err == nil {
			prefs = model.Preferences{
				EmailEnabled: p.EmailEnabled, InappEnabled: p.InappEnabled, RealtimeEnabled: p.RealtimeEnabled,
				PushEnabled: p.PushEnabled,
			}
		}
	}

	payloadIn, err := s.stampOrganization(ctx, in.OrganizationID, in.Payload)
	if err != nil {
		return nil, err
	}

	var out []model.Notification
	for _, ch := range in.Channels {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		if !channelAllowed(ch, prefs, in.SecurityEmail) {
			s.log.Debug("notification_channel_skipped_by_preference", "channel", ch, "template", in.TemplateCode)
			continue
		}
		title, body := in.Title, in.Body
		if in.TemplateCode != "" {
			tpl, err := s.q.GetTemplateByCodeChannelLang(ctx, db.GetTemplateByCodeChannelLangParams{
				Code: in.TemplateCode, Channel: ch, Language: lang,
			})
			if errors.Is(err, pgx.ErrNoRows) && lang != "en" {
				tpl, err = s.q.GetTemplateByCodeChannelLang(ctx, db.GetTemplateByCodeChannelLangParams{
					Code: in.TemplateCode, Channel: ch, Language: "en",
				})
			}
			if err == nil {
				title = templates.Render(tpl.Subject, in.TemplateVars)
				body = templates.Render(tpl.Body, in.TemplateVars)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			} else if title == "" {
				title = in.TemplateCode
			}
		}
		payload, err := json.Marshal(withPushVars(ch, payloadIn, in.TemplateVars))
		if err != nil {
			payload = []byte("{}")
		}
		recipient := textPtr(in.Recipient)
		if ch == model.ChannelEmail && !recipient.Valid && in.UserID != nil {
			if u, err := s.q.GetUserByID(ctx, *in.UserID); err == nil {
				recipient = pgtype.Text{String: u.Email, Valid: true}
			}
		}
		key := model.RowKey(strings.TrimSpace(in.IdempotencyKey), ch)
		row, err := s.q.CreateNotification(ctx, db.CreateNotificationParams{
			UserID:         int8Ptr(in.UserID),
			Channel:        ch,
			Status:         model.StatusQueued,
			Priority:       priority,
			Title:          title,
			Body:           body,
			Payload:        payload,
			ActionUrl:      textPtr(in.ActionURL),
			Recipient:      recipient,
			TemplateCode:   pgtype.Text{String: in.TemplateCode, Valid: in.TemplateCode != ""},
			SourceEvent:    pgtype.Text{String: in.SourceEvent, Valid: in.SourceEvent != ""},
			ScheduledAt:    pgtype.Timestamptz{},
			MaxAttempts:    5,
			OrganizationID: int8Ptr(in.OrganizationID),
			IdempotencyKey: pgtype.Text{String: key, Valid: key != ""},
		})
		if errors.Is(err, pgx.ErrNoRows) && key != "" {
			// Already created for this cause, channel and recipient (a
			// replayed event): the existing row, delivered once.
			existing, gerr := s.q.GetNotificationByIdempotencyKey(ctx, pgtype.Text{String: key, Valid: true})
			if gerr != nil {
				return nil, fmt.Errorf("existing notification: %w", gerr)
			}
			out = append(out, s.project(existing))
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create notification: %w", err)
		}
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: row.ID,
			Event:          "queued",
			Metadata:       []byte("{}"),
		})
		if err := s.scheduleDeliver(ctx, row.ID, row.Uuid); err != nil {
			return nil, err
		}
		out = append(out, s.project(row))
	}
	return out, nil
}

func (s *Service) scheduleDeliver(ctx context.Context, id int64, rowUUID uuid.UUID) error {
	if s.syncMode || s.queue == nil {
		return s.Deliver(ctx, id)
	}
	return enqueueDeliver(s.queue, id, rowUUID)
}

// DeliverTaskID is the Asynq task id of a notification's delivery: a second
// enqueue of the same row (the queued sweep, a replay) is refused while the
// first task exists, and Deliver's status check covers the rest.
func DeliverTaskID(rowUUID uuid.UUID) string { return "notification:deliver:" + rowUUID.String() }

// enqueueDeliver enqueues the delivery task; an existing task of the row
// (task id conflict) counts as enqueued.
func enqueueDeliver(enq Enqueuer, id int64, rowUUID uuid.UUID) error {
	task, err := queue.NewNotificationDeliverTask(id)
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.Queue(queue.QueueNotifications)}
	if rowUUID != uuid.Nil {
		opts = append(opts, asynq.TaskID(DeliverTaskID(rowUUID)))
	}
	if _, err = enq.Enqueue(task, opts...); errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// DefaultStuckProcessingMinutes: a delivery in processing this long is
// taken over by ReclaimStuck (worker crash, restore).
const DefaultStuckProcessingMinutes int32 = 2

// DefaultStuckQueuedMinutes: a queued row whose task has not run for this
// long is handed to delivery again by ReclaimStuck (task lost with Redis).
const DefaultStuckQueuedMinutes int32 = 5

// Deliver processes one queued notification. A row another delivery holds
// (processing) or that is final is left alone: a duplicate task never
// delivers twice; ReclaimStuck takes over rows left in processing.
func (s *Service) Deliver(ctx context.Context, id int64) error {
	row, err := s.q.MarkNotificationProcessing(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // gone, final, or another delivery holds it
	}
	if err != nil {
		return err
	}
	_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
		NotificationID: row.ID, Event: "processing", Metadata: []byte("{}"),
	})
	return s.deliverRow(ctx, row)
}

// deliverRow delivers a row this caller holds (processing).
func (s *Service) deliverRow(ctx context.Context, row db.Notification) error {
	id := row.ID
	p, ok := s.providers[row.Channel]
	if !ok {
		p = providers.NoopProvider{Name: row.Channel, Log: s.log}
	}
	var userUUID *uuid.UUID
	if row.UserID.Valid {
		if u, err := s.q.GetUserByID(ctx, row.UserID.Int64); err == nil {
			uid := u.Uuid
			userUUID = &uid
		}
	}
	res, err := p.Deliver(ctx, row, userUUID)
	if err != nil {
		_, _ = s.q.MarkNotificationFailed(ctx, db.MarkNotificationFailedParams{
			ID: id, LastError: pgtype.Text{String: err.Error(), Valid: true},
		})
		_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
			NotificationID: id, Event: "failed",
			Metadata: mustJSON(map[string]string{"error": err.Error()}),
		})
		return err
	}
	status := res.Status
	if status == "" {
		status = model.StatusSent
	}
	_, err = s.q.MarkNotificationSent(ctx, db.MarkNotificationSentParams{
		ID: id, Status: status,
		Provider:          pgtype.Text{String: res.Provider, Valid: res.Provider != ""},
		ProviderReference: pgtype.Text{String: res.ProviderReference, Valid: res.ProviderReference != ""},
	})
	if err != nil {
		return err
	}
	_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
		NotificationID: id, Event: status, Metadata: []byte("{}"),
	})

	// Fan-out realtime mirror for inapp deliveries.
	if row.Channel == model.ChannelInapp {
		if rp, ok := s.providers[model.ChannelRealtime]; ok && userUUID != nil {
			_, _ = rp.Deliver(ctx, row, userUUID)
		}
	}
	// Push fan-out: browser Web Push (VAPID) + mobile devices (Expo). In-app
	// rows are mirrored; explicit push-channel rows use the same path.
	if row.Channel == model.ChannelInapp || row.Channel == model.ChannelPush {
		s.scheduleMobilePush(ctx, row)
		if row.UserID.Valid {
			data := map[string]any{
				"notification_uuid": row.Uuid.String(),
			}
			if row.ActionUrl.Valid && row.ActionUrl.String != "" {
				data["action_url"] = row.ActionUrl.String
			}
			s.pushBestEffort(ctx, row.UserID.Int64, row.Title, row.Body, data)
		}
	}
	return nil
}

// ReclaimStuck is the delivery sweep (worker start and every few
// minutes, TEC-962):
//
//   - rows left in processing for staleMinutes (worker crash, failed status
//     write, a restore of a backup taken mid-delivery) are taken over, one
//     sweep per row, and delivered again (at least once);
//   - rows still queued after DefaultStuckQueuedMinutes (or staleMinutes
//     when larger) lost their task (a restore without the Redis snapshot):
//     they are enqueued again under their task id, so a task that still
//     exists makes the enqueue a no-op (inline delivery without a queue).
func (s *Service) ReclaimStuck(ctx context.Context, staleMinutes int32) (int, error) {
	if staleMinutes < 0 {
		staleMinutes = 0
	}
	ids, err := s.q.ListStuckProcessingNotificationIDs(ctx, staleMinutes)
	if err != nil {
		return 0, fmt.Errorf("list stuck notifications: %w", err)
	}
	var n int
	for _, id := range ids {
		row, err := s.q.ReclaimProcessingNotification(ctx, db.ReclaimProcessingNotificationParams{ID: id, StaleMinutes: staleMinutes})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // finished meanwhile, or another sweep took it
		}
		if err == nil {
			_, _ = s.q.InsertNotificationHistory(ctx, db.InsertNotificationHistoryParams{
				NotificationID: row.ID, Event: "reclaimed", Metadata: []byte("{}"),
			})
			err = s.deliverRow(ctx, row)
		}
		if err != nil {
			s.log.Error("notification_reclaim_failed", "id", id, "error", err)
			continue
		}
		n++
	}
	queuedMinutes := DefaultStuckQueuedMinutes
	if staleMinutes > queuedMinutes {
		queuedMinutes = staleMinutes
	}
	queued, err := s.q.ListStaleQueuedNotifications(ctx, queuedMinutes)
	if err != nil {
		return n, fmt.Errorf("list queued notifications: %w", err)
	}
	for _, r := range queued {
		if s.redeliver != nil {
			err = enqueueDeliver(s.redeliver, r.ID, r.Uuid)
		} else if s.queue != nil && !s.syncMode {
			err = enqueueDeliver(s.queue, r.ID, r.Uuid)
		} else {
			err = s.Deliver(ctx, r.ID)
		}
		if err != nil {
			s.log.Error("notification_requeue_failed", "id", r.ID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// ListInbox returns the caller's notifications page. organizationID is the
// shell the inbox is opened in: nil (platform / account shell) lists only
// un-stamped notifications, an organization lists its own notifications plus
// the un-stamped ones. Another organization's rows never appear.
func (s *Service) ListInbox(ctx context.Context, userID int64, organizationID *int64, q apiquery.Query, status, channel string, unread *bool) (apiquery.Page[model.Notification], error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.NotificationsSort); err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	params := db.ListNotificationsForUserParams{
		UserID:         pgtype.Int8{Int64: userID, Valid: true},
		OrganizationID: int8Ptr(organizationID),
		Status:         optionalText(status),
		Channel:        optionalText(channel),
		Unread:         optionalBool(unread),
		Q:              optionalText(q.Q),
		LimitCount:     q.Limit,
		OffsetCount:    q.Offset,
	}
	rows, err := s.q.ListNotificationsForUser(ctx, params)
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	total, err := s.q.CountNotificationsForUser(ctx, db.CountNotificationsForUserParams{
		UserID: params.UserID, OrganizationID: params.OrganizationID, Status: params.Status, Channel: params.Channel,
		Unread: params.Unread, Q: params.Q,
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	items := make([]model.Notification, 0, len(rows))
	for _, r := range rows {
		items = append(items, s.project(r))
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// UnreadCount returns the unread in-app count of the inbox scope (see
// ListInbox for organizationID).
func (s *Service) UnreadCount(ctx context.Context, userID int64, organizationID *int64) (int64, error) {
	return s.q.CountUnreadInappForUser(ctx, db.CountUnreadInappForUserParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, OrganizationID: int8Ptr(organizationID),
	})
}

// GetOwned returns a notification owned by userID.
func (s *Service) GetOwned(ctx context.Context, userID int64, id uuid.UUID) (model.Notification, error) {
	row, err := s.q.GetNotificationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, ErrNotFound
		}
		return model.Notification{}, err
	}
	if !row.UserID.Valid || row.UserID.Int64 != userID {
		return model.Notification{}, ErrForbidden
	}
	return s.project(row), nil
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, userID int64, id uuid.UUID) (model.Notification, error) {
	row, err := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		Uuid: id, UserID: pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, ErrNotFound
		}
		return model.Notification{}, err
	}
	return s.project(row), nil
}

// RedeemAction verifies a signed action link, marks the in-app row read, and
// returns the stored destination.
func (s *Service) RedeemAction(ctx context.Context, userID int64, id uuid.UUID, exp int64, sig string) (model.Notification, string, error) {
	row, err := s.q.GetNotificationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Notification{}, "", ErrNotFound
		}
		return model.Notification{}, "", err
	}
	if !row.UserID.Valid || row.UserID.Int64 != userID {
		return model.Notification{}, "", ErrForbidden
	}
	if !row.ActionUrl.Valid || strings.TrimSpace(row.ActionUrl.String) == "" {
		return model.Notification{}, "", fmt.Errorf("%w: action url missing", ErrInvalidRequest)
	}
	dest := row.ActionUrl.String
	if !actionlink.Verify(s.actionSecret, id, dest, sig, exp) {
		return model.Notification{}, "", fmt.Errorf("%w: invalid or expired action link", ErrInvalidRequest)
	}
	if row.Channel == model.ChannelInapp && !row.ReadAt.Valid {
		marked, markErr := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
			Uuid: id, UserID: pgtype.Int8{Int64: userID, Valid: true},
		})
		if markErr == nil {
			row = marked
		}
	}
	return s.project(row), dest, nil
}

// MarkAllRead marks every unread in-app notification of the inbox scope read
// (see ListInbox for organizationID); other organizations' rows stay unread.
func (s *Service) MarkAllRead(ctx context.Context, userID int64, organizationID *int64) error {
	_, err := s.q.MarkAllNotificationsReadForUser(ctx, db.MarkAllNotificationsReadForUserParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, OrganizationID: int8Ptr(organizationID),
	})
	return err
}

// PayloadOrganizationUUID is the payload key that carries the organization of
// a stamped notification to clients (inbox rows and realtime pushes), so the
// tenant shell can tell its own live notifications from another
// organization's.
const PayloadOrganizationUUID = "organization_uuid"

// stampOrganization copies payload and adds the organization uuid of a
// tenant notification. A nil organizationID returns payload unchanged.
func (s *Service) stampOrganization(ctx context.Context, organizationID *int64, payload map[string]any) (map[string]any, error) {
	if organizationID == nil {
		return payload, nil
	}
	org, err := s.q.GetOrganizationByID(ctx, *organizationID)
	if err != nil {
		return nil, fmt.Errorf("notification organization: %w", err)
	}
	out := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		out[k] = v
	}
	out[PayloadOrganizationUUID] = org.Uuid.String()
	return out, nil
}

// GetPreferences returns preferences with defaults.
func (s *Service) GetPreferences(ctx context.Context, userID int64) (model.Preferences, error) {
	row, err := s.q.GetNotificationPreferences(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Preferences{EmailEnabled: true, InappEnabled: true, RealtimeEnabled: true, PushEnabled: false}, nil
	}
	if err != nil {
		return model.Preferences{}, err
	}
	return model.Preferences{
		EmailEnabled: row.EmailEnabled, InappEnabled: row.InappEnabled, RealtimeEnabled: row.RealtimeEnabled,
		PushEnabled: row.PushEnabled,
	}, nil
}

// UpsertPreferences stores preferences.
func (s *Service) UpsertPreferences(ctx context.Context, userID int64, p model.Preferences) (model.Preferences, error) {
	row, err := s.q.UpsertNotificationPreferences(ctx, db.UpsertNotificationPreferencesParams{
		UserID: userID, EmailEnabled: p.EmailEnabled, InappEnabled: p.InappEnabled, RealtimeEnabled: p.RealtimeEnabled,
		PushEnabled: p.PushEnabled,
	})
	if err != nil {
		return model.Preferences{}, err
	}
	return model.Preferences{
		EmailEnabled: row.EmailEnabled, InappEnabled: row.InappEnabled, RealtimeEnabled: row.RealtimeEnabled,
		PushEnabled: row.PushEnabled,
	}, nil
}

// ListPlatform returns notifications for platform operators.
// Without platform.notifications.read_all the page is always the caller's own rows.
func (s *Service) ListPlatform(
	ctx context.Context,
	actor authctx.Principal,
	q apiquery.Query,
	status, channel, scope, userUUID string,
) (apiquery.Page[model.Notification], error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.NotificationsSort); err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	audience, err := resolvePlatformAudience(actor, scope, userUUID, func(id uuid.UUID) (int64, error) {
		return s.ResolveUserID(ctx, id)
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	params := db.ListPlatformNotificationsParams{
		Status: optionalText(status), Channel: optionalText(channel), Q: optionalText(q.Q),
		UserID: audience.UserID, LimitCount: q.Limit, OffsetCount: q.Offset,
	}
	rows, err := s.q.ListPlatformNotifications(ctx, params)
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	total, err := s.q.CountPlatformNotifications(ctx, db.CountPlatformNotificationsParams{
		Status: params.Status, Channel: params.Channel, Q: params.Q, UserID: params.UserID,
	})
	if err != nil {
		return apiquery.Page[model.Notification]{}, err
	}
	items := make([]model.Notification, 0, len(rows))
	users := map[int64]model.NotificationUser{}
	for _, r := range rows {
		n := s.project(r)
		if audience.IncludeUser && r.UserID.Valid {
			if u, ok := users[r.UserID.Int64]; ok {
				copied := u
				n.User = &copied
			} else if row, err := s.q.GetUserByID(ctx, r.UserID.Int64); err == nil {
				u := model.NotificationUser{
					UUID: row.Uuid, Name: row.Name, Surname: row.Surname, Email: row.Email,
				}
				users[r.UserID.Int64] = u
				n.User = &u
			}
		}
		items = append(items, n)
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// ResolveUserID resolves public uuid to internal id.
func (s *Service) ResolveUserID(ctx context.Context, id uuid.UUID) (int64, error) {
	u, err := s.q.GetUserByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return u.ID, nil
}

func channelAllowed(ch string, prefs model.Preferences, securityEmail bool) bool {
	switch ch {
	case model.ChannelEmail:
		return prefs.EmailEnabled || securityEmail
	case model.ChannelInapp:
		return prefs.InappEnabled
	case model.ChannelRealtime:
		return prefs.RealtimeEnabled
	default:
		return true
	}
}

func (s *Service) project(row db.Notification) model.Notification {
	n := model.Notification{
		UUID: row.Uuid, Channel: row.Channel, Status: row.Status, Priority: row.Priority,
		Title: row.Title, Body: row.Body, Payload: map[string]any{},
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	_ = json.Unmarshal(row.Payload, &n.Payload)
	if row.OrganizationID.Valid {
		if raw, ok := n.Payload[PayloadOrganizationUUID].(string); ok {
			if id, err := uuid.Parse(raw); err == nil {
				n.OrganizationUUID = &id
			}
		}
	}
	if row.ActionUrl.Valid {
		dest := row.ActionUrl.String
		n.ActionURL = &dest
		if signed := actionlink.URL(s.actionSecret, row.Uuid, dest, s.actionTTL); signed != "" {
			n.SignedActionURL = &signed
		}
	}
	if row.Recipient.Valid {
		s := row.Recipient.String
		n.Recipient = &s
	}
	if row.TemplateCode.Valid {
		s := row.TemplateCode.String
		n.TemplateCode = &s
	}
	if row.SourceEvent.Valid {
		s := row.SourceEvent.String
		n.SourceEvent = &s
	}
	if row.SentAt.Valid {
		t := row.SentAt.Time
		n.SentAt = &t
	}
	if row.ReadAt.Valid {
		t := row.ReadAt.Time
		n.ReadAt = &t
	}
	return n
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func optionalText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// Ensure time import used (CreatedAt mapping).
var _ = time.Time{}
