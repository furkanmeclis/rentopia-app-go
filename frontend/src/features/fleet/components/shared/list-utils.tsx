"use client";

import type { ColumnFiltersState } from "@tanstack/react-table";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import type { AppLocale } from "@/config/i18n";
import { datetime } from "@/lib/utils/format";

/** Values of one DataTable column filter (faceted filters hold arrays). */
export function filterValues(
  filters: ColumnFiltersState,
  id: string,
): string[] {
  const value = filters.find((filter) => filter.id === id)?.value;
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** Comma-joined multi-value filter, or undefined when empty. */
export function joinFilter(values: string[]): string | undefined {
  return values.length ? values.join(",") : undefined;
}

/** Boolean facet: both values selected = no filter. */
export function activeFilter(values: string[]): "true" | "false" | undefined {
  return values.length === 1 ? (values[0] as "true" | "false") : undefined;
}

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Shared `is_active` column (faceted, sortable). */
export function activeColumn<T extends { is_active: boolean }>(
  t: Translate,
  labelKey: string,
) {
  return createColumn<T>({
    id: "is_active",
    accessorFn: (row) => String(row.is_active),
    labelKey,
    enableSorting: true,
    filterVariant: "faceted",
    filterOptions: [
      {
        value: "true",
        labelKey: "fleet.active.true",
        label: t("fleet.active.true"),
      },
      {
        value: "false",
        labelKey: "fleet.active.false",
        label: t("fleet.active.false"),
      },
    ],
    cell: ({ row }) => (
      <StatusChip
        label={
          row.original.is_active
            ? t("fleet.active.true")
            : t("fleet.active.false")
        }
        tone={row.original.is_active ? "success" : "default"}
      />
    ),
  });
}

/** Shared hidden `created_at` / `uuid` columns. */
export function auditColumns<T extends { created_at: string; uuid: string }>(
  keys: { createdAt: string; uuid: string },
  locale: AppLocale,
) {
  return [
    createColumn<T>({
      id: "created_at",
      accessorFn: (row) => row.created_at,
      labelKey: keys.createdAt,
      enableSorting: true,
      defaultHidden: true,
      cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
    }),
    createColumn<T>({
      id: "uuid",
      accessorFn: (row) => row.uuid,
      labelKey: keys.uuid,
      enableSorting: false,
      defaultHidden: true,
      cell: ({ row }) => (
        <code className="text-muted-foreground text-xs">
          {row.original.uuid}
        </code>
      ),
    }),
  ];
}

/** Code badge used in list cells and headers. */
export function CodeBadge({ code }: { code: string }) {
  return (
    <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
      {code}
    </code>
  );
}
