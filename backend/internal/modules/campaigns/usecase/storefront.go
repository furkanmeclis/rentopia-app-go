package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func localized(raw []byte) map[string]string {
	out := map[string]string{"tr": "", "en": ""}
	_ = json.Unmarshal(raw, &out)
	return out
}

func storefront(row db.GetCampaignStorefrontRow) model.Storefront {
	out := model.Storefront{ShowOnSite: row.ShowOnSite, SiteSort: row.SiteSort,
		SiteTitle: localized(row.SiteTitle), SiteSummary: localized(row.SiteSummary), SiteCTALabel: localized(row.SiteCtaLabel)}
	if row.SiteMediaUuid.Valid {
		id := uuid.UUID(row.SiteMediaUuid.Bytes)
		out.SiteMediaUUID = &id
	}
	return out
}

func (s *Service) GetStorefront(ctx context.Context, org int64, id uuid.UUID) (model.Storefront, error) {
	row, err := s.q.GetCampaignStorefront(ctx, db.GetCampaignStorefrontParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Storefront{}, ErrNotFound
	}
	return storefront(row), err
}

func (s *Service) PatchStorefront(ctx context.Context, org int64, id uuid.UUID, in model.StorefrontInput) (model.Storefront, error) {
	current, err := s.GetStorefront(ctx, org, id)
	if err != nil {
		return model.Storefront{}, err
	}
	ve := &ValidationError{}
	normalize := func(field string, input *map[string]string, old map[string]string, limit int) []byte {
		values := old
		if input != nil {
			values = *input
		}
		out := map[string]string{"tr": "", "en": ""}
		for loc, value := range values {
			if loc != "tr" && loc != "en" {
				ve.add(field+"."+loc, "invalid", "unsupported locale")
				continue
			}
			value = strings.TrimSpace(value)
			if utf8.RuneCountInString(value) > limit || strings.ContainsAny(value, "<>") {
				ve.add(field+"."+loc, "invalid", "length-limited plain text required")
			}
			out[loc] = value
		}
		b, _ := json.Marshal(out)
		return b
	}
	params := db.PatchCampaignStorefrontParams{OrganizationID: org, Uuid: id, ShowOnSite: current.ShowOnSite, SiteSort: current.SiteSort,
		SiteTitle:    normalize("site_title", in.SiteTitle, current.SiteTitle, 160),
		SiteSummary:  normalize("site_summary", in.SiteSummary, current.SiteSummary, 1000),
		SiteCtaLabel: normalize("site_cta_label", in.SiteCTALabel, current.SiteCTALabel, 80)}
	if in.ShowOnSite != nil {
		params.ShowOnSite = *in.ShowOnSite
	}
	if in.SiteSort != nil {
		params.SiteSort = *in.SiteSort
	}
	media := current.SiteMediaUUID
	if in.SiteMediaUUID.Set {
		media = in.SiteMediaUUID.Value
	}
	if media != nil {
		if _, err := s.q.GetSiteMedia(ctx, db.GetSiteMediaParams{OrganizationID: org, Uuid: *media}); errors.Is(err, pgx.ErrNoRows) {
			ve.add("site_media_uuid", "not_found", "unknown media")
		} else if err != nil {
			return model.Storefront{}, err
		}
		params.SiteMediaUuid = pgtype.UUID{Bytes: *media, Valid: true}
	}
	if err := ve.orNil(); err != nil {
		return model.Storefront{}, err
	}
	row, err := s.q.PatchCampaignStorefront(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Storefront{}, ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.ConstraintName == "fk_campaign_site_media" {
		ve.add("site_media_uuid", "not_found", "unknown media")
		return model.Storefront{}, ve
	}
	return storefront(db.GetCampaignStorefrontRow(row)), err
}

// PublicCampaigns filters at the database boundary, including channel and Istanbul validity dates.
func PublicCampaigns(ctx context.Context, q *db.Queries, org int64, channel string, id *uuid.UUID) ([]model.PublicCampaign, error) {
	params := db.ListPublicCampaignsParams{OrganizationID: org, Channel: channel}
	if id != nil {
		params.Uuid = pgtype.UUID{Bytes: *id, Valid: true}
	}
	rows, err := q.ListPublicCampaigns(ctx, params)
	if err != nil {
		return nil, err
	}
	out := []model.PublicCampaign{}
	for _, r := range rows {
		c := model.PublicCampaign{UUID: r.Uuid, Title: localized(r.SiteTitle), Summary: localized(r.SiteSummary), CTALabel: localized(r.SiteCtaLabel), Code: r.Code}
		if r.StartDate.Valid {
			d := datePtr(r.StartDate)
			c.StartDate = d
		}
		if r.EndDate.Valid {
			d := datePtr(r.EndDate)
			c.EndDate = d
		}
		if r.SiteUuid.Valid && r.SiteMediaUuid.Valid {
			path := fmt.Sprintf("/v1/public/site/media/%s/%s", uuid.UUID(r.SiteUuid.Bytes), uuid.UUID(r.SiteMediaUuid.Bytes))
			c.MediaURL = &path
		}
		out = append(out, c)
	}
	return out, nil
}
