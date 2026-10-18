"use client";

import type {
  ColumnDef,
  ColumnFiltersState,
  OnChangeFn,
} from "@tanstack/react-table";
import { Globe, Pencil, Plus, Star } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { useSiteArticlesList } from "@/features/site-articles/hooks/use-site-articles";
import {
  ARTICLE_STATUSES,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
  articleTitle,
} from "@/features/site-articles/lib/article-display";
import { ARTICLE_LOCALES } from "@/features/site-articles/lib/article-form";
import type {
  ListSiteArticlesParams,
  SiteArticle,
} from "@/features/site-articles/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Filters the API takes one value of: the last choice wins. */
const SINGLE_FILTERS = new Set(["status", "locale"]);

function singleValued(filters: ColumnFiltersState): ColumnFiltersState {
  return filters.map((f) =>
    SINGLE_FILTERS.has(f.id) && Array.isArray(f.value) && f.value.length > 1
      ? { ...f, value: [f.value[f.value.length - 1]] }
      : f,
  );
}

const SORTS = new Set([
  "created_at",
  "-created_at",
  "updated_at",
  "-updated_at",
  "sort",
  "-sort",
]);

/**
 * Ayarlar → Site rehberi (TEC-785): the organization's guide / blog
 * articles. Filters: status, language, search; the editor creates, edits,
 * publishes and deletes.
 */
export function SiteArticlesPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.site.manage);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const params = useMemo<ListSiteArticlesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    const sort = listState.params.sort ?? "-created_at";
    return {
      limit: listState.params.limit,
      offset: listState.params.offset,
      q: listState.params.q,
      status: joinFilter(
        column("status").slice(-1),
      ) as ListSiteArticlesParams["status"],
      locale: joinFilter(
        column("locale").slice(-1),
      ) as ListSiteArticlesParams["locale"],
      sort: (SORTS.has(sort)
        ? sort
        : "-created_at") as ListSiteArticlesParams["sort"],
    };
  }, [listState.columnFilters, listState.params]);

  const { onColumnFiltersChange } = listState;
  const tableState = useMemo(() => {
    const onChange: OnChangeFn<ColumnFiltersState> = (updater) =>
      onColumnFiltersChange((prev) =>
        singleValued(typeof updater === "function" ? updater(prev) : updater),
      );
    return { ...listState.tableState, onColumnFiltersChange: onChange };
  }, [listState.tableState, onColumnFiltersChange]);

  const query = useSiteArticlesList(params);
  const total = query.data?.total ?? 0;
  const open = (row: SiteArticle) =>
    router.push(routes.tenant.settings.siteArticles.detail(slug, row.uuid));

  const columns = useMemo(
    () =>
      [
        createColumn<SiteArticle>({
          id: "title",
          accessorFn: (row) => articleTitle(row, locale),
          labelKey: "site.articles.columns.title",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex min-w-0 items-center gap-2">
              {row.original.featured ? (
                <Star
                  className="fill-primary text-primary size-3.5 shrink-0"
                  aria-label={t("site.articles.columns.featured")}
                />
              ) : null}
              <span className="truncate font-medium">
                {articleTitle(row.original, locale) || (
                  <span className="text-muted-foreground">
                    {t("site.articles.untitled")}
                  </span>
                )}
              </span>
            </span>
          ),
        }),
        createColumn<SiteArticle>({
          accessorKey: "status",
          labelKey: "site.articles.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ARTICLE_STATUSES.map((v) => ({
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
        createColumn<SiteArticle>({
          id: "locale",
          accessorFn: (row) =>
            ARTICLE_LOCALES.filter((l) => row.slug?.[l]).join(","),
          labelKey: "site.articles.columns.locale",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ARTICLE_LOCALES.map((v) => ({
            value: v,
            labelKey: `site.locales.${v}`,
            label: t(`site.locales.${v}`),
          })),
          cell: ({ row }) => (
            <span className="flex gap-1">
              {ARTICLE_LOCALES.filter((l) => row.original.slug?.[l]).map(
                (l) => (
                  <Badge key={l} variant="outline" className="uppercase">
                    {l}
                  </Badge>
                ),
              )}
            </span>
          ),
        }),
        createColumn<SiteArticle>({
          id: "slug",
          accessorFn: (row) => row.slug?.tr || row.slug?.en || "",
          labelKey: "site.articles.columns.slug",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.slug?.[locale === "en" ? "en" : "tr"] ||
                row.original.slug?.tr ||
                row.original.slug?.en}
            </code>
          ),
        }),
        createColumn<SiteArticle>({
          accessorKey: "published_at",
          labelKey: "site.articles.columns.published_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.published_at ? (
              <span className="whitespace-nowrap tabular-nums">
                {datetime(row.original.published_at, undefined, locale)}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<SiteArticle>({
          accessorKey: "sort",
          labelKey: "site.articles.columns.sort",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.sort}</span>
          ),
        }),
        createColumn<SiteArticle>({
          accessorKey: "updated_at",
          labelKey: "site.articles.columns.updated_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.updated_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<SiteArticle>({
          accessorKey: "created_at",
          labelKey: "site.articles.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<SiteArticle>({
          id: "actions",
          labelKey: "site.articles.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t(
                    canManage
                      ? "site.articles.actions.edit"
                      : "site.articles.actions.open",
                  ),
                  icon: Pencil,
                  onSelect: () => open(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<SiteArticle, unknown>[],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [canManage, locale, t],
  );

  const hasFilters = Boolean(params.q || params.status || params.locale);

  return (
    <EntityPage
      title={t("site.articles.title")}
      description={t("site.articles.description")}
      permission={permissions.tenant.site.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("site.articles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("site.title"), href: routes.tenant.settings.site(slug) },
        { label: t("site.articles.title") },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.settings.site(slug)}>
              <Globe className="size-4" />
              {t("site.articles.actions.site_settings")}
            </Link>
          </Button>
          {canManage ? (
            <Button asChild size="sm">
              <Link href={routes.tenant.settings.siteArticles.create(slug)}>
                <Plus className="size-4" />
                {t("site.articles.actions.create")}
              </Link>
            </Button>
          ) : null}
        </div>
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={open}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("site.articles.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={
          hasFilters
            ? t("site.articles.empty_filtered_title")
            : t("site.articles.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("site.articles.empty_filtered_description")
            : t("site.articles.empty_description")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={tableState}
        features={{
          persistKey: "tenant-site-articles-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
