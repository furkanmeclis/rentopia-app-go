"use client";

import {
  Activity,
  ArrowLeftRight,
  Gauge,
  Hammer,
  MapPin,
  MoreHorizontal,
  Pencil,
  Power,
  PowerOff,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BODY_TYPE_LABEL_KEYS,
  FUEL_LABEL_KEYS,
  TRANSMISSION_LABEL_KEYS,
} from "@/features/fleet/constants";
import { VehicleTimelineCard } from "@/features/fleet-calendar";
import { VehicleBlocksTab, VehicleTransfersTab } from "@/features/fleet-ops";
import {
  BlockDialog,
  type BlockDialogMode,
} from "@/features/fleet-ops/components/block-dialogs";
import { TransferDialog } from "@/features/fleet-ops/components/transfer-dialogs";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  VehicleDocumentsTab,
  VehicleMaintenanceTab,
} from "@/features/vehicle-records";
import {
  GroupChips,
  KaskoValue,
  Km,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { VehicleOdometerTab } from "@/features/vehicles/components/vehicle-odometer-tab";
import { VehicleStatusDialog } from "@/features/vehicles/components/vehicle-status-dialog";
import {
  VehicleExpiryCards,
  VehicleOperationBanners,
} from "@/features/vehicles/components/vehicle-summary";
import {
  VehicleActivityCard,
  VehicleEventsTable,
  VehicleGroupsTab,
  VehicleKaskoTab,
} from "@/features/vehicles/components/vehicle-tabs";
import { OWNERSHIP_LABEL_KEYS } from "@/features/vehicles/constants";
import {
  useDeleteVehicle,
  useUpdateVehicle,
  useVehicle,
} from "@/features/vehicles/hooks/use-vehicles";
import type { Vehicle } from "@/features/vehicles/types";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { VehicleTollsTab } from "@/features/tolls/components/vehicle-tolls-tab";
import { VehicleProfitabilityTab } from "@/features/profitability";

/** Tabs of the vehicle page, in display order (`?tab=` values). */
const VEHICLE_TABS = [
  "summary",
  "calendar",
  "history",
  "odometer",
  "groups",
  "blocks",
  "transfers",
  "maintenance",
  "documents",
  "damage",
  "profitability",
  "kasko",
  "tolls",
  "activity",
] as const;

type VehicleTab = (typeof VEHICLE_TABS)[number];

const TAB_LABEL_KEYS: Record<VehicleTab, string> = {
  summary: "vehicles.tabs.summary",
  calendar: "vehicles.tabs.calendar",
  history: "vehicles.tabs.history",
  odometer: "vehicles.tabs.odometer",
  groups: "vehicles.tabs.groups",
  blocks: "vehicles.tabs.blocks",
  transfers: "vehicles.tabs.transfers",
  maintenance: "vehicle_records.tabs.maintenance",
  documents: "vehicle_records.tabs.documents",
  damage: "vehicles.tabs.damage",
  profitability: "vehicles.tabs.profitability",
  kasko: "vehicles.tabs.kasko",
  tolls: "vehicles.tabs.tolls",
  activity: "vehicles.tabs.activity",
};

/** Old deep links keep working (`?tab=general` was the first tab). */
const TAB_ALIASES: Record<string, VehicleTab> = {
  general: "summary",
  // The "Gelir / gider" placeholder became the profitability tab (TEC-727).
  finance: "profitability",
};

function resolveTab(value: string | undefined): VehicleTab {
  if (!value) return "summary";
  if (TAB_ALIASES[value]) return TAB_ALIASES[value];
  return (VEHICLE_TABS as readonly string[]).includes(value)
    ? (value as VehicleTab)
    : "summary";
}

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("vehicles.forbidden")}
    />
  );
}

