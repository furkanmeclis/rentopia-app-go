package channels

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type healthStub struct {
	counts db.ChannelHealthCountsRow
	rows   []db.ListChannelHealthIssuesRow
	listed bool
}

func (h *healthStub) ChannelHealthCounts(context.Context, pgtype.Timestamptz) (db.ChannelHealthCountsRow, error) {
	return h.counts, nil
}

func (h *healthStub) ListChannelHealthIssues(context.Context, db.ListChannelHealthIssuesParams) ([]db.ListChannelHealthIssuesRow, error) {
	h.listed = true
	return h.rows, nil
}

func TestHealthSource(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	healthy := &healthStub{counts: db.ChannelHealthCountsRow{Active: 2}}
	rep, err := NewHealthSource(healthy).Collect(context.Background(), now)
	if err != nil || healthy.listed || len(rep.Issues) != 0 || healthcheck.DeriveStatus(rep) != healthcheck.StatusHealthy {
		t.Fatalf("healthy report %+v %v", rep, err)
	}
	past := pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}
	bad := &healthStub{
		counts: db.ChannelHealthCountsRow{Active: 1, Failing: 1, Expired: 1, ErrorQueue: 3},
		rows: []db.ListChannelHealthIssuesRow{{
			OrganizationID: 7, Uuid: uuid.New(), Provider: "yolcu360", DisplayName: "Yolcu360",
			LastPushStatus: "failed", LastPushError: "unavailable", LastPushAt: past, PushedValidUntil: past,
			PushEnabled: true, UpdatedAt: past, OpenErrors: 3,
		}},
	}
	rep, err = NewHealthSource(bad).Collect(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, i := range rep.Issues {
		kinds[i.Kind] = true
		if i.OrganizationID == nil || *i.OrganizationID != 7 {
			t.Fatalf("issue organization %+v", i)
		}
	}
	if len(rep.Issues) != 3 || !kinds[HealthKindSyncFailed] || !kinds[HealthKindPushExpired] || !kinds[HealthKindErrorQueue] {
		t.Fatalf("issues %+v", rep.Issues)
	}
	if healthcheck.DeriveStatus(rep) != healthcheck.StatusFailing {
		t.Fatalf("status %s", healthcheck.DeriveStatus(rep))
	}
	if err := healthcheck.NewRegistry().Register(NewHealthSource(bad)); err != nil {
		t.Fatalf("register: %v", err)
	}
}
