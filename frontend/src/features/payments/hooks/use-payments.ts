"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { PAYMENT_STATUS_LABEL_KEYS } from "@/features/payments/constants";
import { paymentsService } from "@/features/payments/services/payments.service";
import type {
  ConnectIntegrationRequest,
  ListIntegrationJobsParams,
  ListIntegrationsParams,
  ListLinksParams,
  ListOrphansParams,
  ListPaymentsParams,
  MatchOrphanRequest,
  ReconcileJobRequest,
  UpdateIntegrationRequest,
} from "@/features/payments/types";
import { useInvalidateReservations } from "@/features/reservations/hooks/use-reservations";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { ApiError } from "@/lib/api";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const paymentKeys = {
  all: (slug: string) => ["tenant", slug, "payments"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...paymentKeys.all(slug), "list", params] as const,
  links: (slug: string, params: Record<string, unknown>) =>
    [...paymentKeys.all(slug), "links", params] as const,
  integrations: (slug: string, params: Record<string, unknown>) =>
    [...paymentKeys.all(slug), "integrations", params] as const,
  integrationsMeta: (slug: string) =>
    [...paymentKeys.all(slug), "integrations-meta"] as const,
  integrationJobs: (
    slug: string,
    account: string,
    params: Record<string, unknown>,
  ) => [...paymentKeys.all(slug), "integration-jobs", account, params] as const,
  integrationJob: (slug: string, uuid: string) =>
    [...paymentKeys.all(slug), "integration-job", uuid] as const,
  orphans: (slug: string, params: Record<string, unknown>) =>
    [...paymentKeys.all(slug), "orphans", params] as const,
};

/** Payments touch reservations (paid amount, status) and vice versa. */
function useInvalidatePayments() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  const invalidateReservations = useInvalidateReservations();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) }),
      invalidateReservations(),
    ]);
}

export function usePaymentsList(params: ListPaymentsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.list(slug, params),
    queryFn: () => paymentsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function usePaymentLinks(params: ListLinksParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.links(slug, params),
    queryFn: () => paymentsService.links(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useIntegrations(params: ListIntegrationsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.integrations(slug, params),
    queryFn: () => paymentsService.integrations(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useIntegrationsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.integrationsMeta(slug),
    queryFn: () => paymentsService.integrationsMeta(slug),
    staleTime: 10 * 60_000,
  });
}

// --------------------------------------------------------------- mutations

export function useCreatePaymentLink(reservationUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: (body: { amount?: string; expires_in_minutes?: number }) =>
      paymentsService.createLink(slug, reservationUuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.toast.link_created"));
      void invalidate();
    },
  });
}

export function useCancelPaymentLink() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: (uuid: string) => paymentsService.cancelLink(slug, uuid),
    onSuccess: () => {
      appToast.success(t("payments.toast.link_cancelled"));
      void invalidate();
    },
  });
}

export function useCollectPayment(reservationUuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: (body: { amount?: string; method: string; note?: string }) =>
      paymentsService.collect(slug, reservationUuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.toast.collected"));
      void invalidate();
    },
  });
}

export function useRefundPayment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: ({
      uuid,
      amount,
      reason,
    }: {
      uuid: string;
      amount?: string;
      reason: string;
    }) => paymentsService.refund(slug, uuid, { amount, reason }),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.toast.refunded"));
      void invalidate();
    },
  });
}

export function useVerifyPayment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: (uuid: string) => paymentsService.verify(slug, uuid),
    onSuccess: (payment) => {
      appToast.info(
        t("payments.toast.verified", {
          status: t(PAYMENT_STATUS_LABEL_KEYS[payment.status]),
        }),
      );
      void invalidate();
    },
  });
}

export function useAcceptReservation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: ({
      uuid,
      ...body
    }: {
      uuid: string;
      quote_uuid?: string;
      send_payment_link?: boolean;
      note?: string;
    }) => paymentsService.accept(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.toast.accepted"));
      void invalidate();
    },
  });
}

export function useRejectReservation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      paymentsService.reject(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("payments.toast.rejected"));
      void invalidate();
    },
  });
}

export function useConnectIntegration() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: ConnectIntegrationRequest) =>
      paymentsService.connect(slug, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.integrations.toast.connected"));
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
  });
}

