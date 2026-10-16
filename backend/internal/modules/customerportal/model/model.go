// Package model holds the customer portal types (TEC-715,
// docs/design/customer-portal.md): sign-in, the customer session, the
// customer's view of a reservation, and customer requests (change /
// cancellation) that staff resolve.
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// OTPPurpose is the one-time code purpose of a portal sign-in; the subject
// is "reservation:<uuid>".
const OTPPurpose = "customer_portal"

// Resources (activity entries, live lists).
const (
	// ResourcePortal is the activity resource of sign-in events (resource
	// uuid = the reservation).
	ResourcePortal = "tenant.customer_portal"
	// ResourceRequests is the live list / activity resource of customer
	// requests (read: tenant.reservations.read).
	ResourceRequests = "tenant.customer_requests"
)

// Session roles: who signed in.
const (
	RoleRenter = "renter"
	RoleDriver = "driver"
)

// Contact channels.
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
)

// Request types and statuses.
const (
	RequestChange = "change"
	RequestCancel = "cancel"

	RequestOpen     = "open"
	RequestResolved = "resolved"
	RequestRejected = "rejected"

	OutcomeSelfCancelled = "self_cancelled"
)

// RequestTypes / RequestStatuses list the values in display order.
var (
	RequestTypes    = []string{RequestChange, RequestCancel}
	RequestStatuses = []string{RequestOpen, RequestResolved, RequestRejected}
)

// ClientInfo is the caller's address and user agent (audit).
type ClientInfo struct {
	IP        string
	UserAgent string
}

// StartInput is POST /v1/customer/{orgSlug}/auth/start.
type StartInput struct {
	Reference string `json:"reference"`
	// Contact is the phone number or e-mail address on file.
	Contact string `json:"contact"`
}

// VerifyInput is POST /v1/customer/{orgSlug}/auth/verify.
type VerifyInput struct {
	Reference string `json:"reference"`
	Contact   string `json:"contact"`
	Code      string `json:"code"`
}

// StartResult is the generic answer of a sign-in start: the same whether
// the reference / contact matched or not.
type StartResult struct {
	Status string `json:"status"`
	// Channel is the channel the code goes to when the details match
	// (derived from what the customer typed, never from the records).
	Channel            string `json:"channel"`
	ResendAfterSeconds int    `json:"resend_after_seconds"`
	CodeTTLSeconds     int    `json:"code_ttl_seconds"`
}

// Organization names the portal's organization.
type Organization struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// Session is the signed-in customer as the portal shows it.
type Session struct {
	Organization  Organization `json:"organization"`
	Role          string       `json:"role"`
	DisplayName   string       `json:"display_name"`
	Channel       string       `json:"channel"`
	ContactMasked string       `json:"contact_masked"`
	ExpiresAt     time.Time    `json:"expires_at"`
	HardExpiresAt time.Time    `json:"hard_expires_at"`
}

// SessionIssued is a new session: Token is the bearer the BFF stores in an
// HttpOnly cookie (and strips from the browser answer).
type SessionIssued struct {
	SessionToken string  `json:"session_token"`
	Session      Session `json:"session"`
}

// Price is the money summary of a reservation.
type Price struct {
	Currency      string      `json:"currency"`
	Total         money.Money `json:"total"`
	Prepayment    money.Money `json:"prepayment"`
	Paid          money.Money `json:"paid"`
	PrepaymentDue money.Money `json:"prepayment_due"`
	Balance       money.Money `json:"balance"`
}

