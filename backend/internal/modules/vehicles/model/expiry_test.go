package model

import (
	"testing"
	"time"
)

func TestToday(t *testing.T) {
	t.Parallel()
	// 22:30 UTC is already the next day in Türkiye (UTC+3).
	got := Today(time.Date(2026, 10, 14, 22, 30, 0, 0, time.UTC))
	if want := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Today = %v, want %v", got, want)
	}
}

func TestDocumentState(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	at := func(days int) *time.Time {
		d := today.AddDate(0, 0, days)
		return &d
	}
	cases := []struct {
		until *time.Time
		want  string
	}{
		{nil, ExpiryNone},
		{at(-1), ExpiryExpired},
		{at(0), ExpiryExpiring},
		{at(DocumentReminderDays), ExpiryExpiring},
		{at(DocumentReminderDays + 1), ExpiryValid},
	}
	for _, tc := range cases {
		if got := DocumentState(tc.until, today); got != tc.want {
			t.Errorf("DocumentState(%v) = %s, want %s", tc.until, got, tc.want)
		}
	}
}

func TestMaintenanceState(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	at := func(days int) *time.Time {
		d := today.AddDate(0, 0, days)
		return &d
	}
	km := func(v int32) *int32 { return &v }
	cases := []struct {
		name string
		date *time.Time
		km   *int32
		want string
	}{
		{"no plan", nil, nil, MaintenanceNone},
		{"far date", at(60), nil, MaintenanceScheduled},
		{"date within horizon", at(MaintenanceReminderDays), nil, MaintenanceDue},
		{"date passed", at(-1), nil, MaintenanceOverdue},
		{"km far", nil, km(50000), MaintenanceScheduled},
		{"km within horizon", nil, km(10000 + MaintenanceReminderKm), MaintenanceDue},
		{"km reached", nil, km(10000), MaintenanceOverdue},
		{"far date but km due", at(90), km(10500), MaintenanceDue},
	}
	for _, tc := range cases {
		if got := MaintenanceState(tc.date, tc.km, 10000, today); got != tc.want {
			t.Errorf("%s: MaintenanceState = %s, want %s", tc.name, got, tc.want)
		}
	}
}
