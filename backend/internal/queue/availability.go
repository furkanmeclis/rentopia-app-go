package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskExpireHolds is the availability hold expiry sweep (TEC-697).
const TaskExpireHolds = "app:availability:expire-holds"

// expireHoldsCron runs every minute. Expired holds stop counting at
// expires_at anyway (the engine ignores them and expires them before it
// writes); the sweep turns them into "expired" rows exactly once and
// re-plans what depended on them.
const expireHoldsCron = "@every 1m"

// ExpireHoldsFunc runs the sweep and returns the number of holds expired.
type ExpireHoldsFunc func(ctx context.Context) (int, error)

// NewExpireHoldsTask builds the sweep task.
func NewExpireHoldsTask() *asynq.Task {
	return asynq.NewTask(TaskExpireHolds, []byte("{}"))
}

// WithExpireHolds registers the sweep handler.
func (w *Worker) WithExpireHolds(fn ExpireHoldsFunc) *Worker {
	w.mux.HandleFunc(TaskExpireHolds, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("availability_holds_expired", "holds", n)
		}
		return err
	})
	return w
}

// RegisterExpireHoldsSchedule adds the minute sweep. Unique keeps a slow run
// from piling up copies.
func RegisterExpireHoldsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		expireHoldsCron, NewExpireHoldsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(50*time.Second), asynq.Unique(55*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register hold expiry: %w", err)
	}
	return nil
}
