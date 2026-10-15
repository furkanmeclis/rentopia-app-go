-- Data-loss note: drops every connected integration account (the sealed
-- provider credentials of every organization) and the whole integration
-- job history (outbound calls and inbound notifications). Organizations
-- must connect their providers again after an up.
DROP TABLE IF EXISTS integration_jobs;
DROP TABLE IF EXISTS integration_accounts;
