"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plug } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDetail,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  invoiceProblemText,
  problemFieldText,
} from "@/features/invoices/components/invoice-cells";
import {
  CONNECTION_HINT_KEYS,
  CONNECTION_STATE_LABEL_KEYS,
  LINE_FIELD_LABEL_KEYS,
  LINE_KIND_LABEL_KEYS,
} from "@/features/invoices/constants";
import {
  useInvoiceSettings,
  useSaveInvoiceSettings,
} from "@/features/invoices/hooks/use-invoices";
import type {
  InvoiceSettings,
  InvoiceTaxMapping,
} from "@/features/invoices/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const SETTINGS_FORM = "invoice-settings-form";
const MAPPING_FORM = "invoice-mapping-form";

const CONNECTION_TONES: Record<
  string,
  "default" | "success" | "warning" | "danger"
> = {
  none: "danger",
  waiting: "warning",
  test: "warning",
  unavailable: "danger",
  ready: "success",
};

type SettingsValues = {
  efatura_series: string;
  earsiv_series: string;
  supplier_district: string;
  supplier_city: string;
  supplier_postal_code: string;
  default_note: string;
  taxpayer_cache_hours: string;
};

type MappingValues = {
  vat_rate: string;
  exemption_code: string;
  exemption_reason: string;
};

const SETTINGS_FIELD_KEYS: Record<string, string> = {
  efatura_series: "invoices.settings.fields.efatura_series",
  earsiv_series: "invoices.settings.fields.earsiv_series",
  supplier_district: "invoices.settings.fields.supplier_district",
  supplier_city: "invoices.settings.fields.supplier_city",
  supplier_postal_code: "invoices.settings.fields.supplier_postal_code",
  default_note: "invoices.settings.fields.default_note",
  taxpayer_cache_hours: "invoices.settings.fields.taxpayer_cache_hours",
};

/** API field errors on the settings form, labelled. */
function settingsErrorText(
  t: (key: string, params?: Record<string, string | number>) => string,
  error: unknown,
): string {
  if (error instanceof ApiError && error.details.length > 0) {
    return error.details
      .map((d) => {
        if (!d.field) return d.message;
        const m = /^tax_mappings\.\d+\.(\w+)$/.exec(d.field);
        const key = m
          ? LINE_FIELD_LABEL_KEYS[m[1]!]
          : SETTINGS_FIELD_KEYS[d.field];
        return `${key ? t(key) : problemFieldText(t, d.field)}: ${d.message}`;
      })
      .join(" · ");
  }
  return invoiceProblemText(t, error);
}

function ConnectionCard({ settings }: { settings: InvoiceSettings }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const c = settings.connection;
  const s = settings.supplier;
  return (
    <EntitySectionCard
      title={t("invoices.settings.connection_title")}
      actions={
        can(permissions.tenant.integrations.manage) ? (
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.settings.integrations(slug)}>
              <Plug className="size-4" />
              {t("invoices.settings.open_integrations")}
            </Link>
          </Button>
        ) : undefined
      }
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <StatusChip
            label={t(CONNECTION_STATE_LABEL_KEYS[c.state] ?? c.state)}
            tone={CONNECTION_TONES[c.state] ?? "default"}
          />
          {c.name ? <span className="font-medium">{c.name}</span> : null}
          {c.provider ? (
            <span className="text-muted-foreground font-mono text-xs">
              {c.provider}
            </span>
          ) : null}
        </div>
        <p className="text-muted-foreground text-sm">
          {t(CONNECTION_HINT_KEYS[c.state] ?? CONNECTION_HINT_KEYS.none!)}
        </p>
        {settings.missing.length > 0 ? (
          <Alert variant="destructive">
            <AlertTitle>{t("invoices.settings.missing_title")}</AlertTitle>
            <AlertDescription>
              <ul className="list-disc ps-4">
                {settings.missing.map((f) => (
                  <li key={f}>{problemFieldText(t, f)}</li>
                ))}
              </ul>
              <p className="mt-2">{t("invoices.settings.missing_hint")}</p>
            </AlertDescription>
          </Alert>
        ) : null}
        <EntityDetail
          sections={[
            {
              id: "supplier",
              title: t("invoices.sections.supplier"),
              fields: [
                {
                  key: "legal_name",
                  label: t("invoices.fields.supplier_legal_name"),
                  value: s.legal_name || "—",
                },
                {
                  key: "tax_number",
                  label: t("invoices.fields.supplier_tax_number"),
                  value: s.tax_number ? (
                    <span className="font-mono text-xs">{s.tax_number}</span>
                  ) : (
                    "—"
                  ),
                },
                {
                  key: "tax_office",
                  label: t("invoices.fields.supplier_tax_office"),
                  value: s.tax_office || "—",
                },
                {
                  key: "address",
                  label: t("invoices.fields.address"),
                  value: s.address || "—",
                },
                {
                  key: "contact",
                  label: t("invoices.fields.contact"),
                  value: [s.email, s.phone].filter(Boolean).join(" · ") || "—",
                },
              ],
            },
          ]}
        />
      </div>
    </EntitySectionCard>
  );
}

