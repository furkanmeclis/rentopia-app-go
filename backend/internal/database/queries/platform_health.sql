-- Platform usage and connection health (TEC-739). Platform-wide reads:
-- every query below spans all organizations on purpose and is mounted
-- only behind platform.health.read; per-organization figures are grouped
-- by organization_id.

-- name: ListOrganizationUsage :many
-- One row per organization: active vehicles (not deleted, active, not out
-- of service), reservations created in [month_start, month_end), members,
-- the newest member sign-in and the file bytes recorded on the tenant
-- tables that keep a size (documents, photos, attachments, PDFs).
WITH storage AS (
    SELECT f.organization_id, SUM(f.bytes)::bigint AS bytes
    FROM (
        SELECT rpi.organization_id, rpi.size_bytes AS bytes FROM rental_product_images rpi
        UNION ALL SELECT vd.organization_id, vd.file_size_bytes FROM vehicle_documents vd WHERE vd.deleted_at IS NULL
        UNION ALL SELECT cd.organization_id, cd.file_size_bytes FROM customer_documents cd WHERE cd.deleted_at IS NULL
        UNION ALL SELECT dv.organization_id, dv.pdf_size_bytes FROM document_versions dv
        UNION ALL SELECT ip.organization_id, ip.size_bytes FROM inspection_photos ip
        UNION ALL SELECT isg.organization_id, isg.size_bytes FROM inspection_signatures isg
        UNION ALL SELECT ex.organization_id, ex.attachment_size_bytes FROM expenses ex
        UNION ALL SELECT dd.organization_id, dd.document_size_bytes FROM deposit_deductions dd
        UNION ALL SELECT si.organization_id, si.attachment_size_bytes FROM supplier_invoices si
    ) f
    GROUP BY f.organization_id
),
usage AS (
    SELECT o.id, o.uuid, o.slug, o.name, o.status, o.created_at,
           vc.active_vehicles::bigint AS active_vehicles,
           vc.total_vehicles::bigint AS total_vehicles,
           rc.reservations::bigint AS reservations_month,
           mc.members::bigint AS member_count,
           mc.last_login_at AS last_login_at,
           COALESCE(st.bytes, 0)::bigint AS storage_bytes
    FROM organizations o
    CROSS JOIN LATERAL (
        SELECT COUNT(*) FILTER (
                   WHERE v.is_active AND v.status <> 'out_of_service'
               ) AS active_vehicles,
               COUNT(*) AS total_vehicles
        FROM vehicles v
        WHERE v.organization_id = o.id AND v.deleted_at IS NULL
    ) vc
    CROSS JOIN LATERAL (
        SELECT COUNT(*) AS reservations
        FROM reservations r
        WHERE r.organization_id = o.id
          AND r.created_at >= sqlc.arg(month_start)::timestamptz
          AND r.created_at < sqlc.arg(month_end)::timestamptz
    ) rc
    CROSS JOIN LATERAL (
        SELECT COUNT(*) AS members, MAX(u.last_login_at)::timestamptz AS last_login_at
        FROM organization_members om
        INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL
        WHERE om.organization_id = o.id
    ) mc
    LEFT JOIN storage st ON st.organization_id = o.id
    WHERE o.deleted_at IS NULL
      AND (sqlc.narg(statuses)::text[] IS NULL OR o.status = ANY(sqlc.narg(statuses)::text[]))
      AND (
        sqlc.narg(q)::text IS NULL
        OR o.name ILIKE '%' || sqlc.narg(q) || '%'
        OR o.slug ILIKE '%' || sqlc.narg(q) || '%'
      )
)
SELECT us.id, us.uuid, us.slug, us.name, us.status, us.created_at,
       us.active_vehicles, us.total_vehicles, us.reservations_month,
       us.member_count, us.last_login_at, us.storage_bytes,
       COUNT(*) OVER ()::bigint AS total_count
