"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityDrawer } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  channelProblemText,
  runCountsText,
} from "@/features/channels/components/channel-cells";
import {
  COLLECTED_BY_LABEL_KEYS,
  MESSAGE_TYPE_LABEL_KEYS,
  MESSAGE_TYPES,
} from "@/features/channels/constants";
import {
  useChannelSync,
  useUpdateChannelAccount,
} from "@/features/channels/hooks/use-channels";
import type { ChannelAccount, ChannelSyncRun } from "@/features/channels/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

/** Toast of a finished sync run (succeeded / partial / failed). */
export function toastRun(t: T, run: ChannelSyncRun) {
  const counts = runCountsText(t, run);
  if (run.status === "failed") {
    appToast.error(
      t("channels.toast.run_failed", { error: run.error || counts }),
    );
  } else if (run.status === "partial") {
    appToast.warning(t("channels.toast.run_partial", { counts }));
  } else {
    appToast.success(t("channels.toast.run_succeeded", { counts }));
  }
}

const SETTINGS_FORM = "channel-settings-form";

type SettingsValues = {
  push_enabled: boolean;
  pull_enabled: boolean;
  stop_sale: boolean;
  horizon_days: string;
  push_interval_minutes: string;
  pull_interval_minutes: string;
  validity_minutes: string;
  offline_policy: string;
  offline_quota: string;
  collected_by: string;
  commission_rate: string;
};

function intIn(min: number, max: number) {
  return (v: string) => {
    const n = Number(v);
    return Number.isInteger(n) && n >= min && n <= max;
  };
}

