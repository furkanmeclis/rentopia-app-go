"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  ListChecks,
  MessageSquare,
  Pencil,
  PlugZap,
  Power,
  PowerOff,
} from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { BrandLogo } from "@/components/brand/brand-logo";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CHECK_STATUS_LABEL_KEYS,
  INTEGRATION_FIELD_LABEL_KEYS,
  INTEGRATION_PROVIDER_LABEL_KEYS,
} from "@/features/payments/constants";
import {
  ConnectIntegrationDialog,
  EditIntegrationDialog,
  TestSmsDialog,
} from "@/features/payments/components/integration-dialogs";
import {
  IntegrationJobDrawer,
  IntegrationJobsSection,
} from "@/features/payments/components/integration-jobs";
import {
  useIntegrations,
  useSetIntegrationStatus,
  useTestIntegration,
} from "@/features/payments/hooks/use-payments";
import type { IntegrationAccount } from "@/features/payments/types";
import { reasonText } from "@/features/reservations/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Settings → Entegrasyonlar (TEC-676 / TEC-734 / TEC-751): the
 * organization's provider accounts (iyzico payments, Netgsm SMS). Connect
 * with the organization's own keys, test the connection (and send a test
 * SMS), rotate keys, disable. Secrets are never shown again. "İşler" lists an
 * account's integration jobs; a job opens a drawer with its attempts and
 * the retry.
 */
