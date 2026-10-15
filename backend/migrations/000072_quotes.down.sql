-- Data-loss note: drops every quote and quote line (priced search answers;
-- reservations keep their own snapshot of the accepted quote).
DROP TABLE IF EXISTS quote_lines;
DROP TABLE IF EXISTS quotes;
DROP FUNCTION IF EXISTS quotes_immutable();
