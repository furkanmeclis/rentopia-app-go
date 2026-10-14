"use client";

import { CarFront } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCatalogBreadcrumbs } from "@/features/catalog/components/catalog-tabs";
import { TypeModelAssign } from "@/features/catalog/components/type-model-assign";
import {
  CATALOG_FUELS,
  CATALOG_TRANSMISSIONS,
  formatTRY,
  formatYears,
} from "@/features/catalog/constants";
import {
  useCatalogType,
  usePatchType,
} from "@/features/catalog/hooks/use-catalog";
import type { CatalogTypeDetail } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

const NONE = "__none__";

function SuggestionSelect({
  type,
  field,
}: {
  type: CatalogTypeDetail;
  field: "suggested_fuel" | "suggested_transmission";
}) {
  const { t } = useLocale();
  const patch = usePatchType();
  const values =
    field === "suggested_fuel" ? CATALOG_FUELS : CATALOG_TRANSMISSIONS;
  const prefix =
    field === "suggested_fuel" ? "catalog.fuel" : "catalog.transmission";
  const current = type[field];
  const label = current ? t(`${prefix}.${current}`) : "—";

  return (
    <PermissionGuard permission={permissions.catalog.write} fallback={label}>
      <Select
        value={current ?? NONE}
        disabled={patch.isPending}
        onValueChange={(value) =>
          patch.mutate({
            uuid: type.uuid,
            body: { [field]: value === NONE ? null : value },
          })
        }
      >
        <SelectTrigger size="sm" className="w-44">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>{t("catalog.types.unknown")}</SelectItem>
          {values.map((v) => (
            <SelectItem key={v} value={v}>
              {t(`${prefix}.${v}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </PermissionGuard>
  );
}

export function CatalogTypeDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const query = useCatalogType(uuid);
  const type = query.data;
  const breadcrumbs = useCatalogBreadcrumbs(
    type?.raw_name ?? t("catalog.tabs.types"),
  );

  const history = useMemo(() => {
    const byPeriod = new Map<string, { year: number; value: number }[]>();
    for (const v of type?.history ?? []) {
      const rows = byPeriod.get(v.period) ?? [];
      rows.push({ year: v.model_year, value: v.value });
      byPeriod.set(v.period, rows);
    }
    return [...byPeriod.entries()];
  }, [type?.history]);

  return (
    <EntityPage
      title={type?.raw_name ?? t("catalog.types.detail_title")}
      description={t("catalog.types.detail_description")}
      permission={permissions.catalog.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.forbidden")}
        />
      }
      breadcrumbs={[
        ...breadcrumbs.slice(0, 2),
        { label: t("catalog.tabs.types"), href: routes.platform.catalog.types },
        breadcrumbs[2],
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("catalog.types.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {type ? (
        <div className="space-y-6">
          <EntityHeader
            title={type.raw_name}
            subtitle={`${type.brand.display_name} · ${type.brand_tsb_name} · ${type.brand.tsb_code ?? ""}/${type.tsb_type_code}`}
            badges={
              <StatusChip
                label={t(`catalog.active.${String(type.is_active)}`)}
                tone={type.is_active ? "success" : "default"}
              />
            }
            leading={
              <div className="bg-muted flex size-12 items-center justify-center rounded-lg">
                <CarFront className="text-muted-foreground size-6" />
              </div>
            }
          />

          <EntitySectionCard title={t("catalog.types.section_details")}>
            <EntityDetail
              sections={[
                {
                  id: "details",
                  fields: [
                    {
                      key: "brand",
                      label: t("catalog.columns.brand"),
                      value: (
                        <Link
                          className="hover:underline"
                          href={`${routes.platform.catalog.types}?brand=${type.brand.uuid}`}
                        >
                          {type.brand.display_name}
                        </Link>
                      ),
                    },
                    {
                      key: "model",
                      label: t("catalog.columns.model"),
                      value: <TypeModelAssign type={type} />,
                    },
                    {
                      key: "years",
                      label: t("catalog.columns.years"),
                      value: formatYears(type.years),
                    },
                    {
                      key: "code",
                      label: t("catalog.columns.tsb_type_code"),
                      value: (
                        <code className="text-xs">{type.tsb_type_code}</code>
                      ),
                    },
                    {
                      key: "fuel",
                      label: t("catalog.columns.fuel"),
                      value: (
                        <SuggestionSelect type={type} field="suggested_fuel" />
                      ),
                    },
                    {
                      key: "transmission",
                      label: t("catalog.columns.transmission"),
                      value: (
                        <SuggestionSelect
                          type={type}
                          field="suggested_transmission"
                        />
                      ),
                    },
                    {
                      key: "first_seen",
                      label: t("catalog.columns.first_seen_period"),
                      value: type.first_seen_period,
                    },
                    {
                      key: "last_seen",
                      label: t("catalog.columns.last_seen_period"),
                      value: type.last_seen_period,
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("catalog.types.section_values", {
              period: type.latest_period || "—",
            })}
            badge={type.valuations.length}
          >
            {type.valuations.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("catalog.types.no_values")}
              </p>
            ) : (
              <table className="w-full max-w-md text-sm">
                <thead>
                  <tr className="text-muted-foreground text-left text-xs">
                    <th className="py-1 font-medium">
                      {t("catalog.columns.year")}
                    </th>
                    <th className="py-1 text-right font-medium">
                      {t("catalog.columns.kasko_value")}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-border divide-y">
                  {type.valuations.map((v) => (
                    <tr key={v.model_year}>
                      <td className="py-1.5 tabular-nums">{v.model_year}</td>
                      <td className="py-1.5 text-right tabular-nums">
                        {formatTRY(v.value, locale)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </EntitySectionCard>

          {history.length > 1 ? (
            <EntitySectionCard
              title={t("catalog.types.section_history")}
              collapsible
              defaultOpen={false}
            >
              <div className="space-y-4">
                {history.map(([period, rows]) => (
                  <div key={period}>
                    <p className="mb-1 text-sm font-medium">{period}</p>
                    <p className="text-muted-foreground text-sm tabular-nums">
                      {rows
                        .map((r) => `${r.year}: ${formatTRY(r.value, locale)}`)
                        .join(" · ")}
                    </p>
                  </div>
                ))}
              </div>
            </EntitySectionCard>
          ) : null}
        </div>
      ) : null}
    </EntityPage>
  );
}
