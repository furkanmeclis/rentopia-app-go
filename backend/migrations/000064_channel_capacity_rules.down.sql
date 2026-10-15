-- Data-loss note: drops every channel capacity rule (sales limits,
-- guaranteed quotas, safety margins); every channel sells from the shared
-- pool again.
DROP TABLE IF EXISTS channel_capacity_rules;
