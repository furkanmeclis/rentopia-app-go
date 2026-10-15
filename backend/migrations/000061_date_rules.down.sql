-- Data-loss note: drops every date rule (YAZ, BAYRAM …) with all versions
-- and their product / segment / branch selections.
DROP TABLE IF EXISTS date_rule_version_targets;
DROP FUNCTION IF EXISTS date_rule_version_targets_immutable();
DROP TABLE IF EXISTS date_rule_versions;
DROP FUNCTION IF EXISTS date_rule_versions_immutable();
DROP TABLE IF EXISTS date_rules;
