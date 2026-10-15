-- Data-loss note: drops the extras catalog and every price row (price
-- history included).
DROP TABLE IF EXISTS extra_prices;
DROP FUNCTION IF EXISTS extra_prices_immutable();
DROP TABLE IF EXISTS extras;
