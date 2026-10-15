-- One-time codes by e-mail (TEC-715, docs/design/customer-portal.md §3).
--
-- sms_otp_challenges stays the single store of one-time codes: a challenge
-- now names the channel it was delivered on. For channel 'email' the
-- phone_masked / phone_hash columns hold the masked address and the keyed
-- hash of the normalized address (BlindIndex("sms.email", org, address));
-- the code is still stored as an HMAC only. Existing rows are SMS codes.

ALTER TABLE sms_otp_challenges
    ADD COLUMN channel VARCHAR(8) NOT NULL DEFAULT 'sms'
        CONSTRAINT chk_sms_otp_challenges_channel CHECK (channel IN ('sms', 'email'));
