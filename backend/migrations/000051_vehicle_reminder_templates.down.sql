-- Data-loss note: removes the vehicle reminder templates; notifications
-- already sent keep their rendered title and body.
DELETE FROM notification_templates WHERE code = 'vehicles.reminder';
