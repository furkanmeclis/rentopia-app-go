-- Data-loss note: drops the SMS send log of every organization
-- (sms_messages: masked numbers, redacted bodies, delivery reports) and
-- every OTP challenge (sms_otp_challenges: pending codes stop verifying,
-- verification history is gone). The integration jobs of the sends stay.
DROP TABLE IF EXISTS sms_otp_challenges;
DROP TABLE IF EXISTS sms_messages;
