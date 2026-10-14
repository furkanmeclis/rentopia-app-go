"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { VehicleForm } from "@/features/vehicles/components/vehicle-form";
import {
  useCreateVehicle,
  useUpdateVehicle,
  useVehicle,
} from "@/features/vehicles/hooks/use-vehicles";
import {
  vehicleFormDefaults,
  vehicleFormToRequest,
} from "@/features/vehicles/schemas/vehicle-form";
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