export function IntegrationsPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const manage = permissions.tenant.integrations.manage;
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const [connectOpen, setConnectOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<IntegrationAccount | null>(null);
  const [disableTarget, setDisableTarget] = useState<IntegrationAccount | null>(
    null,
  );
  const [smsTarget, setSmsTarget] = useState<IntegrationAccount | null>(null);
  const [jobsAccount, setJobsAccount] = useState<IntegrationAccount | null>(
    null,
  );
  const [jobUuid, setJobUuid] = useState<string | null>(null);
  const openJob = useCallback((uuid: string) => setJobUuid(uuid), []);
  const query = useIntegrations(listState.params);
  const test = useTestIntegration();
  const setStatus = useSetIntegrationStatus();
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<IntegrationAccount>({
          accessorKey: "display_name",
          labelKey: "payments.integrations.columns.display_name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex items-center gap-3">
              <BrandLogo provider={row.original.provider} />
              <div className="flex min-w-0 flex-col">
                <span className="font-medium">{row.original.display_name}</span>
                <span className="text-muted-foreground text-xs">
                  {reasonText(
                    t,
                    INTEGRATION_PROVIDER_LABEL_KEYS,
                    row.original.provider,
                  )}
                </span>
              </div>
            </div>
          ),
        }),
        createColumn<IntegrationAccount>({
          accessorKey: "mode",
          labelKey: "payments.integrations.columns.mode",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge
              variant={row.original.mode === "live" ? "success" : "warning"}
            >
              {t(
                row.original.mode === "live"
                  ? "payments.integrations.modes.live"
                  : "payments.integrations.modes.sandbox",
              )}
            </Badge>
          ),
        }),
        createColumn<IntegrationAccount>({
          accessorKey: "status",
          labelKey: "payments.integrations.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                row.original.status === "active"
                  ? "payments.integrations.statuses.active"
                  : "payments.integrations.statuses.disabled",
              )}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<IntegrationAccount>({
          id: "credentials",
          accessorFn: (row) => Object.values(row.credentials_hint).join(" "),
          labelKey: "payments.integrations.columns.credentials",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col gap-0.5 font-mono text-xs">
              {Object.entries(row.original.credentials_hint).map(([k, v]) => (
                <span key={k}>
                  <span className="text-muted-foreground">
                    {reasonText(t, INTEGRATION_FIELD_LABEL_KEYS, k)}:
                  </span>{" "}
                  {v}
                </span>
              ))}
            </div>
          ),
        }),
        createColumn<IntegrationAccount>({
          accessorKey: "last_check_status",
          labelKey: "payments.integrations.columns.last_check",
          enableSorting: false,
          cell: ({ row }) => {
            const a = row.original;
            return (
              <div className="flex flex-col gap-0.5">
                <StatusChip
                  label={reasonText(
                    t,
                    CHECK_STATUS_LABEL_KEYS,
                    a.last_check_status,
                  )}
                  tone={
                    a.last_check_status === "ok"
                      ? "success"
                      : a.last_check_status === "failed"
                        ? "danger"
                        : "default"
                  }
                />
                {a.last_checked_at ? (
                  <span className="text-muted-foreground text-xs">
                    {datetime(a.last_checked_at, "d MMM yyyy HH:mm", locale)}
                  </span>
                ) : null}
                {a.last_check_error ? (
                  <span className="text-destructive line-clamp-2 text-xs">
                    {a.last_check_error}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<IntegrationAccount>({
          accessorKey: "created_at",
          labelKey: "payments.integrations.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<IntegrationAccount>({
          id: "actions",
          labelKey: "payments.integrations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const a = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "test",
                    label: t("payments.integrations.actions.test"),
                    icon: PlugZap,
                    permission: manage,
                    disabled: test.isPending,
                    onSelect: () => test.mutate(a.uuid),
                  },
                  {
                    id: "jobs",
                    label: t("payments.integrations.actions.jobs"),
                    icon: ListChecks,
                    permission: manage,
                    onSelect: () => setJobsAccount(a),
                  },
                  ...(a.kind === "sms"
                    ? [
                        {
                          id: "test_sms",
                          label: t("payments.integrations.actions.test_sms"),
                          icon: MessageSquare,
                          permission: manage,
                          onSelect: () => setSmsTarget(a),
                        },
                      ]
                    : []),
                  {
                    id: "edit",
                    label: t("payments.integrations.actions.edit"),
                    icon: Pencil,
                    permission: manage,
                    onSelect: () => setEditTarget(a),
                  },
                  a.status === "active"
                    ? {
                        id: "disable",
                        label: t("payments.integrations.actions.disable"),
                        icon: PowerOff,
                        permission: manage,
                        variant: "destructive" as const,
                        separatorBefore: true,
                        onSelect: () => setDisableTarget(a),
                      }
                    : {
                        id: "enable",
                        label: t("payments.integrations.actions.enable"),
                        icon: Power,
                        permission: manage,
                        separatorBefore: true,
                        onSelect: () =>
                          setStatus.mutate({ uuid: a.uuid, action: "enable" }),
                      },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<IntegrationAccount, unknown>[],
    [locale, manage, setStatus, t, test],
  );

  return (
    <EntityPage
      title={t("payments.integrations.title")}
      description={t("payments.integrations.description")}
      permission={manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("payments.integrations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("payments.integrations.title") },
      ]}
      actions={
        <EntityCreateButton
          label={t("payments.integrations.connect")}
          permission={manage}
          onClick={() => setConnectOpen(true)}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("payments.integrations.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("payments.integrations.empty_title")}
        emptyDescription={t("payments.integrations.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey: "tenant-integrations-v1", rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      {jobsAccount ? (
        <div className="mt-6">
          <IntegrationJobsSection account={jobsAccount} onOpenJob={openJob} />
        </div>
      ) : null}
      <IntegrationJobDrawer
        jobUuid={jobUuid}
        onClose={() => setJobUuid(null)}
      />
      <TestSmsDialog account={smsTarget} onClose={() => setSmsTarget(null)} />
      <ConnectIntegrationDialog
        open={connectOpen}
        onClose={() => setConnectOpen(false)}
      />
      <EditIntegrationDialog
        account={editTarget}
        onClose={() => setEditTarget(null)}
      />
      <ConfirmDialog
        open={Boolean(disableTarget)}
        title={t("payments.integrations.disable_title")}
        description={t("payments.integrations.disable_description")}
        confirmLabel={t("payments.integrations.actions.disable")}
        variant="destructive"
        isPending={setStatus.isPending}
        onCancel={() => setDisableTarget(null)}
        onConfirm={() => {
          if (!disableTarget) return;
          setStatus.mutate(
            { uuid: disableTarget.uuid, action: "disable" },
            { onSuccess: () => setDisableTarget(null) },
          );
        }}
      />
    </EntityPage>
  );
}
