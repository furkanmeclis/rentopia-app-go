-- Data-loss note: drops every operation task (preparation, handover,
-- return, cleaning) with its assignee, notes and completion history.
DROP TABLE IF EXISTS operation_tasks;
