"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, PowerOff, Trash2, Undo2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

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
import { FormActions } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  DateRuleForm,
  dateRuleBody,
  dateRuleDefaults,
} from "@/features/pricing/components/date-rule-form";
import {
  ActivationDialog,
  PricingActivityCard,
  VersionBadge,
  VersionChip,
} from "@/features/pricing/components/pricing-shared";
import {
  KIND_LABEL_KEYS,
  PRODUCT_SCOPE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/pricing/constants";
import {
  useActivateDateRule,
  useDateRule,
  useDateRuleCheck,
  useDateRuleVersions,
  useDeactivateDateRule,
  useDeleteDateRule,
  useDiscardDateRuleDraft,
  useUpdateDateRule,
} from "@/features/pricing/hooks/use-pricing";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { rangeText, ruleValueText } from "@/features/pricing/lib/format";
import type { DateRuleVersion } from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const DRAFT_FORM = "date-rule-draft-form";

/** Human summary of a version's targets. */
function targetsText(
  t: (key: string, params?: Record<string, string | number>) => string,
  v: DateRuleVersion,
) {
  const products =
    v.product_scope === "segments"
      ? v.segments.map((s) => s.name).join(", ")
      : v.product_scope === "products"
        ? v.rental_products.map((p) => p.code).join(", ")
        : t("pricing.product_scope.all");
  const branches = v.all_branches
    ? t("pricing.all_branches")
    : v.branches.map((b) => b.code).join(", ");
  const channels =
    v.channels == null
      ? t("pricing.all_channels")
      : v.channels.map((c) => channelLabel(t, c)).join(", ");
  return { products, branches, channels };
}

function RuleVersionsTable({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const query = useDateRuleVersions(uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<DateRuleVersion>({
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
        createColumn<DateRuleVersion>({
          id: "range",
          accessorFn: (row) => row.start_date,
          labelKey: "pricing.columns.range",
          enableSorting: false,
          cell: ({ row }) =>
            rangeText(
              t,
              row.original.start_date,
              row.original.end_date,
              locale,
            ),
        }),
        createColumn<DateRuleVersion>({
          id: "value",
          accessorFn: (row) => row.value,
          labelKey: "pricing.columns.value",
          enableSorting: false,
          cell: ({ row }) =>
            ruleValueText(row.original.kind, row.original.value, locale),
        }),
        createColumn<DateRuleVersion>({
          accessorKey: "priority",
          labelKey: "pricing.columns.priority",
          enableSorting: false,
        }),
        createColumn<DateRuleVersion>({
          accessorKey: "change_note",
          labelKey: "pricing.columns.change_note",
          enableSorting: false,
          cell: ({ row }) => row.original.change_note || "—",
        }),
        createColumn<DateRuleVersion>({
          id: "activated_at",
          accessorFn: (row) => row.activated_at ?? "",
          labelKey: "pricing.columns.activated_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.activated_at
              ? datetime(row.original.activated_at, undefined, locale)
              : "—",
        }),
      ] as ColumnDef<DateRuleVersion, unknown>[],
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
        persistKey: "tenant-pricing-rule-versions-v1",
        rowSelection: false,
        globalFilter: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
      }}
    />
  );
}

/** Date rule show page: summary, draft editor, versions, activity. */
export function DateRuleDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const query = useDateRule(uuid);
  const rule = query.data;
  const update = useUpdateDateRule();
  const activate = useActivateDateRule();
  const deactivate = useDeactivateDateRule();
  const discard = useDiscardDateRuleDraft();
  const remove = useDeleteDateRule();
  const [activateOpen, setActivateOpen] = useState(false);
  const [confirm, setConfirm] = useState<
    "deactivate" | "discard" | "delete" | null
  >(null);
  const check = useDateRuleCheck(uuid, activateOpen);
  const canWrite = can(permissions.tenant.pricing.write);
  const effective = rule?.draft ?? rule?.active ?? null;
  const targets = effective ? targetsText(t, effective) : null;
  const title = rule?.name ?? t("pricing.rules.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("pricing.rules.detail_description")}
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
          label: t("pricing.rules.title"),
          href: routes.tenant.pricing.rules.root(slug),
        },
        { label: title },
      ]}
      actions={
        rule ? (
          <EntityActions>
            <PermissionGuard permission={permissions.tenant.pricing.write}>
              {rule.has_draft && rule.active_version != null ? (
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
              {rule.status === "active" ? (
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
              {rule.has_draft || rule.status === "inactive" ? (
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
          description={t("pricing.rules.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {rule && effective && targets ? (
        <div className="space-y-6">
          <EntityHeader
            title={rule.name}
            subtitle={rangeText(
              t,
              effective.start_date,
              effective.end_date,
              locale,
            )}
            badges={
              <>
                <StatusChip
                  label={t(STATUS_LABEL_KEYS[rule.status])}
                  tone={STATUS_TONES[rule.status]}
                />
                {rule.has_draft && rule.status !== "draft" ? (
                  <StatusChip
                    label={t("pricing.pending_draft")}
                    tone="warning"
                  />
                ) : null}
                <VersionBadge version={rule.current_version} />
              </>
            }
          />
          <EntitySectionCard title={t("pricing.detail.details")}>
            <EntityDetail
              sections={[
                {
                  id: "rule",
                  fields: [
                    {
                      key: "range",
                      label: t("pricing.columns.range"),
                      value: rangeText(
                        t,
                        effective.start_date,
                        effective.end_date,
                        locale,
                      ),
                    },
                    {
                      key: "value",
                      label: t("pricing.columns.value"),
                      value: `${t(KIND_LABEL_KEYS[effective.kind])}: ${ruleValueText(effective.kind, effective.value, locale)}`,
                    },
                    {
                      key: "priority",
                      label: t("pricing.fields.priority"),
                      value: String(effective.priority),
                    },
                    {
                      key: "products",
                      label: t(
                        PRODUCT_SCOPE_LABEL_KEYS[effective.product_scope],
                      ),
                      value: targets.products,
                    },
                    {
                      key: "branches",
                      label: t("pricing.fields.branches"),
                      value: targets.branches,
                    },
                    {
                      key: "channels",
                      label: t("pricing.fields.channels"),
                      value: targets.channels,
                    },
                    {
                      key: "active_version",
                      label: t("pricing.fields.active_version"),
                      value:
                        rule.active_version != null
                          ? `v${rule.active_version}`
                          : t("pricing.never_activated"),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>
          <Tabs
            defaultValue={canWrite ? "draft" : "versions"}
            className="space-y-4"
          >
            <TabsList className="flex h-auto flex-wrap">
              {canWrite ? (
                <TabsTrigger value="draft">
                  {rule.draft
                    ? t("pricing.tabs.draft_version", {
                        version: rule.draft.version,
                      })
                    : t("pricing.tabs.new_draft")}
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
                    {rule.draft
                      ? t("pricing.draft.description")
                      : t("pricing.draft.from_active", {
                          version: rule.active?.version ?? 1,
                        })}
                  </p>
                  <DateRuleForm
                    key={`${rule.uuid}-${rule.current_version}-${rule.updated_at}`}
                    id={DRAFT_FORM}
                    defaultValues={dateRuleDefaults(rule)}
                    onSubmit={async (values) => {
                      await update.mutateAsync({
                        uuid,
                        body: dateRuleBody(values),
                      });
                    }}
                  />
                  <FormActions sticky={false} className="mt-4">
                    <Button
                      type="submit"
                      form={DRAFT_FORM}
                      disabled={update.isPending}
                    >
                      {t("pricing.actions.save_draft")}
                    </Button>
                  </FormActions>
                </EntitySectionCard>
              </TabsContent>
            ) : null}
            <TabsContent value="versions">
              <EntitySectionCard title={t("pricing.versions.title")}>
                <p className="text-muted-foreground mb-4 text-sm">
                  {t("pricing.versions.help")}
                </p>
                <RuleVersionsTable uuid={uuid} />
              </EntitySectionCard>
            </TabsContent>
            <TabsContent value="activity">
              <PricingActivityCard kind="rules" uuid={uuid} />
            </TabsContent>
          </Tabs>

          <ActivationDialog
            open={activateOpen}
            onOpenChange={setActivateOpen}
            title={t("pricing.activation.rule_title")}
            description={t("pricing.activation.rule_description")}
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
                : t("pricing.confirm.deactivate_rule_body")
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
            entityLabel={rule.name}
            isPending={remove.isPending}
            onCancel={() => setConfirm(null)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.pricing.rules.root(slug)),
                onSettled: () => setConfirm(null),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
