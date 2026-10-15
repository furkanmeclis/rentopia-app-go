// Package repository is the Postgres access path of the pricing resources
// (rate plans, duration bands, date rules). Every sqlc query takes the
// organization id explicitly and filters by it (queries/pricing.sql): a
// uuid of another organization reads as not found.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrBandOverlap is returned when ex_rate_bands_overlap rejects a band.
	ErrBandOverlap = errors.New("duration bands overlap")
	// ErrNameTaken is returned when a live date rule already has the name.
	ErrNameTaken = errors.New("name taken")
	// ErrFrozen is returned when a trigger refuses to change a frozen
	// (activated) version.
	ErrFrozen = errors.New("version is frozen")
	// ErrConcurrentDraft is returned when two writers create a draft (or
	// activate) the same record at once (partial unique indexes).
	ErrConcurrentDraft = errors.New("concurrent version change")
)

// Postgres runs the pricing queries; Q is bound to the transaction inside
// InTx.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil (InTx then runs fn
// without a transaction; unit tests).
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// Q is the sqlc handle (every query takes organization_id).
func (r *Postgres) Q() *db.Queries { return r.q }

// InTx runs fn with a repository bound to one transaction.
func (r *Postgres) InTx(ctx context.Context, fn func(tx *Postgres) error) error {
	if r.pool == nil {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pricing: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Postgres{q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListActivity lists the audit events of one record.
func (r *Postgres) ListActivity(ctx context.Context, orgID int64, resource string, id uuid.UUID, limit, offset int32) ([]db.ListOrganizationResourceActivityRow, int64, error) {
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ru := pgtype.UUID{Bytes: id, Valid: true}
	rows, err := r.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: resource, ResourceUuid: ru, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: resource, ResourceUuid: ru,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// MapErr maps driver errors to the repository errors.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "23P01" && pgErr.ConstraintName == "ex_rate_bands_overlap":
			return ErrBandOverlap
		case pgErr.Code == "23505" && pgErr.ConstraintName == "uq_date_rules_organization_name":
			return ErrNameTaken
		case pgErr.Code == "23505" && (pgErr.ConstraintName == "uq_rate_plan_versions_organization_draft" ||
			pgErr.ConstraintName == "uq_rate_plan_versions_organization_active" ||
			pgErr.ConstraintName == "uq_date_rule_versions_organization_draft" ||
			pgErr.ConstraintName == "uq_date_rule_versions_organization_active"):
			return ErrConcurrentDraft
		case pgErr.Code == "23514" && pgErr.Where != "" && pgErr.ConstraintName == "":
			// RAISE ... USING ERRCODE = 'check_violation' from the
			// immutability triggers (no constraint name).
			return ErrFrozen
		}
	}
	return err
}

// Rows maps an :execrows result (0 rows = not found).
func Rows(n int64, err error) error {
	if err != nil {
		return MapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
