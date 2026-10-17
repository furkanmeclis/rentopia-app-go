-- name: InsertSiteLead :one
INSERT INTO site_leads (organization_id, kind, name, email, phone, company, city, message, fleet_size,
 locale, consent_kvkk, consent_marketing, source_host, ip_hash, user_agent)
VALUES (sqlc.arg(organization_id),sqlc.arg(kind),sqlc.arg(name),sqlc.arg(email),sqlc.arg(phone),sqlc.arg(company),sqlc.arg(city),
 sqlc.arg(message),sqlc.narg(fleet_size),sqlc.arg(locale),sqlc.arg(consent_kvkk),sqlc.arg(consent_marketing),sqlc.arg(source_host),sqlc.arg(ip_hash),sqlc.arg(user_agent))
ON CONFLICT (organization_id, lower(email)) WHERE kind = 'newsletter' DO NOTHING RETURNING *;

-- name: GetSiteLead :one
SELECT * FROM site_leads WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: PatchSiteLead :one
UPDATE site_leads SET status = COALESCE(sqlc.narg(status)::text,status), note = COALESCE(sqlc.narg(note)::text,note),
 handled_by = sqlc.narg(handled_by), handled_at = now()
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) RETURNING *;

-- name: ListSiteLeads :many
SELECT * FROM site_leads WHERE organization_id = sqlc.arg(organization_id)
 AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
 AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
 AND (sqlc.narg(date_from)::timestamptz IS NULL OR created_at >= sqlc.narg(date_from))
 AND (sqlc.narg(date_to)::timestamptz IS NULL OR created_at < sqlc.narg(date_to))
 AND (sqlc.arg(q)::text = '' OR name ILIKE '%' || sqlc.arg(q) || '%' OR email ILIKE '%' || sqlc.arg(q) || '%' OR company ILIKE '%' || sqlc.arg(q) || '%')
ORDER BY
 CASE WHEN sqlc.arg(sort_desc)::boolean THEN created_at END DESC,
 CASE WHEN NOT sqlc.arg(sort_desc)::boolean THEN created_at END ASC,
 id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountSiteLeads :one
SELECT count(*) FROM site_leads WHERE organization_id = sqlc.arg(organization_id)
 AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
 AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
 AND (sqlc.narg(date_from)::timestamptz IS NULL OR created_at >= sqlc.narg(date_from))
 AND (sqlc.narg(date_to)::timestamptz IS NULL OR created_at < sqlc.narg(date_to))
 AND (sqlc.arg(q)::text = '' OR name ILIKE '%' || sqlc.arg(q) || '%' OR email ILIKE '%' || sqlc.arg(q) || '%' OR company ILIKE '%' || sqlc.arg(q) || '%');

-- name: SiteLeadRecipients :many
SELECT DISTINCT m.user_id FROM organization_members m
JOIN organization_member_roles mr ON mr.organization_id = m.organization_id AND mr.member_id = m.id
JOIN role_permissions rp ON rp.role_id = mr.role_id
JOIN permissions p ON p.id = rp.permission_id AND p.slug = 'tenant.site_leads.read'
JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
WHERE m.organization_id = sqlc.arg(organization_id);