/** "Özet" tab: banners, expiry cards, identity, attributes, ownership. */
function VehicleSummaryTab({
  vehicle,
  onOpenTab,
}: {
  vehicle: Vehicle;
  onOpenTab: (tab: string) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const unknown = t("vehicles.form.unknown");
  return (
    <div className="space-y-6">
      <VehicleOperationBanners
        vehicleUuid={vehicle.uuid}
        onOpenTab={onOpenTab}
      />
      <VehicleExpiryCards vehicle={vehicle} onOpenTab={onOpenTab} />

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
                ? t("vehicles.kasko.period", { period: vehicle.kasko_period })
                : t("vehicles.kasko.none")}
            </p>
            <Button
              type="button"
              variant="link"
              size="sm"
              className="h-auto px-0"
              onClick={() => onOpenTab("kasko")}
            >
              {t("vehicles.summary.open_kasko")}
            </Button>
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
                      vehicle.ownership === "supplied" ? (
                        <span className="flex flex-wrap items-center gap-1">
                          {t(OWNERSHIP_LABEL_KEYS.supplied)} ·{" "}
                          {vehicle.supplier &&
                          can(permissions.tenant.suppliers.read) ? (
                            <Link
                              href={routes.tenant.finance.supplier(
                                slug,
                                vehicle.supplier.uuid,
                              )}
                              className="font-medium hover:underline"
                            >
                              {vehicle.supplier.name}
                            </Link>
                          ) : (
                            vehicle.supplier_name
                          )}
                        </span>
                      ) : (
                        t(OWNERSHIP_LABEL_KEYS.owned)
                      ),
                  },
                  {
                    key: "service",
                    label: t("vehicles.fields.service_period"),
                    value: `${vehicle.in_service_date ? date(vehicle.in_service_date, undefined, locale) : "—"} → ${vehicle.out_of_service_date ? date(vehicle.out_of_service_date, undefined, locale) : "—"}`,
                  },
                  {
                    key: "updated_at",
                    label: t("vehicles.fields.updated_at"),
                    value: datetime(vehicle.updated_at, undefined, locale),
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>
      </div>
    </div>
  );
}

/**
 * "Hasar kayıtları": damages are marked on the handover / return forms of
 * the vehicle's rentals; the forms list opens searched by the plate.
 */
