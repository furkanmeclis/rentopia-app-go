// Package model holds the KABİS notification types (TEC-726, RT-A21,
// docs/design/kabis.md).
package model

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Resource is the live list / activity resource of KABİS notifications.
const Resource = "tenant.kabis_notifications"

// Job kinds (integration_jobs.kind): one job per notification.
const (
	KindRentalStart = "kabis.rental_start"
	KindRentalEnd   = "kabis.rental_end"
	KindCorrection  = "kabis.correction"
)

// KindOf maps an event to its job kind.
func KindOf(event string) string { return "kabis." + event }

// Causes: what produced a notification.
const (
	CauseHandover       = "handover"
	CauseReturn         = "return"
	CauseDriversChanged = "drivers_changed"
	CauseVehicleSwap    = "vehicle_swap"
)

// Hold reasons of a job waiting on a dependency.
const (
	HoldNoAccount          = "no_account"
	HoldAdapterUnverified  = "adapter_unverified"
	HoldAdapterUnavailable = "adapter_unavailable"
)

// Statuses are the job statuses a notification shows (integration_jobs).
var Statuses = []string{
	"blocked", "waiting_dependency", "pending", "submitted", "needs_verification", "failed",
	"succeeded", "reported_manually", "reconciled",
}

// NeedsActionStatuses need a person (nav badge, "needs action" filter).
var NeedsActionStatuses = []string{"blocked", "waiting_dependency", "failed", "needs_verification"}

// OfficialStatuses are the only statuses that mean the authority has the
// notification: a provider confirmation, or a staff resolution with a
// reference. Everything else is a local record ("not reported yet").
var OfficialStatuses = []string{"succeeded", "reported_manually", "reconciled"}

// Events.
var Events = []string{"rental_start", "rental_end", "correction"}

// Resolutions a person records.
const (
	ResolutionManual     = "reported_manually"
	ResolutionReconciled = "reconciled"
)

// Contains reports whether list holds v.
func Contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Ref is a small reference.
type Ref struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code,omitempty"`
	Name      string    `json:"name,omitempty"`
	Reference string    `json:"reference,omitempty"`
}

// Actor is a user reference.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Notification is one KABİS notification as lists show it. Status is the
// integration job's status; Official is true only when the authority has
// the record (OfficialStatuses): a pending record is never an official
// notification.
type Notification struct {
	UUID                uuid.UUID  `json:"uuid"`
	Event               string     `json:"event"`
	Sequence            int32      `json:"sequence"`
	Cause               string     `json:"cause"`
	OccurredAt          time.Time  `json:"occurred_at"`
	Plate               string     `json:"plate"`
	Status              string     `json:"status"`
	Official            bool       `json:"official"`
	NeedsAction         bool       `json:"needs_action"`
	Attempts            int32      `json:"attempts"`
	MaxAttempts         int32      `json:"max_attempts"`
	LastError           string     `json:"last_error"`
	Provider            string     `json:"provider"`
	ExternalReference   string     `json:"external_reference"`
	NextAttemptAt       *time.Time `json:"next_attempt_at"`
	SubmittedAt         *time.Time `json:"submitted_at"`
	CompletedAt         *time.Time `json:"completed_at"`
	LastVerifiedAt      *time.Time `json:"last_verified_at"`
	MissingFields       []string   `json:"missing_fields"`
	HoldReason          string     `json:"hold_reason"`
	Resolution          *string    `json:"resolution"`
	ResolutionReference string     `json:"resolution_reference"`
	ResolvedAt          *time.Time `json:"resolved_at"`
	Rental              Ref        `json:"rental"`
	Branch              Ref        `json:"branch"`
	JobUUID             uuid.UUID  `json:"job_uuid"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// Attachment is the file of a staff resolution (streamed by the API).
type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// Attempt is one provider call of the job.
type Attempt struct {
	UUID            uuid.UUID      `json:"uuid"`
	AttemptNo       int32          `json:"attempt_no"`
	Operation       string         `json:"operation"`
	Trigger         string         `json:"trigger"`
	Outcome         string         `json:"outcome"`
	RequestSummary  map[string]any `json:"request_summary"`
	ResponseSummary map[string]any `json:"response_summary"`
	Error           string         `json:"error"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      time.Time      `json:"finished_at"`
	DurationMs      int32          `json:"duration_ms"`
}

