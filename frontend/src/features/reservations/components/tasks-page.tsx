"use client";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TasksTable } from "@/features/reservations/components/tasks-table";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/** Resolution tasks of every reservation in the member's branch scope. */
export function ResolutionTasksPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("reservations.tasks.title")}
      description={t("reservations.tasks.description")}
      permission={permissions.tenant.resolutionTasks.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.tasks.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("reservations.tasks.title") },
      ]}
    >
      <TasksTable persistKey="tenant-resolution-tasks-v1" fromUrl />
    </EntityPage>
  );
}
