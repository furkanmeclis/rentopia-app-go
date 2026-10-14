"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { GuaranteeBadgeChip } from "@/features/fleet/components/products/guarantee-badge";
import { ProductCardPreview } from "@/features/fleet/components/products/product-card-preview";
import { ProductForm } from "@/features/fleet/components/products/product-form";
import { ProductImagesManager } from "@/features/fleet/components/products/product-images-manager";
import {
  ConditionsView,
  GuaranteeView,
  VersionsTable,
} from "@/features/fleet/components/products/version-views";
import { FleetActivityCard } from "@/features/fleet/components/shared/fleet-activity-card";
import { CodeBadge } from "@/features/fleet/components/shared/list-utils";
import {
  GUARANTEE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/fleet/constants";
import {
  useCreateProduct,
  useDeleteProduct,
  useProduct,
  useUpdateProduct,
} from "@/features/fleet/hooks/use-fleet";
import {
  productFormDefaults,
  productFormToRequest,
} from "@/features/fleet/schemas/product-form";
import type { RentalProduct } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ProductVehiclesTable } from "@/features/vehicles/components/vehicle-sub-tables";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("fleet.forbidden")}
    />
  );
}

export function ProductCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateProduct();

  return (
    <EntityPage
      title={t("fleet.products.create_title")}
      description={t("fleet.products.create_description")}
      permission={permissions.tenant.fleet.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.products.title"),
          href: routes.tenant.fleet.products.root(slug),
        },
        { label: t("fleet.products.create_title") },
      ]}
    >
      <EntityForm>
        <ProductForm
          mode="create"
          defaultValues={productFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.fleet.products.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              productFormToRequest(values),
            );
            router.push(
              routes.tenant.fleet.products.detail(slug, created.uuid),
            );
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

