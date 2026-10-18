package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const ArticleResource = "tenant.site_articles"

// ArticleInput replaces the editorial document. Publication is a separate action.
type ArticleInput struct {
	Version        *int                       `json:"version,omitempty"`
	Slug           map[string]string          `json:"slug"`
	Title          map[string]string          `json:"title"`
	Summary        map[string]string          `json:"summary"`
	CoverMediaUUID *uuid.UUID                 `json:"cover_media_uuid"`
	Body           map[string]json.RawMessage `json:"body"`
	SEOTitle       map[string]string          `json:"seo_title"`
	SEODescription map[string]string          `json:"seo_description"`
	Tags           []string                   `json:"tags"`
	Sort           int32                      `json:"sort"`
	Featured       bool                       `json:"featured"`
}

type Article struct {
	UUID           uuid.UUID                  `json:"uuid"`
	Status         string                     `json:"status"`
	PublishedAt    *time.Time                 `json:"published_at"`
	Slug           map[string]string          `json:"slug"`
	Title          map[string]string          `json:"title"`
	Summary        map[string]string          `json:"summary"`
	CoverMediaUUID *uuid.UUID                 `json:"cover_media_uuid"`
	Body           map[string]json.RawMessage `json:"body"`
	SEOTitle       map[string]string          `json:"seo_title"`
	SEODescription map[string]string          `json:"seo_description"`
	Tags           []string                   `json:"tags"`
	Sort           int32                      `json:"sort"`
	Featured       bool                       `json:"featured"`
	CreatedBy      *int64                     `json:"created_by"`
	UpdatedBy      *int64                     `json:"updated_by"`
	CreatedAt      time.Time                  `json:"created_at"`
	UpdatedAt      time.Time                  `json:"updated_at"`
	Version        int                        `json:"version"`
}

type ArticleCard struct {
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Slug        string    `json:"slug"`
	CoverURL    *string   `json:"cover_url"`
	PublishedAt time.Time `json:"published_at"`
}

type PublicArticle struct {
	ArticleCard
	Locale         string            `json:"locale"`
	Body           json.RawMessage   `json:"body"`
	SEOTitle       string            `json:"seo_title"`
	SEODescription string            `json:"seo_description"`
	Tags           []string          `json:"tags"`
	Featured       bool              `json:"featured"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Media          map[string]string `json:"media"`
}

type SitemapEntry struct {
	Path    string    `json:"path"`
	Locale  string    `json:"locale,omitempty"`
	Lastmod time.Time `json:"lastmod"`
}
