-- Data-loss note: drops every vehicle document (metadata only; the scanned
-- files stay in object storage under organizations/{org}/vehicles/…), every
-- maintenance record and the reminder delivery log.
DROP TABLE IF EXISTS vehicle_reminder_deliveries;
DROP TABLE IF EXISTS vehicle_maintenance;
DROP TABLE IF EXISTS vehicle_documents;