FROM usage us
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN us.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN us.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'active_vehicles' AND NOT sqlc.arg(sort_desc)::boolean THEN us.active_vehicles END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'active_vehicles' AND sqlc.arg(sort_desc)::boolean THEN us.active_vehicles END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'reservations_month' AND NOT sqlc.arg(sort_desc)::boolean THEN us.reservations_month END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'reservations_month' AND sqlc.arg(sort_desc)::boolean THEN us.reservations_month END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'member_count' AND NOT sqlc.arg(sort_desc)::boolean THEN us.member_count END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'member_count' AND sqlc.arg(sort_desc)::boolean THEN us.member_count END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'storage_bytes' AND NOT sqlc.arg(sort_desc)::boolean THEN us.storage_bytes END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'storage_bytes' AND sqlc.arg(sort_desc)::boolean THEN us.storage_bytes END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_login_at' AND NOT sqlc.arg(sort_desc)::boolean THEN us.last_login_at END ASC NULLS FIRST,
  CASE WHEN sqlc.arg(sort_field)::text = 'last_login_at' AND sqlc.arg(sort_desc)::boolean THEN us.last_login_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN us.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN us.created_at END DESC,
  us.id ASC
LIMIT sqlc.arg(row_limit)::int OFFSET sqlc.arg(row_offset)::int;

-- name: PlatformUsageTotals :one
-- Platform-wide totals of the usage list (live organizations only).
SELECT
    (SELECT COUNT(*) FROM organizations o WHERE o.deleted_at IS NULL)::bigint AS organizations,
    (SELECT COUNT(*) FROM organizations o WHERE o.deleted_at IS NULL AND o.status = 'active')::bigint AS active_organizations,
    (SELECT COUNT(*) FROM vehicles v
        INNER JOIN organizations o ON o.id = v.organization_id AND o.deleted_at IS NULL
        WHERE v.deleted_at IS NULL AND v.is_active AND v.status <> 'out_of_service')::bigint AS active_vehicles,
    (SELECT COUNT(*) FROM reservations r
        INNER JOIN organizations o ON o.id = r.organization_id AND o.deleted_at IS NULL
        WHERE r.created_at >= sqlc.arg(month_start)::timestamptz
          AND r.created_at < sqlc.arg(month_end)::timestamptz)::bigint AS reservations_month,
    (SELECT COUNT(DISTINCT om.user_id) FROM organization_members om
        INNER JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL
        INNER JOIN users u ON u.id = om.user_id AND u.deleted_at IS NULL)::bigint AS members,
    (SELECT COALESCE(SUM(f.bytes), 0) FROM (
        SELECT rpi.organization_id, rpi.size_bytes AS bytes FROM rental_product_images rpi
        UNION ALL SELECT vd.organization_id, vd.file_size_bytes FROM vehicle_documents vd WHERE vd.deleted_at IS NULL
        UNION ALL SELECT cd.organization_id, cd.file_size_bytes FROM customer_documents cd WHERE cd.deleted_at IS NULL
        UNION ALL SELECT dv.organization_id, dv.pdf_size_bytes FROM document_versions dv
        UNION ALL SELECT ip.organization_id, ip.size_bytes FROM inspection_photos ip
        UNION ALL SELECT isg.organization_id, isg.size_bytes FROM inspection_signatures isg
        UNION ALL SELECT ex.organization_id, ex.attachment_size_bytes FROM expenses ex
        UNION ALL SELECT dd.organization_id, dd.document_size_bytes FROM deposit_deductions dd
        UNION ALL SELECT si.organization_id, si.attachment_size_bytes FROM supplier_invoices si
    ) f)::bigint AS storage_bytes;

-- name: IntegrationAccountHealthCounts :one
-- Connected provider accounts of live organizations by check state.
SELECT
    COUNT(*) FILTER (WHERE ia.status = 'active')::bigint AS active,
    COUNT(*) FILTER (WHERE ia.status = 'active' AND ia.last_check_status = 'failed')::bigint AS failing,
    COUNT(*) FILTER (WHERE ia.status = 'active' AND ia.last_check_status = 'ok')::bigint AS healthy,
    COUNT(*) FILTER (WHERE ia.status = 'active' AND ia.last_checked_at IS NULL)::bigint AS unchecked,
    COUNT(*) FILTER (WHERE ia.status = 'disabled')::bigint AS disabled
FROM integration_accounts ia
INNER JOIN organizations o ON o.id = ia.organization_id AND o.deleted_at IS NULL;

-- name: ListFailingIntegrationAccounts :many
-- Active accounts whose last connection test failed, newest first.
SELECT ia.uuid, ia.organization_id, ia.kind, ia.provider, ia.display_name, ia.mode,
       ia.last_checked_at, ia.last_check_error, ia.updated_at
