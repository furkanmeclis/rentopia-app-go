package usecase

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// BranchRef names a branch.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// Window is a half-open instant range [Start, End).
type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Overview is GET /v1/tenant/overview. A nil KPI or section means the
// caller lacks the permission it needs: the panel hides it.
type Overview struct {
	GeneratedAt time.Time `json:"generated_at"`
	// Today is the Europe/Istanbul calendar day of GeneratedAt.
	Today Window `json:"today"`
	// BranchScope is the caller's scope: all or selected.
	BranchScope string `json:"branch_scope"`
	// Branch is the applied branch filter (nil = every branch in scope).
	Branch *BranchRef `json:"branch"`
	// Branches are the branches the caller may filter by.
	Branches []BranchRef `json:"branches"`
	KPIs     KPIs        `json:"kpis"`

	TodayOperations *TodayOperations  `json:"today_operations"`
	OverdueReturns  *OverdueReturns   `json:"overdue_returns"`
	Approvals       *WaitingSection   `json:"approvals"`
	AwaitingPayment *WaitingSection   `json:"awaiting_payment"`
	Availability    *ResolutionTasks  `json:"availability_problems"`
	IntegrationJobs *IntegrationJobs  `json:"integration_jobs"`
	Documents       *PendingDocuments `json:"documents"`
}

// KPIs are the key figures; each is nil without its permission.
type KPIs struct {
	Occupancy     *Occupancy     `json:"occupancy"`
	ActiveRentals *ActiveRentals `json:"active_rentals"`
	Collections   *Collections   `json:"collections_today"`
}

// Occupancy is today's fleet utilisation: vehicles out on an active rental
// divided by rentable vehicles.
type Occupancy struct {
	Rented   int64 `json:"rented"`
	Rentable int64 `json:"rentable"`
	// Rate is Rented / Rentable in percent with one decimal (0 without
	// rentable vehicles).
	Rate float64 `json:"rate"`
}

// ActiveRentals counts rentals on the road.
type ActiveRentals struct {
	Active  int64 `json:"active"`
	Overdue int64 `json:"overdue"`
}

// Collections are today's TRY payments: confirmed money, refunds and net.
type Collections struct {
	Currency string      `json:"currency"`
	Count    int64       `json:"count"`
	Amount   money.Money `json:"amount"`
	Refunded money.Money `json:"refunded"`
	Net      money.Money `json:"net"`
}

// TaskCounts splits handover or return tasks by progress.
type TaskCounts struct {
	Total int64 `json:"total"`
	Done  int64 `json:"done"`
	Open  int64 `json:"open"`
}

// TodayOperations are today's handover and return tasks.
type TodayOperations struct {
	Total     int64           `json:"total"`
	Handovers TaskCounts      `json:"handovers"`
	Returns   TaskCounts      `json:"returns"`
	Items     []OperationItem `json:"items"`
}

// RentalRef names a rental.
type RentalRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status,omitempty"`
}

