package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskNotifyEngineSweep is the notification engine sweep (TEC-733):
// reminders and sweep-detected events become dispatches, due dispatches
// are sent (fallback included).
const TaskNotifyEngineSweep = "app:notify-engine:sweep"

// notifyEngineCron runs every minute; dispatch and delivery idempotency
// keys make overlapping runs harmless.
const notifyEngineCron = "@every 1m"

// NotifyEngineFunc runs the sweep and returns the dispatches processed.
type NotifyEngineFunc func(ctx context.Context) (int, error)

// NewNotifyEngineSweepTask builds the sweep task.
func NewNotifyEngineSweepTask() *asynq.Task {
	return asynq.NewTask(TaskNotifyEngineSweep, []byte("{}"))
}

// WithNotifyEngine registers the sweep handler.
func (w *Worker) WithNotifyEngine(fn NotifyEngineFunc) *Worker {
	w.mux.HandleFunc(TaskNotifyEngineSweep, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("notify_engine_processed", "dispatches", n)
		}
		return err
	})
	return w
}

// RegisterNotifyEngineSchedule adds the minute sweep.
func RegisterNotifyEngineSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		notifyEngineCron, NewNotifyEngineSweepTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register notify engine sweep: %w", err)
	}
	return nil
}
