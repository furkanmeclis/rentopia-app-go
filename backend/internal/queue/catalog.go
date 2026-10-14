package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskCatalogImport runs one step (diff or apply) of a vehicle catalog import.
const TaskCatalogImport = "app:catalog:import"

// Catalog import steps.
const (
	CatalogImportDiff  = "diff"
	CatalogImportApply = "apply"
)

// CatalogImportPayload identifies a catalog import row and the step to run.
type CatalogImportPayload struct {
	ImportID int64  `json:"import_id"`
	Action   string `json:"action"`
}

// ProcessCatalogImportFunc runs one catalog import step.
type ProcessCatalogImportFunc func(ctx context.Context, importID int64, action string) error

// NewCatalogImportTask builds a catalog import task.
func NewCatalogImportTask(importID int64, action string) (*asynq.Task, error) {
	body, err := json.Marshal(CatalogImportPayload{ImportID: importID, Action: action})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal catalog import: %w", err)
	}
	return asynq.NewTask(TaskCatalogImport, body), nil
}

// ParseCatalogImportPayload decodes a catalog import payload.
func ParseCatalogImportPayload(data []byte) (CatalogImportPayload, error) {
	var payload CatalogImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return CatalogImportPayload{}, fmt.Errorf("queue: unmarshal catalog import: %w", err)
	}
	return payload, nil
}

// WithCatalogImport registers the vehicle catalog import processor.
func (w *Worker) WithCatalogImport(fn ProcessCatalogImportFunc) *Worker {
	w.mux.HandleFunc(TaskCatalogImport, func(ctx context.Context, task *asynq.Task) error {
		p, err := ParseCatalogImportPayload(task.Payload())
		if err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		if fn == nil {
			w.log.Warn("catalog_import_handler_missing", "id", p.ImportID)
			return nil
		}
		return fn(ctx, p.ImportID, p.Action)
	})
	return w
}
