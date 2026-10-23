-- Organization hard delete (TEC-752). Every organization-scoped table reaches
-- organizations through ON DELETE CASCADE, so one
-- `DELETE FROM organizations WHERE id = ...` removes the whole tenant. The
-- cascade runs in waves (direct children first, then their children, ...)
-- and a plain NO ACTION foreign key is checked as soon as its wave reaches
-- the deleted parent row. The four keys below point from a deeper table at a
-- shallower one, so the check fired while the referencing rows were still
-- waiting for their own cascade and the delete failed:
--
--   rental_product_versions (org -> rental_products -> versions)
--     -> rental_groups (org -> rental_groups)
--   date_rule_version_targets (org -> date_rules -> versions -> targets)
--     -> rental_products / vehicle_segments / branches (org -> ...)
--
-- They become DEFERRABLE INITIALLY DEFERRED: the check runs at commit, when
-- the cascade has removed both sides. ON DELETE CASCADE is not used on
-- purpose: product versions and activated date rule targets are immutable
-- history (bookings keep pointing at the version they were sold with), so a
-- hard delete of a rental group, product, segment or branch on its own must
-- still be refused (now at commit instead of at the statement). The
-- date_rule_version_targets immutability trigger would also refuse a cascade
-- that reaches targets before their version is gone.
--
-- vehicle_allocations -> rental_product_versions is left as is: allocations
-- cascade straight from organizations, so they are always gone before any
-- product version is deleted.
--
-- ALTER CONSTRAINT keeps the existing keys (no re-validation scan).

ALTER TABLE rental_product_versions
    ALTER CONSTRAINT fk_rental_product_versions_group DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE date_rule_version_targets
    ALTER CONSTRAINT fk_date_rule_version_targets_product DEFERRABLE INITIALLY DEFERRED,
    ALTER CONSTRAINT fk_date_rule_version_targets_segment DEFERRABLE INITIALLY DEFERRED,
    ALTER CONSTRAINT fk_date_rule_version_targets_branch DEFERRABLE INITIALLY DEFERRED;
