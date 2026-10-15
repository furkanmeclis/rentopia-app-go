-- Toll passes (TEC-725, docs/design/tolls.md). Every query names the
-- organization. Branch scope: a pass is visible when its vehicle's home or
-- current branch, or its rental's planned pick-up / return or actual
-- return branch, is in the member's scope (branch_ids NULL = every
-- branch). A pass without a vehicle and a rental is visible to members of
-- every branch only.

-- name: ListTollPasses :many
SELECT tp.uuid, tp.provider, tp.external_id, tp.source, tp.passed_at, tp.plate, tp.tag_number, tp.station,
    tp.exit_station, tp.currency, tp.amount, tp.status, tp.review_reason, tp.match_method, tp.manual, tp.revision,
    tp.last_fetched_at, tp.created_at, tp.updated_at,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    fc.amount AS fee_amount,
    COUNT(*) OVER ()::bigint AS total
FROM toll_passes tp
LEFT JOIN vehicles v ON v.organization_id = tp.organization_id AND v.id = tp.vehicle_id
LEFT JOIN rentals r ON r.organization_id = tp.organization_id AND r.id = tp.rental_id
LEFT JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
LEFT JOIN rental_charges fc ON fc.organization_id = tp.organization_id AND fc.id = tp.fee_charge_id
WHERE tp.organization_id = sqlc.arg(organization_id)
  AND (
    sqlc.narg(branch_ids)::bigint[] IS NULL
    OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
    OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
  )
  AND (sqlc.narg(statuses)::text[] IS NULL OR tp.status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(sources)::text[] IS NULL OR tp.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(providers)::text[] IS NULL OR tp.provider = ANY(sqlc.narg(providers)::text[]))
  AND (sqlc.narg(review_reasons)::text[] IS NULL OR tp.review_reason = ANY(sqlc.narg(review_reasons)::text[]))
  AND (sqlc.narg(vehicle_uuids)::uuid[] IS NULL OR v.uuid = ANY(sqlc.narg(vehicle_uuids)::uuid[]))
  AND (sqlc.narg(rental_uuids)::uuid[] IS NULL OR r.uuid = ANY(sqlc.narg(rental_uuids)::uuid[]))
  AND (sqlc.narg(has_rental)::boolean IS NULL OR (tp.rental_id IS NOT NULL) = sqlc.narg(has_rental)::boolean)
  AND (sqlc.narg(passed_from)::timestamptz IS NULL OR tp.passed_at >= sqlc.narg(passed_from)::timestamptz)
  AND (sqlc.narg(passed_to)::timestamptz IS NULL OR tp.passed_at < sqlc.narg(passed_to)::timestamptz)
  AND (
    sqlc.narg(q)::text IS NULL
    OR tp.plate ILIKE '%' || sqlc.narg(q) || '%'
    OR tp.tag_number ILIKE '%' || sqlc.narg(q) || '%'
    OR tp.station ILIKE '%' || sqlc.narg(q) || '%'
    OR tp.exit_station ILIKE '%' || sqlc.narg(q) || '%'
    OR tp.external_id ILIKE '%' || sqlc.narg(q) || '%'
    OR r.reference ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
    CASE WHEN sqlc.arg(sort_field)::text = 'passed_at' AND NOT sqlc.arg(sort_desc)::boolean THEN tp.passed_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'passed_at' AND sqlc.arg(sort_desc)::boolean THEN tp.passed_at END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND NOT sqlc.arg(sort_desc)::boolean THEN tp.amount END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'amount' AND sqlc.arg(sort_desc)::boolean THEN tp.amount END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND NOT sqlc.arg(sort_desc)::boolean THEN tp.status END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'status' AND sqlc.arg(sort_desc)::boolean THEN tp.status END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'plate' AND NOT sqlc.arg(sort_desc)::boolean THEN tp.plate_key END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'plate' AND sqlc.arg(sort_desc)::boolean THEN tp.plate_key END DESC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN tp.created_at END ASC,
    CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN tp.created_at END DESC,
    tp.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: TollPassFacets :many
-- Counts per status, source, provider and review reason in the caller's
-- branch scope (facet = the column).
WITH scoped AS (
    SELECT tp.status, tp.source, tp.provider, tp.review_reason
    FROM toll_passes tp
    LEFT JOIN vehicles v ON v.organization_id = tp.organization_id AND v.id = tp.vehicle_id
    LEFT JOIN rentals r ON r.organization_id = tp.organization_id AND r.id = tp.rental_id
    LEFT JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
    WHERE tp.organization_id = sqlc.arg(organization_id)
      AND (
        sqlc.narg(branch_ids)::bigint[] IS NULL
        OR v.home_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR v.current_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR res.pickup_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR res.return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
        OR r.actual_return_branch_id = ANY(sqlc.narg(branch_ids)::bigint[])
      )
)
SELECT 'status'::text AS facet, s.status::text AS value, COUNT(*)::bigint AS count FROM scoped s GROUP BY s.status
UNION ALL
SELECT 'source'::text, s.source::text, COUNT(*)::bigint FROM scoped s GROUP BY s.source
UNION ALL
SELECT 'provider'::text, s.provider::text, COUNT(*)::bigint FROM scoped s GROUP BY s.provider
UNION ALL
SELECT 'review_reason'::text, s.review_reason::text, COUNT(*)::bigint FROM scoped s
WHERE s.review_reason <> '' AND s.status IN ('unmatched', 'needs_review') GROUP BY s.review_reason;

-- name: GetTollPass :one
-- One pass with its vehicle / rental references (detail view).
SELECT tp.id, tp.uuid, tp.provider, tp.external_id, tp.source, tp.passed_at, tp.plate, tp.tag_number, tp.station,
    tp.exit_station, tp.currency, tp.amount, tp.status, tp.review_reason, tp.match_method, tp.manual, tp.revision,
    tp.first_fetched_at, tp.last_fetched_at, tp.raw, tp.decided_at, tp.created_at, tp.updated_at,
    tp.vehicle_id, tp.rental_id, tp.charge_id, tp.fee_charge_id,
    v.uuid AS vehicle_uuid, v.plate AS vehicle_plate, v.home_branch_id AS vehicle_home_branch_id,
    v.current_branch_id AS vehicle_current_branch_id,
    r.uuid AS rental_uuid, r.reference AS rental_reference, r.status AS rental_status,
    res.pickup_branch_id AS rental_pickup_branch_id, res.return_branch_id AS rental_return_branch_id,
    r.actual_return_branch_id AS rental_actual_return_branch_id,
    a.uuid AS account_uuid, a.display_name AS account_name, fc.amount AS fee_amount,
    du.uuid AS decided_by_uuid, du.name AS decided_by_name, du.surname AS decided_by_surname, du.email AS decided_by_email
FROM toll_passes tp
LEFT JOIN vehicles v ON v.organization_id = tp.organization_id AND v.id = tp.vehicle_id
LEFT JOIN rentals r ON r.organization_id = tp.organization_id AND r.id = tp.rental_id
LEFT JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
LEFT JOIN integration_accounts a ON a.organization_id = tp.organization_id AND a.id = tp.integration_account_id
LEFT JOIN rental_charges fc ON fc.organization_id = tp.organization_id AND fc.id = tp.fee_charge_id
LEFT JOIN users du ON du.id = tp.decided_by
WHERE tp.organization_id = sqlc.arg(organization_id)
  AND tp.uuid = sqlc.arg(uuid);

-- name: LockTollPassByUUID :one
SELECT tp.id, tp.uuid, tp.provider, tp.external_id, tp.source, tp.passed_at, tp.plate, tp.plate_key, tp.tag_number,
    tp.tag_key, tp.station, tp.exit_station, tp.currency, tp.amount, tp.status, tp.review_reason, tp.vehicle_id,
    tp.rental_id, tp.match_method, tp.manual, tp.charge_id, tp.fee_charge_id, tp.revision, tp.raw
FROM toll_passes tp
WHERE tp.organization_id = sqlc.arg(organization_id)
  AND tp.uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: LockTollPassByExternal :one
SELECT tp.id, tp.uuid, tp.provider, tp.external_id, tp.source, tp.passed_at, tp.plate, tp.plate_key, tp.tag_number,
    tp.tag_key, tp.station, tp.exit_station, tp.currency, tp.amount, tp.status, tp.review_reason, tp.vehicle_id,
    tp.rental_id, tp.match_method, tp.manual, tp.charge_id, tp.fee_charge_id, tp.revision, tp.raw
FROM toll_passes tp
WHERE tp.organization_id = sqlc.arg(organization_id)
  AND tp.provider = sqlc.arg(provider)
  AND tp.external_id = sqlc.arg(external_id)
FOR UPDATE;

-- name: InsertTollPass :one
-- A new pass; a concurrent arrival of the same transaction loses the
-- unique index and reads the row again (inserted = false).
INSERT INTO toll_passes (
    organization_id, provider, external_id, source, integration_account_id, passed_at, plate, tag_number, station,
    exit_station, currency, amount, raw
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(provider), sqlc.arg(external_id), sqlc.arg(source),
    sqlc.narg(integration_account_id), sqlc.arg(passed_at), sqlc.arg(plate), sqlc.arg(tag_number), sqlc.arg(station),
    sqlc.arg(exit_station), sqlc.arg(currency), sqlc.arg(amount), sqlc.arg(raw)
)
ON CONFLICT (organization_id, provider, external_id) DO NOTHING
RETURNING id, uuid;

-- name: UpdateTollPassFacts :exec
-- A pass that arrived again with changed facts (a new revision).
UPDATE toll_passes
SET passed_at = sqlc.arg(passed_at), plate = sqlc.arg(plate), tag_number = sqlc.arg(tag_number),
    station = sqlc.arg(station), exit_station = sqlc.arg(exit_station), currency = sqlc.arg(currency),
    amount = sqlc.arg(amount), raw = sqlc.arg(raw), revision = sqlc.arg(revision), source = sqlc.arg(source),
    integration_account_id = COALESCE(sqlc.narg(integration_account_id), integration_account_id),
    last_fetched_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: TouchTollPassFetched :exec
UPDATE toll_passes
SET last_fetched_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: SetTollPassMatch :exec
UPDATE toll_passes
SET status = sqlc.arg(status), review_reason = sqlc.arg(review_reason), vehicle_id = sqlc.narg(vehicle_id),
    rental_id = sqlc.narg(rental_id), match_method = sqlc.arg(match_method), manual = sqlc.arg(manual),
    charge_id = sqlc.narg(charge_id), fee_charge_id = sqlc.narg(fee_charge_id),
    decided_by = CASE WHEN sqlc.arg(decided)::boolean THEN sqlc.narg(decided_by) ELSE decided_by END,
    decided_at = CASE WHEN sqlc.arg(decided)::boolean THEN NOW() ELSE decided_at END
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: DeleteTollPass :exec
-- Import rollback of a pass the import created (its charges were reversed
-- first; they keep their rows without the pass link).
DELETE FROM toll_passes
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: InsertTollPassRevision :exec
INSERT INTO toll_pass_revisions (
    organization_id, toll_pass_id, revision, source, changed_fields, previous, current, amount_before, amount_after,
    created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(toll_pass_id), sqlc.arg(revision), sqlc.arg(source), sqlc.arg(changed_fields),
    sqlc.arg(previous), sqlc.arg(current), sqlc.arg(amount_before), sqlc.arg(amount_after), sqlc.narg(created_by)
);

-- name: ListTollPassRevisions :many
SELECT rv.uuid, rv.revision, rv.source, rv.changed_fields, rv.previous, rv.current, rv.amount_before,
    rv.amount_after, rv.created_at
FROM toll_pass_revisions rv
WHERE rv.organization_id = sqlc.arg(organization_id)
  AND rv.toll_pass_id = sqlc.arg(toll_pass_id)
ORDER BY rv.revision DESC;

-- name: InsertTollPassEvent :exec
INSERT INTO toll_pass_events (organization_id, toll_pass_id, action, from_status, to_status, reason, data, actor_id)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(toll_pass_id), sqlc.arg(action), sqlc.narg(from_status), sqlc.narg(to_status),
    sqlc.arg(reason), sqlc.arg(data), sqlc.narg(actor_id)
);

