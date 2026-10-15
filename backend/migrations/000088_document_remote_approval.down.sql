-- Data-loss note: deletes every remote (SMS) approval of every
-- organization (document_approvals rows with method remote_sms: the
-- verified numbers, OTP challenge references, IP addresses and user agents
-- of the approvals) and drops document_approval_links (every remote
-- approval link, its sealed token and number snapshot, its counters and
-- revocation history). Documents whose current version was approved
-- remotely keep status approved without an approval row. The approval
-- PDFs stay in object storage, unreferenced. In-person approvals are kept.
DROP INDEX IF EXISTS uq_document_approvals_organization_link;

DELETE FROM document_approvals WHERE method = 'remote_sms';

ALTER TABLE document_approvals
    DROP CONSTRAINT IF EXISTS chk_document_approvals_remote,
    DROP CONSTRAINT IF EXISTS fk_document_approvals_link,
    DROP CONSTRAINT IF EXISTS fk_document_approvals_driver,
    DROP COLUMN IF EXISTS pdf_sha256,
    DROP COLUMN IF EXISTS pdf_size_bytes,
    DROP COLUMN IF EXISTS pdf_object_key,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS ip_address,
    DROP COLUMN IF EXISTS verified_phone_hash,
    DROP COLUMN IF EXISTS verified_phone_masked,
    DROP COLUMN IF EXISTS otp_challenge_uuid,
    DROP COLUMN IF EXISTS link_id,
    DROP COLUMN IF EXISTS driver_id;

ALTER TABLE document_approvals DROP CONSTRAINT document_approvals_method_check;
ALTER TABLE document_approvals
    ADD CONSTRAINT document_approvals_method_check CHECK (method IN ('in_person', 'click', 'signature'));

DROP TABLE IF EXISTS document_approval_links;
