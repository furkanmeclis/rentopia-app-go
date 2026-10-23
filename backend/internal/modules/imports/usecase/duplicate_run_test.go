package usecase

import (
	"context"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// claimedDB answers every query like a job that is no longer queued.
type claimedDB struct{}

func (claimedDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (claimedDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}
func (claimedDB) QueryRow(context.Context, string, ...interface{}) pgx.Row { return claimedRow{} }

type claimedRow struct{}

func (claimedRow) Scan(...any) error { return pgx.ErrNoRows }

// A task that runs again for a job another run took (or finished) does
// nothing and does not fail (no Asynq retry, no dead-task alert; TEC-962).
func TestProcessTwiceIsNoop(t *testing.T) {
	t.Parallel()
	svc := New(db.New(claimedDB{}), nil, nil, nil, nil, nil, nil)
	if err := svc.ProcessImport(context.Background(), 42); err != nil {
		t.Fatalf("second run: %v, want nil", err)
	}
}
