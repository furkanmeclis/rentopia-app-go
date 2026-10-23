"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, LifeBuoy } from "lucide-react";
import { useMemo, useState } from "react";

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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { OpenGrantDialog } from "@/features/support-access/components/open-grant-dialog";
import {
  SCOPE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
  SUPPORT_STATUSES,
} from "@/features/support-access/constants";
import {
  useRevokeSupportGrant,
  useSupportGrants,
} from "@/features/support-access/hooks/use-support-access";
import type {
  ListSupportGrantsParams,
  SupportGrant,
} from "@/features/support-access/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DATE_FORMAT = "d MMM yyyy HH:mm";

function personName(p: SupportGrant["created_by"]) {
  if (!p) return "—";
  const name = `${p.name} ${p.surname}`.trim();
  return name || p.email;
}

/**
 * Settings → Support access (TEC-678): the organization decides when
 * platform support staff may enter its panel, for how long and whether
 * they may change anything. Every grant is listed with its status;
 * active ones can be revoked at any time.
 */
export function SupportAccessPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const manage = permissions.tenant.supportAccess.manage;
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const params = useMemo<ListSupportGrantsParams>(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      status: joinFilter(filterValues(listState.columnFilters, "status")),
    }),
    [listState.columnFilters, listState.params],
  );
  const query = useSupportGrants(params);
  const activeQuery = useSupportGrants({ status: "active", limit: 1 });
  const active = activeQuery.data?.items[0];
  const revoke = useRevokeSupportGrant();
  const total = query.data?.total ?? 0;
  const [formOpen, setFormOpen] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<SupportGrant | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<SupportGrant>({
          accessorKey: "status",
          labelKey: "support_access.columns.status",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: SUPPORT_STATUSES.map((v) => ({
            value: v,
            labelKey: STATUS_LABEL_KEYS[v],
            label: t(STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<SupportGrant>({
          accessorKey: "scope",
          labelKey: "support_access.columns.scope",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge
              variant={row.original.scope === "full" ? "warning" : "outline"}
            >
              {t(SCOPE_LABEL_KEYS[row.original.scope])}
            </Badge>
          ),
        }),
        createColumn<SupportGrant>({
          accessorKey: "expires_at",
          labelKey: "support_access.columns.expires_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.expires_at, DATE_FORMAT, locale),
        }),
        createColumn<SupportGrant>({
          accessorKey: "note",
          labelKey: "support_access.columns.note",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.note ? (
              <span className="line-clamp-2 max-w-xs">{row.original.note}</span>
            ) : (
              "—"
            ),
        }),
        createColumn<SupportGrant>({
          id: "created_by",
          accessorFn: (row) => personName(row.created_by),
          labelKey: "support_access.columns.created_by",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex min-w-0 flex-col">
              <span>{personName(row.original.created_by)}</span>
              <span className="text-muted-foreground text-xs">
                {datetime(row.original.created_at, DATE_FORMAT, locale)}
              </span>
            </div>
          ),
        }),
        createColumn<SupportGrant>({
          id: "revoked",
          accessorFn: (row) => row.revoked_at ?? "",
          labelKey: "support_access.columns.revoked",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.revoked_at ? (
              <div className="flex min-w-0 flex-col">
                <span>{personName(row.original.revoked_by)}</span>
                <span className="text-muted-foreground text-xs">
                  {datetime(row.original.revoked_at, DATE_FORMAT, locale)}
                </span>
              </div>
            ) : (
              "—"
            ),
        }),
        createColumn<SupportGrant>({
          id: "actions",
          labelKey: "support_access.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            row.original.status === "active" ? (
              <EntityRowActions
                actions={[
                  {
                    id: "revoke",
                    label: t("support_access.actions.revoke"),
                    icon: Ban,
                    permission: manage,
                    variant: "destructive" as const,
                    onSelect: () => setRevokeTarget(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<SupportGrant, unknown>[],
    [locale, manage, t],
  );

  return (
    <EntityPage
      title={t("support_access.title")}
      description={t("support_access.description")}
      permission={manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("support_access.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("support_access.title") },
      ]}
    >
      <Alert className="mb-6">
        <LifeBuoy className="size-4" />
        <AlertTitle>
          {active
            ? t("support_access.banner.active_title", {
                scope: t(SCOPE_LABEL_KEYS[active.scope]),
              })
            : t("support_access.banner.none_title")}
        </AlertTitle>
        <AlertDescription>
          {active
            ? t("support_access.banner.active_body", {
                date: datetime(active.expires_at, DATE_FORMAT, locale),
              })
            : t("support_access.banner.none_body")}
        </AlertDescription>
      </Alert>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("support_access.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t(
          params.status
            ? "support_access.empty_filtered_title"
            : "support_access.empty_title",
        )}
        emptyDescription={t(
          params.status
            ? "support_access.empty_filtered_description"
            : "support_access.empty_description",
        )}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-support-access-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <div className="flex items-center gap-2">
            <EntityToolbar
              onRefresh={() => {
                void query.refetch();
                void activeQuery.refetch();
              }}
              refreshDisabled={query.isFetching}
            />
            <EntityCreateButton
              label={t("support_access.open.button")}
              permission={manage}
              onClick={() => setFormOpen(true)}
            />
          </div>
        }
      />
      <OpenGrantDialog
        open={formOpen}
        hasActive={Boolean(active)}
        onClose={() => setFormOpen(false)}
      />
      <ConfirmDialog
        open={Boolean(revokeTarget)}
        title={t("support_access.revoke.title")}
        description={t("support_access.revoke.description")}
        confirmLabel={t("support_access.actions.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onCancel={() => setRevokeTarget(null)}
        onConfirm={() => {
          if (!revokeTarget) return;
          revoke.mutate(revokeTarget.uuid, {
            onSuccess: () => setRevokeTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
