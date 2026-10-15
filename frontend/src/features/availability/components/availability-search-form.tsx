"use client";

import { Search } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { AppDatePicker, AppForm, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { DEFAULT_CHANNEL } from "@/features/availability/constants";
import type { AvailabilitySearchRequest } from "@/features/availability/types";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import {
  SALES_CHANNELS,
  SALES_CHANNEL_LABEL_KEYS,
} from "@/features/tenant-settings/constants";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "availability-search-form";
/** Select value of "return to the pick-up branch" (Radix items cannot be empty). */
const SAME = "same";

type Values = {
  pickup_branch_uuid: string;
  return_branch_uuid: string;
  starts_at: string;
  ends_at: string;
  channel: string;
};

/**
 * Pick-up / return branch, date-time range and sales channel of an
 * availability search. Submitting sets the request the results table reads.
 */
export function AvailabilitySearchForm({
  pending,
  onSearch,
}: {
  pending: boolean;
  onSearch: (request: AvailabilitySearchRequest) => void;
}) {
  const { t } = useLocale();
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const schema = useMemo(
    () =>
      z
        .object({
          pickup_branch_uuid: z
            .string()
            .min(1, t("availability.validation.pickup")),
          return_branch_uuid: z.string(),
          starts_at: z.string().min(1, t("availability.validation.starts_at")),
          ends_at: z.string().min(1, t("availability.validation.ends_at")),
          channel: z.string().min(1),
        })
        .refine(
          (v) =>
            !v.starts_at ||
            !v.ends_at ||
            new Date(v.ends_at) > new Date(v.starts_at),
          {
            path: ["ends_at"],
            message: t("availability.validation.ends_after_start"),
          },
        ),
    [t],
  );
  const branchOptions = (branches.data?.items ?? [])
    .filter((b) => b.is_active)
    .map((b) => ({ value: b.uuid, label: `${b.code} · ${b.name}` }));
  const returnOptions = [
    { value: SAME, label: t("availability.fields.same_branch") },
    ...branchOptions,
  ];
  const channelOptions = SALES_CHANNELS.map((code) => ({
    value: code,
    label: t(SALES_CHANNEL_LABEL_KEYS[code]),
  }));

  return (
    <AppForm<Values>
      id={FORM_ID}
      className="grid gap-4 sm:grid-cols-2 xl:grid-cols-6"
      schema={schema}
      defaultValues={{
        pickup_branch_uuid: "",
        return_branch_uuid: SAME,
        starts_at: localInputFromNow(24),
        ends_at: localInputFromNow(72),
        channel: DEFAULT_CHANNEL,
      }}
      onSubmit={(values) => {
        const pickup = values.pickup_branch_uuid;
        onSearch({
          pickup_branch_uuid: pickup,
          return_branch_uuid:
            values.return_branch_uuid === SAME
              ? pickup
              : values.return_branch_uuid,
          starts_at: localInputToISO(values.starts_at) ?? values.starts_at,
          ends_at: localInputToISO(values.ends_at) ?? values.ends_at,
          channel: values.channel,
        });
      }}
    >
      <AppSelect
        name="pickup_branch_uuid"
        label={t("availability.fields.pickup_branch")}
        placeholder={t("availability.fields.branch_placeholder")}
        options={branchOptions}
      />
      <AppSelect
        name="return_branch_uuid"
        label={t("availability.fields.return_branch")}
        options={returnOptions}
      />
      <AppDatePicker
        name="starts_at"
        withTime
        label={t("availability.fields.starts_at")}
      />
      <AppDatePicker
        name="ends_at"
        withTime
        label={t("availability.fields.ends_at")}
      />
      <AppSelect
        name="channel"
        label={t("availability.fields.channel")}
        description={t("availability.fields.channel_hint")}
        options={channelOptions}
      />
      <div className="flex items-end">
        <Button
          type="submit"
          form={FORM_ID}
          disabled={pending}
          className="w-full"
        >
          <Search className="size-4" />
          {t("availability.actions.search")}
        </Button>
      </div>
    </AppForm>
  );
}
