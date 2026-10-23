"use client";

import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityDetail, EntitySectionCard } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppSelect,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  FEATURE_DESCRIPTION_KEYS,
  FEATURE_LABEL_KEYS,
  FEATURE_USAGE_KEYS,
  PLAN_LABEL_KEYS,
  isPlanCode,
} from "@/config/features";
import { permissions } from "@/config/permissions";
import {
  useOrganizationPlan,
  usePlans,
  useSetOrganizationFeature,
  useSetOrganizationPlan,
} from "@/features/organizations/hooks/use-organizations";
import type {
  OrganizationFeatureState,
  OrganizationPlan,
  Plan,
} from "@/features/organizations/types";
import { isoToLocalInput, localInputToISO } from "@/lib/utils/datetime-input";
import { date as formatDate } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function planLabel(t: Translate, code: string, fallback: string) {
  return isPlanCode(code) ? t(PLAN_LABEL_KEYS[code]) : fallback;
}

/** `yyyy-MM-dd` of an API instant (local day). */
function isoToDate(value: string | null | undefined) {
  return isoToLocalInput(value).slice(0, 10);
}

/** Local midnight of a `yyyy-MM-dd` day as an API instant, or null. */
function dateToISO(value: string) {
  return value ? (localInputToISO(`${value}T00:00`) ?? null) : null;
}

function createPlanSchema(t: Translate) {
  return z
    .object({
      code: z.string().min(1, t("organizations.plan.validation.plan_required")),
      period_starts_at: z.string(),
      period_ends_at: z.string(),
    })
    .superRefine((values, ctx) => {
      if (
        values.period_starts_at &&
        values.period_ends_at &&
        values.period_ends_at <= values.period_starts_at
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["period_ends_at"],
          message: t("organizations.plan.validation.period_end_after_start"),
        });
      }
    });
}

type PlanValues = z.infer<ReturnType<typeof createPlanSchema>>;