/** Sync settings of a channel account (drawer). */
export function ChannelSettingsDrawer({
  account,
  onClose,
}: {
  account: ChannelAccount | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useUpdateChannelAccount();
  const schema = useMemo(
    () =>
      z.object({
        push_enabled: z.boolean(),
        pull_enabled: z.boolean(),
        stop_sale: z.boolean(),
        horizon_days: z
          .string()
          .refine(
            intIn(1, 90),
            t("channels.validation.range", { min: 1, max: 90 }),
          ),
        push_interval_minutes: z
          .string()
          .refine(
            intIn(5, 1440),
            t("channels.validation.range", { min: 5, max: 1440 }),
          ),
        pull_interval_minutes: z
          .string()
          .refine(
            intIn(1, 1440),
            t("channels.validation.range", { min: 1, max: 1440 }),
          ),
        validity_minutes: z
          .string()
          .refine(
            intIn(15, 2880),
            t("channels.validation.range", { min: 15, max: 2880 }),
          ),
        offline_policy: z.string().min(1),
        offline_quota: z
          .string()
          .refine(
            intIn(0, 100),
            t("channels.validation.range", { min: 0, max: 100 }),
          ),
        collected_by: z.string().min(1),
        commission_rate: z.string().refine(
          (v) => {
            const n = Number(v);
            return v.trim() !== "" && !Number.isNaN(n) && n >= 0 && n <= 100;
          },
          t("channels.validation.range", { min: 0, max: 100 }),
        ),
      }),
    [t],
  );
  const s = account?.settings;
  return (
    <EntityDrawer
      open={Boolean(account)}
      onOpenChange={(open) => (!open ? onClose() : undefined)}
      title={t("channels.settings.title")}
      description={account?.display_name}
      size="md"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={SETTINGS_FORM} disabled={save.isPending}>
            {t("channels.actions.save")}
          </Button>
        </div>
      }
    >
      {account && s ? (
        <AppForm<SettingsValues>
          key={account.uuid}
          id={SETTINGS_FORM}
          className="space-y-4 px-4"
          schema={schema}
          defaultValues={{
            push_enabled: s.push_enabled,
            pull_enabled: s.pull_enabled,
            stop_sale: s.stop_sale,
            horizon_days: String(s.horizon_days),
            push_interval_minutes: String(s.push_interval_minutes),
            pull_interval_minutes: String(s.pull_interval_minutes),
            validity_minutes: String(s.validity_minutes),
            offline_policy: s.offline_policy,
            offline_quota: String(s.offline_quota),
            collected_by: s.collected_by,
            commission_rate: s.commission_rate,
          }}
          onSubmit={async (v) => {
            await save.mutateAsync({
              uuid: account.uuid,
              body: {
                push_enabled: v.push_enabled,
                pull_enabled: v.pull_enabled,
                stop_sale: v.stop_sale,
                horizon_days: Number(v.horizon_days),
                push_interval_minutes: Number(v.push_interval_minutes),
                pull_interval_minutes: Number(v.pull_interval_minutes),
                validity_minutes: Number(v.validity_minutes),
                offline_policy: v.offline_policy as "close" | "quota",
                offline_quota: Number(v.offline_quota),
                collected_by: v.collected_by as "channel" | "us",
                commission_rate: v.commission_rate.trim(),
              },
            });
            onClose();
          }}
        >
          <AppSwitch
            name="push_enabled"
            label={t("channels.settings.push_enabled")}
            description={t("channels.settings.push_enabled_hint")}
          />
          <AppSwitch
            name="pull_enabled"
            label={t("channels.settings.pull_enabled")}
          />
          <AppSwitch
            name="stop_sale"
            label={t("channels.settings.stop_sale")}
            description={t("channels.settings.stop_sale_hint")}
          />
          <AppInput
            name="horizon_days"
            type="number"
            inputMode="numeric"
            label={t("channels.settings.horizon_days")}
          />
          <AppInput
            name="push_interval_minutes"
            type="number"
            inputMode="numeric"
            label={t("channels.settings.push_interval_minutes")}
          />
          <AppInput
            name="pull_interval_minutes"
            type="number"
            inputMode="numeric"
            label={t("channels.settings.pull_interval_minutes")}
          />
          <AppInput
            name="validity_minutes"
            type="number"
            inputMode="numeric"
            label={t("channels.settings.validity_minutes")}
            description={t("channels.settings.validity_minutes_hint")}
          />
          <AppSelect
            name="offline_policy"
            label={t("channels.settings.offline_policy")}
            options={[
              { value: "close", label: t("channels.settings.offline_close") },
              {
                value: "quota",
                label: t("channels.settings.offline_quota_mode"),
              },
            ]}
          />
          <AppInput
            name="offline_quota"
            type="number"
            inputMode="numeric"
            label={t("channels.settings.offline_quota")}
            description={t("channels.settings.offline_quota_hint")}
          />
          <AppSelect
            name="collected_by"
            label={t("channels.settings.collected_by")}
            options={["channel", "us"].map((v) => ({
              value: v,
              label: t(COLLECTED_BY_LABEL_KEYS[v]),
            }))}
          />
          <AppInput
            name="commission_rate"
            inputMode="decimal"
            label={t("channels.settings.commission_rate")}
            description={t("channels.settings.commission_rate_hint")}
          />
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}

const SIMULATE_FORM = "channel-simulate-form";

type SimulateValues = {
  type: string;
  external_ref: string;
  product_code: string;
  pickup_location_code: string;
  return_location_code: string;
  offer_ref: string;
  starts_at: string;
  ends_at: string;
  first_name: string;
  last_name: string;
  total: string;
  commission: string;
  collected_by: string;
};

function isoOf(local: string): string | undefined {
  if (!local) return undefined;
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

/**
 * "Rezervasyon simüle et" on a development adapter: the message is queued
 * as the channel would send it and pulled at once.
 */
export function ChannelSimulateDialog({
  account,
  onClose,
}: {
  account: ChannelAccount | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const sync = useChannelSync();
  const schema = useMemo(
    () =>
      z
        .object({
          type: z.string().min(1),
          external_ref: z.string().max(64),
          product_code: z.string().max(64),
          pickup_location_code: z.string().max(64),
          return_location_code: z.string().max(64),
          offer_ref: z.string().max(64),
          starts_at: z.string(),
          ends_at: z.string(),
          first_name: z.string().max(100),
          last_name: z.string().max(100),
          total: z.string(),
          commission: z.string(),
          collected_by: z.string(),
        })
        .refine((v) => v.type === "booking" || v.external_ref.trim() !== "", {
          path: ["external_ref"],
          message: t("channels.validation.external_ref"),
        })
        .refine(
          (v) =>
            v.type === "cancellation" ||
            (v.product_code.trim() !== "" &&
              v.pickup_location_code.trim() !== ""),
          {
            path: ["product_code"],
            message: t("channels.validation.codes"),
          },
        )
        .refine(
          (v) =>
            v.type === "cancellation" ||
            (Boolean(isoOf(v.starts_at)) &&
              Boolean(isoOf(v.ends_at)) &&
              new Date(v.ends_at) > new Date(v.starts_at)),
          { path: ["ends_at"], message: t("channels.validation.range_dates") },
        ),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(account)}
      onOpenChange={(open) => (!open ? onClose() : undefined)}
      title={t("channels.simulate.title")}
      description={t("channels.simulate.description")}
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={SIMULATE_FORM} disabled={sync.isPending}>
            {t("channels.simulate.submit")}
          </Button>
        </>
      }
    >
      {account ? (
        <AppForm<SimulateValues>
          id={SIMULATE_FORM}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            type: "booking",
            external_ref: "",
            product_code: "",
            pickup_location_code: "",
            return_location_code: "",
            offer_ref: "",
            starts_at: "",
            ends_at: "",
            first_name: "",
            last_name: "",
            total: "",
            commission: "",
            collected_by: "channel",
          }}
          onSubmit={(v) =>
            sync.mutate(
              {
                uuid: account.uuid,
                action: "simulate",
                body: {
                  type: v.type as "booking" | "modification" | "cancellation",
                  external_ref: v.external_ref.trim() || undefined,
                  product_code: v.product_code.trim() || undefined,
                  pickup_location_code:
                    v.pickup_location_code.trim() || undefined,
                  return_location_code:
                    v.return_location_code.trim() || undefined,
                  offer_ref: v.offer_ref.trim() || undefined,
                  starts_at: isoOf(v.starts_at),
                  ends_at: isoOf(v.ends_at),
                  customer: {
                    first_name: v.first_name.trim() || undefined,
                    last_name: v.last_name.trim() || undefined,
                  },
                  total: v.total.trim() || undefined,
                  commission: v.commission.trim() || undefined,
                  collected_by: v.collected_by as "channel" | "us",
                },
              },
              {
                onSuccess: (run) => {
                  toastRun(t, run);
                  onClose();
                },
                onError: (error) =>
                  appToast.error(channelProblemText(t, error)),
              },
            )
          }
        >
          <AppSelect
            name="type"
            label={t("channels.simulate.type")}
            options={MESSAGE_TYPES.map((v) => ({
              value: v,
              label: t(MESSAGE_TYPE_LABEL_KEYS[v]),
            }))}
          />
          <AppInput
            name="external_ref"
            label={t("channels.simulate.external_ref")}
            description={t("channels.simulate.external_ref_hint")}
          />
          <AppInput
            name="product_code"
            label={t("channels.simulate.product_code")}
          />
          <AppInput name="offer_ref" label={t("channels.simulate.offer_ref")} />
          <AppInput
            name="pickup_location_code"
            label={t("channels.simulate.pickup_location_code")}
          />
          <AppInput
            name="return_location_code"
            label={t("channels.simulate.return_location_code")}
          />
          <AppDatePicker
            name="starts_at"
            withTime
            label={t("channels.simulate.starts_at")}
          />
          <AppDatePicker
            name="ends_at"
            withTime
            label={t("channels.simulate.ends_at")}
          />
          <AppInput
            name="first_name"
            label={t("channels.simulate.first_name")}
          />
          <AppInput name="last_name" label={t("channels.simulate.last_name")} />
          <AppInput
            name="total"
            inputMode="decimal"
            label={t("channels.simulate.total")}
          />
          <AppInput
            name="commission"
            inputMode="decimal"
            label={t("channels.simulate.commission")}
          />
          <AppSelect
            name="collected_by"
            label={t("channels.simulate.collected_by")}
            options={["channel", "us"].map((v) => ({
              value: v,
              label: t(COLLECTED_BY_LABEL_KEYS[v]),
            }))}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
