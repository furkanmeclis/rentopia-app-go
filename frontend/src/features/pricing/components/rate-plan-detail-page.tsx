"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Calculator,
  CheckCircle2,
  PowerOff,
  Trash2,
  Undo2,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { AppForm, AppInput, FormActions } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BandsEditor,
  bandRowsSchema,
  bandsToRows,
  rowsToBands,
  type BandRow,
} from "@/features/pricing/components/bands-editor";
import {
  ActivationDialog,
  BandsTable,
  PricingActivityCard,
  VersionBadge,
  VersionChip,
} from "@/features/pricing/components/pricing-shared";
import {
  SCOPE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/pricing/constants";
import {
  useActivateRatePlan,
  useDeactivateRatePlan,
  useDeleteRatePlan,
  useDiscardRatePlanDraft,
  useRatePlan,
  useRatePlanCheck,
  useRatePlanVersions,
  useUpdateRatePlan,
} from "@/features/pricing/hooks/use-pricing";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { bandLabel, moneyText } from "@/features/pricing/lib/format";
import type { RatePlan, RatePlanVersion } from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type DraftValues = {
  name: string;
  priority: string;
  change_note: string;
  bands: BandRow[];
};

/** Draft editor: name, priority, change note and bands (PATCH). */
function DraftEditor({ plan }: { plan: RatePlan }) {
  const { t } = useLocale();
  const update = useUpdateRatePlan();
  const source = plan.draft ?? plan.active;
  const schema = useMemo(
    () =>
      z.object({
        name: z.string().max(255),
        priority: z
          .string()
          .trim()
          .refine(
            (v) => /^\d{1,4}$/.test(v) && Number(v) <= 1000,
            t("pricing.validation.priority"),
          ),
        change_note: z.string().max(2000),
        bands: bandRowsSchema(t),
      }),
    [t],
  ) as z.ZodType<DraftValues>;
  return (
    <AppForm<DraftValues>
      key={`${plan.uuid}-${plan.current_version}-${plan.updated_at}`}
      schema={schema}
      className="space-y-4"
      defaultValues={{
        name: plan.name,
        priority: String(source?.priority ?? 0),
        change_note: plan.draft?.change_note ?? "",
        bands: bandsToRows(source?.bands),
      }}
      onSubmit={async (values) => {
        await update.mutateAsync({
          uuid: plan.uuid,
          body: {
            name: values.name.trim(),
            priority: Number(values.priority),
            change_note: values.change_note.trim(),
            bands: rowsToBands(values.bands),
          },
        });
      }}
    >
      {!plan.draft && plan.active ? (
        <p className="text-muted-foreground text-sm">
          {t("pricing.draft.from_active", { version: plan.active.version })}
        </p>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput name="name" label={t("pricing.fields.name")} />
        <AppInput
          name="priority"
          inputMode="numeric"
          label={t("pricing.fields.priority")}
          description={t("pricing.fields.priority_hint")}
        />
      </div>
      <BandsEditor />
      <AppInput
        name="change_note"
        label={t("pricing.fields.change_note")}
        placeholder={t("pricing.activation.note_placeholder")}
      />
      <FormActions sticky={false}>
        <Button type="submit" disabled={update.isPending}>
          {t("pricing.actions.save_draft")}
        </Button>
      </FormActions>
    </AppForm>
  );
}

function VersionsTable({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const query = useRatePlanVersions(uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<RatePlanVersion>({
          accessorKey: "version",
          labelKey: "pricing.columns.version",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <VersionBadge version={row.original.version} />
              <VersionChip status={row.original.status} />
            </span>
          ),
        }),
        createColumn<RatePlanVersion>({
          accessorKey: "priority",
          labelKey: "pricing.columns.priority",
          enableSorting: false,
        }),
        createColumn<RatePlanVersion>({
          id: "bands",
          accessorFn: (row) => row.bands.length,
          labelKey: "pricing.columns.bands",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.bands
              .map(
                (b) =>
                  `${bandLabel(t, b.min_days, b.max_days)}: ${moneyText(b.daily_price, locale)}`,
              )
              .join(" · ") || "—",
        }),
        createColumn<RatePlanVersion>({
          accessorKey: "change_note",
          labelKey: "pricing.columns.change_note",
          enableSorting: false,
          cell: ({ row }) => row.original.change_note || "—",
        }),
        createColumn<RatePlanVersion>({
          id: "activated_at",
          accessorFn: (row) => row.activated_at ?? "",
          labelKey: "pricing.columns.activated_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.activated_at
              ? datetime(row.original.activated_at, undefined, locale)
              : "—",
        }),
        createColumn<RatePlanVersion>({
          id: "activated_by",
          accessorFn: (row) => row.activated_by?.email ?? "",
          labelKey: "pricing.columns.activated_by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.activated_by
              ? `${row.original.activated_by.name} ${row.original.activated_by.surname}`.trim()
              : "—",
        }),
      ] as ColumnDef<RatePlanVersion, unknown>[],
    [locale, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      emptyTitle={t("pricing.versions.empty")}
      features={{
        persistKey: "tenant-pricing-plan-versions-v1",
        rowSelection: false,
        globalFilter: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
      }}
    />
  );
}

/** Rate plan show page: draft editor, active version, versions, activity. */
export function RatePlanDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const query = useRatePlan(uuid);
  const plan = query.data;
  const [activateOpen, setActivateOpen] = useState(false);
  const [confirm, setConfirm] = useState<
    "deactivate" | "discard" | "delete" | null
  >(null);
  const check = useRatePlanCheck(uuid, activateOpen);
  const activate = useActivateRatePlan();
  const deactivate = useDeactivateRatePlan();
  const discard = useDiscardRatePlanDraft();
  const remove = useDeleteRatePlan();
  const canWrite = can(permissions.tenant.pricing.write);
  const title = plan
    ? `${plan.rental_product.code} · ${plan.rental_product.title}`
    : t("pricing.plans.detail_title");
  const canActivate = plan && (plan.has_draft || plan.status === "inactive");

  return (
    <EntityPage
      title={title}
      description={t("pricing.plans.detail_description")}
      permission={permissions.tenant.pricing.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("pricing.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("pricing.plans.title"),
          href: routes.tenant.pricing.plans.root(slug),
        },
        { label: title },
      ]}
      actions={
        plan ? (
          <EntityActions>
            <Button asChild size="sm" variant="outline">
              <Link
                href={`${routes.tenant.pricing.preview(slug)}?product=${plan.rental_product.uuid}${plan.has_draft ? `&plan=${plan.uuid}` : ""}`}
              >
                <Calculator className="size-4" />
                {t("pricing.actions.preview")}
              </Link>
            </Button>
            <PermissionGuard permission={permissions.tenant.pricing.write}>
              {plan.has_draft && plan.active_version != null ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setConfirm("discard")}
                >
                  <Undo2 className="size-4" />
                  {t("pricing.actions.discard_draft")}
                </Button>
              ) : null}
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirm("delete")}
              >
                <Trash2 className="size-4" />
                {t("pricing.actions.delete")}
              </Button>
            </PermissionGuard>
            <PermissionGuard permission={permissions.tenant.pricing.activate}>
              {plan.status === "active" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setConfirm("deactivate")}
                >
                  <PowerOff className="size-4" />
                  {t("pricing.actions.deactivate")}
                </Button>
              ) : null}
              {canActivate ? (
                <Button
                  type="button"
                  size="sm"
                  onClick={() => {
                    activate.reset();
                    setActivateOpen(true);
                  }}
                >
                  <CheckCircle2 className="size-4" />
                  {t("pricing.actions.activate")}
                </Button>
              ) : null}
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("pricing.plans.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {plan ? (
        <div className="space-y-6">
          <EntityHeader
            title={title}
            subtitle={plan.name || t(SCOPE_LABEL_KEYS[plan.scope])}
            badges={
              <>
                <StatusChip
                  label={t(STATUS_LABEL_KEYS[plan.status])}
                  tone={STATUS_TONES[plan.status]}
                />
                {plan.has_draft && plan.status !== "draft" ? (
                  <StatusChip
                    label={t("pricing.pending_draft")}
                    tone="warning"
                  />
                ) : null}
                <Badge variant="secondary">
                  {t(SCOPE_LABEL_KEYS[plan.scope])}
                </Badge>
                <VersionBadge version={plan.current_version} />
              </>
            }
          />
          <div className="grid gap-6 lg:grid-cols-3">
            <EntitySectionCard
              title={t("pricing.detail.details")}
              className="lg:col-span-1"
            >
              <EntityDetail
                sections={[
                  {
                    id: "details",
                    fields: [
                      {
                        key: "product",
                        label: t("pricing.fields.rental_product"),
                        value: `${plan.rental_product.code} · ${plan.rental_product.title}`,
                      },
                      {
                        key: "scope",
                        label: t("pricing.fields.scope"),
                        value: t(SCOPE_LABEL_KEYS[plan.scope]),
                      },
                      {
                        key: "branch",
                        label: t("pricing.fields.branch"),
                        value: plan.branch
                          ? `${plan.branch.code} · ${plan.branch.name}`
                          : t("pricing.all_branches"),
                      },
                      {
                        key: "channel",
                        label: t("pricing.fields.channel"),
                        value: plan.channel
                          ? channelLabel(t, plan.channel)
                          : t("pricing.all_channels"),
                      },
                      {
                        key: "priority",
                        label: t("pricing.fields.priority"),
                        value: String(plan.priority),
                      },
                      {
                        key: "active_version",
                        label: t("pricing.fields.active_version"),
                        value:
                          plan.active_version != null
                            ? `v${plan.active_version}`
                            : t("pricing.never_activated"),
                      },
                      {
                        key: "activated_at",
                        label: t("pricing.columns.activated_at"),
                        value: plan.activated_at
                          ? datetime(plan.activated_at, undefined, locale)
                          : "—",
                      },
                    ],
                  },
                ]}
              />
            </EntitySectionCard>
            <EntitySectionCard
              title={
                plan.active
                  ? t("pricing.detail.active_version", {
                      version: plan.active.version,
                    })
                  : t("pricing.detail.no_active")
              }
              className="lg:col-span-2"
            >
              {plan.active ? (
                <BandsTable
                  bands={plan.active.bands}
                  persistKey="tenant-pricing-active-bands-v1"
                />
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("pricing.detail.no_active_hint")}
                </p>
              )}
            </EntitySectionCard>
          </div>
          <Tabs
            defaultValue={canWrite ? "draft" : "versions"}
            className="space-y-4"
          >
            <TabsList className="flex h-auto flex-wrap">
              {canWrite ? (
                <TabsTrigger value="draft">
                  {plan.draft
                    ? t("pricing.tabs.draft_version", {
                        version: plan.draft.version,
                      })
                    : t("pricing.tabs.new_draft")}
                </TabsTrigger>
              ) : null}
              {!canWrite && plan.draft ? (
                <TabsTrigger value="draft_view">
                  {t("pricing.tabs.draft_version", {
                    version: plan.draft.version,
                  })}
                </TabsTrigger>
              ) : null}
              <TabsTrigger value="versions">
                {t("pricing.tabs.versions")}
              </TabsTrigger>
              <TabsTrigger value="activity">
                {t("pricing.tabs.activity")}
              </TabsTrigger>
            </TabsList>
            {canWrite ? (
              <TabsContent value="draft">
                <EntitySectionCard title={t("pricing.draft.title")}>
                  <p className="text-muted-foreground mb-4 text-sm">
                    {t("pricing.draft.description")}
                  </p>
                  <DraftEditor plan={plan} />
                </EntitySectionCard>
              </TabsContent>
            ) : null}
            {!canWrite && plan.draft ? (
              <TabsContent value="draft_view">
                <EntitySectionCard title={t("pricing.draft.title")}>
                  <BandsTable
                    bands={plan.draft.bands}
                    persistKey="tenant-pricing-draft-bands-v1"
                  />
                </EntitySectionCard>
              </TabsContent>
            ) : null}
            <TabsContent value="versions">
              <EntitySectionCard title={t("pricing.versions.title")}>
                <p className="text-muted-foreground mb-4 text-sm">
                  {t("pricing.versions.help")}
                </p>
                <VersionsTable uuid={uuid} />
              </EntitySectionCard>
            </TabsContent>
            <TabsContent value="activity">
              <PricingActivityCard kind="plans" uuid={uuid} />
            </TabsContent>
          </Tabs>

          <ActivationDialog
            open={activateOpen}
            onOpenChange={setActivateOpen}
            title={t("pricing.activation.plan_title")}
            description={t("pricing.activation.plan_description")}
            check={check.data}
            checkLoading={check.isLoading}
            error={activate.error}
            pending={activate.isPending}
            onConfirm={(note) =>
              activate.mutate(
                { uuid, body: note ? { change_note: note } : {} },
                { onSuccess: () => setActivateOpen(false) },
              )
            }
          />
          <ConfirmDialog
            open={confirm === "deactivate" || confirm === "discard"}
            title={
              confirm === "discard"
                ? t("pricing.confirm.discard_title")
                : t("pricing.confirm.deactivate_title")
            }
            description={
              confirm === "discard"
                ? t("pricing.confirm.discard_body")
                : t("pricing.confirm.deactivate_plan_body")
            }
            variant="destructive"
            isPending={deactivate.isPending || discard.isPending}
            onCancel={() => setConfirm(null)}
            onConfirm={() => {
              const done = { onSettled: () => setConfirm(null) };
              if (confirm === "discard") discard.mutate(uuid, done);
              else deactivate.mutate(uuid, done);
            }}
          />
          <EntityDeleteDialog
            open={confirm === "delete"}
            entityLabel={title}
            isPending={remove.isPending}
            onCancel={() => setConfirm(null)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.pricing.plans.root(slug)),
                onSettled: () => setConfirm(null),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
