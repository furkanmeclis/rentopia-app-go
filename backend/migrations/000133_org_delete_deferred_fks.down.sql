-- Restores the immediate (NOT DEFERRABLE) checks of 000045 / 000061. No data
-- is touched; organization hard deletes fail again once a tenant has a
-- GROUP product version or a date rule with targets.
ALTER TABLE date_rule_version_targets
    ALTER CONSTRAINT fk_date_rule_version_targets_product NOT DEFERRABLE,
    ALTER CONSTRAINT fk_date_rule_version_targets_segment NOT DEFERRABLE,
    ALTER CONSTRAINT fk_date_rule_version_targets_branch NOT DEFERRABLE;

ALTER TABLE rental_product_versions
    ALTER CONSTRAINT fk_rental_product_versions_group NOT DEFERRABLE;
