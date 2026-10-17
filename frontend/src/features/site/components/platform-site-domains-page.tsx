"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { RefreshCw, ShieldCheck } from "lucide-react";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { SiteDomain } from "@/features/site/types";
import { platformRequest } from "@/lib/api/platform-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const BASE = "/v1/platform/site-domains";
const KEY = ["platform", "site-domains"] as const;

/**
 * Every organization's custom domain (TEC-731). In manual TLS mode the
 * platform team adds verified hosts to the `site` service in Dokploy and
 * then marks them active here.
 */
export function PlatformSiteDomainsPage() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const list = useQuery({
    queryKey: KEY,
    queryFn: () =>
      platformRequest<SiteDomain[]>("GET", BASE, { query: { limit: 200 } }),
  });
  const action = useMutation({
    mutationFn: ({
      uuid,
      op,
    }: {
      uuid: string;
      op: "recheck" | "mark-tls-active";
    }) =>
      platformRequest<SiteDomain>(
        "POST",
        `${BASE}/${encodeURIComponent(uuid)}/${op}`,
        { body: {} },
      ),
    onSuccess: (_d, { op }) => {
      appToast.success(t(`site.platform_domains.toast.${op}`));
      void queryClient.invalidateQueries({ queryKey: KEY });
    },
  });

  const columns = useMemo(
    () =>
      [
        createColumn<SiteDomain>({
          accessorKey: "host",
          labelKey: "site.domains.columns.host",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.host}</span>
              {row.original.last_error ? (
                <span className="text-destructive text-xs">
                  {row.original.last_error}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<SiteDomain>({
          id: "organization",
          accessorFn: (row) => String(row.organization_id),
          labelKey: "site.platform_domains.organization",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<SiteDomain>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "site.domains.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`site.domains.status.${row.original.status}`)}
              tone={
                row.original.status === "verified"
                  ? "success"
                  : row.original.status === "failed"
                    ? "danger"
                    : "warning"
              }
            />
          ),
        }),
        createColumn<SiteDomain>({
          id: "tls",
          accessorFn: (row) => row.tls_status,
          labelKey: "site.domains.columns.tls",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`site.domains.tls.${row.original.tls_status}`)}
              tone={
                row.original.tls_status === "active" ? "success" : "warning"
              }
            />
          ),
        }),
        createColumn<SiteDomain>({
          id: "actions",
          accessorFn: () => "",
          labelKey: "site.domains.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "recheck",
                  label: t("site.platform_domains.recheck"),
                  icon: RefreshCw,
                  permission: permissions.siteDomains.manage,
                  disabled: action.isPending,
                  onSelect: () =>
                    action.mutate({ uuid: row.original.uuid, op: "recheck" }),
                },
                {
                  id: "tls",
                  label: t("site.platform_domains.mark_tls_active"),
                  icon: ShieldCheck,
                  permission: permissions.siteDomains.manage,
                  disabled:
                    action.isPending ||
                    row.original.status !== "verified" ||
                    row.original.tls_status === "active",
                  onSelect: () =>
                    action.mutate({
                      uuid: row.original.uuid,
                      op: "mark-tls-active",
                    }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<SiteDomain, unknown>[],
    [action, t],
  );

  return (
    <EntityPage
      title={t("site.platform_domains.title")}
      description={t("site.platform_domains.description")}
      permission={permissions.siteDomains.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("site.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("site.platform_domains.title") },
      ]}
    >
      <EntityTable
        isLoading={list.isLoading}
        isError={list.isError}
        errorDescription={t("site.domains.load_failed")}
        onRetry={() => void list.refetch()}
        manual={{ filtering: false, sorting: false, pagination: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
        columns={columns}
        data={list.data ?? []}
        getRowId={(row) => row.uuid}
        emptyTitle={t("site.domains.empty")}
        features={{
          persistKey: "platform-site-domains-v1",
          rowSelection: false,
        }}
      />
    </EntityPage>
  );
}