// ReservationSummary is a row of the customer's reservations.
type ReservationSummary struct {
	UUID          uuid.UUID `json:"uuid"`
	Reference     string    `json:"reference"`
	Status        string    `json:"status"`
	PaymentStatus string    `json:"payment_status"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	PickupBranch  string    `json:"pickup_branch"`
	ReturnBranch  string    `json:"return_branch"`
	Product       string    `json:"product"`
	Price         Price     `json:"price"`
}

// Extra is one extra service line of the reservation.
type Extra struct {
	Name     string      `json:"name"`
	Quantity int         `json:"quantity"`
	Amount   money.Money `json:"amount"`
}

// Policies are the policy texts of the reservation's policy snapshot.
type Policies struct {
	Cancellation string `json:"cancellation"`
	NoShow       string `json:"no_show"`
	EarlyReturn  string `json:"early_return"`
	Extension    string `json:"extension"`
	Fuel         string `json:"fuel"`
	Mileage      string `json:"mileage"`
}

// Cancellation is what the cancel request shows: the policy text and the
// fee of the snapshot (when the snapshot names one), and whether the
// snapshot lets the customer cancel without staff.
type Cancellation struct {
	PolicyText string       `json:"policy_text"`
	Fee        *money.Money `json:"fee"`
	SelfCancel bool         `json:"self_cancel"`
}

// Actions are what the customer may do now.
type Actions struct {
	Pay           bool         `json:"pay"`
	PayAmount     *money.Money `json:"pay_amount"`
	RequestChange bool         `json:"request_change"`
	RequestCancel bool         `json:"request_cancel"`
}

// Document is a document of the reservation (contract, handover / return
// forms). Downloads go through DownloadPath (the API streams the PDF).
type Document struct {
	UUID          uuid.UUID `json:"uuid"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	VersionNumber int32     `json:"version_number"`
	PDFAvailable  bool      `json:"pdf_available"`
	// DownloadPath is the API path of the PDF (relative to /v1).
	DownloadPath string `json:"download_path,omitempty"`
	// ApprovalURL is the remote approval page of an open link for the
	// signed-in person (when one exists).
	ApprovalURL string     `json:"approval_url,omitempty"`
	ApprovalDue *time.Time `json:"approval_due,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Request is a customer request as the customer sees it.
type Request struct {
	UUID              uuid.UUID    `json:"uuid"`
	Type              string       `json:"type"`
	Status            string       `json:"status"`
	Message           string       `json:"message"`
	RequestedStartsAt *time.Time   `json:"requested_starts_at"`
	RequestedEndsAt   *time.Time   `json:"requested_ends_at"`
	PolicyText        string       `json:"policy_text"`
	CancellationFee   *money.Money `json:"cancellation_fee"`
	Currency          string       `json:"currency"`
	Outcome           string       `json:"outcome"`
	ResolutionNote    string       `json:"resolution_note"`
	ResolvedAt        *time.Time   `json:"resolved_at"`
	CreatedAt         time.Time    `json:"created_at"`
}

// Reservation is the customer's view of one reservation.
type Reservation struct {
	ReservationSummary
	PickupPoint     string       `json:"pickup_point"`
	ReturnPoint     string       `json:"return_point"`
	Vehicle         string       `json:"vehicle"`
	Drivers         []string     `json:"drivers"`
	Extras          []Extra      `json:"extras"`
	PaymentDeadline *time.Time   `json:"payment_deadline"`
	Policies        Policies     `json:"policies"`
	Cancellation    Cancellation `json:"cancellation"`
	Actions         Actions      `json:"actions"`
	Documents       []Document   `json:"documents"`
	// Invoices are the reservation's issued invoices (renter sessions only;
	// TEC-722).
	Invoices []Invoice `json:"invoices"`
	Requests []Request `json:"requests"`
}

// Invoice is an issued invoice of the reservation, with the API path of
// its PDF (relative to /v1).
type Invoice struct {
	UUID         uuid.UUID   `json:"uuid"`
	Number       string      `json:"number"`
	Profile      string      `json:"profile"`
	Type         string      `json:"invoice_type"`
	IssueDate    string      `json:"issue_date"`
	Payable      money.Money `json:"payable_amount"`
	Currency     string      `json:"currency"`
	DownloadPath string      `json:"download_path"`
}

// PayResult is the pay page link of "pay remaining".
type PayResult struct {
	URL       string      `json:"url"`
	Amount    money.Money `json:"amount"`
	Currency  string      `json:"currency"`
	ExpiresAt time.Time   `json:"expires_at"`
	Reused    bool        `json:"reused"`
}

// CreateRequestInput is POST …/reservations/{uuid}/requests.
type CreateRequestInput struct {
	Type              string     `json:"type"`
	Message           string     `json:"message"`
	RequestedStartsAt *time.Time `json:"requested_starts_at"`
	RequestedEndsAt   *time.Time `json:"requested_ends_at"`
}

// Actor is a staff member.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ReservationRef names the reservation of a request.
type ReservationRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

// StaffRequest is a customer request in the panel.
type StaffRequest struct {
	Request
	Reservation   ReservationRef `json:"reservation"`
	CustomerUUID  uuid.UUID      `json:"customer_uuid"`
	CustomerName  string         `json:"customer_name"`
	RequesterRole string         `json:"requester_role"`
	RequesterName string         `json:"requester_name"`
	ContactMasked string         `json:"contact_masked"`
	ResolvedBy    *Actor         `json:"resolved_by"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// RequestFilter selects a page of requests.
type RequestFilter struct {
	Statuses         []string
	Types            []string
	ReservationUUIDs []uuid.UUID
	Q                string
	SortDesc         bool
	Limit, Offset    int32
}

// ResolveInput is POST /v1/tenant/customer-requests/{uuid}/resolve.
type ResolveInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// RequestStats feeds the nav badge.
type RequestStats struct {
	Open int64 `json:"open"`
}
