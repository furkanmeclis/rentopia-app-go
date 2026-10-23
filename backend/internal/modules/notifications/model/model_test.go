package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestEventKeyIsDeterministicPerEventKindAndRecipient(t *testing.T) {
	t.Parallel()
	ev := uuid.MustParse("0b5a3f0e-1c2d-4e5f-8a9b-0c1d2e3f4a5b")
	k := EventKey(ev, "customers.assigned", 42)
	if k != "event:0b5a3f0e-1c2d-4e5f-8a9b-0c1d2e3f4a5b|customers.assigned|user:42" {
		t.Fatalf("key = %q", k)
	}
	if EventKey(ev, "customers.assigned", 42) != k {
		t.Fatal("same event, kind and recipient must give the same key")
	}
	for _, other := range []string{
		EventKey(ev, "customers.assigned", 43),
		EventKey(ev, "customers.status_blocked", 42),
		EventKey(uuid.New(), "customers.assigned", 42),
	} {
		if other == k {
			t.Fatalf("different cause or recipient gave the same key %q", k)
		}
	}
}

func TestRowKey(t *testing.T) {
	t.Parallel()
	if RowKey("", ChannelEmail) != "" {
		t.Fatal("no base key: no row key")
	}
	if got := RowKey("event:x|k|user:1", ChannelEmail); got != "event:x|k|user:1|email" {
		t.Fatalf("row key = %q", got)
	}
	if RowKey("b", ChannelEmail) == RowKey("b", ChannelInapp) {
		t.Fatal("channels must get their own key")
	}
	long := strings.Repeat("a", 300)
	got := RowKey(long, ChannelEmail)
	if len(got) > maxKeyLen || !strings.HasPrefix(got, "sha256:") || got != RowKey(long, ChannelEmail) {
		t.Fatalf("long key = %q (len %d)", got, len(got))
	}
}
