package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskWhatsAppWatch is the WhatsApp health sweep (TEC-735): session state
// of linked organizations (drop alert, reconnect attempt) and messages
// whose delivery receipt never came.
const TaskWhatsAppWatch = "app:whatsapp:watch"

// whatsAppWatchCron: every two minutes; state writes are idempotent.
const whatsAppWatchCron = "@every 2m"

// WhatsAppWatchFunc runs the sweep and returns the sessions checked.
type WhatsAppWatchFunc func(ctx context.Context) (int, error)

// NewWhatsAppWatchTask builds the sweep task.
func NewWhatsAppWatchTask() *asynq.Task {
	return asynq.NewTask(TaskWhatsAppWatch, []byte("{}"))
}

// WithWhatsApp registers the sweep handler.
func (w *Worker) WithWhatsApp(fn WhatsAppWatchFunc) *Worker {
	w.mux.HandleFunc(TaskWhatsAppWatch, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Debug("whatsapp_watch", "sessions", n)
		}
		return err
	})
	return w
}

// RegisterWhatsAppSchedule adds the sweep.
func RegisterWhatsAppSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		whatsAppWatchCron, NewWhatsAppWatchTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute), asynq.Unique(110*time.Second),
	); err != nil {
		return fmt.Errorf("queue: register whatsapp watch: %w", err)
	}
	return nil
}
