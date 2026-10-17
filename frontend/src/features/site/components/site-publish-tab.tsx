"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { RotateCcw, Upload } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useRestoreSiteVersion,
  useSiteVersions,
} from "@/features/site/hooks/use-site";
import type { SiteRevision, SiteSettings } from "@/features/site/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Yayın: the publish state and the history of published revisions
 * (DataTable). Restoring copies a revision into the draft; publishing it
 * again rolls the site back.
 */
export function SitePublishTab({
  settings,
  canManage,
  dirty,
  onPublish,
  publishing,
  onDraftReplaced,
}: {
  settings: SiteSettings;
  canManage: boolean;
  /** The form has unsaved edits (publish needs a saved draft). */
  dirty: boolean;
  onPublish: () => void;
  publishing: boolean;
  /** A restore replaced the draft: the form reloads it. */
  onDraftReplaced: () => void;
}) {
  const { t, locale } = useLocale();
  const versions = useSiteVersions();
  const restore = useRestoreSiteVersion();
  const [restoring, setRestoring] = useState<SiteRevision | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<SiteRevision>({
          accessorKey: "revision",
          labelKey: "site.publish.columns.revision",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-2 font-medium tabular-nums">
              r{row.original.revision}
              {row.original.current ? (
                <Badge variant="secondary">{t("site.publish.current")}</Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<SiteRevision>({
          accessorKey: "published_at",
          labelKey: "site.publish.columns.published_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.published_at, undefined, locale),
        }),
        createColumn<SiteRevision>({
          accessorKey: "published_by",
          labelKey: "site.publish.columns.published_by",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.published_by || (
              <span className="text-muted-foreground">
                {t("site.publish.system")}
              </span>
            ),
        }),
        createColumn<SiteRevision>({
          id: "images",
          accessorFn: (row) =>
            [row.assets.logo, row.assets.favicon, row.assets.cover].filter(
              Boolean,
            ).length,
          labelKey: "site.publish.columns.images",
          enableSorting: false,
          cell: ({ row }) => {
            const kinds = (["logo", "favicon", "cover"] as const).filter(
              (k) => row.original.assets[k],
            );
            return kinds.length ? (
              <span className="flex flex-wrap gap-1">
                {kinds.map((k) => (
                  <StatusChip key={k} label={t(`site.images.kinds.${k}`)} />
                ))}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            );
          },
        }),
        createColumn<SiteRevision>({
          id: "actions",
          labelKey: "site.publish.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canManage ? (
              <EntityRowActions
                actions={[
                  {
                    id: "restore",
                    label: t("site.publish.restore"),
                    icon: RotateCcw,
                    onSelect: () => setRestoring(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<SiteRevision, unknown>[],
    [canManage, locale, t],
  );

  return (
    <div className="space-y-4">
      <EntitySectionCard title={t("site.publish.state_title")}>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="space-y-1 text-sm">
            <p>
              {settings.published_revision
                ? t("site.publish.live_revision", {
                    revision: settings.published_revision,
                    date: settings.published_at
                      ? datetime(settings.published_at, undefined, locale)
                      : "—",
                  })
                : t("site.general.never_published")}
            </p>
            <p className="text-muted-foreground">
              {settings.has_unpublished_changes
                ? t("site.publish.pending_changes")
                : t("site.publish.up_to_date")}
            </p>
          </div>
          {canManage ? (
            <Button
              type="button"
              disabled={
                dirty || publishing || !settings.has_unpublished_changes
              }
              onClick={onPublish}
            >
              <Upload className="size-4" />
              {t("site.publish.publish")}
            </Button>
          ) : null}
        </div>
        {dirty ? (
          <Alert className="mt-4">
            <AlertTitle>{t("site.publish.save_first_title")}</AlertTitle>
            <AlertDescription>
              {t("site.publish.save_first_description")}
            </AlertDescription>
          </Alert>
        ) : null}
      </EntitySectionCard>
      <EntityTable
        columns={columns}
        data={versions.data ?? []}
        getRowId={(row) => row.uuid}
        isLoading={versions.isLoading}
        isError={versions.isError}
        errorDescription={t("site.publish.load_failed")}
        onRetry={() => void versions.refetch()}
        emptyTitle={t("site.publish.empty_title")}
        emptyDescription={t("site.publish.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-site-versions-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void versions.refetch()}
            refreshDisabled={versions.isFetching}
          />
        }
      />
      <ConfirmDialog
        open={restoring !== null}
        title={t("site.publish.restore_title", {
          revision: restoring?.revision ?? "",
        })}
        description={t("site.publish.restore_description")}
        confirmLabel={t("site.publish.restore")}
        cancelLabel={t("common.cancel")}
        isPending={restore.isPending}
        onCancel={() => setRestoring(null)}
        onConfirm={() => {
          if (!restoring) return;
          restore.mutate(
            { revision: restoring.uuid, version: settings.version },
            { onSuccess: onDraftReplaced, onSettled: () => setRestoring(null) },
          );
        }}
      />
    </div>
  );
}
