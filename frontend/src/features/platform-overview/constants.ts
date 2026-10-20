/** React Query stale time for overview aggregates (~60s). */
export const OVERVIEW_STALE_MS = 60_000;

/** Months shown in the reservation trend (current month included). */
export const TREND_MONTHS = 6;

/** An access window ending within this many days counts as "ending soon". */
export const ACCESS_ENDING_DAYS = 14;

/** Rows fetched to find access windows ending soon (soonest first). */
export const ACCESS_ENDING_SCAN = 50;

/** Rows shown in the attention panel before "Tümünü gör". */
export const ATTENTION_VISIBLE = 8;

/** Rows in the recent organizations / activity lists. */
export const RECENT_LIMIT = 5;
