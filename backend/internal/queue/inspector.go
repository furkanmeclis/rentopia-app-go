package queue

import (
	"context"
	"errors"
	"sort"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// QueueSnapshot is the state of one Asynq queue (platform health, TEC-744).
// Archived tasks are dead: their retries are exhausted.
type QueueSnapshot struct {
	Queue     string
	Pending   int
	Active    int
	Scheduled int
	Retry     int
	Archived  int
	Paused    bool
}

// Inspector reads queue sizes from Redis (asynq.Inspector).
type Inspector struct {
	insp *asynq.Inspector
}

// NewInspector reads the queues through an existing Redis client (the
// caller owns and closes it).
func NewInspector(rdb redis.UniversalClient) *Inspector {
	if rdb == nil {
		return nil
	}
	return &Inspector{insp: asynq.NewInspectorFromRedisClient(rdb)}
}

// KnownQueues are the queues the worker serves; they are listed even
// before their first task.
var KnownQueues = []string{
	"default", QueueNotifications, QueueExports, QueueImports, QueueBulk, QueueSearch,
	QueueMaintenance, QueueIntegrations, QueueIntegrationsCritical,
}

// Snapshot returns every known and existing queue, sorted by name.
func (i *Inspector) Snapshot(_ context.Context) ([]QueueSnapshot, error) {
	if i == nil || i.insp == nil {
		return nil, errors.New("queue: inspector unavailable")
	}
	names, err := i.insp.Queues()
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, n := range names {
		exists[n] = true
	}
	all := append([]string{}, names...)
	for _, n := range KnownQueues {
		if !exists[n] {
			all = append(all, n)
		}
	}
	sort.Strings(all)
	out := make([]QueueSnapshot, 0, len(all))
	for _, n := range all {
		if !exists[n] {
			// Never used yet: Redis has no keys for it.
			out = append(out, QueueSnapshot{Queue: n})
			continue
		}
		info, err := i.insp.GetQueueInfo(n)
		if err != nil {
			return nil, err
		}
		out = append(out, QueueSnapshot{
			Queue: n, Pending: info.Pending, Active: info.Active, Scheduled: info.Scheduled,
			Retry: info.Retry, Archived: info.Archived, Paused: info.Paused,
		})
	}
	return out, nil
}