FROM integration_accounts ia
INNER JOIN organizations o ON o.id = ia.organization_id AND o.deleted_at IS NULL
WHERE ia.status = 'active' AND ia.last_check_status = 'failed'
ORDER BY COALESCE(ia.last_checked_at, ia.updated_at) DESC, ia.id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: IntegrationJobHealthCounts :one
-- Integration jobs of live organizations (and platform jobs, organization
-- NULL) by state: final failures and successes since since_at, everything
-- currently stuck in a state that needs attention, and due work that is
-- overdue by more than stale_before.
SELECT
    COUNT(*) FILTER (WHERE ij.status = 'failed' AND ij.updated_at >= sqlc.arg(since_at)::timestamptz)::bigint AS failed,
    COUNT(*) FILTER (WHERE ij.status = 'succeeded' AND ij.updated_at >= sqlc.arg(since_at)::timestamptz)::bigint AS succeeded,
    COUNT(*) FILTER (WHERE ij.status = 'needs_verification')::bigint AS needs_verification,
    COUNT(*) FILTER (WHERE ij.status = 'waiting_dependency')::bigint AS waiting_dependency,
    COUNT(*) FILTER (WHERE ij.status = 'blocked')::bigint AS blocked,
    COUNT(*) FILTER (
        WHERE ij.status IN ('pending', 'submitted')
          AND ij.next_attempt_at IS NOT NULL
          AND ij.next_attempt_at < sqlc.arg(stale_before)::timestamptz
    )::bigint AS overdue
FROM integration_jobs ij
LEFT JOIN organizations o ON o.id = ij.organization_id
WHERE (ij.organization_id IS NULL OR o.deleted_at IS NULL);

-- name: ListProblemIntegrationJobs :many
-- Jobs that need attention, newest first: failures since since_at and
-- every job waiting on verification, a dependency or a block.
SELECT ij.uuid, ij.organization_id, ij.provider, ij.kind, ij.status, ij.attempts,
       ij.last_error, ij.ref_type, ij.updated_at, ij.created_at
FROM integration_jobs ij
LEFT JOIN organizations o ON o.id = ij.organization_id
WHERE (ij.organization_id IS NULL OR o.deleted_at IS NULL)
  AND (
    (ij.status = 'failed' AND ij.updated_at >= sqlc.arg(since_at)::timestamptz)
    OR ij.status IN ('needs_verification', 'waiting_dependency', 'blocked')
  )
ORDER BY ij.updated_at DESC, ij.id DESC
LIMIT sqlc.arg(row_limit)::int;

-- name: ListHealthOrganizations :many
-- Names of the organizations an issue list refers to.
SELECT o.id, o.uuid, o.slug, o.name
FROM organizations o
WHERE o.id = ANY(sqlc.arg(ids)::bigint[]);

-- name: GetPlatformHealthAlertCursor :one
SELECT source, last_seen_at, last_alert_at, last_alert_count
FROM platform_health_alert_cursors
WHERE source = sqlc.arg(source);

-- name: UpsertPlatformHealthAlertCursor :exec
INSERT INTO platform_health_alert_cursors (source, last_seen_at, last_alert_at, last_alert_count)
VALUES (sqlc.arg(source), sqlc.arg(last_seen_at), sqlc.narg(last_alert_at), sqlc.arg(last_alert_count))
ON CONFLICT (source) DO UPDATE
SET last_seen_at = EXCLUDED.last_seen_at,
    last_alert_at = COALESCE(EXCLUDED.last_alert_at, platform_health_alert_cursors.last_alert_at),
    last_alert_count = EXCLUDED.last_alert_count;

-- name: ListPlatformHealthAlertCursors :many
SELECT source, last_seen_at, last_alert_at, last_alert_count
FROM platform_health_alert_cursors
ORDER BY source;

-- name: ListPlatformUsersWithPermission :many
-- Active platform users holding a permission through a platform role
-- (the alert recipients).
SELECT DISTINCT u.id, u.locale
FROM users u
INNER JOIN user_roles ur ON ur.user_id = u.id
INNER JOIN roles r ON r.id = ur.role_id AND r.scope = 'platform'
INNER JOIN role_permissions rp ON rp.role_id = r.id
INNER JOIN permissions p ON p.id = rp.permission_id
WHERE p.slug = sqlc.arg(slug)
  AND u.deleted_at IS NULL
  AND u.deactivated_at IS NULL
  AND u.status = 'active'
ORDER BY u.id;
