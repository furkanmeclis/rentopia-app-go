-- Data-loss note: drops every cash / bank account, every cash movement
-- (transfers, deposits, expense payments, refunds, adjustments, reversals),
-- every expense record and every expense category, customised ones
-- included. Expense attachments stay in storage (unreferenced).
DROP TABLE IF EXISTS cash_movements;
DROP FUNCTION IF EXISTS cash_movements_insert_rules();
DROP FUNCTION IF EXISTS cash_movements_append_only();
DROP TABLE IF EXISTS expenses;
DROP TABLE IF EXISTS expense_categories;
DROP TABLE IF EXISTS cash_accounts;
