-- Notification templates of the daily vehicle reminder job (TEC-693). The
-- job stamps every notification with the organization and links it to the
-- vehicle's documents or maintenance tab. {{items}} is a localized list
-- built by the job ("Muayene: 3 gün kaldı (17.10.2026)").
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('vehicles.reminder', 'inapp', 'tr', '{{plate}}: belge / bakım hatırlatması',
     '{{plate}} ({{vehicle}}) için yaklaşan veya geciken kayıtlar: {{items}}.', TRUE),
    ('vehicles.reminder', 'inapp', 'en', '{{plate}}: document / maintenance reminder',
     'Upcoming or overdue records for {{plate}} ({{vehicle}}): {{items}}.', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
