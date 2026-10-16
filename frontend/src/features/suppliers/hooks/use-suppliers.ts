"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { financeKeys } from "@/features/finance/hooks/use-finance";
import {
  DUE_WINDOW_DAYS,
  SUPPLIER_INVOICES_RESOURCE,
  SUPPLIER_PAYMENTS_RESOURCE,
} from "@/features/suppliers/constants";
import { suppliersService } from "@/features/suppliers/services/suppliers.service";
import type {
  ListParams,
  SupplierInput,
  SupplierInvoiceInput,
  SupplierPaymentInput,
} from "@/features/suppliers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys; every key starts with the organization slug. */
export const suppliersKeys = {
  all: (slug: string) => ["suppliers", slug] as const,
  meta: (slug: string, resource: string) =>
    ["suppliers", slug, "meta", resource] as const,
  facets: (slug: string) => ["suppliers", slug, "facets"] as const,
  list: (slug: string, p: ListParams) =>
    ["suppliers", slug, "list", p] as const,
  detail: (slug: string, uuid: string) =>
    ["suppliers", slug, "detail", uuid] as const,
  activity: (slug: string, uuid: string, p: ListParams) =>
    ["suppliers", slug, "activity", uuid, p] as const,
  invoices: (slug: string, p: ListParams) =>
    ["suppliers", slug, "invoices", p] as const,
  invoice: (slug: string, uuid: string) =>
    ["suppliers", slug, "invoice", uuid] as const,
  payments: (slug: string, p: ListParams) =>
    ["suppliers", slug, "payments", p] as const,
  payables: (slug: string, window: number) =>
    ["suppliers", slug, "payables", window] as const,
};

const META = {
  suppliers: suppliersService.meta,
  invoices: suppliersService.invoicesMeta,
  payments: suppliersService.paymentsMeta,
} as const;

export function useSuppliersMeta(resource: keyof typeof META) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.meta(slug, resource),
    queryFn: () => META[resource](slug),
    staleTime: 5 * 60_000,
  });
}

export function useSupplierFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.facets(slug),
    queryFn: () => suppliersService.facets(slug),
    staleTime: 60_000,
  });
}

export function useSuppliersList(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.list(slug, params),
    queryFn: () => suppliersService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

/** Active suppliers as select options ("TED-000001 · Unvan"). */
export function useSupplierOptions(enabled = true) {
  const { can } = usePermission();
  const query = useSuppliersList(
    { limit: 200, offset: 0, sort: "title", status: "active" },
    enabled && can(permissions.tenant.suppliers.read),
  );
  return useMemo(
    () =>
      (query.data?.items ?? []).map((s) => ({
        value: s.uuid,
        label: `${s.title} · ${s.code}`,
      })),
    [query.data?.items],
  );
}

export function useSupplier(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.detail(slug, uuid),
    queryFn: () => suppliersService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useSupplierActivity(uuid: string, params: ListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.activity(slug, uuid, params),
    queryFn: () => suppliersService.activity(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useSupplierInvoices(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.invoices(slug, params),
    queryFn: () => suppliersService.invoices(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useSupplierInvoice(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.invoice(slug, uuid ?? ""),
    queryFn: () => suppliersService.invoice(slug, uuid!),
    enabled: Boolean(uuid),
  });
}

export function useSupplierPayments(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: suppliersKeys.payments(slug, params),
    queryFn: () => suppliersService.payments(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

/**
 * Overdue / due-soon supplier invoices (card and nav badge). Refreshes when
 * invoices or payments change (live lists).
 */
export function usePayablesSummary(window = DUE_WINDOW_DAYS) {
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.finance.expenses.read);
  const query = useQuery({
    queryKey: suppliersKeys.payables(slug, window),
    queryFn: () => suppliersService.payablesSummary(slug, window),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    enabled: Boolean(slug) && allowed,
  });
  const resources = useMemo(
    () => [SUPPLIER_INVOICES_RESOURCE, SUPPLIER_PAYMENTS_RESOURCE],
    [],
  );
  useTenantLiveRefetch(
    resources,
    () => query.refetch({ cancelRefetch: false }),
    { enabled: allowed },
  );
  return { ...query, allowed };
}

function useSupplierMutation<V, R>(
  fn: (slug: string, vars: V) => Promise<R>,
  toastKey: string,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: V) => fn(slug, vars),
    onSuccess: () => {
      appToast.success(t(toastKey));
      void queryClient.invalidateQueries({
        queryKey: suppliersKeys.all(slug),
      });
      // Statements, cash balances and expenses follow invoices / payments.
      void queryClient.invalidateQueries({ queryKey: financeKeys.all(slug) });
    },
  });
}

export const useSaveSupplier = () =>
  useSupplierMutation(
    (slug, vars: { uuid?: string; body: SupplierInput }) =>
      vars.uuid
        ? suppliersService.patch(slug, vars.uuid, vars.body)
        : suppliersService.create(slug, vars.body),
    "suppliers.toast.saved",
  );

export const useSaveInvoice = () =>
  useSupplierMutation(
    (slug, vars: { uuid?: string; body: SupplierInvoiceInput }) =>
      vars.uuid
        ? suppliersService.patchInvoice(slug, vars.uuid, vars.body)
        : suppliersService.createInvoice(slug, vars.body),
    "suppliers.toast.invoice_saved",
  );

export const useVoidInvoice = () =>
  useSupplierMutation(
    (slug, vars: { uuid: string; reason: string }) =>
      suppliersService.voidInvoice(slug, vars.uuid, vars.reason),
    "suppliers.toast.invoice_voided",
  );

export const useUploadInvoiceFile = () =>
  useSupplierMutation(
    (slug, vars: { uuid: string; file: File }) =>
      suppliersService.uploadInvoiceFile(slug, vars.uuid, vars.file),
    "suppliers.toast.file_uploaded",
  );

export const useRemoveInvoiceFile = () =>
  useSupplierMutation(
    (slug, uuid: string) => suppliersService.removeInvoiceFile(slug, uuid),
    "suppliers.toast.file_removed",
  );

export const useCreatePayment = () =>
  useSupplierMutation(
    (slug, body: SupplierPaymentInput) =>
      suppliersService.createPayment(slug, body),
    "suppliers.toast.payment_recorded",
  );

export const useVoidPayment = () =>
  useSupplierMutation(
    (slug, vars: { uuid: string; reason: string }) =>
      suppliersService.voidPayment(slug, vars.uuid, vars.reason),
    "suppliers.toast.payment_voided",
  );
