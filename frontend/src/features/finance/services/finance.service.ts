import type { ResourceMeta } from "@/features/io/types";
import type {
  AdjustmentRequest,
  CashAccount,
  CashAccountInput,
  CashMovement,
  CashMovementRequest,
  CashTransfer,
  CashTransferRequest,
  Expense,
  ExpenseCategory,
  ExpenseCategoryInput,
  ExpenseInput,
  FinanceAccount,
  FinanceAccountCreate,
  FinanceAccountPatch,
  FinanceAllocateRequest,
  FinanceCollection,
  FinanceCollectionCreate,
  FinanceCollectionRefundRequest,
  FinanceOpenItems,
  FinanceReleaseRequest,
  FinanceCustomerLedger,
  FinanceRentalBalance,
  FinanceRentalLedger,
  FinanceReport,
  FinanceStatement,
  FinanceSummary,
  LedgerEntry,
  ListParams,
  OffsetRequest,
  Page,
  RefundRequest,
  StatementSendRequest,
  StatementSent,
} from "@/features/finance/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/finance";

/**
 * Finance of one organization (`/v1/tenant/finance/*`): every call names
 * the organization through `tenantRequest` (`X-Organization`).
 */
export const financeService = {
  collectionsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/collections/meta`);
  },
  collections(slug: string, params: ListParams) {
    return tenantRequest<Page<FinanceCollection>>(
      slug,
      "GET",
      `${BASE}/collections`,
      { query: params },
    );
  },
  collection(slug: string, uuid: string) {
    return tenantRequest<FinanceCollection>(
      slug,
      "GET",
      `${BASE}/collections/${uuid}`,
    );
  },
  createCollection(slug: string, body: FinanceCollectionCreate) {
    return tenantRequest<FinanceCollection>(
      slug,
      "POST",
      `${BASE}/collections`,
      { body, silent: true },
    );
  },
  allocateCollection(slug: string, uuid: string, body: FinanceAllocateRequest) {
    return tenantRequest<FinanceCollection>(
      slug,
      "POST",
      `${BASE}/collections/${uuid}/allocations`,
      { body, silent: true },
    );
  },
  releaseAllocation(slug: string, uuid: string, body: FinanceReleaseRequest) {
    return tenantRequest<FinanceCollection>(
      slug,
      "POST",
      `${BASE}/collections/${uuid}/allocations/release`,
      { body, silent: true },
    );
  },
  refundCollection(
    slug: string,
    uuid: string,
    body: FinanceCollectionRefundRequest,
  ) {
    return tenantRequest<FinanceCollection>(
      slug,
      "POST",
      `${BASE}/collections/${uuid}/refund`,
      { body, silent: true },
    );
  },
  voidCollection(slug: string, uuid: string, body: { reason: string }) {
    return tenantRequest<FinanceCollection>(
      slug,
      "POST",
      `${BASE}/collections/${uuid}/void`,
      { body, silent: true },
    );
  },
  openItems(
    slug: string,
    accountUuid: string,
    params: { rental?: string; open?: boolean },
  ) {
    return tenantRequest<FinanceOpenItems>(
      slug,
      "GET",
      `${BASE}/accounts/${accountUuid}/open-items`,
      {
        query: {
          rental: params.rental,
          open: params.open ? "true" : undefined,
        },
      },
    );
  },
  summary(slug: string) {
    return tenantRequest<FinanceSummary>(slug, "GET", `${BASE}/summary`);
  },
  rentalBalances(slug: string, params: ListParams) {
    return tenantRequest<Page<FinanceRentalBalance>>(
      slug,
      "GET",
      `${BASE}/rental-balances`,
      { query: params },
    );
  },

  accountsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/accounts/meta`);
  },
  accounts(slug: string, params: ListParams) {
    return tenantRequest<Page<FinanceAccount>>(
      slug,
      "GET",
      `${BASE}/accounts`,
      {
        query: params,
      },
    );
  },
  createAccount(slug: string, body: FinanceAccountCreate) {
    return tenantRequest<FinanceAccount>(slug, "POST", `${BASE}/accounts`, {
      body,
    });
  },
  account(slug: string, uuid: string) {
    return tenantRequest<FinanceAccount>(
      slug,
      "GET",
      `${BASE}/accounts/${uuid}`,
    );
  },
  patchAccount(slug: string, uuid: string, body: FinanceAccountPatch) {
    return tenantRequest<FinanceAccount>(
      slug,
      "PATCH",
      `${BASE}/accounts/${uuid}`,
      { body },
    );
  },
  statement(slug: string, uuid: string, params: ListParams) {
    return tenantRequest<FinanceStatement>(
      slug,
      "GET",
      `${BASE}/accounts/${uuid}/statement`,
      { query: params },
    );
  },
  refund(slug: string, uuid: string, body: RefundRequest) {
    return tenantRequest<LedgerEntry>(
      slug,
      "POST",
      `${BASE}/accounts/${uuid}/refunds`,
      { body, silent: true },
    );
  },
  adjust(slug: string, uuid: string, body: AdjustmentRequest) {
    return tenantRequest<LedgerEntry>(
      slug,
      "POST",
      `${BASE}/accounts/${uuid}/adjustments`,
      { body, silent: true },
    );
  },
  offset(slug: string, uuid: string, body: OffsetRequest) {
    return tenantRequest<{ items: LedgerEntry[] }>(
      slug,
      "POST",
      `${BASE}/accounts/${uuid}/offsets`,
      { body, silent: true },
    );
  },
  sendStatement(slug: string, uuid: string, body: StatementSendRequest) {
    return tenantRequest<StatementSent>(
      slug,
      "POST",
      `${BASE}/accounts/${uuid}/send-statement`,
      { body, silent: true },
    );
  },

  entriesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/entries/meta`);
  },
  entries(slug: string, params: ListParams) {
    return tenantRequest<Page<LedgerEntry>>(slug, "GET", `${BASE}/entries`, {
      query: params,
    });
  },
  reverseEntry(slug: string, uuid: string, reason: string) {
    return tenantRequest<LedgerEntry>(
      slug,
      "POST",
      `${BASE}/entries/${uuid}/reverse`,
      { body: { reason }, silent: true },
    );
  },
  rentalLedger(slug: string, uuid: string) {
    return tenantRequest<FinanceRentalLedger>(
      slug,
      "GET",
      `${BASE}/rentals/${uuid}`,
    );
  },
  customerLedger(slug: string, uuid: string) {
    return tenantRequest<FinanceCustomerLedger>(
      slug,
      "GET",
      `${BASE}/customers/${uuid}`,
    );
  },

  cashAccountsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(
      slug,
      "GET",
      `${BASE}/cash-accounts/meta`,
    );
  },
  cashAccounts(slug: string, params: ListParams) {
    return tenantRequest<Page<CashAccount>>(
      slug,
      "GET",
      `${BASE}/cash-accounts`,
      { query: params },
    );
  },
  cashAccount(slug: string, uuid: string) {
    return tenantRequest<CashAccount>(
      slug,
      "GET",
      `${BASE}/cash-accounts/${uuid}`,
    );
  },
  createCashAccount(slug: string, body: CashAccountInput) {
    return tenantRequest<CashAccount>(slug, "POST", `${BASE}/cash-accounts`, {
      body,
      silent: true,
    });
  },
  patchCashAccount(slug: string, uuid: string, body: CashAccountInput) {
    return tenantRequest<CashAccount>(
      slug,
      "PATCH",
      `${BASE}/cash-accounts/${uuid}`,
      { body, silent: true },
    );
  },
  recordMovement(slug: string, uuid: string, body: CashMovementRequest) {
    return tenantRequest<CashMovement>(
      slug,
      "POST",
      `${BASE}/cash-accounts/${uuid}/movements`,
      { body, silent: true },
    );
  },
  transfer(slug: string, body: CashTransferRequest) {
    return tenantRequest<CashTransfer>(slug, "POST", `${BASE}/transfers`, {
      body,
      silent: true,
    });
  },
  movementsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/movements/meta`);
  },
  movements(slug: string, params: ListParams) {
    return tenantRequest<Page<CashMovement>>(slug, "GET", `${BASE}/movements`, {
      query: params,
    });
  },
  reverseMovement(slug: string, uuid: string, reason: string) {
    return tenantRequest<{ items: CashMovement[] }>(
      slug,
      "POST",
      `${BASE}/movements/${uuid}/reverse`,
      { body: { reason }, silent: true },
    );
  },

  categories(slug: string, active = false) {
    return tenantRequest<{ items: ExpenseCategory[] }>(
      slug,
      "GET",
      `${BASE}/expense-categories`,
      { query: { active: active ? "true" : undefined } },
    );
  },
  createCategory(slug: string, body: ExpenseCategoryInput) {
    return tenantRequest<ExpenseCategory>(
      slug,
      "POST",
      `${BASE}/expense-categories`,
      { body, silent: true },
    );
  },
  patchCategory(slug: string, uuid: string, body: ExpenseCategoryInput) {
    return tenantRequest<ExpenseCategory>(
      slug,
      "PATCH",
      `${BASE}/expense-categories/${uuid}`,
      { body, silent: true },
    );
  },

  expensesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/expenses/meta`);
  },
  expenses(slug: string, params: ListParams) {
    return tenantRequest<Page<Expense>>(slug, "GET", `${BASE}/expenses`, {
      query: params,
    });
  },
  createExpense(slug: string, body: ExpenseInput) {
    return tenantRequest<Expense>(slug, "POST", `${BASE}/expenses`, {
      body,
      silent: true,
    });
  },
  patchExpense(slug: string, uuid: string, body: ExpenseInput) {
    return tenantRequest<Expense>(slug, "PATCH", `${BASE}/expenses/${uuid}`, {
      body,
      silent: true,
    });
  },
  voidExpense(slug: string, uuid: string, reason: string) {
    return tenantRequest<Expense>(
      slug,
      "POST",
      `${BASE}/expenses/${uuid}/void`,
      { body: { reason }, silent: true },
    );
  },
  uploadAttachment(slug: string, uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<Expense>(
      slug,
      "PUT",
      `${BASE}/expenses/${uuid}/file`,
      form,
    );
  },
  downloadAttachment(slug: string, uuid: string) {
    return tenantDownloadFile(slug, `${BASE}/expenses/${uuid}/file`);
  },
  removeAttachment(slug: string, uuid: string) {
    return tenantRequest<Expense>(
      slug,
      "DELETE",
      `${BASE}/expenses/${uuid}/file`,
    );
  },

  expenseReport(
    slug: string,
    params: { group_by: string; from?: string; to?: string },
  ) {
    return tenantRequest<FinanceReport>(
      slug,
      "GET",
      `${BASE}/reports/expenses`,
      { query: params },
    );
  },
  cashFlowReport(
    slug: string,
    params: { group_by: string; from?: string; to?: string },
  ) {
    return tenantRequest<FinanceReport>(
      slug,
      "GET",
      `${BASE}/reports/cash-flow`,
      { query: params },
    );
  },
};
