"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityForm,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SegmentForm } from "@/features/fleet/components/segments/segment-form";
import { FleetActivityCard } from "@/features/fleet/components/shared/fleet-activity-card";
import { CodeBadge } from "@/features/fleet/components/shared/list-utils";
import {
  GroupsSubTable,
  ProductsSubTable,
} from "@/features/fleet/components/shared/sub-tables";
import {
  useCreateSegment,
  useDeleteSegment,
  useSegment,
  useUpdateSegment,
} from "@/features/fleet/hooks/use-fleet";
import {
  segmentFormDefaults,
  segmentFormToRequest,
} from "@/features/fleet/schemas/segment-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function useForbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("fleet.forbidden")}
    />
  );
}

export function SegmentCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateSegment();
  const forbidden = useForbidden();

  return (
    <EntityPage
      title={t("fleet.segments.create_title")}
      description={t("fleet.segments.create_description")}
      permission={permissions.tenant.fleet.write}
      forbiddenFallback={forbidden}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.segments.title"),
          href: routes.tenant.fleet.segments.root(slug),
        },
        { label: t("fleet.segments.create_title") },
      ]}
    >
      <EntityForm>
        <SegmentForm
          mode="create"
          defaultValues={segmentFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.fleet.segments.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              segmentFormToRequest(values),
            );
            router.push(
              routes.tenant.fleet.segments.detail(slug, created.uuid),
            );
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

export function SegmentEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useSegment(uuid);
  const update = useUpdateSegment();
  const forbidden = useForbidden();
  const segment = query.data;

  return (
    <EntityPage
      title={
        segment
          ? t("fleet.edit_title_with_name", { name: segment.name })
          : t("fleet.segments.edit_title")
      }
      description={t("fleet.segments.edit_description")}
      permission={permissions.tenant.fleet.write}
      forbiddenFallback={forbidden}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.segments.title"),
          href: routes.tenant.fleet.segments.root(slug),
        },
        {
          label: segment?.name ?? t("fleet.segments.detail_title"),
          href: routes.tenant.fleet.segments.detail(slug, uuid),
        },
        { label: t("fleet.segments.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("fleet.segments.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {segment ? (
        <EntityForm>
          <SegmentForm
            key={segment.updated_at}
            mode="edit"
            defaultValues={segmentFormDefaults(segment)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.fleet.segments.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: segmentFormToRequest(values),
              });
              router.push(routes.tenant.fleet.segments.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

/** Show page: details, rental groups and products of the segment, activity. */
export function SegmentDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useSegment(uuid);
  const update = useUpdateSegment();
  const remove = useDeleteSegment();
  const forbidden = useForbidden();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const segment = query.data;
  const title = segment?.name ?? t("fleet.segments.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("fleet.segments.detail_description")}
      permission={permissions.tenant.fleet.read}
      forbiddenFallback={forbidden}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.segments.title"),
          href: routes.tenant.fleet.segments.root(slug),
        },
        { label: title },
      ]}
      actions={
        segment ? (
          <PermissionGuard permission={permissions.tenant.fleet.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !segment.is_active },
                  })
                }
              >
                {segment.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {segment.is_active
                  ? t("fleet.actions.deactivate")
                  : t("fleet.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("fleet.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.fleet.segments.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("fleet.actions.edit")}
                </Link>
              </Button>
            </EntityActions>
          </PermissionGuard>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("fleet.segments.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {segment ? (
        <div className="space-y-6">
          <EntityHeader
            title={segment.name}
            subtitle={<CodeBadge code={segment.code} />}
            badges={
              <StatusChip
                label={
                  segment.is_active
                    ? t("fleet.active.true")
                    : t("fleet.active.false")
                }
                tone={segment.is_active ? "success" : "default"}
              />
            }
            description={segment.description || undefined}
          />

          <EntitySectionCard title={t("fleet.detail.details")}>
            <EntityDetail
              sections={[
                {
                  id: "details",
                  fields: [
                    {
                      key: "code",
                      label: t("fleet.fields.code"),
                      value: segment.code,
                    },
                    {
                      key: "name",
                      label: t("fleet.segments.fields.name"),
                      value: segment.name,
                    },
                    {
                      key: "sort_order",
                      label: t("fleet.segments.fields.sort_order"),
                      value: segment.sort_order,
                    },
                    {
                      key: "groups_count",
                      label: t("fleet.segments.columns.groups_count"),
                      value: segment.groups_count,
                    },
                    {
                      key: "products_count",
                      label: t("fleet.segments.columns.products_count"),
                      value: segment.products_count,
                    },
                    {
                      key: "updated_at",
                      label: t("fleet.fields.updated_at"),
                      value: datetime(segment.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("fleet.segments.detail.groups")}
            badge={segment.groups_count}
          >
            <GroupsSubTable segmentUuid={uuid} />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("fleet.segments.detail.products")}
            badge={segment.products_count}
          >
            <ProductsSubTable
              segmentUuid={uuid}
              persistKey="tenant-fleet-segment-products-v1"
            />
          </EntitySectionCard>

          <FleetActivityCard kind="segments" uuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={`${segment.code} · ${segment.name}`}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.fleet.segments.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
