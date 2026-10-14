package usecase

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// ReminderTemplate is the notification template of the daily job.
const ReminderTemplate = "vehicles.reminder"

// Reminder cadence. Every alert is reminded at most once a day (the
// vehicle_reminder_deliveries row is the idempotency key). Within that, a
// subject is reminded when it enters a state (expiring / overdue), on the
// milestone days before its date (7, 3, 1, 0) and then weekly, so a member
// is not notified about the same document every day for a month.
const reminderRepeatDays = 7

var reminderMilestones = []int{7, 3, 1, 0}

// ReminderStats summarizes one run.
type ReminderStats struct {
	Organizations int
	Subjects      int
	Notifications int
}

// SendReminders runs the daily reminder job for every active organization.
// A failing organization is logged and skipped; the others still run.
func (s *Service) SendReminders(ctx context.Context) (ReminderStats, error) {
	var stats ReminderStats
	if s.notifier == nil {
		return stats, nil
	}
	orgs, err := s.repo.ReminderOrganizations(ctx)
	if err != nil {
		return stats, err
	}
	today := s.today()
	for _, org := range orgs {
		res, err := s.RemindOrganization(ctx, org, today)
		if err != nil {
			s.log.Error("vehicle_reminders_organization_failed", "organization_id", org.ID, "error", err)
			continue
		}
		stats.Organizations++
		stats.Subjects += res.Subjects
		stats.Notifications += res.Notifications
	}
	return stats, nil
}

// RemindOrganization sends the reminders of one organization for day. Run
// twice on the same day it sends nothing the second time.
func (s *Service) RemindOrganization(ctx context.Context, org db.ListVehicleReminderOrganizationsRow, today time.Time) (ReminderStats, error) {
	stats := ReminderStats{Organizations: 1}
	if s.notifier == nil {
		return stats, nil
	}
	ctx = orgctx.WithOrganizationID(ctx, org.ID)
	alerts, err := s.alerts(ctx, org.ID, nil, today)
	if err != nil {
		return stats, err
	}
	if len(alerts) == 0 {
		return stats, nil
	}
	last, err := s.repo.LastReminders(ctx, org.ID, today.AddDate(0, 0, -60), today)
	if err != nil {
		return stats, err
	}
	// Claim the subjects due today, grouped by vehicle (one notification per
	// vehicle and recipient).
	type vehicleBatch struct {
		uuid            uuid.UUID
		plate, label    string
		homeBranchID    int64
		currentBranchID int64
		items           []alert
	}
	batches := map[int64]*vehicleBatch{}
	order := []int64{}
	for _, a := range alerts {
		kind := a.Kind
		prev, seen := last[repository.ReminderKey(kind, a.subjectID)]
		if !shouldRemind(a, prev, seen, today) {
			continue
		}
		claimed, err := s.repo.ClaimReminder(ctx, org.ID, a.vehicleID, kind, a.subjectID, today, a.State)
		if err != nil {
			return stats, err
		}
		if !claimed {
			continue
		}
		stats.Subjects++
		b, ok := batches[a.vehicleID]
		if !ok {
			b = &vehicleBatch{
				uuid: a.Vehicle.UUID, plate: a.Vehicle.Plate, label: a.Vehicle.Label,
				homeBranchID: a.homeBranchID, currentBranchID: a.currentBranchID,
			}
			batches[a.vehicleID] = b
			order = append(order, a.vehicleID)
		}
		b.items = append(b.items, a)
	}
	if len(order) == 0 {
		return stats, nil
	}
	recipients, err := s.repo.ReminderRecipients(ctx, org.ID, rbac.PermTenantVehiclesWrite)
	if err != nil {
		return stats, err
	}
	for _, vehicleID := range order {
		b := batches[vehicleID]
		sent := int32(0)
		for _, r := range recipients {
			if !recipientCovers(r, b.homeBranchID, b.currentBranchID) {
				continue
			}
			loc := i18n.Normalize(r.Locale)
			uid := r.UserID
			orgID := org.ID
			link := reminderLink(org.Slug, b.uuid, b.items)
			_, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
				UserID: &uid, OrganizationID: &orgID, Channels: []string{notifmodel.ChannelInapp},
				TemplateCode: ReminderTemplate, Language: string(loc), ActionURL: &link,
				SourceEvent: ReminderTemplate,
				TemplateVars: map[string]string{
					"plate": b.plate, "vehicle": fallback(b.label, b.plate), "items": reminderItems(loc, b.items),
				},
				Payload: map[string]any{
					"vehicle_uuid": b.uuid.String(), "plate": b.plate, "items": len(b.items),
				},
			})
			if err != nil {
				s.log.Warn("vehicle_reminder_enqueue_failed", "organization_id", org.ID, "user_id", uid, "error", err)
				continue
			}
			sent++
		}
		stats.Notifications += int(sent)
		for _, a := range b.items {
			if err := s.repo.SetReminderRecipients(ctx, org.ID, a.Kind, a.subjectID, today, sent); err != nil {
				s.log.Warn("vehicle_reminder_count_failed", "organization_id", org.ID, "error", err)
			}
		}
	}
	return stats, nil
}

