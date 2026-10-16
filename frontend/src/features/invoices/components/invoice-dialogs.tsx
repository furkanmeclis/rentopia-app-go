"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { useBulkMutation } from "@/features/bulk-engine";
import { Amount } from "@/features/finance/components/finance-cells";
import { invoiceProblemText } from "@/features/invoices/components/invoice-cells";
import {
  INVOICE_PROFILE_LABEL_KEYS,
  INVOICES_RESOURCE,
  LINE_KIND_LABEL_KEYS,
} from "@/features/invoices/constants";
import {
  useCancelInvoice,
  useCreateInvoice,
  useInvoiceItems,
  usePatchInvoice,
  useReturnInvoice,
  useUninvoicedRentals,
} from "@/features/invoices/hooks/use-invoices";
import type {
  InvoiceDetail,
  InvoiceItem,
  InvoiceLine,
  InvoiceProfile,
  UninvoicedRental,
} from "@/features/invoices/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const CREATE_FORM = "invoice-create-form";

/**
 * "Fatura kes": the invoiceable items of an account or a rental (live
 * accruals no standing invoice covers) with a selection; the draft links
 * the selected items and posts nothing to the ledger.
 */
export function CreateInvoiceDialog({
  open,
  onClose,
  account,
  rental,
}: {
  open: boolean;
  onClose: () => void;
  account?: string;
  rental?: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const items = useInvoiceItems({ account, rental }, open);
  const create = useCreateInvoice();
  const [selection, setSelection] = useState<RowSelectionState | null>(null);
  const [issueDate, setIssueDate] = useState("");
  const [note, setNote] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  // One idempotency key per opened dialog: a double click makes one draft.
  const [key, setKey] = useState(() => crypto.randomUUID());
  const rows = useMemo(() => items.data?.items ?? [], [items.data]);
  const selected = useMemo(() => {
    const all = Object.fromEntries(rows.map((r) => [r.entry_uuid, true]));
    return selection ?? all;
  }, [rows, selection]);
  const chosen = rows.filter((r) => selected[r.entry_uuid]);
  const total = chosen.reduce((n, r) => n + Number(r.amount), 0);

  const close = () => {
    setSelection(null);
    setIssueDate("");
    setNote("");
    setProblem(null);
    setKey(crypto.randomUUID());
    onClose();
  };

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<InvoiceItem>(),
        createColumn<InvoiceItem>({
          accessorKey: "line_kind",
          labelKey: "invoices.columns.line_kind",
          enableSorting: false,
          cell: ({ row }) => t(LINE_KIND_LABEL_KEYS[row.original.line_kind]),
        }),
        createColumn<InvoiceItem>({
          accessorKey: "description",
          labelKey: "invoices.columns.description",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="text-sm">{row.original.description || "—"}</span>
              {row.original.rental ? (
                <span className="text-muted-foreground font-mono text-xs">
                  {row.original.rental.reference}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<InvoiceItem>({
          accessorKey: "entry_date",
          labelKey: "invoices.columns.entry_date",
          enableSorting: false,
          cell: ({ row }) =>
            date(row.original.entry_date, "d MMM yyyy", locale),
        }),
        createColumn<InvoiceItem>({
          accessorKey: "amount",
          labelKey: "invoices.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.amount}
              currency={row.original.currency}
            />
          ),
        }),
      ] as ColumnDef<InvoiceItem, unknown>[],
    [locale, t],
  );

  const submit = async () => {
    setProblem(null);
    if (chosen.length === 0) {
      setProblem(t("invoices.create.none_selected"));
      return;
    }
    try {
      const d = await create.mutateAsync({
        account_uuid: chosen[0]!.account.uuid,
        entry_uuids: chosen.map((r) => r.entry_uuid),
        issue_date: issueDate || null,
        note: note.trim() || null,
        idempotency_key: key,
      });
      appToast.success(t("invoices.toast.draft_created"));
      close();
      router.push(routes.tenant.invoices.detail(slug, d.uuid));
    } catch (error) {
      setProblem(invoiceProblemText(t, error));
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("invoices.create.title")}
      description={t("invoices.create.description")}
      className="sm:max-w-3xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={CREATE_FORM}
            disabled={create.isPending || chosen.length === 0}
          >
            {t("invoices.create.submit", { count: chosen.length })}
          </Button>
        </>
      }
    >
      <form
        id={CREATE_FORM}
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <EntityTable
          columns={columns}
          data={rows}
          getRowId={(row) => row.entry_uuid}
          isLoading={items.isLoading}
          isError={items.isError}
          errorDescription={t("invoices.load_failed")}
          onRetry={() => void items.refetch()}
          emptyTitle={t("invoices.create.empty")}
          emptyDescription={t("invoices.create.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          state={{
            rowSelection: selected,
            onRowSelectionChange: (updater) =>
              setSelection(
                typeof updater === "function" ? updater(selected) : updater,
              ),
          }}
          features={{
            persistKey: "tenant-invoice-items-v1",
            rowSelection: true,
            globalFilter: false,
            sorting: false,
            pagination: false,
          }}
        />
        <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span className="text-muted-foreground">
            {t("invoices.create.selected", { count: chosen.length })}
          </span>
          <span className="font-medium">
            <Amount value={total.toFixed(2)} />
          </span>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>{t("invoices.fields.issue_date")}</Label>
            <DatePicker
              value={issueDate}
              onChange={(v) => setIssueDate(v ?? "")}
              placeholder={t("invoices.create.today")}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="invoice-note">{t("invoices.fields.note")}</Label>
            <Textarea
              id="invoice-note"
              rows={2}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
        </div>
        <p className="text-muted-foreground text-xs">
          {t("invoices.create.ledger_note")}
        </p>
        {problem ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
      </form>
    </AppDialog>
  );
}

/** Cancel (reason) or return (reason + issue date) of an invoice. */
export function InvoiceReasonDialog({
  invoice,
  mode,
  onClose,
}: {
  invoice: InvoiceDetail | null;
  mode: "cancel" | "return" | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const cancel = useCancelInvoice();
  const ret = useReturnInvoice();
  const [reason, setReason] = useState("");
  const [issueDate, setIssueDate] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const open = Boolean(invoice && mode);
  const draft = invoice?.status === "draft";
  const needsReason = mode === "return" || !draft;

  const close = () => {
    setReason("");
    setIssueDate("");
    setProblem(null);
    onClose();
  };

  const submit = async () => {
    if (!invoice || !mode) return;
    setProblem(null);
    if (needsReason && reason.trim().length < 3) {
      setProblem(t("invoices.validation.reason"));
      return;
    }
    try {
      if (mode === "cancel") {
        const d = await cancel.mutateAsync({
          uuid: invoice.uuid,
          reason: reason.trim(),
        });
        appToast.success(
          d.status === "cancelled"
            ? t("invoices.toast.cancelled")
            : t("invoices.toast.cancel_requested"),
        );
        close();
      } else {
        const d = await ret.mutateAsync({
          uuid: invoice.uuid,
          reason: reason.trim(),
          issue_date: issueDate || null,
        });
        appToast.success(t("invoices.toast.return_created"));
        close();
        router.push(routes.tenant.invoices.detail(slug, d.uuid));
      }
    } catch (error) {
      setProblem(invoiceProblemText(t, error));
    }
  };

  const title =
    mode === "return"
      ? t("invoices.return.title")
      : draft
        ? t("invoices.cancel.draft_title")
        : t("invoices.cancel.title");
  const description =
    mode === "return"
      ? t("invoices.return.description")
      : invoice?.profile === "EARSIVFATURA" && invoice.status === "succeeded"
        ? t("invoices.cancel.earsiv_description")
        : t("invoices.cancel.description");

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={title}
      description={description}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            variant={mode === "cancel" ? "destructive" : "default"}
            disabled={cancel.isPending || ret.isPending}
            onClick={() => void submit()}
          >
            {mode === "return"
              ? t("invoices.actions.return")
              : t("invoices.actions.cancel")}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="invoice-reason">
            {t("invoices.fields.reason")}
            {needsReason ? "" : ` (${t("invoices.optional")})`}
          </Label>
          <Textarea
            id="invoice-reason"
            rows={3}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
        {mode === "return" ? (
          <div className="space-y-2">
            <Label>{t("invoices.fields.issue_date")}</Label>
            <DatePicker
              value={issueDate}
              onChange={(v) => setIssueDate(v ?? "")}
              placeholder={t("invoices.create.today")}
            />
          </div>
        ) : null}
        {problem ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
      </div>
    </AppDialog>
  );
}

const DRAFT_FORM = "invoice-draft-form";

type DraftValues = {
  profile: string;
  issue_date: string;
  note: string;
  type: string;
  identity_type: string;
  identity_number: string;
  name: string;
  first_name: string;
  last_name: string;
  tax_office: string;
  address: string;
  district: string;
  city: string;
  postal_code: string;
  country: string;
  email: string;
};

/** Edit a draft: profile, issue date, note and the receiver snapshot. */
export function InvoiceDraftDialog({
  invoice,
  open,
  onClose,
}: {
  invoice: InvoiceDetail;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const patch = usePatchInvoice();
  const [problem, setProblem] = useState<string | null>(null);
  const r = invoice.receiver;
  const profiles: InvoiceProfile[] = r.einvoice
    ? ["TEMELFATURA", "TICARIFATURA"]
    : ["EARSIVFATURA"];
  const schema = useMemo(
    () =>
      z.object({
        profile: z.string().min(1),
        issue_date: z.string().min(1, t("invoices.validation.issue_date")),
        note: z.string().max(1000),
        type: z.string(),
        identity_type: z.string(),
        identity_number: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || /^\d{10,11}$/.test(v),
            t("invoices.validation.identity_number"),
          ),
        name: z.string().max(255),
        first_name: z.string().max(128),
        last_name: z.string().max(128),
        tax_office: z.string().max(128),
        address: z.string().max(500),
        district: z.string().max(128),
        city: z.string().max(128),
        postal_code: z.string().max(16),
        country: z.string().trim().length(2, t("invoices.validation.country")),
        email: z.string().max(255),
      }),
    [t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setProblem(null);
          onClose();
        }
      }}
      title={t("invoices.draft.title")}
      description={t("invoices.draft.description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={DRAFT_FORM} disabled={patch.isPending}>
            {t("form.save")}
          </Button>
        </>
      }
    >
      <AppForm<DraftValues>
        id={DRAFT_FORM}
        schema={schema}
        className="space-y-4"
        defaultValues={{
          profile: invoice.profile,
          issue_date: invoice.issue_date,
          note: invoice.note,
          type: r.type,
          identity_type: r.identity_type ?? "tckn",
          identity_number: "",
          name: r.name,
          first_name: r.first_name,
          last_name: r.last_name,
          tax_office: r.tax_office,
          address: r.address,
          district: r.district,
          city: r.city,
          postal_code: r.postal_code,
          country: r.country || "TR",
          email: r.email,
        }}
        onSubmit={async (v) => {
          setProblem(null);
          try {
            await patch.mutateAsync({
              uuid: invoice.uuid,
              body: {
                profile: v.profile as InvoiceProfile,
                issue_date: v.issue_date,
                note: v.note,
                receiver: {
                  type: v.type as "individual" | "corporate",
                  identity_type: v.identity_type as "vkn" | "tckn",
                  ...(v.identity_number.trim()
                    ? { identity_number: v.identity_number.trim() }
                    : {}),
                  name: v.name,
                  first_name: v.first_name,
                  last_name: v.last_name,
                  tax_office: v.tax_office,
                  address: v.address,
                  district: v.district,
                  city: v.city,
                  postal_code: v.postal_code,
                  country: v.country.toUpperCase(),
                  email: v.email,
                },
              },
            });
            appToast.success(t("invoices.toast.saved"));
            onClose();
          } catch (error) {
            setProblem(invoiceProblemText(t, error));
          }
        }}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <AppSelect
            name="profile"
            label={t("invoices.fields.profile")}
            options={profiles.map((p) => ({
              value: p,
              label: t(INVOICE_PROFILE_LABEL_KEYS[p]),
            }))}
            description={
              r.einvoice
                ? t("invoices.draft.profile_einvoice")
                : t("invoices.draft.profile_earsiv")
            }
          />
          <AppDatePicker
            name="issue_date"
            label={t("invoices.fields.issue_date")}
          />
        </div>
        <AppTextarea name="note" rows={2} label={t("invoices.fields.note")} />
        <p className="text-sm font-medium">{t("invoices.sections.receiver")}</p>
        <div className="grid gap-4 sm:grid-cols-2">
          <AppSelect
            name="type"
            label={t("invoices.fields.receiver_type")}
            options={[
              {
                value: "individual",
                label: t("invoices.receiver_type.individual"),
              },
              {
                value: "corporate",
                label: t("invoices.receiver_type.corporate"),
              },
            ]}
          />
          <AppSelect
            name="identity_type"
            label={t("invoices.fields.identity_type")}
            options={[
              { value: "tckn", label: "TCKN" },
              { value: "vkn", label: "VKN" },
            ]}
          />
          <AppInput
            name="identity_number"
            label={t("invoices.fields.identity_number")}
            placeholder={r.identity_hint || undefined}
            description={t("invoices.draft.identity_hint")}
            inputMode="numeric"
          />
          <AppInput name="name" label={t("invoices.fields.receiver_name")} />
          <AppInput name="first_name" label={t("invoices.fields.first_name")} />
          <AppInput name="last_name" label={t("invoices.fields.last_name")} />
          <AppInput name="tax_office" label={t("invoices.fields.tax_office")} />
          <AppInput name="email" label={t("invoices.fields.email")} />
        </div>
        <AppTextarea
          name="address"
          rows={2}
          label={t("invoices.fields.address")}
        />
        <div className="grid gap-4 sm:grid-cols-4">
          <AppInput name="district" label={t("invoices.fields.district")} />
          <AppInput name="city" label={t("invoices.fields.city")} />
          <AppInput
            name="postal_code"
            label={t("invoices.fields.postal_code")}
          />
          <AppInput name="country" label={t("invoices.fields.country")} />
        </div>
        {problem ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
      </AppForm>
    </AppDialog>
  );
}

const LINE_FORM = "invoice-line-form";

type LineValues = {
  name: string;
  vat_rate: string;
  exemption_code: string;
  exemption_reason: string;
};

/** Edit one draft line: description, VAT rate and exemption. */
export function InvoiceLineDialog({
  invoice,
  line,
  onClose,
}: {
  invoice: InvoiceDetail;
  line: InvoiceLine | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const patch = usePatchInvoice();
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z
        .object({
          name: z.string().trim().min(1).max(255),
          vat_rate: z
            .string()
            .trim()
            .refine(
              (v) => /^\d{1,3}([.,]\d{1,2})?$/.test(v) && Number(v) <= 100,
              t("invoices.validation.vat_rate"),
            ),
          exemption_code: z.string().trim().max(8),
          exemption_reason: z.string().trim().max(255),
        })
        .refine(
          (v) => Number(v.vat_rate.replace(",", ".")) > 0 || v.exemption_code,
          {
            path: ["exemption_code"],
            message: t("invoices.validation.exemption_required"),
          },
        ),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(line)}
      onOpenChange={(next) => {
        if (!next) {
          setProblem(null);
          onClose();
        }
      }}
      title={t("invoices.line.title")}
      description={t("invoices.line.description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={LINE_FORM} disabled={patch.isPending}>
            {t("form.save")}
          </Button>
        </>
      }
    >
      {line ? (
        <AppForm<LineValues>
          id={LINE_FORM}
          schema={schema}
          className="space-y-4"
          defaultValues={{
            name: line.name,
            vat_rate: String(Number(line.vat_rate)),
            exemption_code: line.exemption_code,
            exemption_reason: line.exemption_reason,
          }}
          onSubmit={async (v) => {
            setProblem(null);
            try {
              await patch.mutateAsync({
                uuid: invoice.uuid,
                body: {
                  lines: [
                    {
                      uuid: line.uuid,
                      name: v.name.trim(),
                      vat_rate: v.vat_rate.replace(",", "."),
                      exemption_code: v.exemption_code.trim(),
                      exemption_reason: v.exemption_reason.trim(),
                    },
                  ],
                },
              });
              appToast.success(t("invoices.toast.saved"));
              onClose();
            } catch (error) {
              setProblem(invoiceProblemText(t, error));
            }
          }}
        >
          <AppInput name="name" label={t("invoices.fields.line_name")} />
          <div className="grid gap-4 sm:grid-cols-2">
            <AppInput
              name="vat_rate"
              label={t("invoices.fields.vat_rate")}
              inputMode="decimal"
            />
            <AppInput
              name="exemption_code"
              label={t("invoices.fields.exemption_code")}
              description={t("invoices.line.exemption_hint")}
            />
          </div>
          <AppInput
            name="exemption_reason"
            label={t("invoices.fields.exemption_reason")}
          />
          <p className="text-muted-foreground text-xs">
            {t("invoices.line.amount_note")}
          </p>
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/**
 * Bulk "create invoices for the selected rentals": a picker of rentals with
 * invoiceable items; the bulk engine makes one draft per rental (reversible).
 */
export function RentalsInvoiceDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const [q, setQ] = useState("");
  const [page, setPage] = useState({ pageIndex: 0, pageSize: 10 });
  const [selection, setSelection] = useState<RowSelectionState>({});
  const rentals = useUninvoicedRentals(
    {
      limit: page.pageSize,
      offset: page.pageIndex * page.pageSize,
      q: q.trim() || undefined,
    },
    open,
  );
  const bulk = useBulkMutation();
  const ids = Object.keys(selection).filter((k) => selection[k]);
  const total = rentals.data?.total ?? 0;

  const close = () => {
    setSelection({});
    setQ("");
    onClose();
  };

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<UninvoicedRental>(),
        createColumn<UninvoicedRental>({
          accessorKey: "reference",
          labelKey: "invoices.columns.rental",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs font-medium">
              {row.original.reference}
            </span>
          ),
        }),
        createColumn<UninvoicedRental>({
          accessorKey: "customer_name",
          labelKey: "invoices.columns.receiver",
          enableSorting: false,
        }),
        createColumn<UninvoicedRental>({
          id: "branch",
          accessorFn: (r) => r.branch.code,
          labelKey: "invoices.columns.branch",
          enableSorting: false,
          cell: ({ row }) => row.original.branch.name,
        }),
        createColumn<UninvoicedRental>({
          accessorKey: "first_date",
          labelKey: "invoices.columns.entry_date",
          enableSorting: false,
          cell: ({ row }) =>
            date(row.original.first_date, "d MMM yyyy", locale),
        }),
        createColumn<UninvoicedRental>({
          accessorKey: "open_amount",
          labelKey: "invoices.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col items-end">
              <Amount value={row.original.open_amount} />
              <span className="text-muted-foreground text-xs">
                {t("invoices.bulk.items", { count: row.original.item_count })}
              </span>
            </span>
          ),
        }),
      ] as ColumnDef<UninvoicedRental, unknown>[],
    [locale, t],
  );

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={t("invoices.bulk.title")}
      description={t("invoices.bulk.description")}
      className="sm:max-w-3xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={ids.length === 0 || bulk.isPending}
            onClick={() =>
              void bulk.mutateAsync({
                resource: INVOICES_RESOURCE,
                action: "create_for_rentals",
                target: { scope: "ids", ids },
                onComplete: close,
              })
            }
          >
            {t("invoices.bulk.submit", { count: ids.length })}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <Input
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setPage((p) => ({ ...p, pageIndex: 0 }));
          }}
          placeholder={t("invoices.bulk.search")}
        />
        <EntityTable
          columns={columns}
          data={rentals.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={rentals.isLoading}
          isError={rentals.isError}
          errorDescription={t("invoices.load_failed")}
          onRetry={() => void rentals.refetch()}
          emptyTitle={t("invoices.bulk.empty")}
          pageCount={Math.max(1, Math.ceil(total / page.pageSize))}
          manual={{ filtering: true, sorting: false, pagination: true }}
          state={{
            pagination: page,
            onPaginationChange: (u) =>
              setPage(typeof u === "function" ? u(page) : u),
            rowSelection: selection,
            onRowSelectionChange: (u) =>
              setSelection(typeof u === "function" ? u(selection) : u),
          }}
          features={{
            persistKey: "tenant-invoice-rentals-v1",
            rowSelection: true,
            globalFilter: false,
            sorting: false,
          }}
        />
        <p className="text-muted-foreground text-xs">
          {t("invoices.bulk.note")}
        </p>
      </div>
    </AppDialog>
  );
}