export function ProductEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useProduct(uuid);
  const update = useUpdateProduct();
  const product = query.data;

  return (
    <EntityPage
      title={
        product
          ? t("fleet.edit_title_with_name", { name: product.title })
          : t("fleet.products.edit_title")
      }
      description={t("fleet.products.edit_description")}
      permission={permissions.tenant.fleet.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.products.title"),
          href: routes.tenant.fleet.products.root(slug),
        },
        {
          label: product?.title ?? t("fleet.products.detail_title"),
          href: routes.tenant.fleet.products.detail(slug, uuid),
        },
        { label: t("fleet.products.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("fleet.products.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {product ? (
        <EntityForm>
          <ProductForm
            key={`${product.uuid}-${product.current_version}`}
            mode="edit"
            defaultValues={productFormDefaults(product)}
            currentVersion={product.current_version}
            coverUrl={product.images[0]?.public_url ?? null}
            images={
              <ProductImagesManager
                productUuid={uuid}
                images={product.images}
              />
            }
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.fleet.products.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: productFormToRequest(values),
                previousVersion: product.current_version,
              });
              router.push(routes.tenant.fleet.products.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

function productChips(
  product: RentalProduct,
  t: (key: string) => string,
): string[] {
  return [
    product.segment.name,
    t(GUARANTEE_LABEL_KEYS[product.version.guarantee.type]),
  ];
}

/**
 * Show page with tabs: general (+ card preview, images), guarantee & eligible
 * vehicles, conditions, versions, channel mappings, activity.
 */
export function ProductDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useProduct(uuid);
  const update = useUpdateProduct();
  const remove = useDeleteProduct();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const product = query.data;
  const title = product?.title ?? t("fleet.products.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("fleet.products.detail_description")}
      permission={permissions.tenant.fleet.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.products.title"),
          href: routes.tenant.fleet.products.root(slug),
        },
        { label: title },
      ]}
      actions={
        product ? (
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
                    body: { is_active: !product.is_active },
                  })
                }
              >
                {product.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {product.is_active
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
                <Link href={routes.tenant.fleet.products.edit(slug, uuid)}>
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
          description={t("fleet.products.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {product ? (
        <div className="space-y-6">
          <EntityHeader
            title={product.title}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <CodeBadge code={product.code} />
                <span>{product.display_text}</span>
              </span>
            }
            badges={
              <>
                <StatusChip
                  label={t(STATUS_LABEL_KEYS[product.status])}
                  tone={STATUS_TONES[product.status]}
                />
                <GuaranteeBadgeChip badge={product.guarantee_badge} />
                <Badge variant="outline" className="tabular-nums">
                  v{product.current_version}
                </Badge>
                {!product.is_active ? (
                  <StatusChip label={t("fleet.active.false")} />
                ) : null}
              </>
            }
          />

          <Tabs defaultValue="general" className="space-y-4">
            <TabsList className="flex h-auto flex-wrap">
              <TabsTrigger value="general">
                {t("fleet.products.tabs.general")}
              </TabsTrigger>
              <TabsTrigger value="guarantee">
                {t("fleet.products.tabs.guarantee_vehicles")}
              </TabsTrigger>
              <TabsTrigger value="conditions">
                {t("fleet.products.tabs.conditions")}
              </TabsTrigger>
              <TabsTrigger value="versions">
                {t("fleet.products.tabs.versions")}
              </TabsTrigger>
              <TabsTrigger value="channels">
                {t("fleet.products.tabs.channels")}
              </TabsTrigger>
              <TabsTrigger value="activity">
                {t("fleet.products.tabs.activity")}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="general" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-3">
                <EntitySectionCard
                  title={t("fleet.detail.details")}
                  className="lg:col-span-2"
                >
                  <EntityDetail
                    sections={[
                      {
                        id: "details",
                        fields: [
                          {
                            key: "code",
                            label: t("fleet.fields.code"),
                            value: product.code,
                          },
                          {
                            key: "segment",
                            label: t("fleet.fields.segment"),
                            value: (
                              <Link
                                className="underline"
                                href={routes.tenant.fleet.segments.detail(
                                  slug,
                                  product.segment.uuid,
                                )}
                              >
                                {product.segment.name}
                              </Link>
                            ),
                          },
                          {
                            key: "representative",
                            label: t("fleet.products.fields.representative"),
                            value:
                              product.representative_type?.label ??
                              product.representative_model?.label ??
                              "—",
                          },
                          {
                            key: "display_text",
                            label: t("fleet.products.columns.display_text"),
                            value: product.display_text,
                          },
                          {
                            key: "description",
                            label: t("fleet.fields.description"),
                            value: product.description || "—",
                          },
                          {
                            key: "updated_at",
                            label: t("fleet.fields.updated_at"),
                            value: datetime(
                              product.updated_at,
                              undefined,
                              locale,
                            ),
                          },
                        ],
                      },
                    ]}
                  />
                </EntitySectionCard>
                <EntitySectionCard title={t("fleet.products.preview_title")}>
                  <ProductCardPreview
                    title={product.title}
                    displayText={product.display_text}
                    badge={product.guarantee_badge}
                    imageUrl={product.images[0]?.public_url ?? null}
                    chips={productChips(product, t)}
                    caption={product.segment.name}
                  />
                </EntitySectionCard>
              </div>
              <EntitySectionCard
                title={t("fleet.products.form.section_images")}
                badge={product.images.length}
              >
                <ProductImagesManager
                  productUuid={uuid}
                  images={product.images}
                />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="guarantee" className="space-y-6">
              <EntitySectionCard
                title={t("fleet.products.detail.guarantee", {
                  version: product.current_version,
                })}
              >
                <GuaranteeView guarantee={product.version.guarantee} />
              </EntitySectionCard>
              <EntitySectionCard title={t("fleet.products.detail.vehicles")}>
                <p className="text-muted-foreground mb-4 text-sm">
                  {t("fleet.products.detail.vehicles_help")}
                </p>
                <ProductVehiclesTable productUuid={uuid} />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="conditions">
              <EntitySectionCard
                title={t("fleet.products.detail.conditions", {
                  version: product.current_version,
                })}
              >
                <ConditionsView conditions={product.version.conditions} />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="versions">
              <EntitySectionCard
                title={t("fleet.versions.title")}
                badge={product.current_version}
              >
                <p className="text-muted-foreground mb-4 text-sm">
                  {t("fleet.versions.help")}
                </p>
                <VersionsTable productUuid={uuid} />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="channels">
              <EntitySectionCard title={t("fleet.products.tabs.channels")}>
                <EmptyState
                  className="py-10"
                  title={t("fleet.products.channels_placeholder.title")}
                  description={t(
                    "fleet.products.channels_placeholder.description",
                  )}
                />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="activity">
              <FleetActivityCard kind="products" uuid={uuid} />
            </TabsContent>
          </Tabs>

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={`${product.code} · ${product.title}`}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.fleet.products.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
