package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskNotificationSweep is the notification delivery sweep (TEC-962): rows
// left in processing are delivered again and queued rows whose delivery
// task was lost (a restore without the Redis snapshot) are enqueued again
// under their task id (notifications ReclaimStuck).
const TaskNotificationSweep = "app:notification:sweep"

// notificationSweepCron: queued rows wait DefaultStuckQueuedMinutes (5)
// before the sweep touches them, so a short interval only shortens the
// delay after a restore.
const notificationSweepCron = "@every 2m"

// NotificationSweepFunc runs the sweep and returns the rows handed to
// delivery.
type NotificationSweepFunc func(ctx context.Context) (int, error)

// NewNotificationSweepTask builds the sweep task.
func NewNotificationSweepTask() *asynq.Task {
	return asynq.NewTask(TaskNotificationSweep, []byte("{}"))
}

// WithNotificationSweep registers the sweep handler.
func (w *Worker) WithNotificationSweep(fn NotificationSweepFunc) *Worker {
	w.mux.HandleFunc(TaskNotificationSweep, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("notification_sweep_requeued", "count", n)
		}
		return err
	})
	return w
}

// RegisterNotificationSweepSchedule adds the periodic sweep.
func RegisterNotificationSweepSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		notificationSweepCron, NewNotificationSweepTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute), asynq.Unique(110*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register notification sweep: %w", err)
	}
	return nil
}