/** Plan select and contract period. */
function PlanCard({
  uuid,
  plan,
  plans,
  canWrite,
}: {
  uuid: string;
  plan: OrganizationPlan;
  plans: Plan[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const setPlan = useSetOrganizationPlan(uuid);
  const schema = useMemo(() => createPlanSchema(t), [t]);
  const options = (plans.length ? plans : [plan.plan]).map((p) => ({
    value: p.code,
    label: planLabel(t, p.code, p.name),
  }));

  return (
    <EntitySectionCard title={t("organizations.plan.plan_title")}>
      <AppForm<PlanValues>
        key={`${plan.plan.code}|${plan.period_starts_at ?? ""}|${plan.period_ends_at ?? ""}`}
        className="space-y-4"
        schema={schema}
        defaultValues={{
          code: plan.plan.code,
          period_starts_at: isoToDate(plan.period_starts_at),
          period_ends_at: isoToDate(plan.period_ends_at),
        }}
        onSubmit={async (values) => {
          await setPlan.mutateAsync({
            code: values.code,
            period_starts_at: dateToISO(values.period_starts_at),
            period_ends_at: dateToISO(values.period_ends_at),
          });
        }}
      >
        {(form) => (
          <>
            <AppSelect
              name="code"
              label={t("organizations.plan.fields.plan")}
              description={t("organizations.plan.plan_hint")}
              options={options}
              disabled={!canWrite}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <AppDatePicker
                name="period_starts_at"
                label={t("organizations.plan.fields.period_starts_at")}
                disabled={!canWrite}
              />
              <AppDatePicker
                name="period_ends_at"
                label={t("organizations.plan.fields.period_ends_at")}
                description={t("organizations.plan.period_hint")}
                disabled={!canWrite}
              />
            </div>
            {canWrite ? (
              <FormActions>
                <Button
                  type="submit"
                  disabled={
                    !form.formState.isDirty || form.formState.isSubmitting
                  }
                >
                  {t("organizations.actions.save")}
                </Button>
              </FormActions>
            ) : null}
          </>
        )}
      </AppForm>
    </EntitySectionCard>
  );
}

/** Current plan at a glance. */
function SummaryCard({ plan }: { plan: OrganizationPlan }) {
  const { t, locale } = useLocale();
  const period =
    plan.period_starts_at || plan.period_ends_at
      ? `${plan.period_starts_at ? formatDate(plan.period_starts_at, undefined, locale) : "…"} – ${
          plan.period_ends_at
            ? formatDate(plan.period_ends_at, undefined, locale)
            : "…"
        }`
      : t("organizations.plan.period_open");
  return (
    <EntitySectionCard title={t("organizations.plan.summary_title")}>
      <EntityDetail
        sections={[
          {
            id: "plan",
            fields: [
              {
                key: "plan",
                label: t("organizations.plan.fields.plan"),
                value: planLabel(t, plan.plan.code, plan.plan.name),
              },
              {
                key: "period",
                label: t("organizations.plan.fields.period"),
                value: period,
              },
              {
                key: "enabled",
                label: t("organizations.plan.fields.enabled_count"),
                value: t("organizations.plan.enabled_count", {
                  enabled: plan.enabled_features.length,
                  total: plan.features.length,
                }),
              },
            ],
          },
        ]}
      />
      <p className="text-muted-foreground mt-4 text-sm">
        {t("organizations.plan.records_hint")}
      </p>
    </EntitySectionCard>
  );
}

type OverrideValue = "default" | "on" | "off";

function overrideValue(state: OrganizationFeatureState): OverrideValue {
  if (state.override === null || state.override === undefined) return "default";
  return state.override ? "on" : "off";
}

/** One feature: effective state, usage and the three-state override. */
function FeatureRow({
  uuid,
  state,
  canWrite,
}: {
  uuid: string;
  state: OrganizationFeatureState;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const setFeature = useSetOrganizationFeature(uuid);
  const usageKey = FEATURE_USAGE_KEYS[state.key];
  const usage =
    usageKey && state.usage !== null && state.usage !== undefined
      ? t(usageKey, { count: state.usage })
      : null;
  const planDefault = state.plan_default
    ? t("organizations.plan.state.on")
    : t("organizations.plan.state.off");

  return (
    <div className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">
            {t(FEATURE_LABEL_KEYS[state.key])}
          </span>
          <StatusChip
            label={
              state.enabled
                ? t("organizations.plan.state.on")
                : t("organizations.plan.state.off")
            }
            tone={state.enabled ? "success" : "default"}
          />
          {state.override !== null && state.override !== undefined ? (
            <StatusChip
              label={t("organizations.plan.state.overridden")}
              tone="warning"
            />
          ) : null}
        </div>
        <p className="text-muted-foreground text-sm">
          {t(FEATURE_DESCRIPTION_KEYS[state.key])}
        </p>
        {usage ? (
          <p className="text-muted-foreground text-xs">{usage}</p>
        ) : null}
      </div>
      <Select
        value={overrideValue(state)}
        onValueChange={(value) =>
          setFeature.mutate({
            key: state.key,
            enabled: value === "default" ? null : value === "on",
          })
        }
        disabled={!canWrite || setFeature.isPending}
      >
        <SelectTrigger
          className="w-full sm:w-56"
          aria-label={t("organizations.plan.override_label", {
            feature: t(FEATURE_LABEL_KEYS[state.key]),
          })}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="default">
            {t("organizations.plan.override.default", { state: planDefault })}
          </SelectItem>
          <SelectItem value="on">
            {t("organizations.plan.override.on")}
          </SelectItem>
          <SelectItem value="off">
            {t("organizations.plan.override.off")}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

/**
 * "Paket / özellikler" (TEC-674): the organization's plan and contract
 * period, and every plan feature with its effective state, usage counter
 * and override (plan default / on / off). Changing the plan never deletes
 * records; features that turn off only stop being served.
 */
export function OrganizationPlanTab({
  organizationUuid,
}: {
  organizationUuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.organizations.planRead);
  const canWrite = can([
    permissions.organizations.write,
    permissions.organizations.planWrite,
  ]);
  const query = useOrganizationPlan(organizationUuid, canRead);
  const plans = usePlans(canRead);

  if (!canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("organizations.plan.forbidden")}
      />
    );
  }
  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("organizations.plan.load_failed")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const plan = query.data;

  return (
    <div className="space-y-6">
      <div className="grid gap-6 lg:grid-cols-2">
        <PlanCard
          uuid={organizationUuid}
          plan={plan}
          plans={plans.data?.items ?? []}
          canWrite={canWrite}
        />
        <SummaryCard plan={plan} />
      </div>
      <EntitySectionCard
        title={t("organizations.plan.features_title")}
        badge={plan.enabled_features.length}
      >
        <p className="text-muted-foreground text-sm">
          {t("organizations.plan.features_description")}
        </p>
        <div className="divide-y">
          {plan.features.map((state) => (
            <FeatureRow
              key={state.key}
              uuid={organizationUuid}
              state={state}
              canWrite={canWrite}
            />
          ))}
        </div>
      </EntitySectionCard>
    </div>
  );
}
