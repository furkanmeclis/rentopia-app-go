package notifyengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	engine "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/msgreconcile"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TEC-962: outbound messaging idempotency after a restore (docs/operations/
// backup-restore.md RT-A32 (c)). Runs with TEST_DATABASE_URL.

// pending creates a dispatch on the given channels without sending it.
func (e *env) pending(t *testing.T, channels []string, at time.Time) db.NotificationDispatch {
	t.Helper()
	ref := uuid.New()
	out, err := e.eng.EmitTx(e.ctx, e.q, e.org, engine.EmitInput{
		Event: catalog.ReservationConfirmed, CustomerID: e.customer, RefType: "reservation", RefUUID: &ref,
		Key: "status:confirmed", Channels: channels, ScheduledAt: at,
		Vars: map[string]string{"reference": "R-9", "pickup_at": "18.10.2026 10:00", "pickup_branch": "Merkez"},
	})
	if err != nil || !out.Created {
		t.Fatalf("emit: %+v %v", out, err)
	}
	return out.Dispatch
}

// interrupt leaves a dispatch the way a crash (or a backup taken
// mid-send) does: claimed for its first channel, nothing recorded.
func (e *env) interrupt(t *testing.T, d db.NotificationDispatch) uuid.UUID {
	t.Helper()
	if _, err := e.pool.Exec(e.ctx, `UPDATE notification_dispatches SET status = 'sending' WHERE organization_id = $1 AND id = $2`,
		e.org, d.ID); err != nil {
		t.Fatal(err)
	}
	var dl uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `INSERT INTO notification_deliveries (organization_id, dispatch_id, channel, attempt_no, status, subject, body)
		VALUES ($1, $2, $3, 1, 'sending', 'Rezervasyon R-9', 'Rezervasyonunuz onaylandı: R-9') RETURNING uuid`,
		e.org, d.ID, d.Channels[0]).Scan(&dl); err != nil {
		t.Fatal(err)
	}
	return dl
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) status(t *testing.T, d db.NotificationDispatch) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(e.ctx, `SELECT status FROM notification_dispatches WHERE organization_id = $1 AND id = $2`,
		e.org, d.ID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Later moves the engine's clock past the reclaim threshold.
func (e *env) later() { e.eng.SetClock(func() time.Time { return time.Now().Add(time.Hour) }) }

// The same event twice (an outbox replay, a retried transition) creates
// one dispatch; the same delivery sent twice through the SMS module
// creates one message and reaches the provider once.
func TestRestoreSameEventTwiceSendsOnce(t *testing.T) {
	e := newEnv(t, true)
	first := e.emit(t, catalog.ReservationConfirmed, "status:confirmed")
	again := e.emit(t, catalog.ReservationConfirmed, "status:confirmed")
	if !first.Created || again.Created || again.Dispatch.ID != first.Dispatch.ID {
		t.Fatalf("emits: first created %v, again created %v (%d vs %d)", first.Created, again.Created, again.Dispatch.ID, first.Dispatch.ID)
	}
	if n := e.count(t, `SELECT count(*) FROM notification_dispatches WHERE organization_id = $1`, e.org); n != 1 {
		t.Fatalf("dispatches = %d, want 1", n)
	}
	var delivery, smsUUID uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT uuid, external_uuid FROM notification_deliveries WHERE organization_id = $1 AND channel = 'sms'`,
		e.org).Scan(&delivery, &smsUUID); err != nil {
		t.Fatal(err)
	}
	// A replay of the attempt (same delivery) hands nothing new to Netgsm.
	sent, err := e.sms.Send(e.ctx, e.org, smsusecase.SendInput{
		Class: sms.ClassTransactional, Purpose: "notify." + catalog.ReservationConfirmed, Phone: "+905321112233",
		Body: "Rezervasyonunuz onaylandı", RefType: adapters.RefTypeDelivery, RefUUID: &delivery,
		IdempotencyKey: adapters.DeliveryKey(delivery),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent.MessageUUID != smsUUID {
		t.Fatalf("replayed send returned message %s, want the first one %s", sent.MessageUUID, smsUUID)
	}
	if n := e.count(t, `SELECT count(*) FROM sms_messages WHERE organization_id = $1`, e.org); n != 1 {
		t.Fatalf("sms messages = %d, want 1", n)
	}
	if n := len(e.fake.Messages()); n != 1 {
		t.Fatalf("Netgsm received %d messages, want 1", n)
	}
}

// An e-mail attempt interrupted before anything was recorded goes back to
// pending and resumes on the same attempt row (one delivery row, one mail).
func TestRestoreReclaimResumesInterruptedAttempt(t *testing.T) {
	e := newEnv(t, true)
	d := e.pending(t, []string{"email"}, time.Now().Add(30*time.Minute))
	e.interrupt(t, d)
	if n, err := e.eng.ReclaimStuckOrganization(e.ctx, e.org, engine.DefaultStuckSending); err != nil || n != 0 {
		t.Fatalf("fresh sending dispatch reclaimed: %d %v", n, err)
	}
	e.later()
	if n, err := e.eng.ReclaimStuckOrganization(e.ctx, e.org, engine.DefaultStuckSending); err != nil || n != 1 {
		t.Fatalf("reclaimed = %d, %v; want 1", n, err)
	}
	if s := e.status(t, d); s != engine.DispatchPending {
		t.Fatalf("after reclaim: %s, want pending", s)
	}
	if _, err := e.eng.ProcessDueOrganization(e.ctx, e.org); err != nil {
		t.Fatal(err)
	}
	if s := e.status(t, d); s != engine.DispatchDelivered || e.mail.count() != 1 {
		t.Fatalf("after resume: %s, mails %d; want delivered, 1", s, e.mail.count())
	}
	if n := e.count(t, `SELECT count(*) FROM notification_deliveries WHERE organization_id = $1 AND dispatch_id = $2`, e.org, d.ID); n != 1 {
		t.Fatalf("delivery rows = %d, want 1 (the interrupted row resumed)", n)
	}
	if n, _ := e.eng.ReclaimStuckOrganization(e.ctx, e.org, engine.DefaultStuckSending); n != 0 || e.mail.count() != 1 {
		t.Fatalf("second reclaim: %d, mails %d", n, e.mail.count())
	}
}

// An SMS attempt interrupted after the SMS module took the message (the
// send ledger holds it) is recorded as sent, never sent again; the
// delivery report then settles the dispatch.
func TestRestoreReclaimUsesSMSLedger(t *testing.T) {
	e := newEnv(t, true)
	d := e.pending(t, []string{"sms", "email"}, time.Now().Add(30*time.Minute))
	delivery := e.interrupt(t, d)
	sent, err := e.sms.Send(e.ctx, e.org, smsusecase.SendInput{
		Class: sms.ClassTransactional, Purpose: "notify." + catalog.ReservationConfirmed, Phone: "+905321112233",
		Body: "Rezervasyonunuz onaylandı: R-9", RefType: adapters.RefTypeDelivery, RefUUID: &delivery,
		IdempotencyKey: adapters.DeliveryKey(delivery),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(e.fake.Messages()); n != 1 {
		t.Fatalf("Netgsm received %d messages, want 1", n)
	}
	e.later()
	if n, err := e.eng.ReclaimStuckOrganization(e.ctx, e.org, engine.DefaultStuckSending); err != nil || n != 1 {
		t.Fatalf("reclaimed = %d, %v; want 1", n, err)
	}
	var status string
	var ext uuid.UUID
	if err := e.pool.QueryRow(e.ctx, `SELECT status, external_uuid FROM notification_deliveries WHERE organization_id = $1 AND uuid = $2`,
		e.org, delivery).Scan(&status, &ext); err != nil {
		t.Fatal(err)
	}
	if status != engine.DeliverySent || ext != sent.MessageUUID {
		t.Fatalf("attempt after reclaim: %s %s; want sent with the SMS %s", status, ext, sent.MessageUUID)
	}
	if _, err := e.eng.ProcessDueOrganization(e.ctx, e.org); err != nil {
		t.Fatal(err)
	}
	if n := len(e.fake.Messages()); n != 1 || e.mail.count() != 0 {
		t.Fatalf("after reclaim: %d SMS, %d mails; want 1, 0", n, e.mail.count())
	}
	// The report still arrives through the send job.
	e.fake.SetStatus(e.fake.Messages()[0].JobID, 1)
	if _, err := e.runner.Process(e.ctx, e.org, e.jobOf(t, sent.MessageUUID.String()), jobsusecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if s := e.status(t, d); s != engine.DispatchDelivered {
		t.Fatalf("after report: %s, want delivered", s)
	}
}

// queueless keeps integration jobs pending (no inline run).
type queueless struct{}

func (queueless) DispatchIntegrationJob(int64, int64, bool, time.Duration) error { return nil }

// taskSink keeps notification delivery tasks (rows stay queued).
type taskSink struct{ n int }

func (s *taskSink) Enqueue(*asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	s.n++
	return &asynq.TaskInfo{}, nil
}

// Messages still waiting in restored data that the operator marks
// (cmd/messaging-reconcile) are never sent; messages created after the
// backup time are left alone.
func TestRestoreReconcilePendingBeforeCutoff(t *testing.T) {
	e := newEnv(t, true)
	e.runner.SetDispatcher(queueless{})
	user, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		Email: "rec-" + uuid.NewString()[:8] + "@example.test", PasswordHash: "x", Name: "Rec", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = e.pool.Exec(bg, "DELETE FROM notifications WHERE user_id = $1", user.ID)
		_, _ = e.pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
	})
	sink := &taskSink{}
	notif := notifusecase.New(e.q, sink, []providers.Provider{providers.InappProvider{}}, nil)

	// Waiting at backup time: a due e-mail dispatch, a queued SMS, a queued
	// e-mail notification.
	d := e.pending(t, []string{"email"}, time.Now().Add(-time.Minute))
	queued, err := e.sms.Send(e.ctx, e.org, smsusecase.SendInput{
		Class: sms.ClassTransactional, Purpose: "contract_link", Phone: "+905321112233", Body: "Sözleşmeniz hazır",
	})
	if err != nil || queued.Status != smsusecase.StatusQueued {
		t.Fatalf("queued sms: %+v %v", queued, err)
	}
	org, uid, mailTo := e.org, user.ID, "rec@example.test"
	rows, err := notif.Enqueue(e.ctx, notifmodel.EnqueueInput{
		OrganizationID: &org, UserID: &uid, Channels: []string{notifmodel.ChannelEmail}, Title: "Rapor", Body: "Hazır",
		Recipient: &mailTo, SecurityEmail: true,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("notification: %+v %v", rows, err)
	}
	time.Sleep(10 * time.Millisecond)
	backupAt := time.Now()
	time.Sleep(10 * time.Millisecond)
	// Created after the backup: not a candidate.
	later := e.pending(t, []string{"email"}, time.Now().Add(-time.Minute))

	r := msgreconcile.New(e.pool, e.q)
	f := msgreconcile.Filter{Before: backupAt, OrganizationID: &e.org}
	list, err := r.List(e.ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[msgreconcile.Kind][]uuid.UUID{}
	for _, c := range list {
		got[c.Kind] = append(got[c.Kind], c.UUID)
	}
	if len(got[msgreconcile.KindDispatch]) != 1 || got[msgreconcile.KindDispatch][0] != d.Uuid ||
		len(got[msgreconcile.KindSMS]) != 1 || got[msgreconcile.KindSMS][0] != queued.MessageUUID ||
		len(got[msgreconcile.KindNotification]) != 1 || got[msgreconcile.KindNotification][0] != rows[0].UUID {
		t.Fatalf("candidates = %v", got)
	}
	res, err := r.Mark(e.ctx, f, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dispatches != 1 || res.SMS != 1 || res.Notifications != 1 {
		t.Fatalf("marked = %+v", res)
	}

	// The worker starts on the restored data.
	if _, err := e.eng.ProcessDueOrganization(e.ctx, e.org); err != nil {
		t.Fatal(err)
	}
	if s := e.status(t, d); s != engine.DispatchReconciled {
		t.Fatalf("reconciled dispatch: %s", s)
	}
	if s := e.status(t, later); s != engine.DispatchDelivered || e.mail.count() != 1 {
		t.Fatalf("dispatch after the backup: %s, mails %d; want delivered, 1", s, e.mail.count())
	}
	if _, err := e.runner.Process(e.ctx, e.org, e.jobOf(t, queued.MessageUUID.String()), jobsusecase.TriggerSweep); err != nil {
		t.Fatal(err)
	}
	if n := len(e.fake.Messages()); n != 0 {
		t.Fatalf("Netgsm received %d messages, want 0 (reconciled)", n)
	}
	var smsStatus, jobStatus string
	if err := e.pool.QueryRow(e.ctx, `SELECT m.status, j.status FROM sms_messages m
		JOIN integration_jobs j ON j.organization_id = m.organization_id AND j.id = m.integration_job_id
		WHERE m.organization_id = $1 AND m.uuid = $2`, e.org, queued.MessageUUID).Scan(&smsStatus, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if smsStatus != smsusecase.StatusReconciled || jobStatus != jobsusecase.StatusSucceeded {
		t.Fatalf("sms %s, job %s; want reconciled, succeeded", smsStatus, jobStatus)
	}
	var id int64
	var nStatus string
	if err := e.pool.QueryRow(e.ctx, `SELECT id, status FROM notifications WHERE organization_id = $1 AND uuid = $2`, e.org, rows[0].UUID).
		Scan(&id, &nStatus); err != nil {
		t.Fatal(err)
	}
	if err := notif.Deliver(e.ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(e.ctx, `SELECT status FROM notifications WHERE organization_id = $1 AND id = $2`, e.org, id).Scan(&nStatus); err != nil {
		t.Fatal(err)
	}
	if nStatus != notifmodel.StatusCancelled {
		t.Fatalf("reconciled notification: %s, want cancelled (not delivered)", nStatus)
	}
}
