package usecase

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// upsertCursor builds the cursor write: count > 0 records an alert at now.
func upsertCursor(source string, seen time.Time, count int, now time.Time) db.UpsertPlatformHealthAlertCursorParams {
	arg := db.UpsertPlatformHealthAlertCursorParams{
		Source:         source,
		LastSeenAt:     ts(seen),
		LastAlertCount: int32(count), //nolint:gosec // bounded by the issue limit
	}
	if count > 0 {
		arg.LastAlertAt = pgtype.Timestamptz{Time: now, Valid: true}
	}
	return arg
}
