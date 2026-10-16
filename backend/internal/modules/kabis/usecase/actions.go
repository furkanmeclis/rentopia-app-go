package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("kabis: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Retry is the staff "Yeniden dene".
//
//   - blocked / waiting_dependency: nothing was sent. The payload is built
//     again from the current data and the organization's adapter is looked
//     up again; still missing data answers *DataMissingError, still no
//     working adapter *DependencyError (both 409, nothing sent).
//   - pending / failed / needs_verification / submitted: the runner's
//     retry — a status check first; the notification is sent again only
//     when the authority confirms it does not have it
//     (*jobsusecase.VerifyError when the answer is unknown).
func (s *Service) Retry(ctx context.Context, orgID int64, id uuid.UUID) (model.RetryResult, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.RetryResult{}, err
	}
	switch r.JobStatus {
	case jobsusecase.StatusBlocked, jobsusecase.StatusWaitingDependency:
		return s.revalidate(ctx, orgID, r)
	case jobsusecase.StatusPending, jobsusecase.StatusFailed, jobsusecase.StatusNeedsVerification, jobsusecase.StatusSubmitted:
	default:
		return model.RetryResult{}, ErrNotRetryable
	}
	res, err := s.runner.Retry(ctx, orgID, r.JobUuid)
	switch {
	case errors.Is(err, jobsusecase.ErrBusy):
		return model.RetryResult{}, ErrBusy
	case errors.Is(err, jobsusecase.ErrNotRetryable):
		return model.RetryResult{}, ErrNotRetryable
	case err != nil:
		return model.RetryResult{}, err
	}
	return model.RetryResult{Action: res.Action, Status: res.Status}, nil
}

// revalidate rebuilds a held notification and routes it again.
func (s *Service) revalidate(ctx context.Context, orgID int64, r db.GetKabisNotificationRow) (model.RetryResult, error) {
	var c created
	var ok bool
	err := s.tx(ctx, func(q *db.Queries) error {
		var err error
		c, ok, err = s.refresh(ctx, q, orgID, r.ID, r.Uuid, r.IntegrationJobID, r.RentalID, r.Event)
		return err
	})
	if err != nil {
		return model.RetryResult{}, err
	}
	if !ok {
		return model.RetryResult{}, ErrBusy
	}
	if s.runner != nil {
		s.runner.Changed(ctx, orgID, r.IntegrationJobID)
	}
	fresh, err := s.row(orgctx.WithOrganizationID(ctx, orgID), orgID, r.Uuid)
	if err != nil {
		return model.RetryResult{}, err
	}
	switch c.status {
	case jobsusecase.StatusBlocked:
		return model.RetryResult{}, &DataMissingError{Fields: fresh.MissingFields}
	case jobsusecase.StatusWaitingDependency:
		return model.RetryResult{}, &DependencyError{Reason: fresh.HoldReason}
	}
	if fn := s.after(orgID, c); fn != nil {
		fn(ctx)
	}
	st := c.status
	if again, err := s.row(orgctx.WithOrganizationID(ctx, orgID), orgID, r.Uuid); err == nil {
		st = again.JobStatus
	}
	return model.RetryResult{Action: "revalidated", Status: st}, nil
}

// ReleaseWaiting is the sweep for notifications waiting on a dependency:
// once the organization has a working adapter (an account was connected or
// enabled), each waiting notification is re-validated and routed to it.
// It returns the number released.
func (s *Service) ReleaseWaiting(ctx context.Context) (int, error) {
	rows, err := s.q.ListWaitingKabisJobs(ctx, 200)
	if err != nil {
		return 0, err
	}
	ready := map[int64]bool{}
	n := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		octx := orgctx.WithOrganizationID(ctx, row.OrganizationID)
		ok, seen := ready[row.OrganizationID]
		if !seen {
			r, err := s.routeOf(octx, s.q, row.OrganizationID)
			ok = err == nil && r.ready
			ready[row.OrganizationID] = ok
		}
		if !ok {
			continue
		}
		nr, err := s.q.GetKabisNotification(octx, db.GetKabisNotificationParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid})
		if err != nil {
			continue
		}
		if _, err := s.revalidate(octx, row.OrganizationID, nr); err != nil {
			var dm *DataMissingError
			if !errors.As(err, &dm) {
				s.log.Warn("kabis_release_failed", "notification", row.Uuid, "error", err)
			}
			continue
		}
		n++
	}
	return n, nil
}

func validReason(v string) (string, bool) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	return v, n >= 3 && n <= 500
}

// sniff accepts PDF, JPEG, PNG and WebP attachments.
func sniff(head []byte) (string, string, bool) {
	switch ct := http.DetectContentType(head); ct {
	case "application/pdf":
		return ct, "pdf", true
	case "image/jpeg":
		return ct, "jpg", true
	case "image/png":
		return ct, "png", true
	case "image/webp":
		return ct, "webp", true
	}
	return "", "", false
}

