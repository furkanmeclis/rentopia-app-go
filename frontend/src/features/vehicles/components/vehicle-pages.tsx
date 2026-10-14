"use client";

import { Activity, Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BODY_TYPE_LABEL_KEYS,
  FUEL_LABEL_KEYS,
  TRANSMISSION_LABEL_KEYS,
} from "@/features/fleet/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  GroupChips,
  KaskoValue,
  Km,
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import {
  VehicleDocumentsTab,
  VehicleMaintenanceTab,
} from "@/features/vehicle-records";
import { VehicleForm } from "@/features/vehicles/components/vehicle-form";
import { VehicleStatusDialog } from "@/features/vehicles/components/vehicle-status-dialog";
import {
  VehicleActivityCard,
  VehicleEventsTable,
  VehicleGroupsTab,
  VehicleKaskoTab,
} from "@/features/vehicles/components/vehicle-tabs";
import { OWNERSHIP_LABEL_KEYS } from "@/features/vehicles/constants";
import {
  useCreateVehicle,
  useDeleteVehicle,
  useUpdateVehicle,
  useVehicle,
} from "@/features/vehicles/hooks/use-vehicles";
import {
  vehicleFormDefaults,
  vehicleFormToRequest,
} from "@/features/vehicles/schemas/vehicle-form";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("vehicles.forbidden")}
    />
  );
}

export function VehicleCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateVehicle();

  return (
    <EntityPage
      title={t("vehicles.create_title")}
      description={t("vehicles.create_description")}
      permission={permissions.tenant.vehicles.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("vehicles.title"),
          href: routes.tenant.fleet.vehicles.root(slug),
        },
        { label: t("vehicles.create_title") },
      ]}
    >
      <EntityForm>
        <VehicleForm
          mode="create"
          defaultValues={vehicleFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.fleet.vehicles.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              vehicleFormToRequest(values, "create"),
            );
            router.push(
              routes.tenant.fleet.vehicles.detail(slug, created.uuid),
            );
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