function VehicleDamageTab({ plate }: { plate: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  return (
    <EntitySectionCard title={t("vehicles.tabs.damage")}>
      <EmptyState
        className="py-10"
        title={t("vehicles.damage.title")}
        description={t("vehicles.damage.description")}
        action={
          can(permissions.tenant.rentals.read) ? (
            <Button asChild size="sm" variant="outline">
              <Link
                href={`${routes.tenant.inspections.root(slug)}?q=${encodeURIComponent(plate)}`}
              >
                {t("vehicles.damage.open_forms")}
              </Link>
            </Button>
          ) : null
        }
      />
    </EntitySectionCard>
  );
}

/**
 * Vehicle show page (§15.1): header with the plate, identity, status,
 * current branch and quick actions, then the tabs in a fixed order. The
 * active tab is mirrored to `?tab=` so links (notifications:
 * `?tab=documents|maintenance`) and reloads land on it.
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
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const query = useVehicle(uuid);
  const update = useUpdateVehicle();
  const remove = useDeleteVehicle();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [statusOpen, setStatusOpen] = useState(false);
  const [blockDialog, setBlockDialog] = useState<BlockDialogMode | null>(null);
  const [transferOpen, setTransferOpen] = useState(false);
  const [tab, setTab] = useState<VehicleTab>(() => resolveTab(initialTab));
  // A new `?tab=` (notification link while on the page) wins.
  const [seenInitial, setSeenInitial] = useState(initialTab);
  if (seenInitial !== initialTab) {
    setSeenInitial(initialTab);
    setTab(resolveTab(initialTab));
  }
  const openTab = (next: string) => {
    const resolved = resolveTab(next);
    setTab(resolved);
    router.replace(`${pathname}?tab=${resolved}`, { scroll: false });
  };

  const vehicle = query.data;
  const title = vehicle?.plate ?? t("vehicles.detail_title");
  const canWrite = can(permissions.tenant.vehicles.write);
  const canBlock = can(permissions.tenant.blocks.write);
  const canTransfer = can(permissions.tenant.transfers.write);
  const nf = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US");

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
          <EntityActions>
            {canWrite ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setStatusOpen(true)}
              >
                <Activity className="size-4" />
                {t("vehicles.actions.change_status")}
              </Button>
            ) : null}
            {canBlock ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setBlockDialog({ type: "maintenance" })}
              >
                <Hammer className="size-4" />
                {t("vehicle_blocks.actions.start_maintenance")}
              </Button>
            ) : null}
            {canTransfer ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setTransferOpen(true)}
              >
                <ArrowLeftRight className="size-4" />
                {t("vehicles.actions.new_transfer")}
              </Button>
            ) : null}
            {canWrite ? (
              <>
                <Button asChild size="sm">
                  <Link href={routes.tenant.fleet.vehicles.edit(slug, uuid)}>
                    <Pencil className="size-4" />
                    {t("vehicles.actions.edit")}
                  </Link>
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      size="icon"
                      variant="outline"
                      className="size-8"
                      aria-label={t("vehicles.actions.more")}
                    >
                      <MoreHorizontal className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      disabled={update.isPending}
                      onSelect={() =>
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
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      variant="destructive"
                      onSelect={() => setConfirmDelete(true)}
                    >
                      <Trash2 className="size-4" />
                      {t("vehicles.actions.delete")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </>
            ) : null}
          </EntityActions>
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
            leading={
              <div className="bg-muted/40 flex shrink-0 flex-col items-center gap-1 rounded-lg border px-4 py-3">
                <span className="text-muted-foreground text-[10px] font-medium tracking-widest uppercase">
                  TR
                </span>
                <span className="font-mono text-xl font-bold tracking-wide">
                  {vehicle.plate}
                </span>
              </div>
            }
            title={vehicle.label || vehicle.plate}
            subtitle={
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                {[vehicle.brand, vehicle.model, vehicle.type_name]
                  .filter(Boolean)
                  .join(" · ") || "—"}
                <span>· {vehicle.model_year}</span>
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
          >
            <div className="text-muted-foreground flex flex-wrap items-center gap-x-5 gap-y-1 text-sm">
              <span className="flex items-center gap-1.5">
                <MapPin className="size-4" aria-hidden />
                <span>
                  {t("vehicles.header.current_branch")}:{" "}
                  <span className="text-foreground font-medium">
                    {vehicle.current_branch.name}
                  </span>
                </span>
              </span>
              {vehicle.home_branch.uuid !== vehicle.current_branch.uuid ? (
                <span>
                  {t("vehicles.header.home_branch")}:{" "}
                  <span className="text-foreground">
                    {vehicle.home_branch.name}
                  </span>
                </span>
              ) : null}
              <span className="flex items-center gap-1.5">
                <Gauge className="size-4" aria-hidden />
                <span className="tabular-nums">
                  {nf.format(vehicle.odometer_km)} km
                </span>
              </span>
            </div>
          </EntityHeader>

          <Tabs value={tab} onValueChange={openTab} className="space-y-4">
            <div className="-mx-1 overflow-x-auto px-1 pb-1">
              <TabsList className="h-auto w-max">
                {VEHICLE_TABS.filter(
                  (value) =>
                    (value !== "tolls" || can(permissions.tenant.tolls.read)) &&
                    (value !== "profitability" ||
                      can(permissions.tenant.finance.reports.read)),
                ).map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(TAB_LABEL_KEYS[value])}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>

            <TabsContent value="summary">
              <VehicleSummaryTab vehicle={vehicle} onOpenTab={openTab} />
            </TabsContent>

            <TabsContent value="calendar">
              <VehicleTimelineCard vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="history">
              <EntitySectionCard title={t("vehicles.tabs.history")}>
                <VehicleEventsTable uuid={uuid} />
              </EntitySectionCard>
            </TabsContent>

            <TabsContent value="odometer">
              <VehicleOdometerTab uuid={uuid} />
            </TabsContent>

            <TabsContent value="groups">
              <VehicleGroupsTab uuid={uuid} />
            </TabsContent>

            <TabsContent value="blocks">
              <VehicleBlocksTab vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="transfers">
              <VehicleTransfersTab vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="maintenance">
              <VehicleMaintenanceTab vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="documents">
              <VehicleDocumentsTab vehicleUuid={uuid} />
            </TabsContent>

            <TabsContent value="damage">
              <VehicleDamageTab plate={vehicle.plate} />
            </TabsContent>

            {can(permissions.tenant.finance.reports.read) ? (
              <TabsContent value="profitability">
                <VehicleProfitabilityTab uuid={uuid} />
              </TabsContent>
            ) : null}

            <TabsContent value="kasko">
              <EntitySectionCard title={t("vehicles.tabs.kasko")}>
                <VehicleKaskoTab uuid={uuid} />
              </EntitySectionCard>
            </TabsContent>

            {can(permissions.tenant.tolls.read) ? (
              <TabsContent value="tolls">
                <VehicleTollsTab vehicleUuid={uuid} />
              </TabsContent>
            ) : null}

            <TabsContent value="activity">
              <VehicleActivityCard uuid={uuid} />
            </TabsContent>
          </Tabs>

          <VehicleStatusDialog
            vehicle={statusOpen ? vehicle : null}
            onOpenChange={setStatusOpen}
          />
          <BlockDialog
            vehicleUuid={uuid}
            mode={blockDialog}
            onOpenChange={(open) => {
              if (!open) setBlockDialog(null);
            }}
          />
          <TransferDialog
            open={transferOpen}
            vehicleUuid={uuid}
            onOpenChange={setTransferOpen}
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
