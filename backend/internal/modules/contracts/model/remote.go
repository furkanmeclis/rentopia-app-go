package model

import (
	"time"

	"github.com/google/uuid"
)

// Remote approval (TEC-711): a time-limited link plus an SMS code.

// MethodRemoteSMS is the approval method of a remote approval.
const MethodRemoteSMS = "remote_sms"

// OTPPurpose is the SMS one-time code purpose of a remote approval; the
// subject is "document_version:<uuid>".
const OTPPurpose = "contract_approval"

// Link statuses. pending links past expires_at answer expired.
const (
	LinkPending    = "pending"
	LinkApproved   = "approved"
	LinkExpired    = "expired"
	LinkRevoked    = "revoked"
	LinkSuperseded = "superseded"
)

// Signer roles.
const (
	SignerRenter = "renter"
	SignerDriver = "driver"
)

// Disclaimer is the required statement on the public page and the
// approval PDF (Turkish first).
const (
	DisclaimerTR = "SMS ve tablet onayı nitelikli elektronik imza değildir."
	DisclaimerEN = "SMS and tablet approval is not a qualified electronic signature."
)

// Signer is a person who can approve a document remotely.
type Signer struct {
	Role        string     `json:"role"`
	DriverUUID  *uuid.UUID `json:"driver_uuid"`
	Name        string     `json:"name"`
	PhoneMasked string     `json:"phone_masked"`
	HasPhone    bool       `json:"has_phone"`
}

// SignerOptions are the people of a document's reservation.
type SignerOptions struct {
	Signers []Signer `json:"signers"`
	// SMSAvailable: the organization has an active SMS account (codes and
	// link messages can be sent).
	SMSAvailable bool `json:"sms_available"`
}

// CreateLinkInput creates a remote approval link for the version shown.
type CreateLinkInput struct {
	ContentHash      string     `json:"content_hash"`
	SignerRole       string     `json:"signer_role"`
	DriverUUID       *uuid.UUID `json:"driver_uuid"`
	ExpiresInMinutes *int       `json:"expires_in_minutes"`
	SendSMS          bool       `json:"send_sms"`
}

// ApprovalLink is a remote approval link as staff see it.
type ApprovalLink struct {
	UUID          uuid.UUID `json:"uuid"`
	VersionUUID   uuid.UUID `json:"version_uuid"`
	VersionNumber int32     `json:"version_number"`
	ContentHash   string    `json:"content_hash"`
	Status        string    `json:"status"`
	SignerRole    string    `json:"signer_role"`
	SignerName    string    `json:"signer_name"`
	PhoneMasked   string    `json:"phone_masked"`
	// URL is the public page (pending links only).
	URL            string     `json:"url,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CodesSent      int32      `json:"codes_sent"`
	FailedAttempts int32      `json:"failed_attempts"`
	SMSSentCount   int32      `json:"sms_sent_count"`
	LastSMSAt      *time.Time `json:"last_sms_at"`
	ApprovedAt     *time.Time `json:"approved_at"`
	ApprovalUUID   *uuid.UUID `json:"approval_uuid"`
	RevokedAt      *time.Time `json:"revoked_at"`
	RevokedReason  string     `json:"revoked_reason"`
	CreatedBy      *Actor     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

// LinkSMS is the result of sending a link by SMS.
type LinkSMS struct {
	Link        ApprovalLink `json:"link"`
	MessageUUID uuid.UUID    `json:"message_uuid"`
	PhoneMasked string       `json:"phone_masked"`
}

// CreatedLink is a new link (and its SMS when one was requested).
type CreatedLink struct {
	Link ApprovalLink `json:"link"`
	// SMS is set when send_sms was requested; SMSError names why the SMS
	// failed (the link exists anyway and can be copied).
	SMS      *LinkSMS `json:"sms,omitempty"`
	SMSError string   `json:"sms_error,omitempty"`
}

// PublicDocument is what the public approval page shows.
type PublicDocument struct {
	State            string     `json:"state"`
	OrganizationName string     `json:"organization_name"`
	Kind             string     `json:"kind"`
	Reference        string     `json:"reference"`
	VersionNumber    int32      `json:"version_number"`
	ContentHash      string     `json:"content_hash"`
	HTML             string     `json:"html,omitempty"`
	SignerName       string     `json:"signer_name"`
	PhoneMasked      string     `json:"phone_masked,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	ApprovedAt       *time.Time `json:"approved_at"`
	PDFAvailable     bool       `json:"pdf_available"`
}

// PublicCode is an issued code (never the code).
type PublicCode struct {
	PhoneMasked string    `json:"phone_masked"`
	ExpiresAt   time.Time `json:"expires_at"`
	ResendAfter time.Time `json:"resend_after"`
}

// PublicApproveInput approves with the code and the hash shown.
type PublicApproveInput struct {
	Code        string `json:"code"`
	ContentHash string `json:"content_hash"`
}

// ClientInfo is the evidence of the approving device.
type ClientInfo struct {
	IP        string
	UserAgent string
}
