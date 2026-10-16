package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskPublicWebhooks is the public booking API sweep (TEC-728): due
// webhook deliveries (first attempts the API could not make, retries with
// backoff) and expired idempotency records.
const TaskPublicWebhooks = "app:public-api:webhooks"

// publicWebhooksCron runs every minute (the shortest retry backoff).
const publicWebhooksCron = "@every 1m"

// PublicWebhooksFunc runs the sweep and returns the number of delivery
// attempts.
type PublicWebhooksFunc func(ctx context.Context) (int, error)

// NewPublicWebhooksTask builds the sweep task.
func NewPublicWebhooksTask() *asynq.Task {
	return asynq.NewTask(TaskPublicWebhooks, []byte("{}"))
}

// WithPublicWebhooks registers the sweep handler.
func (w *Worker) WithPublicWebhooks(fn PublicWebhooksFunc) *Worker {
	w.mux.HandleFunc(TaskPublicWebhooks, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("public_webhooks_attempted", "deliveries", n)
		}
		return err
	})
	return w
}

// RegisterPublicWebhooksSchedule adds the minute sweep. Deliveries are
// claimed with a lease, so an overlapping run never sends one twice.
func RegisterPublicWebhooksSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		publicWebhooksCron, NewPublicWebhooksTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register public webhooks: %w", err)
	}
	return nil
}
