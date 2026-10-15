"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { financeService } from "@/features/finance/services/finance.service";
import type {
  AdjustmentRequest,
  CashAccountInput,
  CashMovementRequest,
  CashTransferRequest,
  CollectionRequest,
  ExpenseCategoryInput,
  ExpenseInput,
  FinanceAccountCreate,
  FinanceAccountPatch,
  ListParams,
  OffsetRequest,
  RefundRequest,
  StatementSendRequest,
} from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys; every key starts with the organization slug. */
export const financeKeys = {
  all: (slug: string) => ["finance", slug] as const,
  summary: (slug: string) => ["finance", slug, "summary"] as const,
  rentalBalances: (slug: string, p: ListParams) =>
    ["finance", slug, "rental-balances", p] as const,
  meta: (slug: string, resource: string) =>
    ["finance", slug, "meta", resource] as const,
  accounts: (slug: string, p: ListParams) =>
    ["finance", slug, "accounts", p] as const,
  account: (slug: string, uuid: string) =>
    ["finance", slug, "account", uuid] as const,
  statement: (slug: string, uuid: string, p: ListParams) =>
    ["finance", slug, "statement", uuid, p] as const,
  entries: (slug: string, p: ListParams) =>
    ["finance", slug, "entries", p] as const,
  rental: (slug: string, uuid: string) =>
    ["finance", slug, "rental", uuid] as const,
  customer: (slug: string, uuid: string) =>
    ["finance", slug, "customer", uuid] as const,
  cashAccounts: (slug: string, p: ListParams) =>
    ["finance", slug, "cash-accounts", p] as const,
  cashAccount: (slug: string, uuid: string) =>
    ["finance", slug, "cash-account", uuid] as const,
  movements: (slug: string, p: ListParams) =>
    ["finance", slug, "movements", p] as const,
  categories: (slug: string, active: boolean) =>
    ["finance", slug, "categories", active] as const,
  expenses: (slug: string, p: ListParams) =>
    ["finance", slug, "expenses", p] as const,
  report: (slug: string, kind: string, p: Record<string, string | undefined>) =>
    ["finance", slug, "report", kind, p] as const,
};

const META = {
  accounts: financeService.accountsMeta,
  entries: financeService.entriesMeta,
  cash: financeService.cashAccountsMeta,
  movements: financeService.movementsMeta,
  expenses: financeService.expensesMeta,
} as const;

export function useFinanceMeta(resource: keyof typeof META) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.meta(slug, resource),
    queryFn: () => META[resource](slug),
    staleTime: 5 * 60_000,
  });
}

export function useFinanceSummary(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.summary(slug),
    queryFn: () => financeService.summary(slug),
    enabled,
  });
}

