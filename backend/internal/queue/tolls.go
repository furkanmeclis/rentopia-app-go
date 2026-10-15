package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskTollsFetch is the hourly toll pass cycle (TEC-725): one provider
// fetch job per active toll account (integration runner; failures show in
// the operations overview), then the re-match of review passes.
const TaskTollsFetch = "app:tolls:fetch"

// tollsFetchCron runs at minute 17 (off the hour). The jobs are keyed by
// account and hour, so a second run in the same hour creates nothing.
const tollsFetchCron = "17 * * * *"

// TollsCycleFunc schedules the fetches and returns the number queued.
type TollsCycleFunc func(ctx context.Context) (int, error)

// WithTolls registers the toll cycle handler: schedule the fetches, then
// re-match the review queue.
func (w *Worker) WithTolls(schedule, rematch TollsCycleFunc) *Worker {
	w.mux.HandleFunc(TaskTollsFetch, func(ctx context.Context, _ *asynq.Task) error {
		if schedule != nil {
			n, err := schedule(ctx)
			if err != nil {
				return err
			}
			if n > 0 {
				w.log.Info("toll_fetches_scheduled", "jobs", n)
			}
		}
		if rematch != nil {
			n, err := rematch(ctx)
			if n > 0 {
				w.log.Info("toll_passes_rematched", "passes", n)
			}
			return err
		}
		return nil
	})
	return w
}

// RegisterTollsSchedule adds the hourly toll cycle.
func RegisterTollsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		tollsFetchCron, asynq.NewTask(TaskTollsFetch, []byte("{}")),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(2),
		asynq.Timeout(15*time.Minute), asynq.Unique(50*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register toll fetch: %w", err)
	}
	return nil
}
