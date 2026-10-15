-- Data-loss note: drops every contract template (all versions), every
-- document with its rendered versions (HTML, content hashes, data
-- snapshots) and every recorded approval. PDF objects in storage are not
-- deleted (organizations/{org}/documents/...); remove them separately.
DROP TABLE IF EXISTS document_approvals;
ALTER TABLE IF EXISTS documents DROP CONSTRAINT IF EXISTS fk_documents_current_version;
DROP TABLE IF EXISTS document_versions;
DROP FUNCTION IF EXISTS document_versions_immutable();
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS contract_templates;
