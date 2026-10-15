"use client";

import { useCallback } from "react";

import { AppCombobox } from "@/components/forms";
import type { ComboboxOption } from "@/components/ui/async-combobox";
import { tenantMembersService } from "@/features/tenant-members/services/tenant-members.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Form-bound member search (`GET /v1/tenant/members`). Members without
 * `tenant.members.read` get an empty list: the field then accepts only
 * clearing the assignment.
 */
export function MemberPickerField({
  name,
  label,
  description,
}: {
  name: string;
  label: string;
  description?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const loadOptions = useCallback(
    async (query: string): Promise<ComboboxOption[]> => {
      try {
        const page = await tenantMembersService.list(slug, {
          limit: 20,
          offset: 0,
          q: query || undefined,
        });
        return page.items.map((m) => ({
          value: m.user.uuid,
          label: `${m.user.name} ${m.user.surname}`.trim() || m.user.email,
          description: m.user.email,
        }));
      } catch {
        return [];
      }
    },
    [slug],
  );
  return (
    <AppCombobox
      name={name}
      label={label}
      description={description}
      loadOptions={loadOptions}
      placeholder={t("reservations.assign.placeholder")}
      searchPlaceholder={t("reservations.assign.search")}
      emptyText={t("reservations.assign.empty")}
      clearable
    />
  );
}