-- name: ListTollPassEvents :many
SELECT e.uuid, e.action, e.from_status, e.to_status, e.reason, e.data, e.created_at,
    u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname, u.email AS actor_email
FROM toll_pass_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.toll_pass_id = sqlc.arg(toll_pass_id)
ORDER BY e.created_at DESC, e.id DESC;

-- name: ListTollPassCharges :many
-- Every rental charge of the pass (live, reversed and reversals).
SELECT ch.id, ch.uuid, ch.kind, ch.status, ch.currency, ch.amount, ch.reason, ch.created_at,
    rv.uuid AS reversal_of_uuid, r.uuid AS rental_uuid, r.reference AS rental_reference
FROM rental_charges ch
JOIN rentals r ON r.organization_id = ch.organization_id AND r.id = ch.rental_id
LEFT JOIN rental_charges rv ON rv.organization_id = ch.organization_id AND rv.id = ch.reversal_of_id
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND ch.toll_pass_id = sqlc.arg(toll_pass_id)
ORDER BY ch.created_at, ch.id;

-- name: FindTollVehiclesByPlate :many
-- Vehicles with the plate (live ones first; a soft-deleted vehicle still
-- matches a pass from its time).
SELECT v.id, v.uuid, v.plate, v.deleted_at
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.plate_key = sqlc.arg(plate_key)
ORDER BY v.deleted_at IS NULL DESC, v.id
LIMIT 5;

