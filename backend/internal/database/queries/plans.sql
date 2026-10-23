-- Plans and feature entitlements (TEC-674, migration 000134).

-- name: ListFeatureKeys :many
SELECT key FROM features ORDER BY sort_order, key;

-- name: ListPlans :many
SELECT p.id, p.uuid, p.code, p.name, p.name_en, p.sort_order,
       COALESCE(ARRAY(
           SELECT pf.feature_key FROM plan_features pf
           JOIN features f ON f.key = pf.feature_key
           WHERE pf.plan_id = p.id ORDER BY f.sort_order, f.key
       ), '{}')::text[] AS features
FROM plans p
ORDER BY p.sort_order, p.code;

-- name: GetPlanByCode :one
SELECT id, uuid, code, name, name_en, sort_order FROM plans WHERE code = @code;

-- name: ListOrganizationFeatureStates :many
-- Every feature with the organization's plan default and its override.
-- An organization without a plan row reads the default plan (callers pass
-- the code, normally 'complete').
SELECT f.key,
       EXISTS (
           SELECT 1 FROM plan_features pf
           WHERE pf.feature_key = f.key
             AND pf.plan_id = COALESCE(
                 (SELECT op.plan_id FROM organization_plan op WHERE op.organization_id = @organization_id),
                 (SELECT dp.id FROM plans dp WHERE dp.code = @default_plan_code)
             )
       )::bool AS in_plan,
       ofe.enabled AS override
FROM features f
LEFT JOIN organization_features ofe ON ofe.organization_id = @organization_id AND ofe.feature_key = f.key
ORDER BY f.sort_order, f.key;

-- name: GetOrganizationPlan :one
-- The plan of an organization (the default plan when it has no row).
SELECT p.id AS plan_id, p.code, p.name, p.name_en,
       op.period_starts_at, op.period_ends_at, op.updated_at,
       (op.organization_id IS NOT NULL)::bool AS assigned
FROM plans p
LEFT JOIN organization_plan op ON op.organization_id = @organization_id AND op.plan_id = p.id
WHERE p.id = COALESCE(
    (SELECT x.plan_id FROM organization_plan x WHERE x.organization_id = @organization_id),
    (SELECT dp.id FROM plans dp WHERE dp.code = @default_plan_code)
);

-- name: UpsertOrganizationPlan :exec
INSERT INTO organization_plan (organization_id, plan_id, period_starts_at, period_ends_at, updated_by)
VALUES (@organization_id, @plan_id, sqlc.narg(period_starts_at), sqlc.narg(period_ends_at), sqlc.narg(updated_by))
ON CONFLICT (organization_id) DO UPDATE
SET plan_id = EXCLUDED.plan_id, period_starts_at = EXCLUDED.period_starts_at,
    period_ends_at = EXCLUDED.period_ends_at, updated_by = EXCLUDED.updated_by;

-- name: AssignOrganizationPlanByCode :execrows
-- New organizations: the chosen plan (or the default plan).
INSERT INTO organization_plan (organization_id, plan_id, updated_by)
SELECT @organization_id, p.id, sqlc.narg(updated_by) FROM plans p WHERE p.code = @code
ON CONFLICT (organization_id) DO NOTHING;

-- name: UpsertOrganizationFeature :exec
INSERT INTO organization_features (organization_id, feature_key, enabled, updated_by)
VALUES (@organization_id, @feature_key, @enabled, sqlc.narg(updated_by))
ON CONFLICT (organization_id, feature_key) DO UPDATE
SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by;

-- name: DeleteOrganizationFeature :execrows
DELETE FROM organization_features WHERE organization_id = @organization_id AND feature_key = @feature_key;

-- name: GetOrganizationFeatureUsage :one
-- Usage counters of the platform plan screen: one indexed count per
-- feature (organization_id leads every index used here).
SELECT
    (SELECT count(*) FROM site_domains WHERE site_domains.organization_id = @organization_id)::bigint AS site_domains,
    (SELECT count(*) FROM public_api_keys WHERE public_api_keys.organization_id = @organization_id AND public_api_keys.status = 'active')::bigint AS public_api_keys,
    (SELECT count(*) FROM integration_accounts WHERE integration_accounts.organization_id = @organization_id AND integration_accounts.kind = 'channel'
        AND integration_accounts.provider IN ('yolcu360', 'yolcu360_fake'))::bigint AS yolcu360_accounts,
    (SELECT count(*) FROM integration_accounts WHERE integration_accounts.organization_id = @organization_id AND integration_accounts.kind = 'channel'
        AND integration_accounts.provider IN ('obilet', 'obilet_fake'))::bigint AS obilet_accounts,
    (SELECT count(*) FROM kabis_notifications WHERE kabis_notifications.organization_id = @organization_id)::bigint AS kabis_notifications,
    (SELECT count(*) FROM toll_passes WHERE toll_passes.organization_id = @organization_id)::bigint AS toll_passes,
    (SELECT count(*) FROM invoices WHERE invoices.organization_id = @organization_id AND invoices.number IS NOT NULL)::bigint AS einvoices,
    (SELECT count(*) FROM whatsapp_sessions WHERE whatsapp_sessions.organization_id = @organization_id)::bigint AS whatsapp_sessions,
    (SELECT count(*) FROM sms_messages WHERE sms_messages.organization_id = @organization_id)::bigint AS sms_messages;