function SettingsForm({
  settings,
  editable,
}: {
  settings: InvoiceSettings;
  editable: boolean;
}) {
  const { t, locale } = useLocale();
  const save = useSaveInvoiceSettings();
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(() => {
    const series = z
      .string()
      .trim()
      .regex(/^[A-Za-z0-9]{3}$/, t("invoices.settings.validation.series"));
    return z.object({
      efatura_series: series,
      earsiv_series: series,
      supplier_district: z.string().max(128),
      supplier_city: z.string().max(128),
      supplier_postal_code: z.string().max(16),
      default_note: z.string().max(1000),
      taxpayer_cache_hours: z
        .string()
        .trim()
        .refine((v) => {
          const n = Number(v);
          return Number.isInteger(n) && n >= 1 && n <= 720;
        }, t("invoices.settings.validation.cache_hours")),
    });
  }, [t]);
  return (
    <EntitySectionCard
      title={t("invoices.settings.form_title")}
      actions={
        editable ? (
          <Button
            type="submit"
            size="sm"
            form={SETTINGS_FORM}
            disabled={save.isPending}
          >
            {t("form.save")}
          </Button>
        ) : undefined
      }
    >
      <AppForm<SettingsValues>
        key={settings.updated_at ?? "default"}
        id={SETTINGS_FORM}
        schema={schema}
        className="space-y-4"
        defaultValues={{
          efatura_series: settings.efatura_series,
          earsiv_series: settings.earsiv_series,
          supplier_district: settings.supplier_district,
          supplier_city: settings.supplier_city,
          supplier_postal_code: settings.supplier_postal_code,
          default_note: settings.default_note,
          taxpayer_cache_hours: String(settings.taxpayer_cache_hours),
        }}
        onSubmit={async (v) => {
          setProblem(null);
          try {
            await save.mutateAsync({
              efatura_series: v.efatura_series.trim().toUpperCase(),
              earsiv_series: v.earsiv_series.trim().toUpperCase(),
              supplier_district: v.supplier_district,
              supplier_city: v.supplier_city,
              supplier_postal_code: v.supplier_postal_code,
              default_note: v.default_note,
              taxpayer_cache_hours: Number(v.taxpayer_cache_hours),
            });
            appToast.success(t("invoices.settings.saved"));
          } catch (error) {
            setProblem(settingsErrorText(t, error));
          }
        }}
      >
        <fieldset disabled={!editable} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <AppInput
              name="efatura_series"
              maxLength={3}
              label={t("invoices.settings.fields.efatura_series")}
              description={t("invoices.settings.series_hint")}
            />
            <AppInput
              name="earsiv_series"
              maxLength={3}
              label={t("invoices.settings.fields.earsiv_series")}
            />
            <AppInput
              name="supplier_district"
              label={t("invoices.settings.fields.supplier_district")}
            />
            <AppInput
              name="supplier_city"
              label={t("invoices.settings.fields.supplier_city")}
            />
            <AppInput
              name="supplier_postal_code"
              label={t("invoices.settings.fields.supplier_postal_code")}
            />
            <AppInput
              name="taxpayer_cache_hours"
              inputMode="numeric"
              label={t("invoices.settings.fields.taxpayer_cache_hours")}
              description={t("invoices.settings.cache_hint")}
            />
          </div>
          <AppTextarea
            name="default_note"
            rows={3}
            label={t("invoices.settings.fields.default_note")}
            description={t("invoices.settings.note_hint")}
          />
        </fieldset>
        {problem ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        {settings.updated_at ? (
          <p className="text-muted-foreground text-xs">
            {t("invoices.settings.updated_at", {
              at: datetime(settings.updated_at, "d MMM yyyy HH:mm", locale),
            })}
          </p>
        ) : null}
      </AppForm>
    </EntitySectionCard>
  );
}

function MappingDialog({
  mapping,
  onClose,
}: {
  mapping: InvoiceTaxMapping | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveInvoiceSettings();
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z
        .object({
          vat_rate: z
            .string()
            .trim()
            .refine((v) => {
              const n = Number(v.replace(",", "."));
              return v !== "" && Number.isFinite(n) && n >= 0 && n <= 100;
            }, t("invoices.settings.validation.vat_rate")),
          exemption_code: z.string().trim().max(8),
          exemption_reason: z.string().trim().max(255),
        })
        .refine(
          (v) =>
            Number(v.vat_rate.replace(",", ".")) !== 0 ||
            (v.exemption_code !== "" && v.exemption_reason !== ""),
          {
            path: ["exemption_code"],
            message: t("invoices.settings.validation.exemption_required"),
          },
        ),
    [t],
  );
  const close = () => {
    setProblem(null);
    onClose();
  };
  return (
    <AppDialog
      open={mapping !== null}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={
        mapping
          ? t("invoices.settings.mapping_title", {
              kind: t(LINE_KIND_LABEL_KEYS[mapping.line_kind]),
            })
          : ""
      }
      description={t("invoices.settings.mapping_description")}
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={MAPPING_FORM} disabled={save.isPending}>
            {t("form.save")}
          </Button>
        </>
      }
    >
      {mapping ? (
        <AppForm<MappingValues>
          key={mapping.line_kind}
          id={MAPPING_FORM}
          schema={schema}
          className="space-y-4"
          defaultValues={{
            vat_rate: String(Number(mapping.vat_rate)),
            exemption_code: mapping.exemption_code,
            exemption_reason: mapping.exemption_reason,
          }}
          onSubmit={async (v) => {
            setProblem(null);
            try {
              await save.mutateAsync({
                tax_mappings: [
                  {
                    line_kind: mapping.line_kind,
                    vat_rate: v.vat_rate.replace(",", "."),
                    exemption_code: v.exemption_code.trim(),
                    exemption_reason: v.exemption_reason.trim(),
                  },
                ],
              });
              appToast.success(t("invoices.settings.mapping_saved"));
              close();
            } catch (error) {
              setProblem(settingsErrorText(t, error));
            }
          }}
        >
          <AppInput
            name="vat_rate"
            inputMode="decimal"
            label={t("invoices.fields.vat_rate")}
            description={t("invoices.settings.vat_hint")}
          />
          <AppInput
            name="exemption_code"
            className="font-mono"
            label={t("invoices.fields.exemption_code")}
            description={t("invoices.settings.exemption_hint")}
          />
          <AppInput
            name="exemption_reason"
            label={t("invoices.fields.exemption_reason")}
          />
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

function MappingsCard({
  settings,
  editable,
}: {
  settings: InvoiceSettings;
  editable: boolean;
}) {
  const { t } = useLocale();
  const [editing, setEditing] = useState<InvoiceTaxMapping | null>(null);
  const columns = useMemo(
    () =>
      [
        createColumn<InvoiceTaxMapping>({
          accessorKey: "line_kind",
          labelKey: "invoices.columns.line_kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2 text-sm">
              {t(LINE_KIND_LABEL_KEYS[row.original.line_kind])}
              {row.original.default ? (
                <Badge variant="outline" className="font-normal">
                  {t("invoices.settings.default_badge")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<InvoiceTaxMapping>({
          accessorKey: "vat_rate",
          labelKey: "invoices.columns.vat_rate",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              %{Number(row.original.vat_rate)}
            </span>
          ),
        }),
        createColumn<InvoiceTaxMapping>({
          accessorKey: "exemption_code",
          labelKey: "invoices.fields.exemption_code",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.exemption_code || "—"}
            </span>
          ),
        }),
        createColumn<InvoiceTaxMapping>({
          accessorKey: "exemption_reason",
          labelKey: "invoices.fields.exemption_reason",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-sm">
              {row.original.exemption_reason || "—"}
            </span>
          ),
        }),
        ...(editable
          ? [
              createColumn<InvoiceTaxMapping>({
                id: "actions",
                labelKey: "invoices.columns.actions",
                enableSorting: false,
                enableHiding: false,
                cell: ({ row }) => (
                  <EntityRowActions
                    actions={[
                      {
                        id: "edit",
                        label: t("invoices.settings.edit_mapping"),
                        icon: Pencil,
                        onSelect: () => setEditing(row.original),
                      },
                    ]}
                  />
                ),
              }),
            ]
          : []),
      ] as ColumnDef<InvoiceTaxMapping, unknown>[],
    [editable, t],
  );
  return (
    <EntitySectionCard title={t("invoices.settings.mappings_title")}>
      <p className="text-muted-foreground mb-3 text-sm">
        {t("invoices.settings.mappings_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={settings.tax_mappings}
        getRowId={(row) => row.line_kind}
        emptyTitle={t("invoices.settings.mappings_empty")}
        onRowClick={editable ? (row) => setEditing(row) : undefined}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-invoice-tax-mappings-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
        }}
      />
      <MappingDialog mapping={editing} onClose={() => setEditing(null)} />
    </EntitySectionCard>
  );
}

/**
 * "Fatura ayarları": series, supplier address parts the company profile
 * does not hold, the default note, the taxpayer cache and the VAT /
 * exemption mapping per line kind (Turkish defaults until changed), with
 * the integrator connection state.
 */
export function InvoiceSettingsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useInvoiceSettings();
  const editable = can(permissions.tenant.invoices.settings);
  return (
    <EntityPage
      title={t("invoices.settings.title")}
      description={t("invoices.settings.description")}
      permission={permissions.tenant.invoices.settings}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("invoices.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.nav_tenant_settings_general"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("invoices.settings.title") },
      ]}
      actions={
        <Button asChild size="sm" variant="outline">
          <Link href={routes.tenant.invoices.root(slug)}>
            {t("invoices.title")}
          </Link>
        </Button>
      }
    >
      {query.isLoading ? (
        <Loading />
      ) : query.isError || !query.data ? (
        <ErrorState
          title={t("invoices.settings.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          <div className="space-y-4">
            <ConnectionCard settings={query.data} />
            <SettingsForm settings={query.data} editable={editable} />
          </div>
          <MappingsCard settings={query.data} editable={editable} />
        </div>
      )}
    </EntityPage>
  );
}