-- name: FindTollVehiclesByTag :many
-- Vehicles that carried the tag at the pass time.
SELECT t.vehicle_id, v.uuid, v.plate
FROM vehicle_toll_tags t
JOIN vehicles v ON v.organization_id = t.organization_id AND v.id = t.vehicle_id
WHERE t.organization_id = sqlc.arg(organization_id)
  AND v.organization_id = sqlc.arg(organization_id)
  AND t.tag_key = sqlc.arg(tag_key)
  AND t.valid_from <= sqlc.arg(at)
  AND (t.valid_to IS NULL OR t.valid_to > sqlc.arg(at))
LIMIT 5;

-- name: ListTollRentalLegs :many
-- Rental legs of the vehicle whose ACTUAL out → in interval contains the
-- pass time (an open leg runs until now).
SELECT r.id AS rental_id, r.uuid AS rental_uuid, r.status AS rental_status, rv.out_at, rv.in_at
FROM rental_vehicles rv
JOIN rentals r ON r.organization_id = rv.organization_id AND r.id = rv.rental_id
WHERE rv.organization_id = sqlc.arg(organization_id)
  AND r.organization_id = sqlc.arg(organization_id)
  AND rv.vehicle_id = sqlc.arg(vehicle_id)
  AND r.status <> 'cancelled'
  AND rv.out_at IS NOT NULL
  AND rv.out_at <= sqlc.arg(at)
  AND (rv.in_at IS NULL OR rv.in_at >= sqlc.arg(at));

