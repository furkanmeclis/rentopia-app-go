package rentals_test

import (
	"sync"
	"testing"
	"time"

	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// fakeAsynq records render tasks by task id and refuses an id that is
// still queued, like Asynq does.
type fakeAsynq struct {
	mu     sync.Mutex
	queued map[string]bool
	ids    []string
}

func (f *fakeAsynq) EnqueueDocumentPDF(int64, int64) error {
	panic("the sweep must enqueue with a task id")
}

func (f *fakeAsynq) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := ""
	for _, o := range opts {
		if o.Type() == asynq.TaskIDOpt {
			id, _ = o.Value().(string)
		}
	}
	if f.queued[id] {
		return nil, asynq.ErrTaskIDConflict
	}
	f.queued[id] = true
	f.ids = append(f.ids, id)
	return &asynq.TaskInfo{ID: id, Type: task.Type()}, nil
}

func (f *fakeAsynq) count(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, x := range f.ids {
		if x == id {
			n++
		}
	}
	return n
}

// TEC-962: a contract PDF pending for longer than the threshold (its queue
// task was lost, e.g. a restore with an older Redis) is requested again
// exactly once, under the version's task id; a fresh pending one is left
// alone; a task that still exists is never doubled.
func TestPendingPDFSweepRequeuesOnce(t *testing.T) {
	f := newFx(t)
	name := "Kiralama sözleşmesi"
	body := "<p>Kiracı: {{renter.full_name}}</p><p>Araç: {{vehicle.plate}}</p>"
	tpl, err := f.contracts.CreateTemplate(f.ctx, f.org, contractsmodel.TemplateInput{Kind: contractsmodel.KindRentalContract, Name: &name, BodyHTML: &body})
	f.must(err)
	_, err = f.contracts.ActivateTemplate(f.ctx, f.org, tpl.UUID)
	f.must(err)
	version := func(offset time.Duration) uuid.UUID {
		v := f.vehicle()
		start := f.now().Add(offset).Truncate(time.Minute)
		res := f.reserve(start, start.Add(48*time.Hour), nil)
		rental, err := f.rentals.Create(f.ctx, f.org, model.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
		f.must(err)
		doc, err := f.contracts.GetDocument(f.ctx, f.org, *rental.Contract.DocumentUUID)
		f.must(err)
		return doc.Current.UUID
	}
	stale, fresh := version(time.Hour), version(240*time.Hour)
	now := f.now()
	set := func(id uuid.UUID, requested *time.Time) {
		_, err := f.pool.Exec(f.ctx, `UPDATE document_versions SET pdf_status = 'pending', pdf_object_key = NULL, pdf_requested_at = $3
			WHERE organization_id = $1 AND uuid = $2`, f.org, id, requested)
		f.must(err)
	}
	old := now.Add(-contractsusecase.PDFStaleAfter - time.Minute)
	set(stale, &old)
	set(fresh, &now)

	q := &fakeAsynq{queued: map[string]bool{}}
	f.contracts.SetQueue(q)
	if _, err := f.contracts.SweepPendingPDFs(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := q.count(contractsusecase.PDFTaskID(stale)); got != 1 {
		t.Fatalf("stale version enqueued %d times, want 1", got)
	}
	if got := q.count(contractsusecase.PDFTaskID(fresh)); got != 0 {
		t.Fatalf("fresh version enqueued %d times, want 0", got)
	}
	// A second sweep right after: the stale version was re-stamped.
	if _, err := f.contracts.SweepPendingPDFs(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := q.count(contractsusecase.PDFTaskID(stale)); got != 1 {
		t.Fatalf("after a second sweep: enqueued %d times, want 1", got)
	}
	// Later again, while its task still sits in the queue: Asynq refuses
	// the same task id, nothing is doubled.
	f.advance(contractsusecase.PDFStaleAfter + time.Minute)
	if _, err := f.contracts.SweepPendingPDFs(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := q.count(contractsusecase.PDFTaskID(stale)); got != 1 {
		t.Fatalf("with the task still queued: enqueued %d times, want 1", got)
	}
	// The worker renders the queued task: the version is ready.
	var versionID int64
	f.must(f.pool.QueryRow(f.ctx, "SELECT id FROM document_versions WHERE organization_id = $1 AND uuid = $2", f.org, stale).Scan(&versionID))
	f.must(f.contracts.RenderPDF(f.ctx, f.org, versionID))
	var status string
	f.must(f.pool.QueryRow(f.ctx, "SELECT pdf_status FROM document_versions WHERE organization_id = $1 AND id = $2", f.org, versionID).Scan(&status))
	if status != "ready" {
		t.Fatalf("after render: %s", status)
	}
}
