"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Code2, Pencil, RefreshCw } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityCreateButton,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  CHANNELS,
  CHANNEL_LABEL_KEYS,
  KEY_KINDS,
  KEY_KIND_LABEL_KEYS,
  KEY_STATUSES,
  KEY_STATUS_LABEL_KEYS,
  PUBLIC_API_KEYS_RESOURCE,
  SCOPE_LABEL_KEYS,
} from "@/features/public-api/constants";
import {
  KeyFormDialog,
  RotateKeyDialog,
} from "@/features/public-api/components/key-dialogs";
import {
  SecretDialog,
  type ShownSecret,
} from "@/features/public-api/components/secret-dialog";
import {
  usePublicApiKeys,
  useRevokePublicApiKey,
} from "@/features/public-api/hooks/use-public-api";
import type {
  IssuedPublicApiKey,
  ListKeysParams,
  PublicApiKey,
} from "@/features/public-api/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useFeature } from "@/providers/feature-provider";
import { useLocale } from "@/providers/locale-provider";

/** The widget / site snippet of a publishable key. */
function embedSnippet(baseUrl: string, key: string) {
  const api = `${baseUrl || "https://<panel>/api"}/v1/public`;
  return [
    `// 1. ${api}/sessions → booking session (from an allowed origin)`,
    `const res = await fetch("${api}/sessions", {`,
    `  method: "POST",`,
    `  headers: { "Content-Type": "application/json" },`,
    `  body: JSON.stringify({ publishable_key: "${key}" }),`,
    `});`,
    `const { data } = await res.json();`,
    `// 2. Bearer data.token on /availability, /quotes, /reservations …`,
  ].join("\n");
}

/**
 * API keys of the public booking API: secret keys (servers) and
 * publishable keys (site / widget, allowed origins). Secrets are shown once
 * when issued or rotated.
 */