-- name: ListTollPlannedRentals :many
-- Rentals of the vehicle whose PLANNED window contains the pass time while
-- no actual leg does (actual times missing or outside the plan).
SELECT r.id AS rental_id, r.uuid AS rental_uuid, r.status AS rental_status, r.actual_pickup_at, r.actual_return_at
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.status <> 'cancelled'
  AND (
    r.vehicle_id = sqlc.arg(vehicle_id)
    OR EXISTS (
        SELECT 1 FROM rental_vehicles lv
        WHERE lv.organization_id = r.organization_id AND lv.rental_id = r.id AND lv.vehicle_id = sqlc.arg(vehicle_id)
    )
  )
  AND res.starts_at <= sqlc.arg(at)
  AND res.ends_at >= sqlc.arg(at);

-- name: CountTollVehicleTransfersAt :one
-- Transfers of the vehicle (not cancelled) whose actual, else planned,
-- window contains the pass time.
SELECT COUNT(*)::bigint
FROM vehicle_transfers t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.vehicle_id = sqlc.arg(vehicle_id)
  AND t.status <> 'cancelled'
  AND COALESCE(t.actual_departure, lower(t.planned)) <= sqlc.arg(at)
  AND COALESCE(t.actual_arrival, CASE WHEN t.status = 'in_transit' THEN NULL ELSE upper(t.planned) END, 'infinity'::timestamptz) >= sqlc.arg(at);

-- name: GetTollRental :one
-- A rental as a toll charge target (by id): status, customer, currency and
-- the branches its visibility depends on.
SELECT r.id, r.uuid, r.reference, r.status, r.vehicle_id, res.customer_id, res.currency,
    res.pickup_branch_id, res.return_branch_id, r.actual_return_branch_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.id = sqlc.arg(id);

-- name: GetTollRentalByUUID :one
SELECT r.id, r.uuid, r.reference, r.status, r.vehicle_id, res.customer_id, res.currency,
    res.pickup_branch_id, res.return_branch_id, r.actual_return_branch_id
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND r.uuid = sqlc.arg(uuid);

