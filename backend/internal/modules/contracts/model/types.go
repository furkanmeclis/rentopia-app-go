// Package model holds the types of contract templates, documents, their
// immutable versions and approvals (TEC-710) and the placeholder catalog.
// docs/design/contracts.md is the design.
package model

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Live list / activity resources.
const (
	ResourceTemplates = "tenant.contract_templates"
	ResourceDocuments = "tenant.documents"
)

// Document / template kinds.
const (
	KindRentalContract = "rental_contract"
	KindHandoverForm   = "handover_form"
	KindReturnForm     = "return_form"
	KindTerms          = "terms"
)

// Kinds in display order.
var Kinds = []string{KindRentalContract, KindHandoverForm, KindReturnForm, KindTerms}

// Template statuses.
const (
	TemplateDraft    = "draft"
	TemplateActive   = "active"
	TemplateArchived = "archived"
)

// TemplateStatuses in display order.
var TemplateStatuses = []string{TemplateDraft, TemplateActive, TemplateArchived}

// Document statuses.
const (
	DocumentDraft    = "draft"
	DocumentApproved = "approved"
	DocumentVoid     = "void"
)

// DocumentStatuses in display order.
var DocumentStatuses = []string{DocumentDraft, DocumentApproved, DocumentVoid}

// Version statuses.
const (
	VersionDraft      = "draft"
	VersionIssued     = "issued"
	VersionApproved   = "approved"
	VersionSuperseded = "superseded"
)

// PDF render statuses.
const (
	PDFNone    = "none"
	PDFPending = "pending"
	PDFReady   = "ready"
	PDFFailed  = "failed"
)

// Approval parties and methods.
const (
	PartyCustomer = "customer"
	PartyStaff    = "staff"

	MethodInPerson  = "in_person"
	MethodClick     = "click"
	MethodSignature = "signature"
)

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Placeholder is one {{key}} a template may use.
type Placeholder struct {
	Key   string `json:"key"`
	Group string `json:"group"`
	// HTML placeholders render a server-built table (drivers, extras).
	HTML   bool   `json:"html"`
	Sample string `json:"sample"`
}