// Actions says what the caller's state allows (the UI hides the rest).
type Actions struct {
	Retry       bool `json:"retry"`
	MarkManual  bool `json:"mark_manual"`
	Reconcile   bool `json:"reconcile"`
	Revalidates bool `json:"revalidates"`
}

// Detail is a notification with its masked facts and attempt history.
type Detail struct {
	Notification
	// Summary holds the masked facts (names, masked identity / licence
	// numbers, plate, place, time). Never a full identity number.
	Summary          map[string]any `json:"summary"`
	CorrectsUUID     *uuid.UUID     `json:"corrects_uuid"`
	ResolutionReason string         `json:"resolution_reason"`
	ResolvedBy       *Actor         `json:"resolved_by"`
	Attachment       *Attachment    `json:"attachment"`
	Account          *Ref           `json:"account"`
	Actions          Actions        `json:"actions"`
	AttemptLog       []Attempt      `json:"attempt_log"`
}

// Facets are the status / event counts (branch scope applied).
type Facets struct {
	Statuses    map[string]int64 `json:"statuses"`
	Events      map[string]int64 `json:"events"`
	NeedsAction int64            `json:"needs_action"`
	Total       int64            `json:"total"`
}

// ListFilter selects a page of notifications.
type ListFilter struct {
	Q             string
	Statuses      []string
	Events        []string
	RentalUUID    *uuid.UUID
	From, To      *time.Time
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// QueryKeys are the list filters.
var QueryKeys = []string{"status", "event", "rental", "needs_action", "from", "to"}

// Sortable maps sort fields (apiquery whitelist).
var Sortable = map[string]string{"occurred_at": "occurred_at", "created_at": "created_at", "updated_at": "updated_at"}

// QueryError is an invalid list filter.
type QueryError struct{ Field, Message string }

func (e *QueryError) Error() string { return e.Message }

func split(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func istanbul() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*3600)
}

func parseTime(field, v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		u := t.UTC()
		return &u, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, istanbul()); err == nil {
		// A day: from its start; "to" includes the whole day.
		if field == "to" {
			t = t.AddDate(0, 0, 1)
		}
		u := t.UTC()
		return &u, nil
	}
	return nil, &QueryError{Field: field, Message: field + " must be RFC 3339 or YYYY-MM-DD"}
}

// FilterFromQuery parses the list filters.
func FilterFromQuery(v map[string]string) (ListFilter, error) {
	f := ListFilter{Q: strings.TrimSpace(v["q"])}
	for _, s := range split(v["status"]) {
		if !Contains(Statuses, s) {
			return f, &QueryError{Field: "status", Message: fmt.Sprintf("unknown status %q", s)}
		}
		f.Statuses = append(f.Statuses, s)
	}
	if b := strings.TrimSpace(v["needs_action"]); b == "1" || b == "true" {
		if len(f.Statuses) == 0 {
			f.Statuses = append(f.Statuses, NeedsActionStatuses...)
		} else {
			keep := []string{}
			for _, s := range f.Statuses {
				if Contains(NeedsActionStatuses, s) {
					keep = append(keep, s)
				}
			}
			if len(keep) == 0 {
				keep = []string{"-"}
			}
			f.Statuses = keep
		}
	}
	for _, e := range split(v["event"]) {
		if !Contains(Events, e) {
			return f, &QueryError{Field: "event", Message: fmt.Sprintf("unknown event %q", e)}
		}
		f.Events = append(f.Events, e)
	}
	if r := strings.TrimSpace(v["rental"]); r != "" {
		id, err := uuid.Parse(r)
		if err != nil {
			return f, &QueryError{Field: "rental", Message: "rental must be a uuid"}
		}
		f.RentalUUID = &id
	}
	var err error
	if f.From, err = parseTime("from", v["from"]); err != nil {
		return f, err
	}
	if f.To, err = parseTime("to", v["to"]); err != nil {
		return f, err
	}
	return f, nil
}

// ResolveInput is a staff resolution (mark manual / reconcile).
type ResolveInput struct {
	Reason    string
	Reference string
	File      *Upload
}

// Upload is an attached file.
type Upload struct {
	Body     io.Reader
	Size     int64
	Filename string
}

// RetryResult says what a retry did.
type RetryResult struct {
	// revalidated | already_done | in_progress | resubmitted
	Action string `json:"action"`
	Status string `json:"status"`
}
