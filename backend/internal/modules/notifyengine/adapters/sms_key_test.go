package adapters

import (
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/google/uuid"
)

func TestDeliveryKey(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("6f1c1f1e-0000-4000-8000-000000000001")
	if got := DeliveryKey(id); got != "notify:6f1c1f1e-0000-4000-8000-000000000001" {
		t.Fatalf("key = %q (must match the 000137 backfill: 'notify:' || ref_uuid)", got)
	}
	if DeliveryKey(uuid.New()) == DeliveryKey(id) {
		t.Fatal("deliveries must get their own key")
	}
}

func TestSMSResult(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	cases := map[string]channel.Outcome{
		smsusecase.StatusQueued:      channel.OutcomeAccepted,
		smsusecase.StatusSubmitted:   channel.OutcomeAccepted,
		smsusecase.StatusUnknown:     channel.OutcomeAccepted,
		smsusecase.StatusDelivered:   channel.OutcomeDelivered,
		smsusecase.StatusReconciled:  channel.OutcomeDelivered,
		smsusecase.StatusFailed:      channel.OutcomeFailed,
		smsusecase.StatusUndelivered: channel.OutcomeFailed,
		smsusecase.StatusBlocked:     channel.OutcomeFailed,
	}
	for status, want := range cases {
		res := smsResult(id, "+90 *** ** 33", status, "")
		if res.Outcome != want || res.ExternalUUID == nil || *res.ExternalUUID != id {
			t.Errorf("%s: %+v, want %s", status, res, want)
		}
	}
}