-- name: ListTollCandidateRentals :many
-- Rentals of the vehicle around the pass time (assign dialog): planned
-- and actual windows, customer, whether the actual window contains it.
SELECT r.id, r.uuid, r.reference, r.status, res.starts_at AS planned_pickup_at, res.ends_at AS planned_return_at,
    r.actual_pickup_at, r.actual_return_at, c.customer_type, c.first_name, c.last_name, c.company_title,
    res.pickup_branch_id, res.return_branch_id, r.actual_return_branch_id,
    EXISTS (
        SELECT 1 FROM rental_vehicles lv
        WHERE lv.organization_id = r.organization_id AND lv.rental_id = r.id AND lv.vehicle_id = sqlc.arg(vehicle_id)
          AND lv.out_at IS NOT NULL AND lv.out_at <= sqlc.arg(at) AND (lv.in_at IS NULL OR lv.in_at >= sqlc.arg(at))
    )::boolean AS contains_pass
FROM rentals r
JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
JOIN customers c ON c.organization_id = res.organization_id AND c.id = res.customer_id
WHERE r.organization_id = sqlc.arg(organization_id)
  AND res.organization_id = sqlc.arg(organization_id)
  AND c.organization_id = sqlc.arg(organization_id)
  AND r.status <> 'cancelled'
  AND (
    r.vehicle_id = sqlc.arg(vehicle_id)
    OR EXISTS (
        SELECT 1 FROM rental_vehicles lv2
        WHERE lv2.organization_id = r.organization_id AND lv2.rental_id = r.id AND lv2.vehicle_id = sqlc.arg(vehicle_id)
    )
  )
  AND COALESCE(r.actual_pickup_at, res.starts_at) <= sqlc.arg(window_to)
  AND COALESCE(r.actual_return_at, res.ends_at) >= sqlc.arg(window_from)
ORDER BY COALESCE(r.actual_pickup_at, res.starts_at)
LIMIT 20;

