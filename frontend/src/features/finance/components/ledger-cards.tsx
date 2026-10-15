"use client";

import { EntityTable } from "@/components/entity";
import { useEntryColumns } from "@/features/finance/components/entry-columns";
import { Amount } from "@/features/finance/components/finance-cells";
import type { LedgerEntry } from "@/features/finance/types";
import { useLocale } from "@/providers/locale-provider";

type Figure = {
  label: string;
  value: string;
  currency: string;
  signed?: boolean;
};

/** Debit / credit / balance figures above an entries table. */
export function LedgerFigures({ figures }: { figures: Figure[] }) {
  return (
    <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      {figures.map((f) => (
        <div key={f.label} className="space-y-1">
          <dt className="text-muted-foreground text-xs">{f.label}</dt>
          <dd>
            <Amount
              value={f.value}
              currency={f.currency}
              signed={f.signed}
              className="font-semibold"
            />
          </dd>
        </div>
      ))}
    </dl>
  );
}

/** Entries of one rental / customer: a client-side DataTable (bounded list). */
export function LedgerEntriesTable({
  entries,
  persistKey,
  showAccount = false,
}: {
  entries: LedgerEntry[];
  persistKey: string;
  showAccount?: boolean;
}) {
  const { t } = useLocale();
  const columns = useEntryColumns({ showAccount, showBalance: false });
  return (
    <EntityTable
      columns={columns}
      data={entries}
      getRowId={(row) => row.uuid}
      emptyTitle={t("finance.entries.empty")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{ persistKey, rowSelection: false }}
    />
  );
}
