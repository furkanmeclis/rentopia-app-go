"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { pricingService } from "@/features/pricing/services/pricing.service";
import type {
  DateRuleCreateRequest,
  DateRulePatchRequest,
  ListDateRulesParams,
  ListRatePlansParams,
  MatrixQuery,
  PricingActivateRequest,
  PricingPreviewRequest,
  RatePlanCreateRequest,
  RatePlanMatrixSaveRequest,
  RatePlanMatrixTarget,
  RatePlanPatchRequest,
} from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: two organizations never share cache. */
export const pricingKeys = {
  all: (slug: string) => ["tenant", slug, "pricing"] as const,
  plans: (slug: string) => [...pricingKeys.all(slug), "plans"] as const,
  planList: (slug: string, params: Record<string, unknown>) =>
    [...pricingKeys.plans(slug), "list", params] as const,
  plansMeta: (slug: string) => [...pricingKeys.plans(slug), "meta"] as const,
  planFacets: (slug: string) => [...pricingKeys.plans(slug), "facets"] as const,
  plan: (slug: string, uuid: string) =>
    [...pricingKeys.plans(slug), "detail", uuid] as const,
  planCheck: (slug: string, uuid: string) =>
    [...pricingKeys.plans(slug), "check", uuid] as const,
  planVersions: (slug: string, uuid: string) =>
    [...pricingKeys.plans(slug), "versions", uuid] as const,
  planActivity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...pricingKeys.plans(slug), "activity", uuid, params] as const,
  matrix: (slug: string, query: MatrixQuery) =>
    [...pricingKeys.plans(slug), "matrix", query] as const,
  rules: (slug: string) => [...pricingKeys.all(slug), "rules"] as const,
  ruleList: (slug: string, params: Record<string, unknown>) =>
    [...pricingKeys.rules(slug), "list", params] as const,
  rulesMeta: (slug: string) => [...pricingKeys.rules(slug), "meta"] as const,
  ruleFacets: (slug: string) => [...pricingKeys.rules(slug), "facets"] as const,
  rule: (slug: string, uuid: string) =>
    [...pricingKeys.rules(slug), "detail", uuid] as const,
  ruleCheck: (slug: string, uuid: string) =>
    [...pricingKeys.rules(slug), "check", uuid] as const,
  ruleVersions: (slug: string, uuid: string) =>
    [...pricingKeys.rules(slug), "versions", uuid] as const,
  ruleActivity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...pricingKeys.rules(slug), "activity", uuid, params] as const,
};

function useInvalidate(scope: "plans" | "rules") {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey:
        scope === "plans" ? pricingKeys.plans(slug) : pricingKeys.rules(slug),
    });
}

// ---------------------------------------------------------------- plans

export function useRatePlansList(params: ListRatePlansParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.planList(slug, params),
    queryFn: () => pricingService.plans(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useRatePlansMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.plansMeta(slug),
    queryFn: () => pricingService.plansMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useRatePlanFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.planFacets(slug),
    queryFn: () => pricingService.planFacets(slug),
    staleTime: 60_000,
  });
}

export function useRatePlan(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.plan(slug, uuid),
    queryFn: () => pricingService.plan(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useRatePlanCheck(uuid: string, enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.planCheck(slug, uuid),
    queryFn: () => pricingService.planCheck(slug, uuid),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useRatePlanVersions(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.planVersions(slug, uuid),
    queryFn: () => pricingService.planVersions(slug, uuid),
  });
}

export function useRatePlanActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.planActivity(slug, uuid, params),
    queryFn: () => pricingService.planActivity(slug, uuid, params),
  });
}

export function useCreateRatePlan() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: (body: RatePlanCreateRequest) =>
      pricingService.createPlan(slug, body),
    onSuccess: () => {
      appToast.success(t("pricing.plans.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateRatePlan() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: RatePlanPatchRequest;
    }) => pricingService.updatePlan(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("pricing.plans.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteRatePlan() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: (uuid: string) => pricingService.removePlan(slug, uuid),
    onSuccess: () => {
      appToast.success(t("pricing.plans.toast.deleted"));
      void invalidate();
    },
  });
}

export function useDiscardRatePlanDraft() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: (uuid: string) => pricingService.discardPlanDraft(slug, uuid),
    onSuccess: () => {
      appToast.success(t("pricing.toast.draft_discarded"));
      void invalidate();
    },
  });
}