-- name: InsertTollCharge :one
-- A toll / processing fee charge or the reversal of one. Toll charges are
-- facts reported by the provider: they are recorded approved (decided by
-- the system), never edited, and reversed by a negative row.
INSERT INTO rental_charges (
    organization_id, rental_id, kind, source, status, currency, amount, calculation, reason, toll_pass_id,
    reversal_of_id, decided_by, decided_at, decision_note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(rental_id), sqlc.arg(kind), 'toll', 'approved', sqlc.arg(currency),
    sqlc.arg(amount), sqlc.arg(calculation), sqlc.arg(reason), sqlc.narg(toll_pass_id), sqlc.narg(reversal_of_id),
    sqlc.narg(created_by), NOW(), sqlc.arg(decision_note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: GetTollCharge :one
SELECT ch.id, ch.uuid, ch.rental_id, ch.kind, ch.currency, ch.amount, ch.toll_pass_id,
    EXISTS (
        SELECT 1 FROM rental_charges rv WHERE rv.organization_id = ch.organization_id AND rv.reversal_of_id = ch.id
    )::boolean AS reversed
FROM rental_charges ch
WHERE ch.organization_id = sqlc.arg(organization_id)
  AND ch.id = sqlc.arg(id);

-- name: GetTollSettings :one
SELECT s.processing_fee, s.currency, s.auto_fetch, s.lookback_days, s.updated_at
FROM toll_settings s
WHERE s.organization_id = sqlc.arg(organization_id);

-- name: UpsertTollSettings :one
INSERT INTO toll_settings (organization_id, processing_fee, currency, auto_fetch, lookback_days, updated_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(processing_fee), sqlc.arg(currency), sqlc.arg(auto_fetch),
    sqlc.arg(lookback_days), sqlc.narg(updated_by)
)
ON CONFLICT (organization_id) DO UPDATE
SET processing_fee = EXCLUDED.processing_fee, currency = EXCLUDED.currency, auto_fetch = EXCLUDED.auto_fetch,
    lookback_days = EXCLUDED.lookback_days, updated_by = EXCLUDED.updated_by
RETURNING processing_fee, currency, auto_fetch, lookback_days, updated_at;

-- name: ListVehicleTollTags :many
SELECT t.id, t.uuid, t.provider, t.network, t.tag_number, t.valid_from, t.valid_to, t.note, t.created_at, t.updated_at
FROM vehicle_toll_tags t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.vehicle_id = sqlc.arg(vehicle_id)
ORDER BY t.valid_to IS NULL DESC, t.valid_from DESC, t.id DESC;

-- name: GetVehicleTollTag :one
SELECT t.id, t.uuid, t.vehicle_id, t.provider, t.network, t.tag_number, t.valid_from, t.valid_to, t.note,
    t.created_at, t.updated_at
FROM vehicle_toll_tags t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.vehicle_id = sqlc.arg(vehicle_id)
  AND t.uuid = sqlc.arg(uuid);

-- name: InsertVehicleTollTag :one
INSERT INTO vehicle_toll_tags (organization_id, vehicle_id, provider, network, tag_number, valid_from, valid_to, note, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(vehicle_id), sqlc.arg(provider), sqlc.arg(network), sqlc.arg(tag_number),
    sqlc.arg(valid_from), sqlc.narg(valid_to), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING uuid;

-- name: UpdateVehicleTollTag :execrows
UPDATE vehicle_toll_tags
SET provider = sqlc.arg(provider), network = sqlc.arg(network), tag_number = sqlc.arg(tag_number),
    valid_from = sqlc.arg(valid_from), valid_to = sqlc.narg(valid_to), note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: DeleteVehicleTollTag :execrows
DELETE FROM vehicle_toll_tags
WHERE organization_id = sqlc.arg(organization_id)
  AND id = sqlc.arg(id);

-- name: ListTollTagNumbersForFetch :many
-- Tag numbers carried by the organization's vehicles during the window
-- (the provider is asked for these tags).
SELECT DISTINCT t.tag_number
FROM vehicle_toll_tags t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND t.valid_from < sqlc.arg(window_to)
  AND (t.valid_to IS NULL OR t.valid_to > sqlc.arg(window_from))
ORDER BY t.tag_number;

-- name: ListTollFetchAccounts :many
-- The periodic fetch: every active toll provider account whose
-- organization has not switched the automatic fetch off, across
-- organizations (each row carries its organization; the job is created in
-- it).
SELECT a.id, a.uuid, a.organization_id, a.provider, COALESCE(s.lookback_days, 7)::int AS lookback_days
FROM integration_accounts a
JOIN organizations o ON o.id = a.organization_id
LEFT JOIN toll_settings s ON s.organization_id = a.organization_id
WHERE a.kind = 'toll'
  AND a.status = 'active'
  AND o.status = 'active'
  AND COALESCE(s.auto_fetch, TRUE)
ORDER BY a.organization_id, a.id;

-- name: ListTollPassesToRematch :many
-- Passes of the review queue the automatic matching may now resolve
-- (actual times recorded later, a tag added): not decided by a person.
SELECT tp.uuid
FROM toll_passes tp
WHERE tp.organization_id = sqlc.arg(organization_id)
  AND tp.status IN ('unmatched', 'needs_review')
  AND NOT tp.manual
  AND tp.passed_at >= sqlc.arg(since)
ORDER BY tp.passed_at
LIMIT sqlc.arg(limit_count);

-- name: GetTollVehicle :one
-- A vehicle as the owner of toll tags / passes (scope: home or current
-- branch).
SELECT v.id, v.uuid, v.plate, v.home_branch_id, v.current_branch_id, v.deleted_at
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.uuid = sqlc.arg(uuid);

-- name: GetTollVehicleByID :one
SELECT v.id, v.uuid, v.plate, v.home_branch_id, v.current_branch_id, v.deleted_at
FROM vehicles v
WHERE v.organization_id = sqlc.arg(organization_id)
  AND v.id = sqlc.arg(id);

-- name: ListTollReviewPassesByTag :many
-- Review passes of a tag (re-matched when the tag is linked / changed).
SELECT tp.uuid
FROM toll_passes tp
WHERE tp.organization_id = sqlc.arg(organization_id)
  AND tp.tag_key = sqlc.arg(tag_key)
  AND tp.status IN ('unmatched', 'needs_review')
  AND NOT tp.manual
ORDER BY tp.passed_at
LIMIT 500;

-- name: ListOrganizationsWithReviewTollPasses :many
-- Organizations that have passes waiting in the review queue (the
-- periodic re-match walks them).
SELECT DISTINCT tp.organization_id
FROM toll_passes tp
WHERE tp.status IN ('unmatched', 'needs_review')
  AND NOT tp.manual
  AND tp.passed_at >= sqlc.arg(since)
ORDER BY tp.organization_id;
