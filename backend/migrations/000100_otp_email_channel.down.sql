-- Data-loss note: deletes every e-mail one-time code challenge (channel
-- 'email': masked address, keyed address hash, code HMAC, attempts and
-- status) of every organization, then drops the channel column. SMS
-- challenges are kept unchanged.
DELETE FROM sms_otp_challenges WHERE channel = 'email';

ALTER TABLE sms_otp_challenges DROP COLUMN IF EXISTS channel;
