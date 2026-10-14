import type {
  CatalogBrandCategory,
  CatalogFuel,
  CatalogImportStatus,
  CatalogModelSource,
  CatalogTransmission,
} from "@/features/catalog/services/catalog.service";

export const CATALOG_CATEGORIES: CatalogBrandCategory[] = [
  "passenger",
  "light_commercial",
  "other",
];

export const CATALOG_FUELS: CatalogFuel[] = [
  "petrol",
  "diesel",
  "hybrid",
  "electric",
  "lpg",
];

export const CATALOG_TRANSMISSIONS: CatalogTransmission[] = [
  "manual",
  "automatic",
];

export const CATALOG_MODEL_SOURCES: CatalogModelSource[] = ["auto", "manual"];

export const CATALOG_IMPORT_STATUSES: CatalogImportStatus[] = [
  "queued",
  "processing",
  "ready",
  "applying",
  "applied",
  "failed",
];

/** Statuses while a background step is running (poll the job). */
export const CATALOG_IMPORT_RUNNING: CatalogImportStatus[] = [
  "queued",
  "processing",
  "applying",
];

export const CATALOG_IMPORT_TONE: Record<
  CatalogImportStatus,
  "default" | "success" | "warning" | "danger"
> = {
  queued: "default",
  processing: "warning",
  ready: "default",
  applying: "warning",
  applied: "success",
  failed: "danger",
};

/** Turkish lira, no fractions (TSB values are whole TRY). */
export function formatTRY(value: number | null | undefined, locale: string) {
  if (value == null) return "—";
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency: "TRY",
    maximumFractionDigits: 0,
  }).format(value);
}

/** Collapses year lists like [2024, 2023, 2022] into "2022–2024". */
export function formatYears(years: number[] | undefined) {
  if (!years?.length) return "—";
  const sorted = [...years].sort((a, b) => a - b);
  const ranges: string[] = [];
  let start = sorted[0];
  let prev = sorted[0];
  for (const y of sorted.slice(1)) {
    if (y === prev + 1) {
      prev = y;
      continue;
    }
    ranges.push(start === prev ? `${start}` : `${start}–${prev}`);
    start = y;
    prev = y;
  }
  ranges.push(start === prev ? `${start}` : `${start}–${prev}`);
  return ranges.join(", ");
}

export function firstFilterValue(
  filters: { id: string; value: unknown }[],
  id: string,
): string | undefined {
  const value = filters.find((f) => f.id === id)?.value;
  if (Array.isArray(value)) return value[0] as string | undefined;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

/**
 * Live list resources (`resource:{name}` channels, platform.catalog.read).
 * The API publishes after curation edits and after an import is applied.
 */
export const CATALOG_LIVE = {
  brands: "platform.catalog.brands",
  models: "platform.catalog.models",
  types: "platform.catalog.types",
  imports: "platform.catalog.imports",
} as const;
