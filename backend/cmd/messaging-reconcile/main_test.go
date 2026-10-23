package main

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/msgreconcile"
	"github.com/google/uuid"
)

func TestParse(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	o, err := parse([]string{"mark", "--before", "20261020T031500Z", "--kind", "sms,dispatch", "--uuids", id.String(), "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if o.cmd != "mark" || !o.yes || !o.filter.Before.Equal(time.Date(2026, 10, 20, 3, 15, 0, 0, time.UTC)) {
		t.Fatalf("options = %+v", o)
	}
	if len(o.filter.Kinds) != 2 || o.filter.Kinds[0] != msgreconcile.KindSMS || len(o.filter.UUIDs) != 1 || o.filter.UUIDs[0] != id {
		t.Fatalf("filter = %+v", o.filter)
	}
	for _, bad := range [][]string{
		nil,
		{"delete"},
		{"list"},
		{"list", "--before", "yesterday"},
		{"list", "--before", "2026-10-20T03:15:00Z", "--kind", "fax"},
		{"list", "--before", "2026-10-20T03:15:00Z", "--uuids", uuid.NewString()},
		{"mark", "--before", "2026-10-20T03:15:00Z", "--uuids", "nope"},
		{"list", "--before", "2026-10-20T03:15:00Z", "--limit", "0"},
	} {
		if _, err := parse(bad); err == nil {
			t.Errorf("parse(%q) accepted", strings.Join(bad, " "))
		}
	}
}

func TestTruncated(t *testing.T) {
	t.Parallel()
	list := []msgreconcile.Candidate{{Kind: msgreconcile.KindSMS}, {Kind: msgreconcile.KindSMS}, {Kind: msgreconcile.KindDispatch}}
	if !truncated(list, 2) || truncated(list, 3) {
		t.Fatal("truncated must report a kind that reached the limit")
	}
}