// OperationItem is one handover or return task due today.
type OperationItem struct {
	UUID            uuid.UUID  `json:"uuid"`
	TaskType        string     `json:"task_type"`
	Status          string     `json:"status"`
	DueAt           time.Time  `json:"due_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	Branch          BranchRef  `json:"branch"`
	Rental          RentalRef  `json:"rental"`
	ReservationUUID uuid.UUID  `json:"reservation_uuid"`
	Plate           string     `json:"plate"`
	CustomerName    string     `json:"customer_name"`
}

// OverdueReturns are active rentals past their planned return.
type OverdueReturns struct {
	Total int64         `json:"total"`
	Items []OverdueItem `json:"items"`
}

// OverdueItem is one late rental.
type OverdueItem struct {
	UUID            uuid.UUID  `json:"uuid"`
	Reference       string     `json:"reference"`
	ReservationUUID uuid.UUID  `json:"reservation_uuid"`
	PlannedReturnAt time.Time  `json:"planned_return_at"`
	OverdueAt       *time.Time `json:"overdue_at"`
	LateMinutes     int64      `json:"late_minutes"`
	ReturnBranch    BranchRef  `json:"return_branch"`
	Plate           string     `json:"plate"`
	CustomerName    string     `json:"customer_name"`
}

// WaitingSection is the approval queue or the reservations waiting for
// payment. Overdue counts those whose deadline (review SLA or payment
// deadline) has passed.
type WaitingSection struct {
	Total   int64         `json:"total"`
	Overdue int64         `json:"overdue"`
	Items   []WaitingItem `json:"items"`
}

// WaitingItem is one waiting reservation. Deadline is the review deadline
// (approval queue) or the payment deadline (awaiting payment).
type WaitingItem struct {
	UUID         uuid.UUID   `json:"uuid"`
	Reference    string      `json:"reference"`
	Status       string      `json:"status"`
	Channel      string      `json:"channel"`
	StartsAt     time.Time   `json:"starts_at"`
	EndsAt       time.Time   `json:"ends_at"`
	Deadline     *time.Time  `json:"deadline"`
	PickupBranch BranchRef   `json:"pickup_branch"`
	CustomerName string      `json:"customer_name"`
	Currency     string      `json:"currency"`
	TotalAmount  money.Money `json:"total_amount"`
	// AmountDue is the prepayment still due (prepayment − paid, not
	// below zero).
	AmountDue money.Money `json:"amount_due"`
}

// ResolutionTasks are the open availability problems.
type ResolutionTasks struct {
	Total int64 `json:"total"`
	// ByType counts open tasks per type (late_return, late_payment,
	// block_conflict, ...).
	ByType map[string]int64 `json:"by_type"`
	Items  []ResolutionItem `json:"items"`
}

// ReservationRef names a reservation.
type ReservationRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	StartsAt  time.Time `json:"starts_at"`
}

// ResolutionItem is one open resolution task.
type ResolutionItem struct {
	UUID         uuid.UUID      `json:"uuid"`
	TaskType     string         `json:"task_type"`
	Reason       string         `json:"reason"`
	CreatedAt    time.Time      `json:"created_at"`
	Reservation  ReservationRef `json:"reservation"`
	PickupBranch BranchRef      `json:"pickup_branch"`
}

// IntegrationJobs are the organization's failed or unverified external
// jobs (organization-wide, not branch-bound).
type IntegrationJobs struct {
	Total int64     `json:"total"`
	Items []JobItem `json:"items"`
}

// AccountRef names an integration account.
type AccountRef struct {
	UUID uuid.UUID `json:"uuid"`
	Kind string    `json:"kind"`
	Name string    `json:"name"`
}

// JobItem is one job needing attention. Generic over provider and kind.
type JobItem struct {
	UUID      uuid.UUID   `json:"uuid"`
	Direction string      `json:"direction"`
	Provider  string      `json:"provider"`
	Kind      string      `json:"kind"`
	Status    string      `json:"status"`
	Attempts  int32       `json:"attempts"`
	LastError string      `json:"last_error"`
	RefType   string      `json:"ref_type"`
	RefUUID   *uuid.UUID  `json:"ref_uuid"`
	Account   *AccountRef `json:"account"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// PendingDocuments are rental contracts not approved yet for handovers
// in the window (today's start until now + DocumentWindow).
type PendingDocuments struct {
	// Required is false when no rental contract template is active: the
	// handover then needs no approved contract and the list is empty.
	Required    bool           `json:"required"`
	WindowHours int            `json:"window_hours"`
	Window      Window         `json:"window"`
	Total       int64          `json:"total"`
	Items       []DocumentItem `json:"items"`
}

// DocumentRef names a document.
type DocumentRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Status string    `json:"status"`
}

// DocumentItem is one upcoming handover whose contract is not approved.
// Document is nil when no contract was prepared yet.
type DocumentItem struct {
	Reservation  ReservationRef `json:"reservation"`
	PickupBranch BranchRef      `json:"pickup_branch"`
	CustomerName string         `json:"customer_name"`
	Rental       *RentalRef     `json:"rental"`
	Document     *DocumentRef   `json:"document"`
}

// Badges is GET /v1/tenant/overview/badges (sidebar counts).
type Badges struct {
	Approvals       *ApprovalBadge   `json:"approvals"`
	ResolutionTasks *ResolutionBadge `json:"resolution_tasks"`
}

// ApprovalBadge counts the approval queue.
type ApprovalBadge struct {
	Pending int64 `json:"pending"`
	Overdue int64 `json:"overdue"`
}

// ResolutionBadge counts open resolution tasks.
type ResolutionBadge struct {
	Open int64 `json:"open"`
}
