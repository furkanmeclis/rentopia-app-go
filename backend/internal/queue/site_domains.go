package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const TaskSiteDomains = "app:site:domains"

func NewSiteDomainsTask() *asynq.Task { return asynq.NewTask(TaskSiteDomains, []byte("{}")) }
func (c *Client) EnqueueSiteDomains(_ context.Context) error {
	_, err := c.Enqueue(NewSiteDomainsTask(), asynq.Queue(QueueMaintenance), asynq.MaxRetry(3), asynq.Timeout(8*time.Minute))
	return err
}
func (w *Worker) WithSiteDomains(fn func(context.Context) (int, error)) *Worker {
	w.mux.HandleFunc(TaskSiteDomains, func(ctx context.Context, _ *asynq.Task) error { _, err := fn(ctx); return err })
	return w
}
func RegisterSiteDomainsSchedule(s *asynq.Scheduler) error {
	_, err := s.Register("@every 1m", NewSiteDomainsTask(), asynq.Queue(QueueMaintenance),
		asynq.MaxRetry(1), asynq.Timeout(8*time.Minute), asynq.Unique(55*time.Second))
	if err != nil {
		return fmt.Errorf("queue: register site domains: %w", err)
	}
	return nil
}
