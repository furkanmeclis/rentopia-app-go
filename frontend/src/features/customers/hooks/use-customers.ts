"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { customersService } from "@/features/customers/services/customers.service";
import type {
  CustomerConsentRequest,
  CustomerContactCreateRequest,
  CustomerCreateRequest,
  CustomerDocumentCreateRequest,
  CustomerDriverCreateRequest,
  CustomerPatchRequest,
  ListCustomersParams,
} from "@/features/customers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: two organizations never share cache. */
export const customerKeys = {
  all: (slug: string) => ["tenant", slug, "customers"] as const,
  lists: (slug: string) => [...customerKeys.all(slug), "list"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...customerKeys.lists(slug), params] as const,
  meta: (slug: string) => [...customerKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...customerKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...customerKeys.all(slug), "detail", uuid] as const,
  contacts: (slug: string, uuid: string) =>
    [...customerKeys.detail(slug, uuid), "contacts"] as const,
  consents: (slug: string, uuid: string) =>
    [...customerKeys.detail(slug, uuid), "consents"] as const,
  drivers: (slug: string, uuid: string) =>
    [...customerKeys.detail(slug, uuid), "drivers"] as const,
  documents: (slug: string, uuid: string) =>
    [...customerKeys.detail(slug, uuid), "documents"] as const,
  activity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...customerKeys.detail(slug, uuid), "activity", params] as const,
};

function useInvalidateCustomers() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: customerKeys.all(slug) });
}

export function useCustomersList(params: ListCustomersParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.list(slug, params),
    queryFn: () => customersService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCustomersMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.meta(slug),
    queryFn: () => customersService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useCustomerFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.facets(slug),
    queryFn: () => customersService.facets(slug),
    staleTime: 60_000,
  });
}

export function useCustomer(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.detail(slug, uuid ?? ""),
    queryFn: () => customersService.get(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useCreateCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (body: CustomerCreateRequest) =>
      customersService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("customers.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CustomerPatchRequest;
    }) => customersService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("customers.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (uuid: string) => customersService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("customers.toast.deleted"));
      void invalidate();
    },
  });
}

/**
 * Reveal is a mutation on purpose: the plaintext never enters the query
 * cache; the detail page keeps it in memory for `display_seconds`.
 */
export function useRevealCustomer() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      customersService.reveal(slug, uuid, reason),
    onSuccess: (_, vars) => {
      // The reveal is an activity event of the customer.
      void queryClient.invalidateQueries({
        queryKey: [...customerKeys.detail(slug, vars.uuid), "activity"],
      });
    },
  });
}

export function useAnonymizeCustomer() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      customersService.anonymize(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("customers.toast.anonymized"));
      void invalidate();
    },
  });
}

export function useCustomerActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.activity(slug, uuid, params),
    queryFn: () => customersService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

// --- contacts -------------------------------------------------------------------

export function useCustomerContacts(uuid: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.contacts(slug, uuid),
    queryFn: () => customersService.contacts(slug, uuid),
    enabled,
  });
}

export function useSaveCustomerContact(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: ({
      contactUuid,
      body,
    }: {
      contactUuid?: string;
      body: CustomerContactCreateRequest;
    }) =>
      contactUuid
        ? customersService.updateContact(slug, uuid, contactUuid, body)
        : customersService.createContact(slug, uuid, body),
    onSuccess: (_, vars) => {
      appToast.success(
        vars.contactUuid
          ? t("customers.toast.contact_updated")
          : t("customers.toast.contact_created"),
      );
      void invalidate();
    },
  });
}

export function useDeleteCustomerContact(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (contactUuid: string) =>
      customersService.deleteContact(slug, uuid, contactUuid),
    onSuccess: () => {
      appToast.success(t("customers.toast.contact_deleted"));
      void invalidate();
    },
  });
}

// --- consents -------------------------------------------------------------------

export function useCustomerConsents(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.consents(slug, uuid),
    queryFn: () => customersService.consents(slug, uuid),
  });
}

export function useRecordCustomerConsent(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (body: CustomerConsentRequest) =>
      customersService.recordConsent(slug, uuid, body),
    onSuccess: (_, body) => {
      appToast.success(
        body.action === "grant"
          ? t("customers.toast.consent_granted")
          : t("customers.toast.consent_withdrawn"),
      );
      void invalidate();
    },
  });
}

// --- drivers --------------------------------------------------------------------

export function useCustomerDrivers(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.drivers(slug, uuid),
    queryFn: () => customersService.drivers(slug, uuid),
  });
}

export function useSaveCustomerDriver(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: ({
      driverUuid,
      body,
    }: {
      driverUuid?: string;
      body: CustomerDriverCreateRequest;
    }) => {
      if (!driverUuid) return customersService.createDriver(slug, uuid, body);
      // copy_from_customer is a create-only switch.
      const { copy_from_customer: _copy, ...patch } = body;
      return customersService.updateDriver(slug, uuid, driverUuid, patch);
    },
    onSuccess: (_, vars) => {
      appToast.success(
        vars.driverUuid
          ? t("customers.toast.driver_updated")
          : t("customers.toast.driver_created"),
      );
      void invalidate();
    },
  });
}

export function useDeleteCustomerDriver(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (driverUuid: string) =>
      customersService.deleteDriver(slug, uuid, driverUuid),
    onSuccess: () => {
      appToast.success(t("customers.toast.driver_deleted"));
      void invalidate();
    },
  });
}

export function useDriverEligibility() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: ({
      uuid,
      driverUuid,
      minDriverAge,
      minLicenseYears,
    }: {
      uuid: string;
      driverUuid: string;
      minDriverAge: number;
      minLicenseYears: number;
    }) =>
      customersService.eligibility(slug, uuid, driverUuid, {
        min_driver_age: minDriverAge,
        min_license_years: minLicenseYears,
      }),
  });
}

// --- documents ------------------------------------------------------------------

export function useCustomerDocuments(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerKeys.documents(slug, uuid),
    queryFn: () => customersService.documents(slug, uuid),
  });
}

export function useCreateCustomerDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: async ({
      body,
      file,
    }: {
      body: CustomerDocumentCreateRequest;
      file: File | null;
    }) => {
      const saved = await customersService.createDocument(slug, uuid, body);
      if (!file) return saved;
      try {
        return await customersService.uploadDocumentFile(
          slug,
          uuid,
          saved.uuid,
          file,
        );
      } catch (error) {
        void invalidate();
        throw error;
      }
    },
    onSuccess: () => {
      appToast.success(t("customers.toast.document_created"));
      void invalidate();
    },
  });
}

export function useUploadCustomerDocumentFile(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: ({ docUuid, file }: { docUuid: string; file: File }) =>
      customersService.uploadDocumentFile(slug, uuid, docUuid, file),
    onSuccess: () => {
      appToast.success(t("customers.toast.document_uploaded"));
      void invalidate();
    },
  });
}

export function useDeleteCustomerDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (docUuid: string) =>
      customersService.deleteDocument(slug, uuid, docUuid),
    onSuccess: () => {
      appToast.success(t("customers.toast.document_deleted"));
      void invalidate();
    },
  });
}

export function useDownloadCustomerDocument(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      docUuid,
      fallbackName,
    }: {
      docUuid: string;
      fallbackName: string;
    }) => {
      const { blob, filename } = await customersService.downloadDocumentFile(
        slug,
        uuid,
        docUuid,
      );
      const href = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = href;
      link.download = filename ?? fallbackName;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(href);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: [...customerKeys.detail(slug, uuid), "activity"],
      });
    },
    onError: () => appToast.error(t("customers.toast.download_failed")),
  });
}
