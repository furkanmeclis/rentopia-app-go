-- name: ListSiteArticles :many
SELECT * FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
 AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
 AND (sqlc.arg(locale)::text = '' OR COALESCE(slug->>sqlc.arg(locale)::text,'') <> '')
 AND (sqlc.arg(q)::text = '' OR title::text ILIKE '%' || sqlc.arg(q) || '%' OR summary::text ILIKE '%' || sqlc.arg(q) || '%' OR slug::text ILIKE '%' || sqlc.arg(q) || '%')
ORDER BY
 CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN created_at END ASC,
 CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND  sqlc.arg(sort_desc)::boolean THEN created_at END DESC,
 CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN updated_at END ASC,
 CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND  sqlc.arg(sort_desc)::boolean THEN updated_at END DESC,
 CASE WHEN sqlc.arg(sort_field)::text = 'sort' AND NOT sqlc.arg(sort_desc)::boolean THEN sort END ASC,
 CASE WHEN sqlc.arg(sort_field)::text = 'sort' AND  sqlc.arg(sort_desc)::boolean THEN sort END DESC,
 id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountSiteArticles :one
SELECT count(*) FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL
 AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
 AND (sqlc.arg(locale)::text = '' OR COALESCE(slug->>sqlc.arg(locale)::text,'') <> '')
 AND (sqlc.arg(q)::text = '' OR title::text ILIKE '%' || sqlc.arg(q) || '%' OR summary::text ILIKE '%' || sqlc.arg(q) || '%' OR slug::text ILIKE '%' || sqlc.arg(q) || '%');

-- name: GetSiteArticle :one
SELECT * FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL;
-- name: InsertSiteArticle :one
INSERT INTO site_articles (organization_id, slug, title, summary, cover_media_uuid, body, seo_title, seo_description, tags, sort, featured, created_by, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(slug), sqlc.arg(title), sqlc.arg(summary), sqlc.narg(cover_media_uuid), sqlc.arg(body), sqlc.arg(seo_title), sqlc.arg(seo_description), sqlc.arg(tags), sqlc.arg(sort), sqlc.arg(featured), sqlc.narg(actor), sqlc.narg(actor)) RETURNING *;
-- name: UpdateSiteArticle :one
UPDATE site_articles SET slug = sqlc.arg(slug), title = sqlc.arg(title), summary = sqlc.arg(summary), cover_media_uuid = sqlc.narg(cover_media_uuid), body = sqlc.arg(body), seo_title = sqlc.arg(seo_title), seo_description = sqlc.arg(seo_description), tags = sqlc.arg(tags), sort = sqlc.arg(sort), featured = sqlc.arg(featured), updated_by = sqlc.narg(actor), version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND version = sqlc.arg(version) AND deleted_at IS NULL RETURNING *;
-- name: SetSiteArticleStatus :one
UPDATE site_articles SET status = sqlc.arg(status), published_at = CASE WHEN sqlc.arg(status)::text = 'published' THEN now() ELSE NULL END, updated_by = sqlc.narg(actor), version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND version = sqlc.arg(version) AND deleted_at IS NULL RETURNING *;
-- name: DeleteSiteArticle :execrows
UPDATE site_articles SET deleted_at = now(), updated_by = sqlc.narg(actor), version = version + 1
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND version = sqlc.arg(version) AND deleted_at IS NULL;
-- name: ListPublicSiteArticles :many
SELECT * FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL AND status = 'published'
 AND COALESCE(slug->>sqlc.arg(locale)::text,'') <> ''
ORDER BY published_at DESC, id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
-- name: CountPublicSiteArticles :one
SELECT count(*) FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL AND status = 'published' AND COALESCE(slug->>sqlc.arg(locale)::text,'') <> '';
-- name: GetPublicSiteArticle :one
SELECT * FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL AND status = 'published' AND slug->>sqlc.arg(locale)::text = sqlc.arg(slug)::text;
-- name: SiteMediaHasPublicArticle :one
SELECT EXISTS (SELECT 1 FROM site_articles a WHERE a.organization_id = sqlc.arg(organization_id) AND a.deleted_at IS NULL AND a.status = 'published'
 AND (a.cover_media_uuid = sqlc.arg(media_uuid)::uuid OR EXISTS (SELECT 1 FROM site_article_media_ids(a.body) ref WHERE ref = sqlc.arg(media_uuid)::uuid::text)));
-- name: ListSiteSitemapArticles :many
SELECT slug, updated_at FROM site_articles WHERE organization_id = sqlc.arg(organization_id) AND deleted_at IS NULL AND status = 'published' ORDER BY id;
-- name: ListSiteSitemapCatalog :many
SELECT ('/branches/' || b.uuid::text)::text AS path, b.updated_at FROM branches b
WHERE b.organization_id = sqlc.arg(organization_id) AND b.deleted_at IS NULL AND b.is_active AND b.show_online
UNION ALL
SELECT ('/vehicles?segment=' || s.name::text)::text AS path, max(p.updated_at)::timestamptz AS updated_at FROM rental_products p
JOIN vehicle_segments s ON s.organization_id = p.organization_id AND s.id = p.segment_id
JOIN rental_product_versions v ON v.organization_id = p.organization_id AND v.product_id = p.id AND v.version = p.current_version
WHERE v.organization_id = sqlc.arg(organization_id) AND p.organization_id = sqlc.arg(organization_id) AND s.organization_id = sqlc.arg(organization_id)
 AND p.deleted_at IS NULL AND p.is_active AND p.show_online AND p.status = 'published' GROUP BY s.name;
