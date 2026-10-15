-- Data-loss note: drops customer_requests (every change and cancellation
-- request customers sent from the portal: messages, requested dates, the
-- cancellation policy text and fee shown, staff resolutions) and
-- customer_portal_sessions (every customer portal session: token hashes,
-- masked contacts, IP addresses, user agents, expiry and revocation
-- history). Signed-in customers are signed out. Reservation timeline
-- entries about requests (reservation_events) are kept.
DROP TABLE IF EXISTS customer_requests;
DROP TABLE IF EXISTS customer_portal_sessions;