// shouldRemind applies the cadence: new state, milestone day or a week
// since the last reminder.
func shouldRemind(a alert, prev repository.LastReminder, seen bool, today time.Time) bool {
	if !seen || prev.State != a.State {
		return true
	}
	if vehiclemodel.DaysUntil(prev.Day, today) >= reminderRepeatDays {
		return true
	}
	return a.DaysLeft != nil && slices.Contains(reminderMilestones, *a.DaysLeft)
}

// recipientCovers reports whether a member's branch scope covers the
// vehicle (home or current branch, like the vehicle's visibility).
func recipientCovers(r db.ListVehicleReminderRecipientsRow, home, current int64) bool {
	if r.BranchScope != orgctx.BranchScopeSelected {
		return true
	}
	return slices.Contains(r.BranchIds, home) || slices.Contains(r.BranchIds, current)
}

// reminderLink opens the vehicle on the tab of its items (documents unless
// every item is a maintenance plan).
func reminderLink(slug string, vehicleUUID uuid.UUID, items []alert) string {
	tab := "maintenance"
	for _, a := range items {
		if a.Kind == model.AlertDocument {
			tab = "documents"
			break
		}
	}
	return "/t/" + slug + "/fleet/vehicles/" + vehicleUUID.String() + "?tab=" + tab
}

// reminderItems renders the alerts as a localized list.
func reminderItems(loc i18n.Locale, items []alert) string {
	parts := make([]string, 0, len(items))
	for _, a := range items {
		parts = append(parts, ReminderItemText(loc, a.Alert))
	}
	return strings.Join(parts, "; ")
}

// ReminderItemText renders one alert ("Muayene: 3 gün kaldı (17.10.2026)").
func ReminderItemText(loc i18n.Locale, a model.Alert) string {
	date := ""
	if a.DueDate != nil {
		date = formatDay(loc, *a.DueDate)
	}
	if a.Kind == model.AlertDocument {
		label := i18n.Translate(loc, "vehicle_documents.type."+a.Type)
		key := "vehicle_reminders.document_expiring"
		days := 0
		if a.DaysLeft != nil {
			days = *a.DaysLeft
		}
		switch {
		case days < 0:
			key = "vehicle_reminders.document_expired"
		case days == 0:
			key = "vehicle_reminders.document_today"
		}
		return render(i18n.Translate(loc, key), map[string]string{
			"type": label, "days": strconv.Itoa(days), "date": date,
		})
	}
	label := i18n.Translate(loc, "vehicle_maintenance.type."+a.Type)
	targets := []string{}
	if date != "" {
		targets = append(targets, date)
	}
	if a.DueKm != nil {
		targets = append(targets, fmt.Sprintf("%d km", *a.DueKm))
	}
	key := "vehicle_reminders.maintenance_due"
	if a.State == model.AlertOverdue {
		key = "vehicle_reminders.maintenance_overdue"
	}
	return render(i18n.Translate(loc, key), map[string]string{
		"type": label, "target": strings.Join(targets, " / "),
	})
}

func formatDay(loc i18n.Locale, day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	if loc == i18n.LocaleTR {
		return t.Format("02.01.2006")
	}
	return t.Format("2006-01-02")
}

func render(tpl string, vars map[string]string) string {
	for k, v := range vars {
		tpl = strings.ReplaceAll(tpl, "{"+k+"}", v)
	}
	return tpl
}

// RunReminders is the worker entry point of the daily sweep: it returns the
// number of notifications queued.
func (s *Service) RunReminders(ctx context.Context) (int, error) {
	stats, err := s.SendReminders(ctx)
	return stats.Notifications, err
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
