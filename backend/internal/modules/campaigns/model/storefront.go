package model

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

// Storefront contains only editorial presentation; coupon rules remain on Campaign.
type Storefront struct {
	ShowOnSite    bool              `json:"show_on_site"`
	SiteSort      int32             `json:"site_sort"`
	SiteTitle     map[string]string `json:"site_title"`
	SiteSummary   map[string]string `json:"site_summary"`
	SiteCTALabel  map[string]string `json:"site_cta_label"`
	SiteMediaUUID *uuid.UUID        `json:"site_media_uuid"`
}

type StorefrontInput struct {
	ShowOnSite    *bool               `json:"show_on_site"`
	SiteSort      *int32              `json:"site_sort"`
	SiteTitle     *map[string]string  `json:"site_title"`
	SiteSummary   *map[string]string  `json:"site_summary"`
	SiteCTALabel  *map[string]string  `json:"site_cta_label"`
	SiteMediaUUID Optional[uuid.UUID] `json:"site_media_uuid"`
}

// PublicCampaign deliberately excludes eligibility, customer IDs and internal names.
type PublicCampaign struct {
	UUID      uuid.UUID         `json:"uuid"`
	Title     map[string]string `json:"title"`
	Summary   map[string]string `json:"summary"`
	CTALabel  map[string]string `json:"cta_label"`
	MediaURL  *string           `json:"media_url"`
	Code      string            `json:"code,omitempty"`
	StartDate *rentaltime.Date  `json:"start_date"`
	EndDate   *rentaltime.Date  `json:"end_date"`
}
