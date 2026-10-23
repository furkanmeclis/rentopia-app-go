package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDecideReclaim(t *testing.T) {
	t.Parallel()
	at := pgtype.Timestamptz{Time: time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC), Valid: true}
	ext := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	cases := []struct {
		name string
		dl   *db.NotificationDelivery
		want reclaimAction
	}{
		{"no attempt row", nil, reclaimRelease},
		{"claimed, nothing recorded", &db.NotificationDelivery{Status: DeliverySending}, reclaimInterrupted},
		{"claimed, channel record", &db.NotificationDelivery{Status: DeliverySending, ExternalUuid: ext}, reclaimAwait},
		{"claimed, provider reference", &db.NotificationDelivery{Status: DeliverySending, ProviderRef: "17"}, reclaimAwait},
		{"accepted, waiting for the report", &db.NotificationDelivery{Status: DeliverySent, SentAt: at, ExternalUuid: ext}, reclaimAwait},
		{"sent, final", &db.NotificationDelivery{Status: DeliverySent, SentAt: at, FinishedAt: at}, reclaimSettle},
		{"delivered", &db.NotificationDelivery{Status: DeliveryDelivered, FinishedAt: at}, reclaimSettle},
		{"failed", &db.NotificationDelivery{Status: DeliveryFailed, FinishedAt: at}, reclaimSettle},
		{"skipped", &db.NotificationDelivery{Status: DeliverySkipped, FinishedAt: at}, reclaimSettle},
	}
	for _, c := range cases {
		if got := decideReclaim(c.dl); got != c.want {
			t.Errorf("%s: action %d, want %d", c.name, got, c.want)
		}
	}
}

func TestInterruptedNeedsNoEvidence(t *testing.T) {
	t.Parallel()
	if interrupted(nil) {
		t.Fatal("no row is not an interrupted attempt")
	}
	at := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	for _, dl := range []db.NotificationDelivery{
		{Status: DeliverySending, SentAt: at},
		{Status: DeliverySending, FinishedAt: at},
		{Status: DeliverySending, ProviderRef: "x"},
		{Status: DeliverySending, ExternalUuid: pgtype.UUID{Bytes: uuid.New(), Valid: true}},
		{Status: DeliverySent},
	} {
		if interrupted(&dl) {
			t.Errorf("%+v counted as interrupted (it has a recorded send)", dl)
		}
	}
}

func TestFinalOutcome(t *testing.T) {
	t.Parallel()
	cases := map[channel.Outcome][2]bool{
		channel.OutcomeDelivered: {true, true},
		channel.OutcomeSent:      {true, true},
		channel.OutcomeFailed:    {true, false},
		channel.OutcomeSkipped:   {true, false},
		channel.OutcomeAccepted:  {false, false},
		"":                       {false, false},
	}
	for o, want := range cases {
		final, delivered := finalOutcome(o)
		if final != want[0] || delivered != want[1] {
			t.Errorf("%q: final %v delivered %v, want %v", o, final, delivered, want)
		}
	}
}