/** Activation errors (422) are shown by the caller (issue list). */
export function useActivateRatePlan() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PricingActivateRequest;
    }) => pricingService.activatePlan(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("pricing.toast.activated"));
      void invalidate();
    },
  });
}

export function useDeactivateRatePlan() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: (uuid: string) => pricingService.deactivatePlan(slug, uuid),
    onSuccess: () => {
      appToast.success(t("pricing.toast.deactivated"));
      void invalidate();
    },
  });
}

// ---------------------------------------------------------------- matrix

export function useRatePlanMatrix(query: MatrixQuery, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.matrix(slug, query),
    queryFn: () => pricingService.matrix(slug, query),
    enabled,
  });
}

export function useSaveRatePlanMatrix() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: (body: RatePlanMatrixSaveRequest) =>
      pricingService.saveMatrix(slug, body),
    onSuccess: (res) => {
      appToast.success(
        t("pricing.matrix.toast.saved", {
          created: res.created,
          updated: res.updated,
        }),
      );
      void invalidate();
    },
  });
}

export function useActivateRatePlanMatrix() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("plans");
  return useMutation({
    mutationFn: (body: RatePlanMatrixTarget) =>
      pricingService.activateMatrix(slug, body),
    onSuccess: (res) => {
      appToast.success(
        t("pricing.matrix.toast.activated", { count: res.activated }),
      );
      void invalidate();
    },
  });
}

// ---------------------------------------------------------------- rules

export function useDateRulesList(params: ListDateRulesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.ruleList(slug, params),
    queryFn: () => pricingService.rules(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useDateRulesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.rulesMeta(slug),
    queryFn: () => pricingService.rulesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useDateRuleFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.ruleFacets(slug),
    queryFn: () => pricingService.ruleFacets(slug),
    staleTime: 60_000,
  });
}

export function useDateRule(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.rule(slug, uuid),
    queryFn: () => pricingService.rule(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useDateRuleCheck(uuid: string, enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.ruleCheck(slug, uuid),
    queryFn: () => pricingService.ruleCheck(slug, uuid),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useDateRuleVersions(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.ruleVersions(slug, uuid),
    queryFn: () => pricingService.ruleVersions(slug, uuid),
  });
}

export function useDateRuleActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: pricingKeys.ruleActivity(slug, uuid, params),
    queryFn: () => pricingService.ruleActivity(slug, uuid, params),
  });
}

export function useCreateDateRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("rules");
  return useMutation({
    mutationFn: (body: DateRuleCreateRequest) =>
      pricingService.createRule(slug, body),
    onSuccess: () => {
      appToast.success(t("pricing.rules.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateDateRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("rules");
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: DateRulePatchRequest;
    }) => pricingService.updateRule(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("pricing.rules.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteDateRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("rules");
  return useMutation({
    mutationFn: (uuid: string) => pricingService.removeRule(slug, uuid),
    onSuccess: () => {
      appToast.success(t("pricing.rules.toast.deleted"));
      void invalidate();
    },
  });
}

export function useDiscardDateRuleDraft() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("rules");
  return useMutation({
    mutationFn: (uuid: string) => pricingService.discardRuleDraft(slug, uuid),
    onSuccess: () => {
      appToast.success(t("pricing.toast.draft_discarded"));
      void invalidate();
    },
  });
}

export function useActivateDateRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("rules");
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PricingActivateRequest;
    }) => pricingService.activateRule(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("pricing.toast.activated"));
      void invalidate();
    },
  });
}

export function useDeactivateDateRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate("rules");
  return useMutation({
    mutationFn: (uuid: string) => pricingService.deactivateRule(slug, uuid),
    onSuccess: () => {
      appToast.success(t("pricing.toast.deactivated"));
      void invalidate();
    },
  });
}

/** Audit trail of a plan or rule. */
export function usePricingActivity(
  kind: "plans" | "rules",
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey:
      kind === "plans"
        ? pricingKeys.planActivity(slug, uuid, params)
        : pricingKeys.ruleActivity(slug, uuid, params),
    queryFn: () =>
      kind === "plans"
        ? pricingService.planActivity(slug, uuid, params)
        : pricingService.ruleActivity(slug, uuid, params),
  });
}

// ---------------------------------------------------------------- preview

export function usePricingPreview() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: PricingPreviewRequest) =>
      pricingService.preview(slug, body),
  });
}
