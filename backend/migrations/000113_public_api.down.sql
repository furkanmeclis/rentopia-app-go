-- Data-loss note: drops every public API key, idempotency record, public
-- quote / reservation link (the quotes and reservations themselves stay),
-- webhook endpoint and webhook delivery. Integrations using the keys stop
-- working.
DROP TABLE IF EXISTS public_webhook_deliveries;
DROP TABLE IF EXISTS public_webhook_endpoints;
DROP TABLE IF EXISTS public_reservations;
DROP TABLE IF EXISTS public_quotes;
DROP TABLE IF EXISTS public_api_idempotency;
DROP TABLE IF EXISTS public_api_keys;
