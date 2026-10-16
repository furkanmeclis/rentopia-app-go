"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { agreementsService } from "@/features/agreements/services/agreements.service";
import type {
  AgreementContactRequest,
  AgreementDriverRequest,
  AgreementInput,
  AgreementStatusRequest,
  ListAgreementsParams,
} from "@/features/agreements/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const agreementKeys = {
  all: (slug: string) => ["tenant", slug, "agreements"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...agreementKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...agreementKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...agreementKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...agreementKeys.all(slug), "detail", uuid] as const,
  sub: (slug: string, uuid: string, part: string, extra?: unknown) =>
    [...agreementKeys.all(slug), "detail", uuid, part, extra] as const,
};

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: agreementKeys.all(slug) }),
      // The payment term lives on the customer's ledger account too.
      queryClient.invalidateQueries({ queryKey: ["tenant", slug, "finance"] }),
    ]);
}

export function useAgreementsList(
  params: ListAgreementsParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.list(slug, params),
    queryFn: () => agreementsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useAgreementsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.meta(slug),
    queryFn: () => agreementsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useAgreementFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.facets(slug),
    queryFn: () => agreementsService.facets(slug),
    staleTime: 60_000,
  });
}

export function useAgreement(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.detail(slug, uuid ?? ""),
    queryFn: () => agreementsService.get(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

export function useAgreementTariffs(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.sub(slug, uuid, "tariffs"),
    queryFn: () => agreementsService.tariffs(slug, uuid),
  });
}

export function useAgreementRentals(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.sub(slug, uuid, "rentals"),
    queryFn: () => agreementsService.rentals(slug, uuid),
  });
}

export function useAgreementBilling(uuid: string, rental?: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.sub(slug, uuid, "billing", rental ?? ""),
    queryFn: () => agreementsService.billing(slug, uuid, rental),
  });
}

export function useAgreementStatement(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.sub(slug, uuid, "statement"),
    queryFn: () => agreementsService.statement(slug, uuid),
  });
}

export function useAgreementActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: agreementKeys.sub(slug, uuid, "activity", params),
    queryFn: () => agreementsService.activity(slug, uuid, params),
  });
}

export function useCreateAgreement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: AgreementInput) => agreementsService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("agreements.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateAgreement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: AgreementInput }) =>
      agreementsService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("agreements.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteAgreement() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => agreementsService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("agreements.toast.deleted"));
      void invalidate();
    },
  });
}

export function useSetAgreementStatus() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: AgreementStatusRequest;
    }) => agreementsService.setStatus(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("agreements.toast.status_changed"));
      void invalidate();
    },
  });
}

export function useAddAgreementContact() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: AgreementContactRequest;
    }) => agreementsService.addContact(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("agreements.toast.contact_added"));
      void invalidate();
    },
  });
}

export function useRemoveAgreementContact() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, contact }: { uuid: string; contact: string }) =>
      agreementsService.removeContact(slug, uuid, contact),
    onSuccess: () => {
      appToast.success(t("agreements.toast.contact_removed"));
      void invalidate();
    },
  });
}

export function useAddAgreementDriver() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: AgreementDriverRequest;
    }) => agreementsService.addDriver(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("agreements.toast.driver_added"));
      void invalidate();
    },
  });
}

export function useRemoveAgreementDriver() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, driver }: { uuid: string; driver: string }) =>
      agreementsService.removeDriver(slug, uuid, driver),
    onSuccess: () => {
      appToast.success(t("agreements.toast.driver_removed"));
      void invalidate();
    },
  });
}
