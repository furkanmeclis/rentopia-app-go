package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

// TaskFailure describes one failed task attempt (error tracker, logs,
// failed-work alerts). IDs holds the numeric *_id fields of the payload
// (organization_id, job_id, export_job_id …): ids only, never payload text.
type TaskFailure struct {
	Type     string
	TaskID   string
	Queue    string
	Retried  int
	MaxRetry int
	// Final is the last attempt (retries exhausted or SkipRetry): the task
	// goes to the archive (dead).
	Final bool
	IDs   map[string]int64
	Err   error
}

// FailureAlertFunc hears final task failures (see Worker.WithFailureAlerts).
type FailureAlertFunc func(ctx context.Context, f TaskFailure)

// describeFailure reads the task metadata asynq keeps on ctx.
func describeFailure(ctx context.Context, task *asynq.Task, err error) TaskFailure {
	f := TaskFailure{Err: err}
	if task != nil {
		f.Type = task.Type()
		f.IDs = payloadIDs(task.Payload())
	}
	f.TaskID, _ = asynq.GetTaskID(ctx)
	f.Retried, _ = asynq.GetRetryCount(ctx)
	f.MaxRetry, _ = asynq.GetMaxRetry(ctx)
	f.Queue, _ = asynq.GetQueueName(ctx)
	f.Final = f.Retried >= f.MaxRetry || errors.Is(err, asynq.SkipRetry)
	return f
}

// payloadIDs returns the top-level numeric fields of a JSON payload whose
// name is "id" or ends in "_id".
func payloadIDs(payload []byte) map[string]int64 {
	if len(payload) == 0 || payload[0] != '{' {
		return nil
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(payload, &raw) != nil {
		return nil
	}
	out := map[string]int64{}
	for k, v := range raw {
		if k != "id" && !strings.HasSuffix(k, "_id") {
			continue
		}
		var n int64
		if json.Unmarshal(v, &n) == nil && n != 0 {
			out[k] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// logAttrs are the structured log attributes of a failure.
func (f TaskFailure) logAttrs() []any {
	attrs := []any{"type", f.Type, "task_id", f.TaskID, "queue", f.Queue, "retry", f.Retried, "max_retry", f.MaxRetry, "final", f.Final}
	for k, v := range f.IDs {
		attrs = append(attrs, k, v)
	}
	return append(attrs, "error", f.Err)
}

// reportTaskFailure sends a failed task attempt to the error tracker, grouped
// per task type. Attempts that will be retried are warnings; the last attempt
// (or a SkipRetry error) is an error. Tags carry the task id and the
// payload's ids (organization, job) so an event leads to the record.
func reportTaskFailure(ctx context.Context, f TaskFailure) {
	if !observability.Enabled() || f.Err == nil {
		return
	}
	level := sentry.LevelWarning
	if f.Final {
		level = sentry.LevelError
	}
	tags := map[string]string{
		"task_type":     f.Type,
		"task_id":       f.TaskID,
		"queue":         f.Queue,
		"retry":         strconv.Itoa(f.Retried),
		"max_retry":     strconv.Itoa(f.MaxRetry),
		"final_attempt": strconv.FormatBool(f.Final),
	}
	for k, v := range f.IDs {
		tags[k] = strconv.FormatInt(v, 10)
	}
	observability.CaptureError(ctx, f.Err, observability.Options{
		Level:       level,
		Transaction: "task " + f.Type,
		Fingerprint: []string{"queue-task", f.Type},
		Tags:        tags,
	})
}
