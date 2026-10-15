-- Data-loss note: drops every campaign / coupon, its targets, usage
-- counters and the redemption history.
DROP TABLE IF EXISTS campaign_redemptions;
DROP TABLE IF EXISTS campaign_targets;
DROP TABLE IF EXISTS campaigns;
