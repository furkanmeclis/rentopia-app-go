package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskPlatformHealthAlerts is the failed-work alert sweep of the platform
// health dashboard (TEC-739): new critical issues of every health source
// are reported to the platform administrators once.
const TaskPlatformHealthAlerts = "app:platform-health:alerts"

// platformHealthAlertsCron runs every five minutes.
const platformHealthAlertsCron = "@every 5m"

// PlatformHealthAlertsFunc runs the sweep and returns the number of new
// critical issues it reported.
type PlatformHealthAlertsFunc func(ctx context.Context) (int, error)

// NewPlatformHealthAlertsTask builds the sweep task.
func NewPlatformHealthAlertsTask() *asynq.Task {
	return asynq.NewTask(TaskPlatformHealthAlerts, []byte("{}"))
}

// WithPlatformHealthAlerts registers the sweep handler.
func (w *Worker) WithPlatformHealthAlerts(fn PlatformHealthAlertsFunc) *Worker {
	w.mux.HandleFunc(TaskPlatformHealthAlerts, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("platform_health_alerts_sent", "issues", n)
		}
		return err
	})
	return w
}

// RegisterPlatformHealthAlertsSchedule adds the sweep.
func RegisterPlatformHealthAlertsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		platformHealthAlertsCron, NewPlatformHealthAlertsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(2*time.Minute), asynq.Unique(4*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register platform health alerts: %w", err)
	}
	return nil
}
