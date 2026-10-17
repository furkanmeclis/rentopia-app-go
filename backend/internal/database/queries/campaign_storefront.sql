-- name: GetCampaignStorefront :one
SELECT show_on_site, site_sort, site_title, site_summary, site_cta_label, site_media_uuid
FROM campaigns WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;

-- name: PatchCampaignStorefront :one
UPDATE campaigns SET show_on_site = sqlc.arg(show_on_site), site_sort = sqlc.arg(site_sort),
 site_title = sqlc.arg(site_title), site_summary = sqlc.arg(site_summary), site_cta_label = sqlc.arg(site_cta_label), site_media_uuid = sqlc.narg(site_media_uuid)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING show_on_site, site_sort, site_title, site_summary, site_cta_label, site_media_uuid;

-- name: ListPublicCampaigns :many
SELECT c.uuid, c.site_title, c.site_summary, c.site_cta_label, c.site_media_uuid, c.start_date, c.end_date,
 COALESCE(CASE WHEN c.campaign_type = 'coupon' AND (c.customer_uuids IS NULL OR cardinality(c.customer_uuids) = 0)
  THEN c.code ELSE NULL END, '')::text AS code,
 s.uuid AS site_uuid
FROM campaigns c
LEFT JOIN site_settings s ON s.organization_id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL AND c.is_active AND c.show_on_site
 AND (c.start_date IS NULL OR c.start_date <= (now() AT TIME ZONE 'Europe/Istanbul')::date)
 AND (c.end_date IS NULL OR c.end_date >= (now() AT TIME ZONE 'Europe/Istanbul')::date)
 AND (c.usage_limit IS NULL OR c.used_count < c.usage_limit)
 AND (c.channels IS NULL OR sqlc.arg(channel)::varchar = ANY(c.channels))
 AND (sqlc.narg(uuid)::uuid IS NULL OR c.uuid = sqlc.narg(uuid)::uuid)
ORDER BY c.site_sort, c.id;

-- name: SiteMediaHasPublicCampaign :one
SELECT EXISTS (SELECT 1 FROM campaigns c WHERE c.organization_id = sqlc.arg(organization_id)
 AND c.site_media_uuid = sqlc.arg(media_uuid) AND c.deleted_at IS NULL AND c.show_on_site AND c.is_active
 AND (c.start_date IS NULL OR c.start_date <= (now() AT TIME ZONE 'Europe/Istanbul')::date)
 AND (c.end_date IS NULL OR c.end_date >= (now() AT TIME ZONE 'Europe/Istanbul')::date)
 AND (c.usage_limit IS NULL OR c.used_count < c.usage_limit)
 AND (c.channels IS NULL OR 'web' = ANY(c.channels)));
