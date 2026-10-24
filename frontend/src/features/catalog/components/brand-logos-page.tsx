"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Link2, PlugZap, RefreshCw, Save } from "lucide-react";
import { useMemo, useState } from "react";

import { BrandMark } from "@/components/common/brand-mark";
import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import {
  CatalogTabs,
  useCatalogBreadcrumbs,
} from "@/features/catalog/components/catalog-tabs";
import {
  DisplayBrandMotomarksDialog,
  type MotomarksDialogBrand,
} from "@/features/catalog/components/display-brand-motomarks-dialog";
import {
  useBrandLogoRuns,
  useBrandLogoSettings,
  usePatchBrandLogoSettings,
  useRequestBrandLogoSync,
  useSetDisplayBrandMotomarks,
  useTestBrandLogoConnection,
  useUnmatchedDisplayBrands,
} from "@/features/catalog/hooks/use-catalog";
import type {
  BrandLogoSettings,
  BrandLogoSyncRun,
  UnmatchedDisplayBrand,
} from "@/features/catalog/services/catalog.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const RUN_TONE: Record<
  BrandLogoSyncRun["status"],
  "default" | "success" | "warning" | "danger"
> = {
  queued: "default",
  running: "warning",
  done: "success",
  failed: "danger",
};

/** Platform → Vehicle catalog → Logos: mirror settings, sync runs, queue. */
export function CatalogBrandLogosPage() {
  const { t } = useLocale();
  const breadcrumbs = useCatalogBreadcrumbs(t("catalog.tabs.logos"));
  const settings = useBrandLogoSettings({ refetchInterval: 5_000 });
  const active = Boolean(settings.data?.active_run);

  return (
    <EntityPage
      title={t("catalog.title")}
      description={t("catalog.description")}
      breadcrumbs={breadcrumbs}
      permission={permissions.catalog.read}
      forbiddenFallback={<ErrorState title={t("catalog.forbidden")} />}
    >
      <CatalogTabs active="logos" />
      {settings.isError ? (
        <ErrorState
          title={t("catalog.logos.title")}
          onRetry={() => void settings.refetch()}
        />
      ) : null}
      {settings.data ? (
        <div className="space-y-4">
          <SettingsCard settings={settings.data} />
          <StatsRow settings={settings.data} />
          <RunsCard polling={active} />
          <UnmatchedCard />
        </div>
      ) : null}
    </EntityPage>
  );
}

