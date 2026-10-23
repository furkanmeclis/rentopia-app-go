package notifications_test

import (
	"context"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestOutboxReplayCreatesOneNotification (TEC-962): the outbox publishes at
// least once; an event published again (a restore of a backup taken
// before the row was marked published, a retry after a failed status
// write) creates no second notification. Runs with TEST_DATABASE_URL.
func TestOutboxReplayCreatesOneNotification(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "replay-" + suffix + "@example.test", PasswordHash: "x", Name: "Replay", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	var orgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, $2) RETURNING id`,
		"replay-"+suffix, "Replay "+suffix).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	ev := events.New(events.ConversationsAssigned).WithOrganization(orgID).
		WithPayload(map[string]any{"assigned_user_id": user.ID})
	ev.EventID = uuid.New()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM notifications WHERE user_id = $1", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM outbox_events WHERE payload->>'event_id' = $1", ev.EventID.String())
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
	})

	bus := events.NewBus(nil)
	svc := notifusecase.New(q, nil, []providers.Provider{providers.InappProvider{}}, nil)
	notifmodule.RegisterEventHandlers(bus, svc, nil)
	store := outbox.NewStore(pool, q)
	pub := outbox.NewPublisher(store, bus, nil)

	// The same event in the outbox twice (the replayed copy), then once more
	// after the first two were published.
	for i := 0; i < 3; i++ {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Enqueue(ctx, tx, ev); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			continue
		}
		for {
			n, err := pub.Drain(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
		}
	}
	var published, notified int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE payload->>'event_id' = $1 AND status = 'published'`,
		ev.EventID.String()).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND organization_id = $2`,
		user.ID, orgID).Scan(&notified); err != nil {
		t.Fatal(err)
	}
	if published != 3 || notified != 1 {
		t.Fatalf("published %d outbox rows, %d notifications; want 3, 1", published, notified)
	}
}
