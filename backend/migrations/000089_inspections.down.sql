-- Data-loss note: drops every inspection template, handover / return form
-- (all versions), damage record, photo record and signature record. The
-- photo / signature objects stay in storage (unreferenced). Contract
-- approvals keep their rows but lose the link to the signature. Rental
-- charges lose their form link, dispute flag and override history
-- (original amount, reason, who and when); 'equipment' charges become
-- 'other'. Maintenance and deposit operation tasks are deleted.
DELETE FROM operation_tasks WHERE task_type IN ('maintenance', 'deposit');
ALTER TABLE operation_tasks DROP CONSTRAINT operation_tasks_task_type_check;
ALTER TABLE operation_tasks ADD CONSTRAINT operation_tasks_task_type_check
    CHECK (task_type IN ('preparation', 'handover', 'return', 'cleaning'));

DROP INDEX IF EXISTS idx_rental_charges_organization_inspection;
UPDATE rental_charges SET kind = 'other' WHERE kind = 'equipment';
ALTER TABLE rental_charges DROP CONSTRAINT rental_charges_kind_check;
ALTER TABLE rental_charges ADD CONSTRAINT rental_charges_kind_check
    CHECK (kind IN ('late_return', 'early_return', 'damage', 'fuel', 'mileage', 'other'));
ALTER TABLE rental_charges
    DROP CONSTRAINT IF EXISTS fk_rental_charges_inspection,
    DROP COLUMN IF EXISTS overridden_at,
    DROP COLUMN IF EXISTS overridden_by,
    DROP COLUMN IF EXISTS override_reason,
    DROP COLUMN IF EXISTS original_amount,
    DROP COLUMN IF EXISTS disputed,
    DROP COLUMN IF EXISTS inspection_id;

DROP INDEX IF EXISTS idx_document_approvals_organization_signature;
ALTER TABLE document_approvals
    DROP CONSTRAINT IF EXISTS fk_document_approvals_signature,
    DROP COLUMN IF EXISTS signature_id;

DROP TABLE IF EXISTS inspection_signatures;
DROP TABLE IF EXISTS inspection_photos;
DROP TABLE IF EXISTS inspection_damages;
DROP FUNCTION IF EXISTS inspection_children_immutable();
DROP TABLE IF EXISTS inspections;
DROP FUNCTION IF EXISTS inspections_immutable();
DROP TABLE IF EXISTS inspection_templates;

ALTER TABLE rental_vehicles DROP CONSTRAINT IF EXISTS uq_rental_vehicles_organization_id;