export function VehicleEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useVehicle(uuid);
  const update = useUpdateVehicle();
  const vehicle = query.data;

  return (
    <EntityPage
      title={
        vehicle
          ? t("vehicles.edit_title_with_plate", { plate: vehicle.plate })
          : t("vehicles.edit_title")
      }
      description={t("vehicles.edit_description")}
      permission={permissions.tenant.vehicles.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("vehicles.title"),
          href: routes.tenant.fleet.vehicles.root(slug),
        },
        {
          label: vehicle?.plate ?? t("vehicles.detail_title"),
          href: routes.tenant.fleet.vehicles.detail(slug, uuid),
        },
        { label: t("vehicles.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("vehicles.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {vehicle ? (
        <EntityForm>
          <VehicleForm
            key={vehicle.updated_at}
            mode="edit"
            defaultValues={vehicleFormDefaults(vehicle)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.fleet.vehicles.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: vehicleFormToRequest(values, "edit"),
              });
              router.push(routes.tenant.fleet.vehicles.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

/**
 * Show page with tabs: general, rental groups (memberships + suggestions),
 * status history, kasko and activity; a status change dialog.
 */
export function VehicleDetailPage({
  uuid,
  initialTab,
}: {
  uuid: string;
  /** Tab opened first (`?tab=` of notification links). */
  initialTab?: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useVehicle(uuid);
  const update = useUpdateVehicle();
  const remove = useDeleteVehicle();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [statusOpen, setStatusOpen] = useState(false);
  const vehicle = query.data;
  const title = vehicle?.plate ?? t("vehicles.detail_title");
  const unknown = t("vehicles.form.unknown");

  return (
    <EntityPage
      title={title}
      description={t("vehicles.detail_description")}
      permission={permissions.tenant.vehicles.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("vehicles.title"),
          href: routes.tenant.fleet.vehicles.root(slug),
        },
        { label: title },
      ]}
      actions={
        vehicle ? (
          <PermissionGuard permission={permissions.tenant.vehicles.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setStatusOpen(true)}
              >
                <Activity className="size-4" />
                {t("vehicles.actions.change_status")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !vehicle.is_active },
                  })
                }
              >
                {vehicle.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {vehicle.is_active
                  ? t("vehicles.actions.deactivate")
                  : t("vehicles.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("vehicles.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.fleet.vehicles.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("vehicles.actions.edit")}
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
          description={t("vehicles.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {vehicle ? (
        <div className="space-y-6">
          <EntityHeader
            title={vehicle.label || vehicle.plate}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <PlateBadge plate={vehicle.plate} />
                <span>{vehicle.model_year}</span>
                {vehicle.color ? <span>· {vehicle.color}</span> : null}
              </span>
            }
            badges={
              <>
                <VehicleStatusChip status={vehicle.status} />
                {vehicle.manual_entry ? (
                  <Badge variant="outline">{t("vehicles.manual_badge")}</Badge>
                ) : null}
                {!vehicle.in_service ? (
                  <StatusChip label={t("vehicles.not_in_service")} />
                ) : null}
                {!vehicle.is_active ? (
                  <StatusChip label={t("fleet.active.false")} />
                ) : null}
              </>
            }
          />

          <Tabs defaultValue={initialTab ?? "general"} className="space-y-4">
            <TabsList className="flex h-auto flex-wrap">
              <TabsTrigger value="general">
                {t("vehicles.tabs.general")}
              </TabsTrigger>
              <TabsTrigger value="groups">
                {t("vehicles.tabs.groups")}
              </TabsTrigger>
              <TabsTrigger value="history">
                {t("vehicles.tabs.history")}
              </TabsTrigger>
              <TabsTrigger value="kasko">
                {t("vehicles.tabs.kasko")}
              </TabsTrigger>
              <TabsTrigger value="documents">
                {t("vehicle_records.tabs.documents")}
              </TabsTrigger>
              <TabsTrigger value="maintenance">
                {t("vehicle_records.tabs.maintenance")}
              </TabsTrigger>
              <TabsTrigger value="activity">
                {t("vehicles.tabs.activity")}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="general" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-3">
                <EntitySectionCard
                  title={t("vehicles.detail.identity")}
                  className="lg:col-span-2"
                >
                  <EntityDetail
                    sections={[
                      {
                        id: "identity",
                        fields: [
                          {
                            key: "plate",
                            label: t("vehicles.fields.plate"),
                            value: vehicle.plate,
                          },
                          {
                            key: "vin",
                            label: t("vehicles.fields.vin"),
                            value: vehicle.vin || "—",
                          },
                          {
                            key: "brand",
                            label: t("vehicles.fields.brand"),
                            value: vehicle.brand || "—",
                          },
                          {
                            key: "model",
                            label: t("vehicles.fields.model"),
                            value: vehicle.model || "—",
                          },
                          {
                            key: "type",
                            label: t("vehicles.fields.type"),
                            value: vehicle.type_name || "—",
                          },
                          {
                            key: "type_code",
                            label: t("vehicles.fields.type_code"),
                            value: vehicle.catalog?.type_code || "—",
                          },
                          {
                            key: "model_year",
                            label: t("vehicles.fields.model_year"),
                            value: vehicle.model_year,
                          },
                          {
                            key: "color",
                            label: t("vehicles.fields.color"),
                            value: vehicle.color || "—",
                          },
                        ],
                      },
                    ]}
                  />
                </EntitySectionCard>
                <EntitySectionCard title={t("vehicles.detail.kasko")}>
                  <div className="space-y-2">
                    <p className="font-display text-2xl font-semibold">
                      <KaskoValue
                        value={vehicle.kasko_value}
                        period={vehicle.kasko_period}
                      />
                    </p>
                    <p className="text-muted-foreground text-sm">
                      {vehicle.kasko_period
                        ? t("vehicles.kasko.period", {
                            period: vehicle.kasko_period,
                          })
                        : t("vehicles.kasko.none")}
                    </p>
                  </div>
                </EntitySectionCard>
              </div>

              <div className="grid gap-6 lg:grid-cols-2">
                <EntitySectionCard
                  title={t("vehicles.detail.attributes")}
                  badge={
                    vehicle.attributes_verified
                      ? t("vehicles.verified")
                      : t("vehicles.unverified")
                  }
                >
                  <EntityDetail
                    sections={[
                      {
                        id: "attributes",
                        fields: [
                          {
                            key: "transmission",
                            label: t("vehicles.fields.transmission"),
                            value: vehicle.transmission
                              ? t(TRANSMISSION_LABEL_KEYS[vehicle.transmission])
                              : unknown,
                          },
                          {
                            key: "fuel",
                            label: t("vehicles.fields.fuel"),
                            value: vehicle.fuel
                              ? t(FUEL_LABEL_KEYS[vehicle.fuel])
                              : unknown,
                          },
                          {
                            key: "body_type",
                            label: t("vehicles.fields.body_type"),
                            value: vehicle.body_type
                              ? t(BODY_TYPE_LABEL_KEYS[vehicle.body_type])
                              : unknown,
                          },
                          {
                            key: "seats",
                            label: t("vehicles.fields.seats"),
                            value: vehicle.seats ?? unknown,
                          },
                          {
                            key: "luggage",
                            label: t("vehicles.fields.luggage"),
                            value: vehicle.luggage ?? unknown,
                          },
                          {
                            key: "air_conditioning",
                            label: t("vehicles.fields.air_conditioning"),
                            value: vehicle.air_conditioning
                              ? t("fleet.yes")
                              : t("vehicles.no"),
                          },
                          {
                            key: "groups",
                            label: t("vehicles.fields.rental_groups"),
                            value: <GroupChips vehicle={vehicle} />,
                          },
                        ],
                      },
                    ]}
                  />
                </EntitySectionCard>

                <EntitySectionCard title={t("vehicles.detail.operations")}>
                  <EntityDetail
                    sections={[
                      {
                        id: "operations",
                        fields: [
                          {
                            key: "status",
                            label: t("vehicles.fields.status"),
                            value: (
                              <span className="flex flex-wrap items-center gap-2">
                                <VehicleStatusChip status={vehicle.status} />
                                <span className="text-muted-foreground text-xs">
                                  {datetime(
                                    vehicle.status_changed_at,
                                    undefined,
                                    locale,
                                  )}
                                </span>
                              </span>
                            ),
                          },
                          {
                            key: "odometer_km",
                            label: t("vehicles.fields.odometer_km"),
                            value: <Km value={vehicle.odometer_km} />,
                          },
                          {
                            key: "fuel_level",
                            label: t("vehicles.fields.fuel_level"),
                            value:
                              vehicle.fuel_level != null
                                ? `%${vehicle.fuel_level}`
                                : unknown,
                          },
                          {
                            key: "home_branch",
                            label: t("vehicles.fields.home_branch"),
                            value: (
                              <Link
                                className="underline"
                                href={routes.tenant.branches.detail(
                                  slug,
                                  vehicle.home_branch.uuid,
                                )}
                              >
                                {vehicle.home_branch.name}
                              </Link>
                            ),
                          },
                          {
                            key: "current_branch",
                            label: t("vehicles.fields.current_branch"),
                            value: vehicle.current_branch.name,
                          },
                          {
                            key: "fleet_pool",
                            label: t("vehicles.fields.fleet_pool"),
                            value: vehicle.fleet_pool,
                          },
                          {
                            key: "ownership",
                            label: t("vehicles.fields.ownership"),
                            value:
                              vehicle.ownership === "supplied"
                                ? `${t(OWNERSHIP_LABEL_KEYS.supplied)} · ${vehicle.supplier_name}`
                                : t(OWNERSHIP_LABEL_KEYS.owned),
                          },
                          {
                            key: "service",
                            label: t("vehicles.fields.service_period"),
                            value: `${vehicle.in_service_date ? date(vehicle.in_service_date, undefined, locale) : "—"} → ${vehicle.out_of_service_date ? date(vehicle.out_of_service_date, undefined, locale) : "—"}`,
                          },
                          {
                            key: "updated_at",
                            label: t("vehicles.fields.updated_at"),
                            value: datetime(
                              vehicle.updated_at,
                              undefined,
                              locale,
                            ),
                          },
                        ],
                      },
                    ]}
                  />
                </EntitySectionCard>
              </div>
            </TabsContent>

            <TabsContent value="groups">
              <VehicleGroupsTab uuid={uuid} />
            </TabsContent>

            <TabsContent value="history">
              <EntitySectionCard title={t("vehicles.tabs.history")}>
                <VehicleEventsTable uuid={uuid} />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="kasko">
              <EntitySectionCard title={t("vehicles.tabs.kasko")}>
                <VehicleKaskoTab uuid={uuid} />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="documents">
              <VehicleDocumentsTab vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="maintenance">
              <VehicleMaintenanceTab vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="activity">
              <VehicleActivityCard uuid={uuid} />
            </TabsContent>
          </Tabs>

          <VehicleStatusDialog
            vehicle={statusOpen ? vehicle : null}
            onOpenChange={setStatusOpen}
          />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={`${vehicle.plate} · ${vehicle.label}`}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.fleet.vehicles.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
