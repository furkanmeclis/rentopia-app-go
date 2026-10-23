package netgsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/envguard"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// Outside production a live account never reaches Netgsm's official host;
// a live URL pointed at the fake (staging rehearsal) still works.
func TestRefuseOfficialOutsideProduction(t *testing.T) {
	f, _, acc := setup(t)
	acc.Mode = integrations.ModeLive
	refusal := envguard.Refusal("staging", "netgsm", envguard.AllowNetgsmKey, false)
	msg := sms.Message{To: "+905321234567", Body: "x"}

	official := netgsm.New(netgsm.Config{LiveBaseURL: netgsm.DefaultLive, RefuseOfficial: refusal})
	_, err := official.Send(context.Background(), acc, msg)
	var rej *sms.Rejected
	if !errors.As(err, &rej) || rej.Code != "environment" {
		t.Fatalf("official live endpoint must be refused as a definitive rejection: %v", err)
	}
	if err := official.Check(context.Background(), acc); err == nil {
		t.Fatal("connection test must be refused too")
	}
	if netgsm.IsUncertain(err) {
		t.Fatal("a refusal is not an unknown outcome")
	}

	faked := netgsm.New(netgsm.Config{LiveBaseURL: f.URL(), RefuseOfficial: refusal})
	if err := faked.Check(context.Background(), acc); err != nil {
		t.Fatalf("live URL on the fake must stay usable: %v", err)
	}
	if !netgsm.IsOfficial("https://api.netgsm.com.tr/") || netgsm.IsOfficial(f.URL()) {
		t.Fatal("IsOfficial")
	}
}
