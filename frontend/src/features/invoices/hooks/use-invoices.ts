"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { INVOICES_RESOURCE } from "@/features/invoices/constants";
import { invoicesService } from "@/features/invoices/services/invoices.service";
import type {
  InvoiceCreateInput,
  InvoicePatchInput,
  InvoiceSettingsInput,
  ListInvoicesParams,
} from "@/features/invoices/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { download } from "@/lib/utils/format";
import { usePermission } from "@/providers/permission-provider";

/** Query keys carry the organization slug. */
export const invoiceKeys = {
  all: (slug: string) => ["tenant", slug, "invoices"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...invoiceKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...invoiceKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...invoiceKeys.all(slug), "facets"] as const,
  items: (slug: string, query: Record<string, unknown>) =>
    [...invoiceKeys.all(slug), "items", query] as const,
  rentals: (slug: string, query: Record<string, unknown>) =>
    [...invoiceKeys.all(slug), "uninvoiced-rentals", query] as const,
  detail: (slug: string, uuid: string) =>
    [...invoiceKeys.all(slug), "detail", uuid] as const,
  preview: (slug: string, uuid: string, version: string) =>
    [...invoiceKeys.all(slug), "preview", uuid, version] as const,
  settings: (slug: string) => [...invoiceKeys.all(slug), "settings"] as const,
};

export function useInvoicesList(params: ListInvoicesParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.list(slug, params),
    queryFn: () => invoicesService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug) && enabled,
  });
}

export function useInvoicesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.meta(slug),
    queryFn: () => invoicesService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useInvoiceFacets(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.facets(slug),
    queryFn: () => invoicesService.facets(slug),
    staleTime: 60_000,
    enabled: Boolean(slug) && enabled,
  });
}

/**
 * Invoices needing a person (failed / held sends) for the sidebar badge,
 * refreshed live from `tenant.invoices` and every five minutes.
 */
export function useInvoicesNeedsAction() {
  const { can } = usePermission();
  const allowed = can(permissions.tenant.invoices.read);
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: invoiceKeys.facets(slug),
    queryFn: () => invoicesService.facets(slug),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    enabled: Boolean(slug) && allowed,
  });
  const resources = useMemo(() => [INVOICES_RESOURCE], []);
  useTenantLiveRefetch(
    resources,
    () => query.refetch({ cancelRefetch: false }),
    { enabled: allowed },
  );
  return allowed ? query.data : undefined;
}

export function useInvoiceItems(
  query: { account?: string; rental?: string },
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.items(slug, query),
    queryFn: () => invoicesService.items(slug, query),
    enabled: Boolean(slug) && enabled && Boolean(query.account || query.rental),
  });
}

export function useUninvoicedRentals(
  query: { limit: number; offset: number; q?: string },
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.rentals(slug, query),
    queryFn: () => invoicesService.uninvoicedRentals(slug, query),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug) && enabled,
  });
}

export function useInvoice(uuid: string) {
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: invoiceKeys.detail(slug, uuid),
    queryFn: () => invoicesService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
  const resources = useMemo(() => [INVOICES_RESOURCE], []);
  useTenantLiveRefetch(resources, () =>
    query.refetch({ cancelRefetch: false }),
  );
  return query;
}

/** The HTML preview; `version` (updated_at) refetches it after edits. */
export function useInvoicePreview(uuid: string, version: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.preview(slug, uuid, version),
    queryFn: () => invoicesService.preview(slug, uuid),
    enabled: Boolean(uuid && version),
    retry: false,
  });
}

export function useInvoiceSettings(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: invoiceKeys.settings(slug),
    queryFn: () => invoicesService.settings(slug),
    enabled: Boolean(slug) && enabled,
  });
}

function useInvalidateInvoices() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: invoiceKeys.all(slug) });
}

export function useCreateInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (body: InvoiceCreateInput) =>
      invoicesService.create(slug, body),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function usePatchInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (v: { uuid: string; body: InvoicePatchInput }) =>
      invoicesService.patch(slug, v.uuid, v.body),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useValidateInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (uuid: string) => invoicesService.validate(slug, uuid),
    meta: { silent: true },
    onSettled: () => void invalidate(),
  });
}

export function useFinalizeInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (uuid: string) => invoicesService.finalize(slug, uuid),
    meta: { silent: true },
    onSettled: () => void invalidate(),
  });
}

export function useRetryInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (uuid: string) => invoicesService.retry(slug, uuid),
    meta: { silent: true },
    onSettled: () => void invalidate(),
  });
}

export function useCancelInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (v: { uuid: string; reason: string }) =>
      invoicesService.cancel(slug, v.uuid, v.reason),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useReturnInvoice() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (v: {
      uuid: string;
      reason: string;
      issue_date?: string | null;
    }) =>
      invoicesService.returnInvoice(slug, v.uuid, {
        reason: v.reason,
        issue_date: v.issue_date || null,
      }),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useSaveInvoiceSettings() {
  const { slug } = useTenant();
  const invalidate = useInvalidateInvoices();
  return useMutation({
    mutationFn: (body: InvoiceSettingsInput) =>
      invoicesService.saveSettings(slug, body),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

/** XML / PDF download streamed by the API (never a storage URL). */
export function useDownloadInvoice() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: async (v: {
      uuid: string;
      format: "xml" | "pdf";
      fallbackName: string;
    }) => {
      const { blob, filename } = await invoicesService.download(
        slug,
        v.uuid,
        v.format,
      );
      download(blob, filename ?? v.fallbackName);
    },
  });
}
