package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskBrandLogoSync runs one brand logo sync run (motomarks-mirror → own
// storage + display brand matching).
const TaskBrandLogoSync = "app:catalog:brandlogos"

// BrandLogoSyncPayload identifies the run row.
type BrandLogoSyncPayload struct {
	RunID int64 `json:"run_id"`
}

// ProcessBrandLogoSyncFunc runs one sync run.
type ProcessBrandLogoSyncFunc func(ctx context.Context, runID int64) error

// NewBrandLogoSyncTask builds a brand logo sync task.
func NewBrandLogoSyncTask(runID int64) (*asynq.Task, error) {
	body, err := json.Marshal(BrandLogoSyncPayload{RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal brand logo sync: %w", err)
	}
	return asynq.NewTask(TaskBrandLogoSync, body, asynq.MaxRetry(0)), nil
}

// WithBrandLogoSync registers the brand logo sync processor.
func (w *Worker) WithBrandLogoSync(fn ProcessBrandLogoSyncFunc) *Worker {
	w.mux.HandleFunc(TaskBrandLogoSync, func(ctx context.Context, task *asynq.Task) error {
		var p BrandLogoSyncPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		if fn == nil {
			w.log.Warn("brand_logo_sync_handler_missing", "run_id", p.RunID)
			return nil
		}
		return fn(ctx, p.RunID)
	})
	return w
}
