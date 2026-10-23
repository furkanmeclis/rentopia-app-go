import { format } from "date-fns";

// Moved to the shared lib; re-exported so existing imports keep working.
export { isoToLocalInput, localInputToISO } from "@/lib/utils/datetime-input";

/** Local input `hours` from now, rounded to the next quarter hour. */
export function localInputFromNow(hours: number): string {
  const d = new Date(Date.now() + hours * 3_600_000);
  d.setMinutes(Math.ceil(d.getMinutes() / 15) * 15, 0, 0);
  return format(d, "yyyy-MM-dd'T'HH:mm");
}
