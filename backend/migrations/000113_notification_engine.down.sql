-- Data-loss note: drops the notification rules, templates, the delivery
-- log and customer web push subscriptions.
DROP TABLE IF EXISTS customer_push_subscriptions;
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_dispatches;
DROP TABLE IF EXISTS notification_message_templates;
DROP TABLE IF EXISTS notification_rules;
