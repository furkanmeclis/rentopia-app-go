"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, Copy, Eye, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
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
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CONTRACT_KINDS,
  KIND_LABEL_KEYS,
  TEMPLATES_RESOURCE,
  TEMPLATE_STATUSES,
  TEMPLATE_STATUS_LABEL_KEYS,
  TEMPLATE_STATUS_TONES,
} from "@/features/contracts/constants";
import {
  useContractTemplates,
  useTemplateAction,
} from "@/features/contracts/hooks/use-contracts";
import type {
  ContractTemplate,
  ListTemplatesParams,
} from "@/features/contracts/types";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { actorName } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Pending = {
  template: ContractTemplate;
  action: "activate" | "new_version" | "delete";
} | null;

/**
 * Contract templates ("Sözleşme şablonları"): one row per version; one
 * active version per kind. Drafts are edited, activated (the previous
 * active version is archived) or deleted; an active / archived version
 * is copied into a new draft.
 */
export function ContractTemplatesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, TEMPLATES_RESOURCE),
  });
  const action = useTemplateAction();
  const [pending, setPending] = useState<Pending>(null);

  const params = useMemo<ListTemplatesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      kind: joinFilter(column("kind")),
      status: joinFilter(column("status")),
    };
  }, [listState.columnFilters, listState.params]);
  const query = useContractTemplates(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ContractTemplate>({
          accessorKey: "name",
          labelKey: "contracts.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<ContractTemplate>({
          accessorKey: "kind",
          labelKey: "contracts.columns.kind",
          enableSorting: false,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: CONTRACT_KINDS.map((v) => ({
            value: v,
            labelKey: KIND_LABEL_KEYS[v],
            label: t(KIND_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(KIND_LABEL_KEYS[row.original.kind])}
            </Badge>
          ),
        }),
        createColumn<ContractTemplate>({
          accessorKey: "version",
          labelKey: "contracts.columns.version",
          enableSorting: true,
          cell: ({ row }) => `v${row.original.version}`,
        }),
        createColumn<ContractTemplate>({
          accessorKey: "status",
          labelKey: "contracts.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: TEMPLATE_STATUSES.map((v) => ({
            value: v,
            labelKey: TEMPLATE_STATUS_LABEL_KEYS[v],
            label: t(TEMPLATE_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(TEMPLATE_STATUS_LABEL_KEYS[row.original.status])}
              tone={TEMPLATE_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<ContractTemplate>({
          id: "placeholders",
          accessorFn: (row) => row.placeholders.length,
          labelKey: "contracts.columns.placeholders",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.placeholders.length,
        }),
        createColumn<ContractTemplate>({
          accessorKey: "used_by",
          labelKey: "contracts.columns.used_by",
          enableSorting: false,
          cell: ({ row }) => row.original.used_by,
        }),
        createColumn<ContractTemplate>({
          id: "activated_at",
          accessorFn: (row) => row.activated_at ?? "",
          labelKey: "contracts.columns.activated_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.activated_at
              ? datetime(row.original.activated_at, "d MMM yyyy HH:mm", locale)
              : "—",
        }),
        createColumn<ContractTemplate>({
          id: "created_by",
          accessorFn: (row) => row.created_by?.email ?? "",
          labelKey: "contracts.columns.created_by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => actorName(row.original.created_by),
        }),
        createColumn<ContractTemplate>({
          accessorKey: "created_at",
          labelKey: "contracts.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<ContractTemplate>({
          id: "actions",
          labelKey: "contracts.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const tpl = row.original;
            const draft = tpl.status === "draft";
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: draft
                      ? t("contracts.actions.edit")
                      : t("contracts.actions.open"),
                    icon: Eye,
                    onSelect: () =>
                      router.push(
                        routes.tenant.settings.contractTemplates.detail(
                          slug,
                          tpl.uuid,
                        ),
                      ),
                  },
                  ...(draft
                    ? [
                        {
                          id: "activate",
                          label: t("contracts.actions.activate"),
                          icon: CheckCircle2,
                          permission: permissions.tenant.contracts.activate,
                          onSelect: () =>
                            setPending({ template: tpl, action: "activate" }),
                        },
                        {
                          id: "delete",
                          label: t("contracts.actions.delete"),
                          icon: Trash2,
                          permission: permissions.tenant.contracts.write,
                          variant: "destructive" as const,
                          onSelect: () =>
                            setPending({ template: tpl, action: "delete" }),
                        },
                      ]
                    : [
                        {
                          id: "new_version",
                          label: t("contracts.actions.new_version"),
                          icon: Copy,
                          permission: permissions.tenant.contracts.write,
                          onSelect: () =>
                            setPending({
                              template: tpl,
                              action: "new_version",
                            }),
                        },
                      ]),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ContractTemplate, unknown>[],
    [locale, router, slug, t],
  );

  const copy = pending
    ? {
        activate: {
          title: t("contracts.activate.title"),
          description: t("contracts.activate.description", {
            name: pending.template.name,
            version: pending.template.version,
          }),
          label: t("contracts.actions.activate"),
        },
        new_version: {
          title: t("contracts.new_version.title"),
          description: t("contracts.new_version.description", {
            name: pending.template.name,
          }),
          label: t("contracts.actions.new_version"),
        },
        delete: {
          title: t("contracts.delete.title"),
          description: t("contracts.delete.description", {
            name: pending.template.name,
          }),
          label: t("contracts.actions.delete"),
        },
      }[pending.action]
    : null;

  return (
    <EntityPage
      title={t("contracts.title")}
      description={t("contracts.description")}
      permission={permissions.tenant.contracts.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("contracts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("contracts.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() =>
            router.push(routes.tenant.settings.contractTemplates.create(slug))
          }
          label={t("contracts.actions.create")}
          permission={permissions.tenant.contracts.write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(
            routes.tenant.settings.contractTemplates.detail(slug, row.uuid),
          )
        }
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("contracts.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("contracts.empty_title")}
        emptyDescription={t("contracts.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-contract-templates-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <ConfirmDialog
        open={Boolean(pending)}
        title={copy?.title ?? ""}
        description={copy?.description}
        confirmLabel={copy?.label}
        variant={pending?.action === "delete" ? "destructive" : "default"}
        isPending={action.isPending}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (!pending) return;
          action.mutate(
            { uuid: pending.template.uuid, action: pending.action },
            {
              onSuccess: (out) => {
                const next = pending.action;
                setPending(null);
                if (next === "new_version" && out && "uuid" in out) {
                  router.push(
                    routes.tenant.settings.contractTemplates.detail(
                      slug,
                      out.uuid,
                    ),
                  );
                }
              },
            },
          );
        }}
      />
    </EntityPage>
  );
}