export function useUpdateIntegration() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: UpdateIntegrationRequest;
    }) => paymentsService.updateIntegration(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.integrations.toast.updated"));
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
  });
}

export function useTestIntegration() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) => paymentsService.testIntegration(slug, uuid),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.integrations.toast.test_ok"));
    },
    onError: () => {
      appToast.error(t("payments.integrations.toast.test_failed"));
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
  });
}

export function useSetIntegrationStatus() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      uuid,
      action,
    }: {
      uuid: string;
      action: "enable" | "disable";
    }) => paymentsService.setIntegrationStatus(slug, uuid, action),
    onSuccess: (_, variables) => {
      appToast.success(
        t(
          variables.action === "disable"
            ? "payments.integrations.toast.disabled"
            : "payments.integrations.toast.enabled",
        ),
      );
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
  });
}

export function useIntegrationJobs(
  accountUuid: string | null,
  params: ListIntegrationJobsParams,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.integrationJobs(slug, accountUuid ?? "", params),
    queryFn: () =>
      paymentsService.integrationJobs(slug, accountUuid ?? "", params),
    placeholderData: (previous) => previous,
    enabled: Boolean(accountUuid),
  });
}

export function useIntegrationJob(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.integrationJob(slug, uuid ?? ""),
    queryFn: () => paymentsService.integrationJob(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

/** "Yeniden dene": the backend checks the provider first and only resends work that was not done. */
export function useRetryIntegrationJob() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) =>
      paymentsService.retryIntegrationJob(slug, uuid),
    meta: { silent: true },
    onSuccess: (result) => {
      const key =
        result.action === "already_done"
          ? "payments.integrations.jobs.toast.already_done"
          : result.action === "in_progress"
            ? "payments.integrations.jobs.toast.in_progress"
            : "payments.integrations.jobs.toast.resubmitted";
      appToast.success(t(key));
    },
    onError: (error) => {
      const code = error instanceof ApiError ? error.code : "";
      appToast.error(
        t(
          code === "INTEGRATION_JOB_UNVERIFIED"
            ? "payments.integrations.jobs.toast.unverified"
            : code === "INTEGRATION_JOB_BUSY"
              ? "payments.integrations.jobs.toast.busy"
              : code === "INTEGRATION_JOB_NOT_RETRYABLE"
                ? "payments.integrations.jobs.toast.not_retryable"
                : "payments.integrations.jobs.toast.retry_failed",
        ),
      );
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
  });
}

/**
 * "Mutabık işaretle": the work was confirmed outside the runner (provider
 * panel); the job is never sent again. Errors are shown by the dialog.
 */
export function useReconcileIntegrationJob() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ uuid, ...body }: { uuid: string } & ReconcileJobRequest) =>
      paymentsService.reconcileIntegrationJob(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.integrations.jobs.toast.reconciled"));
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
  });
}

export function usePaymentOrphans(params: ListOrphansParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: paymentKeys.orphans(slug, params),
    queryFn: () => paymentsService.orphans(slug, params),
    placeholderData: (previous) => previous,
  });
}

/** Attach an orphan notification to a reservation (provider retrieve first). */
export function useMatchOrphan() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidatePayments();
  return useMutation({
    mutationFn: ({ uuid, ...body }: { uuid: string } & MatchOrphanRequest) =>
      paymentsService.matchOrphan(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("payments.orphans.toast.matched"));
      void invalidate();
    },
  });
}

export function useTestSms() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ uuid, phone }: { uuid: string; phone: string }) =>
      paymentsService.testSms(slug, uuid, phone),
    meta: { silent: true },
    onSuccess: (sent) => {
      appToast.success(
        t("payments.integrations.sms.toast.sent", {
          phone: sent.phone_masked,
        }),
      );
      void queryClient.invalidateQueries({ queryKey: paymentKeys.all(slug) });
    },
    onError: (error) => {
      const code = error instanceof ApiError ? error.code : "";
      if (error instanceof ApiError && error.isValidation) return;
      appToast.error(
        t(
          code === "SMS_RATE_LIMITED"
            ? "payments.integrations.sms.toast.rate_limited"
            : "payments.integrations.sms.toast.failed",
        ),
      );
    },
  });
}
