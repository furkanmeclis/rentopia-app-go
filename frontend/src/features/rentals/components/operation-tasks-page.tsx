"use client";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { OperationTasksTable } from "@/features/rentals/components/operation-tasks-table";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/** Operation tasks of every branch in the member's scope. */
export function OperationTasksPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("rentals.tasks.title")}
      description={t("rentals.tasks.description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("rentals.tasks.title") },
      ]}
    >
      <OperationTasksTable persistKey="tenant-operation-tasks-v1" fromUrl />
    </EntityPage>
  );
}
