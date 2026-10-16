"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { depositsService } from "@/features/deposits/services/deposits.service";
import type {
  SecurityDepositInput,
  SecurityDepositResolveInput,
} from "@/features/deposits/types";
import type { ListParams } from "@/features/finance/types";
import { inspectionsService } from "@/features/inspections/services/inspections.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys; every key starts with the organization slug. */
export const depositKeys = {
  all: (slug: string) => ["deposits", slug] as const,
  meta: (slug: string) => ["deposits", slug, "meta"] as const,
  list: (slug: string, p: ListParams) => ["deposits", slug, "list", p] as const,
  totals: (slug: string, days?: number) =>
    ["deposits", slug, "totals", days ?? 0] as const,
  one: (slug: string, uuid: string) => ["deposits", slug, "one", uuid] as const,
  rental: (slug: string, uuid: string) =>
    ["deposits", slug, "rental", uuid] as const,
};

export function useDepositsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: depositKeys.meta(slug),
    queryFn: () => depositsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useDepositsList(params: ListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: depositKeys.list(slug, params),
    queryFn: () => depositsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useDepositTotals(heldOverDays?: number) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: depositKeys.totals(slug, heldOverDays),
    queryFn: () => depositsService.totals(slug, heldOverDays),
  });
}

export function useDeposit(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: depositKeys.one(slug, uuid ?? ""),
    queryFn: () => depositsService.get(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

export function useRentalDeposits(rentalUuid: string | null, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: depositKeys.rental(slug, rentalUuid ?? ""),
    queryFn: () => depositsService.forRental(slug, rentalUuid ?? ""),
    enabled: enabled && Boolean(rentalUuid),
  });
}

/** New damages of the rental's return form (deduction links). */
export function useReturnDamages(rentalUuid: string, enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...depositKeys.rental(slug, rentalUuid), "damages"],
    queryFn: async () => {
      const page = await inspectionsService.list(slug, {
        limit: 5,
        offset: 0,
        rental: rentalUuid,
        kind: "return",
      });
      const form = page.items.find((i) => i.status !== "superseded");
      if (!form) return [];
      const full = await inspectionsService.get(slug, form.uuid);
      return full.damages.filter((d) => d.is_new);
    },
    enabled: enabled && Boolean(rentalUuid),
  });
}

function useDepositMutation<V, R>(
  fn: (slug: string, vars: V) => Promise<R>,
  toastKey?: string,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: V) => fn(slug, vars),
    onSuccess: () => {
      if (toastKey) appToast.success(t(toastKey));
      void queryClient.invalidateQueries({ queryKey: depositKeys.all(slug) });
      // Ledger cards, cash books, tasks and the wizard show deposits too.
      void queryClient.invalidateQueries({ queryKey: ["finance", slug] });
      for (const area of ["rentals", "inspections", "field-tasks"]) {
        void queryClient.invalidateQueries({
          queryKey: ["tenant", slug, area],
        });
      }
    },
  });
}

export const useTakeDeposit = () =>
  useDepositMutation(
    (slug, body: SecurityDepositInput) => depositsService.take(slug, body),
    "deposits.toast.taken",
  );

export const useVerifyDeposit = () =>
  useDepositMutation((slug, uuid: string) =>
    depositsService.verify(slug, uuid),
  );

export const useResolveDeposit = (uuid: string) =>
  useDepositMutation(
    (slug, body: SecurityDepositResolveInput) =>
      depositsService.resolve(slug, uuid, body),
    "deposits.toast.resolved",
  );

export const useCancelDeposit = (uuid: string) =>
  useDepositMutation(
    (slug, reason: string) => depositsService.cancel(slug, uuid, reason),
    "deposits.toast.cancelled",
  );

export const useUploadDeductionDocument = (uuid: string) =>
  useDepositMutation(
    (slug, v: { deduction: string; file: File }) =>
      depositsService.uploadDocument(slug, uuid, v.deduction, v.file),
    "deposits.toast.document_saved",
  );

export const useRemoveDeductionDocument = (uuid: string) =>
  useDepositMutation(
    (slug, deduction: string) =>
      depositsService.removeDocument(slug, uuid, deduction),
    "deposits.toast.document_removed",
  );
