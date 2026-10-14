package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskVehicleReminders is the daily vehicle document / maintenance reminder
// sweep (TEC-693).
const TaskVehicleReminders = "app:vehicles:reminders"

// vehicleRemindersCron runs at 03:05 UTC (06:05 in Türkiye). The job is
// idempotent per organization, subject and day, so a retry or a second
// scheduler sends nothing twice.
const vehicleRemindersCron = "5 3 * * *"

// VehicleRemindersFunc runs the reminder sweep and returns the number of
// notifications it queued.
type VehicleRemindersFunc func(ctx context.Context) (int, error)

// NewVehicleRemindersTask builds the reminder sweep task.
func NewVehicleRemindersTask() *asynq.Task {
	return asynq.NewTask(TaskVehicleReminders, []byte("{}"))
}

// WithVehicleReminders registers the reminder sweep handler.
func (w *Worker) WithVehicleReminders(fn VehicleRemindersFunc) *Worker {
	w.mux.HandleFunc(TaskVehicleReminders, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("vehicle_reminders_sent", "notifications", n)
		}
		return err
	})
	return w
}

// RegisterVehicleRemindersSchedule adds the daily reminder sweep.
func RegisterVehicleRemindersSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		vehicleRemindersCron, NewVehicleRemindersTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute), asynq.Unique(23*time.Hour),
	); err != nil {
		return fmt.Errorf("queue: register vehicle reminders: %w", err)
	}
	return nil
}
