package model

import (
	"time"

	"github.com/google/uuid"
)

// Domain includes ownership evidence and the records an operator must publish.
type Domain struct {
	UUID              uuid.UUID   `json:"uuid"`
	OrganizationID    int64       `json:"organization_id"`
	Host              string      `json:"host"`
	Status            string      `json:"status"`
	VerificationToken string      `json:"verification_token"`
	LastCheckedAt     *time.Time  `json:"last_checked_at"`
	VerifiedAt        *time.Time  `json:"verified_at"`
	LastError         string      `json:"last_error"`
	TLSStatus         string      `json:"tls_status"`
	Provisioner       string      `json:"provisioner"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	DNSRecords        []DNSRecord `json:"dns_records"`
	TLSInstructions   string      `json:"tls_instructions,omitempty"`
}

type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}