// Template is a contract template version.
type Template struct {
	UUID         uuid.UUID       `json:"uuid"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Version      int32           `json:"version"`
	Status       string          `json:"status"`
	Content      json.RawMessage `json:"content,omitempty"`
	BodyHTML     string          `json:"body_html,omitempty"`
	Placeholders []string        `json:"placeholders"`
	UsedBy       int64           `json:"used_by"`
	ActivatedAt  *time.Time      `json:"activated_at"`
	ArchivedAt   *time.Time      `json:"archived_at"`
	CreatedBy    *Actor          `json:"created_by"`
	ActivatedBy  *Actor          `json:"activated_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// TemplateInput is the body of a template create / update.
type TemplateInput struct {
	Kind     string          `json:"kind"`
	Name     *string         `json:"name"`
	Content  json.RawMessage `json:"content"`
	BodyHTML *string         `json:"body_html"`
}

// PreviewInput renders a body (saved or not) with sample data or with the
// data of a reservation.
type PreviewInput struct {
	Kind            string     `json:"kind"`
	BodyHTML        string     `json:"body_html"`
	ReservationUUID *uuid.UUID `json:"reservation_uuid"`
}

// Preview is a rendered body.
type Preview struct {
	HTML string `json:"html"`
	// Unknown are tokens the catalog does not know (highlighted).
	Unknown []string `json:"unknown"`
	// Empty are known placeholders without a value in the data.
	Empty []string `json:"empty"`
}

// TemplateFilter selects a page of templates.
type TemplateFilter struct {
	Q             string
	Kinds         []string
	Statuses      []string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// TemplateSortable maps the sortable template list fields.
var TemplateSortable = map[string]string{"name": "name", "version": "version", "created_at": "created_at"}

// Document is a document with its versions and approvals.
type Document struct {
	UUID        uuid.UUID     `json:"uuid"`
	Kind        string        `json:"kind"`
	Status      string        `json:"status"`
	Reservation Ref           `json:"reservation"`
	Rental      *Ref          `json:"rental"`
	Current     *VersionInfo  `json:"current_version"`
	Versions    []VersionInfo `json:"versions"`
	Approvals   []Approval    `json:"approvals"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Ref is a related record.
type Ref struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status,omitempty"`
}

// VersionInfo is one version without its HTML.
type VersionInfo struct {
	UUID            uuid.UUID  `json:"uuid"`
	Version         int32      `json:"version"`
	Status          string     `json:"status"`
	TemplateUUID    *uuid.UUID `json:"template_uuid"`
	TemplateName    string     `json:"template_name"`
	TemplateVersion int32      `json:"template_version"`
	ContentHash     string     `json:"content_hash"`
	ChangedKeys     []string   `json:"changed_keys"`
	PDFStatus       string     `json:"pdf_status"`
	PDFError        string     `json:"pdf_error"`
	PDFSizeBytes    int64      `json:"pdf_size_bytes"`
	IssuedAt        *time.Time `json:"issued_at"`
	SupersededAt    *time.Time `json:"superseded_at"`
	CreatedBy       *Actor     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Version is one version with its HTML and source data.
type Version struct {
	VersionInfo
	HTML         string          `json:"html"`
	DataSnapshot json.RawMessage `json:"data_snapshot"`
	PDFSHA256    string          `json:"pdf_sha256"`
}

// Approval is a recorded approval.
type Approval struct {
	UUID              uuid.UUID  `json:"uuid"`
	VersionUUID       uuid.UUID  `json:"version_uuid"`
	VersionNumber     int32      `json:"version_number"`
	ContentHash       string     `json:"content_hash"`
	Party             string     `json:"party"`
	Method            string     `json:"method"`
	ApproverName      string     `json:"approver_name"`
	Note              string     `json:"note"`
	ApprovedAt        time.Time  `json:"approved_at"`
	Valid             bool       `json:"valid"`
	InvalidatedAt     *time.Time `json:"invalidated_at"`
	InvalidatedReason string     `json:"invalidated_reason"`
	RecordedBy        *Actor     `json:"recorded_by"`
	// Remote approvals (method remote_sms): the verified number (masked),
	// the signer role, the link and the device evidence.
	VerifiedPhoneMasked string     `json:"verified_phone_masked"`
	SignerRole          string     `json:"signer_role"`
	LinkUUID            *uuid.UUID `json:"link_uuid"`
	IPAddress           string     `json:"ip_address"`
	UserAgent           string     `json:"user_agent"`
}

// DocumentListItem is a row of the documents list.
type DocumentListItem struct {
	UUID             uuid.UUID  `json:"uuid"`
	Kind             string     `json:"kind"`
	Status           string     `json:"status"`
	Reservation      Ref        `json:"reservation"`
	Rental           *Ref       `json:"rental"`
	VersionUUID      *uuid.UUID `json:"version_uuid"`
	VersionNumber    int32      `json:"version_number"`
	VersionStatus    string     `json:"version_status"`
	ContentHash      string     `json:"content_hash"`
	PDFStatus        string     `json:"pdf_status"`
	TemplateName     string     `json:"template_name"`
	TemplateVersion  int32      `json:"template_version"`
	VersionCreatedAt *time.Time `json:"version_created_at"`
	// RemoteLinkStatus is the status of the latest remote approval link
	// ("" = none).
	RemoteLinkStatus string    `json:"remote_link_status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DocumentFilter selects a page of documents.
type DocumentFilter struct {
	Q                string
	Kinds            []string
	Statuses         []string
	ReservationUUIDs []uuid.UUID
	RentalUUIDs      []uuid.UUID
	Limit, Offset    int32
}

// PrepareInput creates or refreshes the document of one kind.
type PrepareInput struct {
	Kind            string     `json:"kind"`
	ReservationUUID *uuid.UUID `json:"reservation_uuid"`
}

// ApproveInput records an approval of the exact content shown.
type ApproveInput struct {
	ContentHash  string `json:"content_hash"`
	Party        string `json:"party"`
	Method       string `json:"method"`
	ApproverName string `json:"approver_name"`
	Note         string `json:"note"`
}

// PrepareResult is what a prepare / refresh did.
type PrepareResult struct {
	Document Document `json:"document"`
	// Created: a new version was rendered (false = the content is the
	// same as the current version).
	Created bool `json:"created"`
}

// Split / parse helpers for query values.

// SplitList splits a comma-joined list.
func SplitList(raw string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Allowed keeps the values in set.
func Allowed(values, set []string) []string {
	out := []string{}
	for _, v := range values {
		for _, a := range set {
			if v == a {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// UUIDs parses a comma-joined uuid list (invalid entries dropped).
func UUIDs(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range SplitList(raw) {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// Contains reports whether set holds v.
func Contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
