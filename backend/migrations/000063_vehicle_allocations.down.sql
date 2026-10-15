-- Data-loss note: drops every vehicle allocation (active holds and
-- reservation plate plans, released / expired history). The busy intervals
-- view goes back to blocks and transfers only.
CREATE OR REPLACE VIEW vehicle_busy_intervals AS
SELECT
    b.organization_id,
    b.vehicle_id,
    'block'::text AS kind,
    b.uuid AS ref_uuid,
    b.during,
    NULL::bigint AS branch_after_id
FROM vehicle_blocks b
WHERE b.deleted_at IS NULL
UNION ALL
SELECT
    t.organization_id,
    t.vehicle_id,
    'transfer'::text AS kind,
    t.uuid AS ref_uuid,
    CASE t.status
        WHEN 'planned' THEN t.planned
        WHEN 'in_transit' THEN tstzrange(
            t.actual_departure, GREATEST(upper(t.planned), NOW(), t.actual_departure), '[)'
        )
        ELSE tstzrange(t.actual_departure, t.actual_arrival, '[)')
    END AS during,
    t.to_branch_id AS branch_after_id
FROM vehicle_transfers t
WHERE t.status <> 'cancelled';

DROP TABLE IF EXISTS vehicle_allocations;
DROP FUNCTION IF EXISTS vehicle_allocations_guard();
ALTER TABLE service_points DROP CONSTRAINT IF EXISTS uq_service_points_organization_id;
