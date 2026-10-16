import { ApiError } from "@/lib/api";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** "2 saat önce" style label of a reminder offset (minutes). */
export function offsetLabel(t: Translate, minutes: number): string {
  if (minutes === 0) return t("notification_engine.offsets.at_time");
  if (minutes % 1440 === 0) {
    return t("notification_engine.offsets.days", { count: minutes / 1440 });
  }
  if (minutes % 60 === 0) {
    return t("notification_engine.offsets.hours", { count: minutes / 60 });
  }
  return t("notification_engine.offsets.minutes", { count: minutes });
}

/** Field problems of an API error joined for an inline alert. */
export function problemText(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    const fields = Object.values(error.fieldErrors());
    if (fields.length > 0) return fields.join(" · ");
    return error.message || fallback;
  }
  return fallback;
}
