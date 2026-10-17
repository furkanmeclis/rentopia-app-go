// Package model defines site enquiry and subscription records.
package model

import (
	"github.com/google/uuid"
	"time"
)

const Resource = "tenant.site_leads"

var Kinds = []string{"newsletter", "contact", "corporate", "franchise"}
var Statuses = []string{"new", "in_progress", "done", "spam"}

type Input struct {
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	Company          string `json:"company"`
	City             string `json:"city"`
	Message          string `json:"message"`
	FleetSize        *int32 `json:"fleet_size"`
	Locale           string `json:"locale"`
	ConsentKVKK      bool   `json:"consent_kvkk"`
	ConsentMarketing bool   `json:"consent_marketing"`
	Honeypot         string `json:"honeypot"`
}

type Patch struct {
	Status *string `json:"status"`
	Note   *string `json:"note"`
}

type Lead struct {
	UUID             uuid.UUID  `json:"uuid"`
	Kind             string     `json:"kind"`
	Name             string     `json:"name"`
	Email            string     `json:"email"`
	Phone            string     `json:"phone"`
	Company          string     `json:"company"`
	City             string     `json:"city"`
	Message          string     `json:"message"`
	FleetSize        *int32     `json:"fleet_size"`
	Locale           string     `json:"locale"`
	ConsentKVKK      bool       `json:"consent_kvkk"`
	ConsentMarketing bool       `json:"consent_marketing"`
	SourceHost       string     `json:"source_host"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	HandledBy        *int64     `json:"handled_by"`
	HandledAt        *time.Time `json:"handled_at"`
	Note             string     `json:"note"`
}
