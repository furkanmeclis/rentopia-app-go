package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

func TestValidateCalendarWindow(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	codeOf := func(err error) string {
		ve, ok := err.(*ValidationError)
		if !ok || len(ve.Details) == 0 {
			return ""
		}
		return ve.Details[0].Code
	}
	cases := []struct {
		name     string
		from, to time.Time
		want     string
	}{
		{"missing", time.Time{}, from, "required"},
		{"equal", from, from, "before_from"},
		{"reversed", from, from.Add(-time.Hour), "before_from"},
		{"one day", from, from.AddDate(0, 0, 1), ""},
		{"max", from, from.AddDate(0, 0, model.CalendarMaxDays), ""},
		{"too wide", from, from.AddDate(0, 0, model.CalendarMaxDays).Add(time.Second), "range_too_large"},
	}
	for _, tc := range cases {
		if got := codeOf(ValidateCalendarWindow(tc.from, tc.to)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClipInterval(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	at := func(days int) time.Time { return from.AddDate(0, 0, days) }
	ptr := func(t time.Time) *time.Time { return &t }
	cases := []struct {
		name          string
		start         time.Time
		end           *time.Time
		cs, ce        time.Time
		before, after bool
	}{
		{"inside", at(1), ptr(at(2)), at(1), at(2), false, false},
		{"starts before", at(-3), ptr(at(2)), from, at(2), true, false},
		{"ends after", at(5), ptr(at(9)), at(5), to, false, true},
		{"open-ended", at(2), nil, at(2), to, false, true},
		{"covers the window", at(-1), nil, from, to, true, true},
		{"ends at the edge", at(3), ptr(to), at(3), to, false, false},
	}
	for _, tc := range cases {
		cs, ce, before, after := ClipInterval(tc.start, tc.end, from, to)
		if !cs.Equal(tc.cs) || !ce.Equal(tc.ce) || before != tc.before || after != tc.after {
			t.Errorf("%s: [%s, %s) before=%v after=%v", tc.name, cs, ce, before, after)
		}
	}
}

func TestCalendarDays(t *testing.T) {
	t.Parallel()
	// Local (UTC+3) midnights of 1 and 8 October.
	from := time.Date(2026, 9, 30, 21, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 7, 21, 0, 0, 0, time.UTC)
	fromDay, toDay := CalendarDays(from, to)
	if fromDay.Format(time.DateOnly) != "2026-10-01" || toDay.Format(time.DateOnly) != "2026-10-08" {
		t.Errorf("days = %s .. %s", fromDay, toDay)
	}
	// A window ending mid-day still covers that day.
	_, toDay = CalendarDays(from, to.Add(5*time.Hour))
	if toDay.Format(time.DateOnly) != "2026-10-09" {
		t.Errorf("mid-day end = %s", toDay)
	}
}

func TestMaintenanceDayState(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		kind string
		day  time.Time
		want string
	}{
		{"done", today.AddDate(0, 0, -3), model.MaintenanceStateDone},
		{"planned", today.AddDate(0, 0, -1), model.MaintenanceStateOverdue},
		{"planned", today, model.MaintenanceStateDue},
		{"planned", today.AddDate(0, 0, 15), model.MaintenanceStateDue},
		{"planned", today.AddDate(0, 0, 16), model.MaintenanceStateScheduled},
	}
	for i, tc := range cases {
		if got := MaintenanceDayState(tc.kind, tc.day, today); got != tc.want {
			t.Errorf("case %d: %s, want %s", i, got, tc.want)
		}
	}
}

func TestCalendarLayers(t *testing.T) {
	t.Parallel()
	only := func(perms ...string) context.Context {
		return orgctx.WithScope(context.Background(), orgctx.Scope{ID: 1, Permissions: perms})
	}
	if got := CalendarLayers(only(rbac.PermTenantVehiclesRead)); len(got) != 1 || got[0] != model.LayerMaintenance {
		t.Errorf("vehicles only = %v", got)
	}
	got := CalendarLayers(only(rbac.PermTenantVehiclesRead, rbac.PermTenantBlocksRead, rbac.PermTenantTransfersRead))
	if len(got) != 3 || got[0] != model.LayerBlock || got[1] != model.LayerTransfer {
		t.Errorf("all = %v", got)
	}
}
