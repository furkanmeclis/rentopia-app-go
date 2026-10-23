package usecase

import (
	"context"
	"log/slog"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type deliverTestQuerier struct {
	pushTestQuerier
	row      db.Notification
	claimErr error
	sent     int
	stuck    []int64
	// reclaimErr: ReclaimProcessingNotification's answer (nil = the row).
	reclaimErr error
	queued     []db.ListStaleQueuedNotificationsRow
	// createErr / existing: CreateNotification's answer and the row behind
	// an idempotency key.
	createErr error
	existing  db.Notification
	created   []db.CreateNotificationParams
	history   int
}

func (d *deliverTestQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	if d.claimErr != nil {
		return db.Notification{}, d.claimErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	return d.row, nil
}

func (d *deliverTestQuerier) MarkNotificationSent(context.Context, db.MarkNotificationSentParams) (db.Notification, error) {
	d.sent++
	d.row.Status = model.StatusDelivered
	return d.row, nil
}

func (d *deliverTestQuerier) ListStuckProcessingNotificationIDs(context.Context, int32) ([]int64, error) {
	return d.stuck, nil
}

func (d *deliverTestQuerier) ReclaimProcessingNotification(context.Context, db.ReclaimProcessingNotificationParams) (db.Notification, error) {
	if d.reclaimErr != nil {
		return db.Notification{}, d.reclaimErr
	}
	return d.row, nil
}

func (d *deliverTestQuerier) ListStaleQueuedNotifications(context.Context, int32) ([]db.ListStaleQueuedNotificationsRow, error) {
	return d.queued, nil
}

func (d *deliverTestQuerier) CreateNotification(_ context.Context, p db.CreateNotificationParams) (db.Notification, error) {
	d.created = append(d.created, p)
	if d.createErr != nil {
		return db.Notification{}, d.createErr
	}
	return db.Notification{ID: 1, Uuid: uuid.New(), Channel: p.Channel, Status: p.Status}, nil
}

func (d *deliverTestQuerier) GetNotificationByIdempotencyKey(_ context.Context, key pgtype.Text) (db.Notification, error) {
	if d.existing.ID == 0 || !d.existing.IdempotencyKey.Valid || d.existing.IdempotencyKey.String != key.String {
		return db.Notification{}, pgx.ErrNoRows
	}
	return d.existing, nil
}

func (d *deliverTestQuerier) InsertNotificationHistory(context.Context, db.InsertNotificationHistoryParams) (db.NotificationHistory, error) {
	d.history++
	return db.NotificationHistory{}, nil
}

// taskQueue records enqueued task ids; a known id answers like Asynq.
type taskQueue struct {
	ids map[string]bool
	n   int
}

func (q *taskQueue) Enqueue(_ *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	q.n++
	for _, o := range opts {
		if o.Type() != asynq.TaskIDOpt {
			continue
		}
		id, _ := o.Value().(string)
		if q.ids[id] {
			return nil, asynq.ErrTaskIDConflict
		}
		if q.ids == nil {
			q.ids = map[string]bool{}
		}
		q.ids[id] = true
	}
	return &asynq.TaskInfo{}, nil
}

// A second delivery task of a row another delivery holds (processing) does
// nothing: a duplicate task never delivers twice (TEC-962).
func TestDeliverLeavesProcessingRowToItsHolder(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
			Title:   "Hello",
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestDeliverSkipsTerminalStatus(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      7,
			Channel: model.ChannelInapp,
			Status:  model.StatusDelivered,
		},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if err := svc.Deliver(context.Background(), 7); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if q.sent != 0 {
		t.Fatalf("MarkNotificationSent calls = %d, want 0", q.sent)
	}
}

func TestReclaimStuckDeliversProcessingRows(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		claimErr: pgx.ErrNoRows,
		row: db.Notification{
			ID:      3,
			Channel: model.ChannelInapp,
			Status:  model.StatusProcessing,
		},
		stuck: []int64{3},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if n != 1 {
		t.Fatalf("reclaimed = %d, want 1", n)
	}
	if q.sent != 1 {
		t.Fatalf("MarkNotificationSent calls = %d, want 1", q.sent)
	}
}

// A processing row another sweep took over (or that finished meanwhile) is
// not delivered by this sweep.
func TestReclaimStuckSkipsRowTakenByAnotherSweep(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		row:        db.Notification{ID: 3, Channel: model.ChannelInapp, Status: model.StatusProcessing},
		stuck:      []int64{3},
		reclaimErr: pgx.ErrNoRows,
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	n, err := svc.ReclaimStuck(context.Background(), 0)
	if err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if n != 0 || q.sent != 0 {
		t.Fatalf("reclaimed = %d, sent = %d, want 0, 0", n, q.sent)
	}
}

// Queued rows whose task was lost are enqueued again under their task id;
// a second sweep while the task exists is a no-op at the queue.
func TestReclaimStuckRequeuesQueuedRowsByTaskID(t *testing.T) {
	t.Parallel()

	rowUUID := uuid.New()
	q := &deliverTestQuerier{queued: []db.ListStaleQueuedNotificationsRow{{ID: 9, Uuid: rowUUID}}}
	tq := &taskQueue{}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default()).WithRedeliverQueue(tq)
	for i := 0; i < 2; i++ {
		n, err := svc.ReclaimStuck(context.Background(), DefaultStuckProcessingMinutes)
		if err != nil {
			t.Fatalf("ReclaimStuck: %v", err)
		}
		if n != 1 {
			t.Fatalf("run %d: requeued = %d, want 1", i, n)
		}
	}
	if tq.n != 2 || len(tq.ids) != 1 || !tq.ids[DeliverTaskID(rowUUID)] {
		t.Fatalf("enqueues = %d, task ids = %v; want 2 attempts of one id", tq.n, tq.ids)
	}
	if q.sent != 0 {
		t.Fatalf("delivered inline %d times, want 0 (queue set)", q.sent)
	}
}

// Without a queue the sweep delivers stale queued rows inline.
func TestReclaimStuckDeliversQueuedRowsInlineWithoutQueue(t *testing.T) {
	t.Parallel()

	q := &deliverTestQuerier{
		row:    db.Notification{ID: 9, Channel: model.ChannelInapp, Status: model.StatusProcessing},
		queued: []db.ListStaleQueuedNotificationsRow{{ID: 9, Uuid: uuid.New()}},
	}
	svc := New(q, nil, []providers.Provider{providers.InappProvider{}}, slog.Default())
	if _, err := svc.ReclaimStuck(context.Background(), 0); err != nil {
		t.Fatalf("ReclaimStuck: %v", err)
	}
	if q.sent != 1 {
		t.Fatalf("delivered = %d, want 1", q.sent)
	}
}

// A replayed cause (same idempotency key) gets the existing row back and
// schedules nothing.
func TestEnqueueReturnsExistingRowForReplayedKey(t *testing.T) {
	t.Parallel()

	base := model.EventKey(uuid.New(), "conversations.assigned", 5)
	key := model.RowKey(base, model.ChannelInapp)
	existing := db.Notification{
		ID: 4, Uuid: uuid.New(), Channel: model.ChannelInapp, Status: model.StatusDelivered,
		IdempotencyKey: pgtype.Text{String: key, Valid: true},
	}
	q := &deliverTestQuerier{createErr: pgx.ErrNoRows, existing: existing}
	q.prefs.InappEnabled = true
	tq := &taskQueue{}
	svc := New(q, tq, []providers.Provider{providers.InappProvider{}}, slog.Default())
	user := int64(5)
	out, err := svc.Enqueue(context.Background(), model.EnqueueInput{
		UserID: &user, Channels: []string{model.ChannelInapp}, Title: "x", IdempotencyKey: base,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(out) != 1 || out[0].UUID != existing.Uuid {
		t.Fatalf("enqueued = %+v, want the existing row", out)
	}
	if len(q.created) != 1 || q.created[0].IdempotencyKey.String != key {
		t.Fatalf("create params = %+v, want key %q", q.created, key)
	}
	if tq.n != 0 || q.history != 0 {
		t.Fatalf("enqueues = %d, history rows = %d; want 0, 0", tq.n, q.history)
	}
}