func cleanName(name, ext string) string {
	base := strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if base == "" || base == "." || base == "/" {
		base = "ek." + ext
	}
	if len(base) > 200 {
		base = base[len(base)-200:]
	}
	return base
}

// Resolve records a staff resolution: reported_manually (filed through
// the official manual channel, TEC-682 procedure) or reconciled
// (confirmed with the authority). A reason (3-500) and the reference are
// required; an attachment (PDF / image, 10 MiB) is optional. The job ends
// in that state; nothing is sent afterwards.
func (s *Service) Resolve(ctx context.Context, orgID int64, id uuid.UUID, resolution string, in model.ResolveInput) (model.Detail, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	from := manualFrom
	if resolution == model.ResolutionReconciled {
		from = reconcileFrom
	} else if resolution != model.ResolutionManual {
		return model.Detail{}, errors.New("kabis: unknown resolution")
	}
	ve := &ValidationError{}
	reason, ok := validReason(in.Reason)
	if !ok {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	ref := strings.TrimSpace(in.Reference)
	if ref == "" || utf8.RuneCountInString(ref) > 120 {
		ve.add("reference", "required", "the official reference is required (at most 120 characters)")
	}
	var file *model.Upload
	var head []byte
	var mime, ext string
	if in.File != nil && in.File.Body != nil && in.File.Size > 0 {
		file = in.File
		if file.Size > MaxAttachmentBytes {
			ve.add("file", "too_large", "the attachment must be at most 10 MiB")
		} else {
			head = make([]byte, 512)
			n, _ := io.ReadFull(file.Body, head)
			head = head[:n]
			var okType bool
			if mime, ext, okType = sniff(head); !okType {
				ve.add("file", "invalid_type", "the attachment must be a PDF, JPEG, PNG or WebP file")
			}
		}
	}
	if len(ve.Details) > 0 {
		return model.Detail{}, ve
	}
	if !model.Contains(from, r.JobStatus) {
		return model.Detail{}, ErrNotResolvable
	}
	var key pgtype.Text
	var name string
	if file != nil {
		if s.store == nil {
			return model.Detail{}, ErrStorageUnavailable
		}
		orgUUID, err := s.q.GetKabisOrganizationUUID(ctx, orgID)
		if err != nil {
			return model.Detail{}, err
		}
		k := fmt.Sprintf("%skabis/%s/%s.%s", storage.OrganizationPrefix(orgUUID.String()), r.Uuid, uuid.NewString(), ext)
		name = cleanName(file.Filename, ext)
		if err := s.store.Upload(ctx, storage.File{
			Body: io.MultiReader(bytes.NewReader(head), file.Body), Size: file.Size, ContentType: mime, Filename: name,
		}, k); err != nil {
			return model.Detail{}, err
		}
		key = pgtype.Text{String: k, Valid: true}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.ResolveKabisJob(ctx, db.ResolveKabisJobParams{
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ID: r.IntegrationJobID, Status: resolution, FromStatuses: from,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrBusy
		}
		size := int64(0)
		if file != nil {
			size = file.Size
		}
		return q.SetKabisResolution(ctx, db.SetKabisResolutionParams{
			OrganizationID: orgID, ID: r.ID, Resolution: pgtype.Text{String: resolution, Valid: true}, Reason: reason,
			Reference: ref, ResolvedBy: actorID(ctx), AttachmentKey: key, AttachmentName: name,
			AttachmentContentType: mime, AttachmentSize: size,
		})
	})
	if err != nil {
		if key.Valid {
			_ = s.store.Delete(ctx, key.String)
		}
		return model.Detail{}, err
	}
	if r.AttachmentKey.Valid && r.AttachmentKey.String != "" && key.Valid && s.store != nil {
		_ = s.store.Delete(ctx, r.AttachmentKey.String)
	}
	if s.runner != nil {
		s.runner.Changed(ctx, orgID, r.IntegrationJobID)
	}
	return s.Get(ctx, orgID, id)
}

// AttachmentFile is an open attachment.
type AttachmentFile struct {
	Body        io.ReadCloser
	Size        int64
	Name        string
	ContentType string
}

// OpenAttachment streams the attachment of a resolution (the key is
// checked against the organization before it is read).
func (s *Service) OpenAttachment(ctx context.Context, orgID int64, id uuid.UUID) (AttachmentFile, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !r.AttachmentKey.Valid || r.AttachmentKey.String == "" {
		return AttachmentFile{}, ErrNoAttachment
	}
	if s.store == nil {
		return AttachmentFile{}, ErrStorageUnavailable
	}
	orgUUID, err := s.q.GetKabisOrganizationUUID(ctx, orgID)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !storage.OrganizationOwnsKey(r.AttachmentKey.String, orgUUID.String()) {
		return AttachmentFile{}, ErrNoAttachment
	}
	body, size, err := s.store.Download(ctx, r.AttachmentKey.String)
	if err != nil {
		return AttachmentFile{}, err
	}
	return AttachmentFile{Body: body, Size: size, Name: r.AttachmentName, ContentType: r.AttachmentContentType}, nil
}
