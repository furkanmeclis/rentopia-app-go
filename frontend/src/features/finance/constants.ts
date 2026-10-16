import type {
  AllocationStatus,
  BalanceStatus,
  CollectionMethod,
  CollectionStatus,
  EntryKind,
  EntryPurpose,
  MovementKind,
  OpenItemStatus,
} from "@/features/finance/types";

/** Engine / live list resources. */
export const ACCOUNTS_RESOURCE = "tenant.finance_accounts";
export const ENTRIES_RESOURCE = "tenant.ledger_entries";
export const CASH_ACCOUNTS_RESOURCE = "tenant.cash_accounts";
export const MOVEMENTS_RESOURCE = "tenant.cash_movements";
export const EXPENSES_RESOURCE = "tenant.expenses";
export const CATEGORIES_RESOURCE = "tenant.expense_categories";
export const COLLECTIONS_RESOURCE = "tenant.finance_collections";

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
  "deposit_hold",
  "deposit_return",
  "deposit_capture",
];

export const ENTRY_KIND_LABEL_KEYS: Record<EntryKind, string> = {
  accrual: "finance.kind.accrual",
  collection: "finance.kind.collection",
  payment: "finance.kind.payment",
  refund: "finance.kind.refund",
  offset: "finance.kind.offset",
  adjustment: "finance.kind.adjustment",
  reversal: "finance.kind.reversal",
  deposit_hold: "finance.kind.deposit_hold",
  deposit_return: "finance.kind.deposit_return",
  deposit_capture: "finance.kind.deposit_capture",
};

export const ENTRY_KIND_TONES: Record<EntryKind, Tone> = {
  accrual: "warning",
  collection: "success",
  payment: "default",
  refund: "default",
  offset: "default",
  adjustment: "default",
  reversal: "danger",
  deposit_hold: "default",
  deposit_return: "default",
  deposit_capture: "warning",
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
  "deposit_in",
  "deposit_out",
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
  deposit_in: "finance.movement_kind.deposit_in",
  deposit_out: "finance.movement_kind.deposit_out",
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
  clearing: "finance.cash_kind.clearing",
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
  "system:payment_refund": "finance.reason.payment_refund",
};

export const COLLECTION_METHODS = [
  "cash",
  "card_pos",
  "bank_transfer",
  "other",
] as const;

/** In-branch collection methods in the "Tahsilat al" order. */
export const BRANCH_METHODS = [
  "cash",
  "bank_transfer",
  "card_pos",
  "other",
] as const;

/** Every collection method (list filters). */
export const ALL_COLLECTION_METHODS: CollectionMethod[] = [
  "cash",
  "bank_transfer",
  "card_pos",
  "online",
  "other",
];

export const METHOD_LABEL_KEYS: Record<CollectionMethod, string> = {
  cash: "finance.method.cash",
  card_pos: "finance.method.card_pos",
  bank_transfer: "finance.method.bank_transfer",
  online: "finance.method.online",
  other: "finance.method.other",
};

/** The reference label of each method (receipt, transfer, POS slip). */
export const METHOD_REFERENCE_LABEL_KEYS: Record<CollectionMethod, string> = {
  cash: "finance.collect.reference_cash",
  bank_transfer: "finance.collect.reference_bank_transfer",
  card_pos: "finance.collect.reference_card_pos",
  online: "finance.collect.reference_online",
  other: "finance.collect.reference_other",
};

export const COLLECTION_STATUSES: CollectionStatus[] = [
  "recorded",
  "partially_refunded",
  "refunded",
  "void",
];

export const COLLECTION_STATUS_LABEL_KEYS: Record<CollectionStatus, string> = {
  recorded: "finance.collection_status.recorded",
  partially_refunded: "finance.collection_status.partially_refunded",
  refunded: "finance.collection_status.refunded",
  void: "finance.collection_status.void",
};

export const COLLECTION_STATUS_TONES: Record<CollectionStatus, Tone> = {
  recorded: "success",
  partially_refunded: "warning",
  refunded: "default",
  void: "danger",
};

export const ALLOCATION_STATUSES: AllocationStatus[] = [
  "unallocated",
  "partially_allocated",
  "allocated",
  "not_allocatable",
];

export const ALLOCATION_STATUS_LABEL_KEYS: Record<AllocationStatus, string> = {
  unallocated: "finance.allocation_status.unallocated",
  partially_allocated: "finance.allocation_status.partially_allocated",
  allocated: "finance.allocation_status.allocated",
  not_allocatable: "finance.allocation_status.not_allocatable",
};

export const ALLOCATION_STATUS_TONES: Record<AllocationStatus, Tone> = {
  unallocated: "warning",
  partially_allocated: "warning",
  allocated: "success",
  not_allocatable: "default",
};

export const ITEM_STATUS_LABEL_KEYS: Record<OpenItemStatus, string> = {
  open: "finance.item_status.open",
  partially_paid: "finance.item_status.partially_paid",
  paid: "finance.item_status.paid",
};

export const ITEM_STATUS_TONES: Record<OpenItemStatus, Tone> = {
  open: "danger",
  partially_paid: "warning",
  paid: "success",
};

export const SETTLEMENT_STATUS_LABEL_KEYS: Record<string, string> = {
  none: "finance.settlement_status.none",
  pending: "finance.settlement_status.pending",
  settled: "finance.settlement_status.settled",
};

export const REFUND_STATUS_LABEL_KEYS: Record<string, string> = {
  pending: "finance.refund_status.pending",
  succeeded: "finance.refund_status.succeeded",
  failed: "finance.refund_status.failed",
};

export const REFUND_STATUS_TONES: Record<string, Tone> = {
  pending: "warning",
  succeeded: "success",
  failed: "danger",
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
