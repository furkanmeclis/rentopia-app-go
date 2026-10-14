package model

import "time"

// Reminder horizons of the document / maintenance tracking (TEC-693).
// Organization settings have no field for them yet, so every organization
// uses these constants: a document is "expiring" DocumentReminderDays before
// its valid_until, a maintenance plan is "due" MaintenanceReminderDays before
// its planned date or MaintenanceReminderKm before its planned km.
const (
	DocumentReminderDays    = 30
	MaintenanceReminderDays = 15
	MaintenanceReminderKm   = 1000
)

// Document expiry states (Vehicle.document_status, list filter, alerts).
const (
	ExpiryNone     = "none"
	ExpiryValid    = "valid"
	ExpiryExpiring = "expiring"
	ExpiryExpired  = "expired"
)

// DocumentStatuses lists the document_status filter values.
var DocumentStatuses = []string{ExpiryExpired, ExpiryExpiring, ExpiryValid, ExpiryNone}

// Maintenance states of a vehicle (Vehicle.maintenance_status).
const (
	MaintenanceNone      = "none"
	MaintenanceScheduled = "scheduled"
	MaintenanceDue       = "due"
	MaintenanceOverdue   = "overdue"
)

// turkey is the operating time zone (UTC+3 all year since 2016); "today"
// for expiry checks is the calendar day there.
var turkey = time.FixedZone("TRT", 3*60*60)

// Today returns the calendar day of now in Türkiye as midnight UTC (the
// representation of DATE columns).
func Today(now time.Time) time.Time {
	t := now.In(turkey)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// DaysUntil is the number of calendar days from today to day (negative when
// day is in the past). Both are midnight UTC dates.
func DaysUntil(today, day time.Time) int {
	return int(day.Sub(today).Hours() / 24)
}

// DocumentState is the expiry state of a document valid until `until` (nil
// = no expiry date).
func DocumentState(until *time.Time, today time.Time) string {
	if until == nil {
		return ExpiryNone
	}
	days := DaysUntil(today, *until)
	switch {
	case days < 0:
		return ExpiryExpired
	case days <= DocumentReminderDays:
		return ExpiryExpiring
	default:
		return ExpiryValid
	}
}

// MaintenanceState is the state of the next plan of a vehicle: overdue when
// the planned date passed or the odometer reached the planned km, due inside
// the reminder horizon, scheduled otherwise; none without a plan.
func MaintenanceState(date *time.Time, km *int32, odometer int32, today time.Time) string {
	if date == nil && km == nil {
		return MaintenanceNone
	}
	state := MaintenanceScheduled
	if date != nil {
		days := DaysUntil(today, *date)
		if days < 0 {
			return MaintenanceOverdue
		}
		if days <= MaintenanceReminderDays {
			state = MaintenanceDue
		}
	}
	if km != nil {
		left := *km - odometer
		if left <= 0 {
			return MaintenanceOverdue
		}
		if left <= MaintenanceReminderKm {
			state = MaintenanceDue
		}
	}
	return state
}