function SettingsCard({ settings }: { settings: BrandLogoSettings }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.catalog.write);
  const patch = usePatchBrandLogoSettings();
  const test = useTestBrandLogoConnection();
  const sync = useRequestBrandLogoSync();
  const [baseUrl, setBaseUrl] = useState(settings.base_url);
  const [token, setToken] = useState("");
  const [enabled, setEnabled] = useState(settings.enabled);
  const [minScore, setMinScore] = useState(
    String(settings.auto_match_min_score),
  );
  const configured =
    settings.base_url.trim() !== "" && settings.token_configured;
  const running = Boolean(settings.active_run);
  const dirty =
    baseUrl !== settings.base_url ||
    token !== "" ||
    enabled !== settings.enabled ||
    minScore !== String(settings.auto_match_min_score);

  const save = async () => {
    const score = Number(minScore);
    await patch.mutateAsync({
      base_url: baseUrl,
      ...(token !== "" ? { api_token: token } : {}),
      enabled,
      auto_match_min_score: Number.isFinite(score) ? score : undefined,
    });
    setToken("");
  };

  return (
    <EntitySectionCard
      title={t("catalog.logos.settings_title")}
      actions={
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!configured || test.isPending || !canWrite}
            onClick={() => test.mutate()}
          >
            <PlugZap className="size-4" />
            {t("catalog.logos.test")}
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={
              !configured ||
              !settings.enabled ||
              running ||
              sync.isPending ||
              !canWrite
            }
            title={!configured ? t("catalog.logos.sync_blocked") : undefined}
            onClick={() => sync.mutate()}
          >
            <RefreshCw className={running ? "size-4 animate-spin" : "size-4"} />
            {running
              ? t("catalog.logos.sync_running")
              : t("catalog.logos.sync_now")}
          </Button>
        </div>
      }
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("catalog.logos.description")}
      </p>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="logo-base-url">{t("catalog.logos.base_url")}</Label>
          <Input
            id="logo-base-url"
            placeholder="https://logos.example.com"
            value={baseUrl}
            disabled={!canWrite}
            onChange={(e) => setBaseUrl(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("catalog.logos.base_url_hint")}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="logo-api-token">
            {t("catalog.logos.api_token")}{" "}
            <Badge
              variant={settings.token_configured ? "secondary" : "outline"}
            >
              {settings.token_configured
                ? t("catalog.logos.api_token_configured")
                : t("catalog.logos.api_token_missing")}
            </Badge>
          </Label>
          <Input
            id="logo-api-token"
            type="password"
            autoComplete="off"
            placeholder="••••••••"
            value={token}
            disabled={!canWrite}
            onChange={(e) => setToken(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("catalog.logos.api_token_hint")}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="logo-min-score">{t("catalog.logos.min_score")}</Label>
          <Input
            id="logo-min-score"
            type="number"
            min={0}
            max={100}
            value={minScore}
            disabled={!canWrite}
            onChange={(e) => setMinScore(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("catalog.logos.min_score_hint")}
          </p>
        </div>
        <div className="flex items-start gap-3 pt-6">
          <Switch
            id="logo-enabled"
            checked={enabled}
            disabled={!canWrite}
            onCheckedChange={setEnabled}
          />
          <div className="space-y-1">
            <Label htmlFor="logo-enabled">{t("catalog.logos.enabled")}</Label>
            <p className="text-muted-foreground text-xs">
              {t("catalog.logos.enabled_hint")}
            </p>
          </div>
        </div>
      </div>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-xs">
          {t("catalog.logos.last_run")}:{" "}
          {settings.last_run_at
            ? `${datetime(settings.last_run_at)} · ${t(`catalog.logos.run.${settings.last_status ?? "done"}`)}`
            : t("catalog.logos.never")}
          {settings.last_error ? (
            <span className="text-destructive"> · {settings.last_error}</span>
          ) : null}
        </p>
        {canWrite ? (
          <Button
            type="button"
            size="sm"
            variant={dirty ? "default" : "outline"}
            disabled={!dirty || patch.isPending}
            onClick={() => void save()}
          >
            <Save className="size-4" />
            {t("catalog.logos.save")}
          </Button>
        ) : null}
      </div>
    </EntitySectionCard>
  );
}

function StatsRow({ settings }: { settings: BrandLogoSettings }) {
  const { t } = useLocale();
  const counts = settings.match_counts ?? {};
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
      <StatsCard
        title={t("catalog.logos.stats_sources")}
        value={settings.sources_count}
      />
      <StatsCard
        title={t("catalog.logos.stats_auto")}
        value={counts.auto ?? 0}
      />
      <StatsCard
        title={t("catalog.logos.stats_manual")}
        value={counts.manual ?? 0}
      />
      <StatsCard
        title={t("catalog.logos.stats_unmatched")}
        value={counts.none ?? 0}
      />
      <StatsCard
        title={t("catalog.logos.stats_skipped")}
        value={counts.skip ?? 0}
      />
    </div>
  );
}

function RunsCard({ polling }: { polling: boolean }) {
  const { t } = useLocale();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 10,
  });
  const runs = useBrandLogoRuns(listState.params, {
    refetchInterval: polling ? 5_000 : undefined,
  });
  const columns = useMemo(
    () =>
      [
        createColumn<BrandLogoSyncRun>({
          accessorKey: "created_at",
          labelKey: "catalog.logos.run_started",
          cell: ({ row }) =>
            datetime(row.original.started_at ?? row.original.created_at),
        }),
        createColumn<BrandLogoSyncRun>({
          accessorKey: "status",
          labelKey: "catalog.logos.run_status",
          cell: ({ row }) => (
            <div className="flex items-center gap-2">
              <StatusChip
                label={t(`catalog.logos.run.${row.original.status}`)}
                tone={RUN_TONE[row.original.status]}
              />
              {row.original.not_modified ? (
                <Badge variant="outline">
                  {t("catalog.logos.run_not_modified")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<BrandLogoSyncRun>({
          accessorKey: "brands_seen",
          labelKey: "catalog.logos.run_brands",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.brands_seen}</span>
          ),
        }),
        createColumn<BrandLogoSyncRun>({
          accessorKey: "downloaded",
          labelKey: "catalog.logos.run_downloaded",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.downloaded}</span>
          ),
        }),
        createColumn<BrandLogoSyncRun>({
          accessorKey: "matched",
          labelKey: "catalog.logos.run_matched",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.matched} / {row.original.unmatched}
            </span>
          ),
        }),
        createColumn<BrandLogoSyncRun>({
          accessorKey: "requests",
          labelKey: "catalog.logos.run_requests",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.requests}</span>
          ),
        }),
        createColumn<BrandLogoSyncRun>({
          id: "triggered_by",
          accessorFn: (row) => row.triggered_by?.name ?? "",
          labelKey: "catalog.logos.run_by",
          cell: ({ row }) => row.original.triggered_by?.name ?? "—",
        }),
        createColumn<BrandLogoSyncRun>({
          accessorKey: "error",
          labelKey: "catalog.logos.run_error",
          cell: ({ row }) =>
            row.original.error ? (
              <span className="text-destructive text-xs">
                {row.original.error}
              </span>
            ) : (
              "—"
            ),
        }),
      ] as ColumnDef<BrandLogoSyncRun, unknown>[],
    [t],
  );
  const pageCount = Math.max(
    1,
    Math.ceil((runs.data?.total ?? 0) / (listState.pagination.pageSize || 10)),
  );
  return (
    <EntitySectionCard title={t("catalog.logos.runs_title")}>
      <EntityTable
        columns={columns}
        data={runs.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={runs.isLoading}
        isError={runs.isError}
        onRetry={() => void runs.refetch()}
        emptyTitle={t("catalog.logos.runs_empty_title")}
        emptyDescription={t("catalog.logos.runs_empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[10, 20, 50]}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-logo-runs-v1",
          rowSelection: false,
        }}
      />
    </EntitySectionCard>
  );
}