export function useRentalBalances(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.rentalBalances(slug, params),
    queryFn: () => financeService.rentalBalances(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useAccountsList(params: ListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.accounts(slug, params),
    queryFn: () => financeService.accounts(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useAccount(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.account(slug, uuid),
    queryFn: () => financeService.account(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useStatement(uuid: string, params: ListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.statement(slug, uuid, params),
    queryFn: () => financeService.statement(slug, uuid, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(uuid),
  });
}

export function useEntriesList(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.entries(slug, params),
    queryFn: () => financeService.entries(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useRentalLedger(uuid: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.rental(slug, uuid),
    queryFn: () => financeService.rentalLedger(slug, uuid),
    enabled: enabled && Boolean(uuid),
  });
}

export function useCustomerLedger(uuid: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.customer(slug, uuid),
    queryFn: () => financeService.customerLedger(slug, uuid),
    enabled: enabled && Boolean(uuid),
  });
}

export function useCashAccountsList(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.cashAccounts(slug, params),
    queryFn: () => financeService.cashAccounts(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCashAccount(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.cashAccount(slug, uuid),
    queryFn: () => financeService.cashAccount(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useMovementsList(params: ListParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.movements(slug, params),
    queryFn: () => financeService.movements(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCategories(active = false, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.categories(slug, active),
    queryFn: () => financeService.categories(slug, active),
    staleTime: 60_000,
    enabled,
  });
}

export function useExpensesList(params: ListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.expenses(slug, params),
    queryFn: () => financeService.expenses(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useFinanceReport(
  kind: "expenses" | "cash-flow",
  params: { group_by: string; from?: string; to?: string },
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: financeKeys.report(slug, kind, params),
    queryFn: () =>
      kind === "expenses"
        ? financeService.expenseReport(slug, params)
        : financeService.cashFlowReport(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

function useFinanceMutation<V, R>(
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
      // Rentals / customers show finance cards too.
      void queryClient.invalidateQueries({ queryKey: financeKeys.all(slug) });
    },
  });
}

export const useCreateAccount = () =>
  useFinanceMutation(
    (slug, body: FinanceAccountCreate) =>
      financeService.createAccount(slug, body),
    "finance.toast.account_saved",
  );

export const usePatchAccount = (uuid: string) =>
  useFinanceMutation(
    (slug, body: FinanceAccountPatch) =>
      financeService.patchAccount(slug, uuid, body),
    "finance.toast.account_saved",
  );

export const useCollect = (uuid: string) =>
  useFinanceMutation(
    (slug, body: CollectionRequest) => financeService.collect(slug, uuid, body),
    "finance.toast.collection_recorded",
  );

export const useRefund = (uuid: string) =>
  useFinanceMutation(
    (slug, body: RefundRequest) => financeService.refund(slug, uuid, body),
    "finance.toast.refund_recorded",
  );

export const useAdjust = (uuid: string) =>
  useFinanceMutation(
    (slug, body: AdjustmentRequest) => financeService.adjust(slug, uuid, body),
    "finance.toast.adjustment_posted",
  );

export const useOffset = (uuid: string) =>
  useFinanceMutation(
    (slug, body: OffsetRequest) => financeService.offset(slug, uuid, body),
    "finance.toast.offset_posted",
  );

export const useSendStatement = (uuid: string) =>
  useFinanceMutation(
    (slug, body: StatementSendRequest) =>
      financeService.sendStatement(slug, uuid, body),
    "finance.toast.statement_sent",
  );

export const useReverseEntry = () =>
  useFinanceMutation(
    (slug, vars: { uuid: string; reason: string }) =>
      financeService.reverseEntry(slug, vars.uuid, vars.reason),
    "finance.toast.entry_reversed",
  );

export const useSaveCashAccount = () =>
  useFinanceMutation(
    (slug, vars: { uuid?: string; body: CashAccountInput }) =>
      vars.uuid
        ? financeService.patchCashAccount(slug, vars.uuid, vars.body)
        : financeService.createCashAccount(slug, vars.body),
    "finance.toast.cash_account_saved",
  );

export const useRecordMovement = () =>
  useFinanceMutation(
    (slug, vars: { uuid: string; body: CashMovementRequest }) =>
      financeService.recordMovement(slug, vars.uuid, vars.body),
    "finance.toast.movement_recorded",
  );

export const useTransfer = () =>
  useFinanceMutation(
    (slug, body: CashTransferRequest) => financeService.transfer(slug, body),
    "finance.toast.transfer_made",
  );

export const useReverseMovement = () =>
  useFinanceMutation(
    (slug, vars: { uuid: string; reason: string }) =>
      financeService.reverseMovement(slug, vars.uuid, vars.reason),
    "finance.toast.movement_reversed",
  );

export const useSaveCategory = () =>
  useFinanceMutation(
    (slug, vars: { uuid?: string; body: ExpenseCategoryInput }) =>
      vars.uuid
        ? financeService.patchCategory(slug, vars.uuid, vars.body)
        : financeService.createCategory(slug, vars.body),
    "finance.toast.category_saved",
  );

export const useSaveExpense = () =>
  useFinanceMutation(
    (slug, vars: { uuid?: string; body: ExpenseInput }) =>
      vars.uuid
        ? financeService.patchExpense(slug, vars.uuid, vars.body)
        : financeService.createExpense(slug, vars.body),
    "finance.toast.expense_saved",
  );

export const useVoidExpense = () =>
  useFinanceMutation(
    (slug, vars: { uuid: string; reason: string }) =>
      financeService.voidExpense(slug, vars.uuid, vars.reason),
    "finance.toast.expense_voided",
  );

export const useUploadAttachment = () =>
  useFinanceMutation(
    (slug, vars: { uuid: string; file: File }) =>
      financeService.uploadAttachment(slug, vars.uuid, vars.file),
    "finance.toast.attachment_uploaded",
  );

export const useRemoveAttachment = () =>
  useFinanceMutation(
    (slug, uuid: string) => financeService.removeAttachment(slug, uuid),
    "finance.toast.attachment_removed",
  );
