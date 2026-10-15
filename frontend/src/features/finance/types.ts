import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type FinanceRef = Schemas["FinanceRef"];
export type FinanceBalance = Schemas["FinanceBalance"];
export type FinanceAccount = Schemas["FinanceAccount"];
export type FinanceAccountCreate = Schemas["FinanceAccountCreate"];
export type FinanceAccountPatch = Schemas["FinanceAccountPatch"];
export type LedgerEntry = Schemas["LedgerEntry"];
export type FinanceStatement = Schemas["FinanceStatement"];
export type FinanceSummary = Schemas["FinanceSummary"];
export type FinanceRentalBalance = Schemas["FinanceRentalBalance"];
export type FinanceRentalLedger = Schemas["FinanceRentalLedger"];
export type FinanceCustomerLedger = Schemas["FinanceCustomerLedger"];
export type CollectionRequest = Schemas["FinanceCollectionRequest"];
export type RefundRequest = Schemas["FinanceRefundRequest"];
export type AdjustmentRequest = Schemas["FinanceAdjustmentRequest"];
export type OffsetRequest = Schemas["FinanceOffsetRequest"];
export type StatementSendRequest = Schemas["FinanceStatementSendRequest"];
export type StatementSent = Schemas["FinanceStatementSent"];
export type CashAccount = Schemas["CashAccount"];
export type CashAccountInput = Schemas["CashAccountInput"];
export type CashMovement = Schemas["CashMovement"];
export type CashMovementRequest = Schemas["CashMovementRequest"];
export type CashTransferRequest = Schemas["CashTransferRequest"];
export type CashTransfer = Schemas["CashTransfer"];
export type ExpenseCategory = Schemas["ExpenseCategory"];
export type ExpenseCategoryInput = Schemas["ExpenseCategoryInput"];
export type Expense = Schemas["Expense"];
export type ExpenseInput = Schemas["ExpenseInput"];
export type FinanceReport = Schemas["FinanceReport"];
export type FinanceReportRow = Schemas["FinanceReportRow"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type BalanceStatus = FinanceBalance["status"];
export type EntryKind = LedgerEntry["kind"];
export type EntryPurpose = LedgerEntry["purpose"];
export type MovementKind = CashMovement["kind"];

/** Query of the list endpoints (comma-joined multi-value filters). */
export type ListParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  [filter: string]: string | number | undefined;
};