function UnmatchedCard() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.catalog.write);
  const set = useSetDisplayBrandMotomarks();
  const [dialog, setDialog] = useState<MotomarksDialogBrand | null>(null);
  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 20,
  });
  const list = useUnmatchedDisplayBrands(listState.params);
  const columns = useMemo(
    () =>
      [
        createColumn<UnmatchedDisplayBrand>({
          accessorKey: "name",
          labelKey: "catalog.columns.display_name",
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex items-center gap-2">
              <BrandMark
                name={row.original.name}
                logoUrl={row.original.logo_url}
              />
              <div className="min-w-0">
                <p className="truncate font-medium">{row.original.name}</p>
                <p className="text-muted-foreground truncate font-mono text-xs">
                  {row.original.slug}
                </p>
              </div>
            </div>
          ),
        }),
        createColumn<UnmatchedDisplayBrand>({
          accessorKey: "types_count",
          labelKey: "catalog.columns.types_count",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.types_count}</span>
          ),
        }),
        createColumn<UnmatchedDisplayBrand>({
          id: "suggestion",
          accessorFn: (row) => row.suggestion?.name ?? "",
          labelKey: "catalog.logos.suggestion",
          cell: ({ row }) =>
            row.original.suggestion ? (
              <span>
                {row.original.suggestion.name}{" "}
                <span className="text-muted-foreground text-xs">
                  ({row.original.suggestion.id}
                  {row.original.suggestion.score != null
                    ? ` · ${Math.round(row.original.suggestion.score)}`
                    : ""}
                  )
                </span>
              </span>
            ) : (
              <span className="text-muted-foreground">
                {t("catalog.logos.no_suggestion")}
              </span>
            ),
        }),
        createColumn<UnmatchedDisplayBrand>({
          id: "actions",
          labelKey: "catalog.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "link",
                  label: t("catalog.logos.link"),
                  icon: Link2,
                  permission: permissions.catalog.write,
                  onSelect: () => setDialog(row.original),
                },
                ...(row.original.suggestion
                  ? [
                      {
                        id: "link-suggestion",
                        label: t("catalog.logos.link_suggestion"),
                        icon: Link2,
                        permission: permissions.catalog.write,
                        onSelect: () =>
                          void set.mutateAsync({
                            uuid: row.original.uuid,
                            mode: "link",
                            motomarksId: row.original.suggestion!.id,
                          }),
                      },
                    ]
                  : []),
                {
                  id: "skip",
                  label: t("catalog.logos.skip"),
                  permission: permissions.catalog.write,
                  onSelect: () =>
                    void set.mutateAsync({
                      uuid: row.original.uuid,
                      mode: "skip",
                    }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<UnmatchedDisplayBrand, unknown>[],
    [set, t],
  );
  const pageCount = Math.max(
    1,
    Math.ceil((list.data?.total ?? 0) / (listState.pagination.pageSize || 20)),
  );
  return (
    <EntitySectionCard
      title={t("catalog.logos.unmatched_title")}
      badge={list.data?.total}
    >
      <p className="text-muted-foreground mb-3 text-sm">
        {t("catalog.logos.unmatched_description")}
      </p>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={canWrite ? (row) => setDialog(row) : undefined}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("catalog.logos.unmatched_empty_title")}
        emptyDescription={t("catalog.logos.unmatched_empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[20, 50, 100]}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-logo-unmatched-v1",
          rowSelection: false,
        }}
      />
      <DisplayBrandMotomarksDialog
        brand={dialog}
        onOpenChange={(open) => {
          if (!open) setDialog(null);
        }}
      />
    </EntitySectionCard>
  );
}
