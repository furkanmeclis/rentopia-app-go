-- Data-loss note: drops every customer, corporate contact, marketing consent
-- record (consent history included), driver and customer document row. The
-- document files stay in object storage under
-- organizations/{org}/customers/ and must be purged separately.
DROP TABLE IF EXISTS customer_documents;
DROP TABLE IF EXISTS drivers;
DROP TABLE IF EXISTS customer_marketing_consents;
DROP TABLE IF EXISTS customer_contacts;
DROP TABLE IF EXISTS customers;
