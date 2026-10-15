package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskDocumentPDF renders the PDF of one document version (TEC-710,
// headless Chromium in the worker).
const TaskDocumentPDF = "app:documents:render-pdf"

// DocumentPDFPayload names the version (the worker re-scopes from it).
type DocumentPDFPayload struct {
	OrganizationID int64 `json:"organization_id"`
	VersionID      int64 `json:"version_id"`
}

// NewDocumentPDFTask builds a render task.
func NewDocumentPDFTask(orgID, versionID int64) (*asynq.Task, error) {
	body, err := json.Marshal(DocumentPDFPayload{OrganizationID: orgID, VersionID: versionID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal document pdf payload: %w", err)
	}
	return asynq.NewTask(TaskDocumentPDF, body), nil
}

// EnqueueDocumentPDF queues the render of a version (exports queue: the
// same file-producing class of work). Unique per version for a minute.
func (c *Client) EnqueueDocumentPDF(orgID, versionID int64) error {
	task, err := NewDocumentPDFTask(orgID, versionID)
	if err != nil {
		return err
	}
	_, err = c.Enqueue(task, asynq.Queue(QueueExports), asynq.MaxRetry(3), asynq.Timeout(2*time.Minute),
		asynq.Unique(time.Minute))
	return err
}

// DocumentPDFFunc renders one version.
type DocumentPDFFunc func(ctx context.Context, orgID, versionID int64) error

// WithDocumentPDF registers the render handler.
func (w *Worker) WithDocumentPDF(fn DocumentPDFFunc) *Worker {
	w.mux.HandleFunc(TaskDocumentPDF, func(ctx context.Context, task *asynq.Task) error {
		if fn == nil {
			return nil
		}
		var p DocumentPDFPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return fn(ctx, p.OrganizationID, p.VersionID)
	})
	return w
}
