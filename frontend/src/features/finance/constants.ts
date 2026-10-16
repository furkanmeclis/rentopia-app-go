import type {
  BalanceStatus,
  EntryKind,
  EntryPurpose,
  MovementKind,
} from "@/features/finance/types";

/** Engine / live list resources. */
export const ACCOUNTS_RESOURCE = "tenant.finance_accounts";
export const ENTRIES_RESOURCE = "tenant.ledger_entries";
export const CASH_ACCOUNTS_RESOURCE = "tenant.cash_accounts";
export const MOVEMENTS_RESOURCE = "tenant.cash_movements";
export const EXPENSES_RESOURCE = "tenant.expenses";
export const CATEGORIES_RESOURCE = "tenant.expense_categories";

type Tone = "default" | "success" | "warning" | "danger";

export const BALANCE_STATUSES: BalanceStatus[] = [
  "overdue",
  "due_soon",
  "receivable",
  "payable",
  "closed",
];

export const BALANCE_STATUS_LABEL_KEYS: Record<BalanceStatus, string> = {
  overdue: "finance.balance_status.overdue",
  due_soon: "finance.balance_status.due_soon",
  receivable: "finance.balance_status.receivable",
  payable: "finance.balance_status.payable",
  closed: "finance.balance_status.closed",
};

export const BALANCE_STATUS_TONES: Record<BalanceStatus, Tone> = {
  overdue: "danger",
  due_soon: "warning",
  receivable: "default",
  payable: "success",
  closed: "default",
};

export const ENTRY_KINDS: EntryKind[] = [
  "accrual",
  "collection",
  "payment",
  "refund",
  "offset",
  "adjustment",
  "reversal",
];

export const ENTRY_KIND_LABEL_KEYS: Record<EntryKind, string> = {
  accrual: "finance.kind.accrual",
  collection: "finance.kind.collection",
  payment: "finance.kind.payment",
  refund: "finance.kind.refund",
  offset: "finance.kind.offset",
  adjustment: "finance.kind.adjustment",
  reversal: "finance.kind.reversal",
};

export const ENTRY_KIND_TONES: Record<EntryKind, Tone> = {
  accrual: "warning",
  collection: "success",
  payment: "default",
  refund: "default",
  offset: "default",
  adjustment: "default",
  reversal: "danger",
};

export const PURPOSE_LABEL_KEYS: Record<EntryPurpose, string> = {
  rent: "finance.purpose.rent",
  charge: "finance.purpose.charge",
  prepayment: "finance.purpose.prepayment",
  deposit: "finance.purpose.deposit",
  general: "finance.purpose.general",
};

export const MOVEMENT_KINDS: MovementKind[] = [
  "collection",
  "expense",
  "refund",
  "supplier_payment",
  "transfer_in",
  "transfer_out",
  "adjustment",
  "reversal",
];

export const MOVEMENT_KIND_LABEL_KEYS: Record<MovementKind, string> = {
  collection: "finance.movement_kind.collection",
  expense: "finance.movement_kind.expense",
  refund: "finance.movement_kind.refund",
  supplier_payment: "finance.movement_kind.supplier_payment",
  transfer_in: "finance.movement_kind.transfer_in",
  transfer_out: "finance.movement_kind.transfer_out",
  adjustment: "finance.movement_kind.adjustment",
  reversal: "finance.movement_kind.reversal",
};

export const MANUAL_MOVEMENT_KINDS = [
  "collection",
  "refund",
  "supplier_payment",
  "adjustment",
] as const;

export const CASH_KIND_LABEL_KEYS = {
  cash: "finance.cash_kind.cash",
  bank: "finance.cash_kind.bank",
} as const;

export const ACCOUNT_STATUS_LABEL_KEYS = {
  active: "finance.account_status.active",
  archived: "finance.account_status.archived",
} as const;

export const EXPENSE_STATUS_LABEL_KEYS = {
  recorded: "finance.expense_status.recorded",
  void: "finance.expense_status.void",
} as const;

export const DUE_BUCKET_LABEL_KEYS: Record<string, string> = {
  overdue_30_plus: "finance.due.overdue_30_plus",
  overdue: "finance.due.overdue",
  due_7_days: "finance.due.due_7_days",
  later: "finance.due.later",
};

/** Automatic reversal reasons (`system:<code>`). */
export const SYSTEM_REASON_LABEL_KEYS: Record<string, string> = {
  "system:amount_changed": "finance.reason.amount_changed",
  "system:no_longer_due": "finance.reason.no_longer_due",
  "system:account_changed": "finance.reason.account_changed",
  "system:rental_cancelled": "finance.reason.rental_cancelled",
  "system:charge_reversed": "finance.reason.charge_reversed",
  "system:invoice_voided": "finance.reason.invoice_voided",
  "system:due_changed": "finance.reason.due_changed",
};

export const COLLECTION_METHODS = [
  "cash",
  "card_pos",
  "bank_transfer",
  "other",
] as const;

export const METHOD_LABEL_KEYS: Record<
  (typeof COLLECTION_METHODS)[number],
  string
> = {
  cash: "finance.method.cash",
  card_pos: "finance.method.card_pos",
  bank_transfer: "finance.method.bank_transfer",
  other: "finance.method.other",
};

export const REPORT_GROUPS = {
  expenses: ["category", "branch", "vehicle", "month"],
  cashFlow: ["account", "branch", "month"],
} as const;

export const REPORT_GROUP_LABEL_KEYS: Record<string, string> = {
  category: "finance.reports.group_category",
  branch: "finance.reports.group_branch",
  vehicle: "finance.reports.group_vehicle",
  month: "finance.reports.group_month",
  account: "finance.reports.group_account",
};

export const REPORT_VALUE_LABEL_KEYS: Record<string, string> = {
  net_amount: "finance.reports.net_amount",
  vat_amount: "finance.reports.vat_amount",
  total_amount: "finance.reports.total_amount",
  total_in: "finance.reports.total_in",
  total_out: "finance.reports.total_out",
  collections: "finance.reports.collections",
  expenses: "finance.reports.expenses",
  refunds: "finance.reports.refunds",
  supplier_payments: "finance.reports.supplier_payments",
};

/** Typed amount: "1.300,50" / "1300.5" → valid. */
export const AMOUNT_INPUT_RE =
  /^\d{1,3}(\.\d{3})*(,\d{1,2})?$|^\d+([.,]\d{1,2})?$/;
