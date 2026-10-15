-- Data-loss note: drops every payment, refund and payment link of every
-- organization (amounts, provider references, commission and refund
-- records). Reservations keep their paid_amount / payment_status columns,
-- but the records behind them are gone.
DROP TABLE IF EXISTS payment_refunds;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS payment_links;
