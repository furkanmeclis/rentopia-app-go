package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskAgreementPeriods is the periodic billing sweep of long-term /
// corporate agreement rentals (TEC-723): every due billing period is
// accrued once through the ledger.
const TaskAgreementPeriods = "app:agreements:accrue-periods"

// agreementPeriodsCron runs every hour at minute 17. Periods start on
// Istanbul calendar days, so an hourly sweep posts a period within the
// first hour of its day; the job is idempotent (a second run, or a second
// scheduler, posts nothing twice).
const agreementPeriodsCron = "17 * * * *"

// AgreementPeriodsFunc runs the sweep and returns the number of rentals
// it reconciled.
type AgreementPeriodsFunc func(ctx context.Context) (int, error)

// NewAgreementPeriodsTask builds the sweep task.
func NewAgreementPeriodsTask() *asynq.Task {
	return asynq.NewTask(TaskAgreementPeriods, []byte("{}"))
}

// WithAgreementPeriods registers the sweep handler.
func (w *Worker) WithAgreementPeriods(fn AgreementPeriodsFunc) *Worker {
	w.mux.HandleFunc(TaskAgreementPeriods, func(ctx context.Context, _ *asynq.Task) error {
		if fn == nil {
			return nil
		}
		n, err := fn(ctx)
		if n > 0 {
			w.log.Info("agreement_periods_accrued", "rentals", n)
		}
		return err
	})
	return w
}

// RegisterAgreementPeriodsSchedule adds the hourly sweep.
func RegisterAgreementPeriodsSchedule(scheduler *asynq.Scheduler) error {
	if _, err := scheduler.Register(
		agreementPeriodsCron, NewAgreementPeriodsTask(),
		asynq.Queue(QueueMaintenance), asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute), asynq.Unique(50*time.Minute),
	); err != nil {
		return fmt.Errorf("queue: register agreement periods: %w", err)
	}
	return nil
}
