// Package repository stores support access grants (TEC-678). Every method
// takes the organization id explicitly, except the impersonation summary
// (by user) and the platform overview (every organization).
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned for a missing grant (or one of another organization).
var ErrNotFound = errors.New("support grant not found")

// Postgres implements the support access repository.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository.
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func int8p(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Open revokes the organization's active grant (if any) and stores a new
// one, in one transaction.
func (r *Postgres) Open(ctx context.Context, orgID, actorID int64, scope string, expiresAt time.Time, note string, now time.Time) (int64, uuid.UUID, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if _, err := q.RevokeActiveSupportGrants(ctx, db.RevokeActiveSupportGrantsParams{
		OrganizationID: orgID, RevokedBy: int8p(actorID), Now: ts(now),
	}); err != nil {
		return 0, uuid.Nil, err
	}
	row, err := q.CreateSupportGrant(ctx, db.CreateSupportGrantParams{
		OrganizationID: orgID, CreatedBy: int8p(actorID), Scope: scope, ExpiresAt: ts(expiresAt), Note: note,
	})
	if err != nil {
		return 0, uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, uuid.Nil, err
	}
	return row.ID, row.Uuid, nil
}

// Revoke closes an active grant. ErrNotFound when the grant is unknown in
// the organization, already revoked or expired.
func (r *Postgres) Revoke(ctx context.Context, orgID, actorID int64, id uuid.UUID, now time.Time) error {
	_, err := r.q.RevokeSupportGrant(ctx, db.RevokeSupportGrantParams{
		OrganizationID: orgID, Uuid: id, RevokedBy: int8p(actorID), Now: ts(now),
	})
	return mapErr(err)
}

// Get loads one grant of the organization.
func (r *Postgres) Get(ctx context.Context, orgID int64, id uuid.UUID, now time.Time) (model.Grant, error) {
	row, err := r.q.GetSupportGrant(ctx, db.GetSupportGrantParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Grant{}, mapErr(err)
	}
	return toGrant(db.ListSupportGrantsRow(row), now), nil
}

// List pages the organization's grants, newest first.
func (r *Postgres) List(ctx context.Context, orgID int64, f model.ListFilter, now time.Time) ([]model.Grant, int64, error) {
	var statuses []string
	if len(f.Statuses) > 0 {
		statuses = f.Statuses
	}
	rows, err := r.q.ListSupportGrants(ctx, db.ListSupportGrantsParams{
		OrganizationID: orgID, Statuses: statuses, Now: ts(now), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountSupportGrants(ctx, db.CountSupportGrantsParams{OrganizationID: orgID, Statuses: statuses, Now: ts(now)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Grant, 0, len(rows))
	for _, row := range rows {
		out = append(out, toGrant(row, now))
	}
	return out, total, nil
}

// Active returns the organization's active grant (ErrNotFound when none).
func (r *Postgres) Active(ctx context.Context, orgID int64, now time.Time) (model.Grant, error) {
	row, err := r.q.GetActiveSupportGrant(ctx, db.GetActiveSupportGrantParams{OrganizationID: orgID, Now: ts(now)})
	if err != nil {
		return model.Grant{}, mapErr(err)
	}
	return model.Grant{
		ID: row.ID, UUID: row.Uuid, Scope: row.Scope, Status: model.StatusActive, Note: row.Note,
		ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// UserSummary counts the user's live memberships and those whose
// organization has an active grant.
func (r *Postgres) UserSummary(ctx context.Context, userID int64, now time.Time) (memberships, granted int64, err error) {
	row, err := r.q.GetUserSupportAccessSummary(ctx, db.GetUserSupportAccessSummaryParams{UserID: userID, Now: ts(now)})
	if err != nil {
		return 0, 0, err
	}
	return row.Memberships, row.Granted, nil
}

// ListActivePlatform lists active grants across organizations.
func (r *Postgres) ListActivePlatform(ctx context.Context, now time.Time) ([]model.PlatformGrant, error) {
	rows, err := r.q.ListActiveSupportGrantsPlatform(ctx, ts(now))
	if err != nil {
		return nil, err
	}
	out := make([]model.PlatformGrant, 0, len(rows))
	for _, row := range rows {
		g := model.PlatformGrant{
			UUID: row.Uuid, Scope: row.Scope, Note: row.Note,
			ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time,
			Organization: model.OrganizationRef{UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName},
		}
		if row.CreatedByEmail.Valid {
			g.CreatedBy = &model.UserRef{Name: row.CreatedByName.String, Surname: row.CreatedBySurname.String, Email: row.CreatedByEmail.String}
		}
		out = append(out, g)
	}
	return out, nil
}

func userRef(id pgtype.UUID, name, surname, email pgtype.Text) *model.UserRef {
	if !id.Valid {
		return nil
	}
	return &model.UserRef{UUID: uuid.UUID(id.Bytes), Name: name.String, Surname: surname.String, Email: email.String}
}

func toGrant(row db.ListSupportGrantsRow, now time.Time) model.Grant {
	g := model.Grant{
		ID: row.ID, UUID: row.Uuid, Scope: row.Scope, Note: row.Note,
		ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time,
		CreatedBy: userRef(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		RevokedBy: userRef(row.RevokedByUuid, row.RevokedByName, row.RevokedBySurname, row.RevokedByEmail),
	}
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		g.RevokedAt = &t
	}
	g.Status = model.StatusAt(g.RevokedAt, g.ExpiresAt, now)
	return g
}
