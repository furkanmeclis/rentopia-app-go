"use client";

import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import {
  FIELD_TASK_LIVE_STATUSES,
  MY_FIELD_TASKS_RESOURCE,
} from "@/features/field-tasks/constants";
import { useMyFieldTasks } from "@/features/field-tasks/hooks/use-field-tasks";
import { dayWindow } from "@/features/field-tasks/lib/field-tasks";
import { createNavAdornment, type NavAdornment } from "@/features/nav-engine";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** "Görevlerim": the member's open tasks until the end of today, live. */
function useMyFieldTasksNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.fieldTasks.execute);
  const until = useMemo(() => dayWindow(0).to, []);
  const query = useMyFieldTasks(
    {
      limit: 1,
      offset: 0,
      status: FIELD_TASK_LIVE_STATUSES.join(","),
      due_to: until,
    },
    allowed,
  );
  useTenantLiveRefetch([MY_FIELD_TASKS_RESOURCE], () => query.refetch(), {
    enabled: allowed,
  });
  const open = query.data?.total ?? 0;
  return {
    badges: [{ kind: "count", value: open, variant: "warning" }],
    info:
      open > 0
        ? {
            title: t("field_tasks.my.title"),
            rows: [
              {
                label: t("field_tasks.my.today"),
                value: open,
                tone: "warning",
              },
            ],
          }
        : null,
  };
}

export const MyFieldTasksNavAdornment = createNavAdornment(
  useMyFieldTasksNavAdornment,
);