export function KeysTable({ baseUrl }: { baseUrl: string }) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const manage = permissions.tenant.publicApi.manage;
  const inPlan = useFeature("public_api");
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, PUBLIC_API_KEYS_RESOURCE),
  });
  const params = useMemo<ListKeysParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      kind: joinFilter(column("kind")),
      channel: joinFilter(column("channel")),
      status: joinFilter(column("status")),
    };
  }, [listState.columnFilters, listState.params]);
  const query = usePublicApiKeys(params);
  const revoke = useRevokePublicApiKey();
  const total = query.data?.total ?? 0;
  const [formOpen, setFormOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<PublicApiKey | null>(null);
  const [rotateTarget, setRotateTarget] = useState<PublicApiKey | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<PublicApiKey | null>(null);
  const [shown, setShown] = useState<ShownSecret | null>(null);

  const showIssued = (issued: IssuedPublicApiKey) => {
    const publishable = issued.key.kind === "publishable";
    setShown({
      title: t(
        publishable
          ? "public_api.secret.publishable_title"
          : "public_api.secret.secret_title",
      ),
      description: t(
        publishable
          ? "public_api.secret.publishable_description"
          : "public_api.secret.secret_description",
      ),
      secret: issued.secret,
      once: !publishable,
      snippet: publishable ? embedSnippet(baseUrl, issued.secret) : undefined,
    });
  };

  const columns = useMemo(
    () =>
      [
        createColumn<PublicApiKey>({
          accessorKey: "name",
          labelKey: "public_api.keys.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex min-w-0 flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground font-mono text-xs">
                {row.original.key_prefix}…{row.original.key_last4}
              </span>
            </div>
          ),
        }),
        createColumn<PublicApiKey>({
          accessorKey: "kind",
          labelKey: "public_api.keys.columns.kind",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: KEY_KINDS.map((v) => ({
            value: v,
            labelKey: KEY_KIND_LABEL_KEYS[v],
            label: t(KEY_KIND_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge
              variant={row.original.kind === "secret" ? "warning" : "outline"}
            >
              {t(KEY_KIND_LABEL_KEYS[row.original.kind])}
            </Badge>
          ),
        }),
        createColumn<PublicApiKey>({
          accessorKey: "channel",
          labelKey: "public_api.keys.columns.channel",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CHANNELS.map((v) => ({
            value: v,
            labelKey: CHANNEL_LABEL_KEYS[v],
            label: t(CHANNEL_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => t(CHANNEL_LABEL_KEYS[row.original.channel]),
        }),
        createColumn<PublicApiKey>({
          accessorKey: "status",
          labelKey: "public_api.keys.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: KEY_STATUSES.map((v) => ({
            value: v,
            labelKey: KEY_STATUS_LABEL_KEYS[v]!,
            label: t(KEY_STATUS_LABEL_KEYS[v]!),
          })),
          cell: ({ row }) => {
            const k = row.original;
            return (
              <div className="flex flex-col gap-0.5">
                <StatusChip
                  label={t(KEY_STATUS_LABEL_KEYS[k.status]!)}
                  tone={k.status === "active" ? "success" : "default"}
                />
                {k.status === "active" && k.expires_at ? (
                  <span className="text-muted-foreground text-xs">
                    {t("public_api.keys.expires", {
                      date: datetime(k.expires_at, "d MMM yyyy HH:mm", locale),
                    })}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<PublicApiKey>({
          id: "scopes",
          accessorFn: (row) => row.scopes.join(" "),
          labelKey: "public_api.keys.columns.scopes",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.scopes.map((s) => (
                <Badge key={s} variant="secondary">
                  {t(SCOPE_LABEL_KEYS[s])}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<PublicApiKey>({
          id: "allowed_origins",
          accessorFn: (row) => row.allowed_origins.join(" "),
          labelKey: "public_api.keys.columns.allowed_origins",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.allowed_origins.length ? (
              <div className="flex flex-col font-mono text-xs">
                {row.original.allowed_origins.map((o) => (
                  <span key={o}>{o}</span>
                ))}
              </div>
            ) : (
              "—"
            ),
        }),
        createColumn<PublicApiKey>({
          accessorKey: "rate_limit_per_minute",
          labelKey: "public_api.keys.columns.rate_limit",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            t("public_api.keys.per_minute", {
              count: row.original.rate_limit_per_minute,
            }),
        }),
        createColumn<PublicApiKey>({
          accessorKey: "last_used_at",
          labelKey: "public_api.keys.columns.last_used_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.last_used_at
              ? datetime(row.original.last_used_at, "d MMM yyyy HH:mm", locale)
              : t("public_api.keys.never_used"),
        }),
        createColumn<PublicApiKey>({
          accessorKey: "created_at",
          labelKey: "public_api.keys.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<PublicApiKey>({
          id: "actions",
          labelKey: "public_api.keys.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const k = row.original;
            if (k.status !== "active") return null;
            return (
              <EntityRowActions
                actions={[
                  ...(k.publishable_key
                    ? [
                        {
                          id: "snippet",
                          label: t("public_api.keys.actions.snippet"),
                          icon: Code2,
                          onSelect: () =>
                            setShown({
                              title: t("public_api.secret.publishable_title"),
                              description: t(
                                "public_api.secret.publishable_description",
                              ),
                              secret: k.publishable_key ?? "",
                              once: false,
                              snippet: embedSnippet(
                                baseUrl,
                                k.publishable_key ?? "",
                              ),
                            }),
                        },
                      ]
                    : []),
                  {
                    id: "edit",
                    label: t("public_api.keys.actions.edit"),
                    icon: Pencil,
                    permission: manage,
                    onSelect: () => setEditTarget(k),
                  },
                  {
                    id: "rotate",
                    label: t("public_api.keys.actions.rotate"),
                    icon: RefreshCw,
                    permission: manage,
                    onSelect: () => setRotateTarget(k),
                  },
                  {
                    id: "revoke",
                    label: t("public_api.keys.actions.revoke"),
                    icon: Ban,
                    permission: manage,
                    variant: "destructive" as const,
                    separatorBefore: true,
                    onSelect: () => setRevokeTarget(k),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<PublicApiKey, unknown>[],
    [baseUrl, locale, manage, t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("public_api.keys.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("public_api.keys.empty_title")}
        emptyDescription={t("public_api.keys.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-public-api-keys-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <div className="flex items-center gap-2">
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
            {inPlan ? (
              <EntityCreateButton
                label={t("public_api.keys.create")}
                permission={manage}
                onClick={() => setFormOpen(true)}
              />
            ) : null}
          </div>
        }
      />
      <KeyFormDialog
        open={formOpen || Boolean(editTarget)}
        apiKey={editTarget}
        onClose={() => {
          setFormOpen(false);
          setEditTarget(null);
        }}
        onIssued={showIssued}
      />
      <RotateKeyDialog
        apiKey={rotateTarget}
        onClose={() => setRotateTarget(null)}
        onIssued={showIssued}
      />
      <SecretDialog shown={shown} onClose={() => setShown(null)} />
      <ConfirmDialog
        open={Boolean(revokeTarget)}
        title={t("public_api.keys.revoke_title")}
        description={t("public_api.keys.revoke_description")}
        confirmLabel={t("public_api.keys.actions.revoke")}
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
    </>
  );
}
