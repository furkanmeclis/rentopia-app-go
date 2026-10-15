package usecase_test

import (
	"context"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
)

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) Publish(_ context.Context, resource, action string, uuids ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range uuids {
		r.events = append(r.events, resource+" "+action+" "+id)
	}
}

func (r *recorder) has(event string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e == event {
			return true
		}
	}
	return false
}

// Live lists: quotes, reservations and their changes are published on the
// tenant resource channels (scoped by the organization resolver).
func TestLiveEventsPublished(t *testing.T) {
	f := newFx(t)
	rec := &recorder{}
	f.svc.SetResourceEvents(rec)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(24))
	if !rec.has(model.ResourceReservations + " created " + r.UUID.String()) {
		t.Fatalf("no created event: %v", rec.events)
	}
	if r.QuoteUUID == nil || !rec.has(model.ResourceQuotes+" created "+r.QuoteUUID.String()) {
		t.Fatalf("no quote event: %v", rec.events)
	}
	if _, err := f.svc.Cancel(f.ctx, f.org, r.UUID, "canlı liste"); err != nil {
		t.Fatal(err)
	}
	if !rec.has(model.ResourceReservations + " updated " + r.UUID.String()) {
		t.Fatalf("no updated event: %v", rec.events)
	}
}
