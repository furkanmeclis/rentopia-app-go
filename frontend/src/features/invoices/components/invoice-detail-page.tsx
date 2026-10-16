"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  CheckCircle2,
  FileCode2,
  FileDown,
  Pencil,
  RotateCcw,
  Send,
  Trash2,
  Undo2,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  InvoiceProfileBadge,
  InvoiceStatusChip,
  groupProblems,
  invoiceProblemText,
  problemFieldText,
} from "@/features/invoices/components/invoice-cells";
import {
  InvoiceDraftDialog,
  InvoiceLineDialog,
  InvoiceReasonDialog,
} from "@/features/invoices/components/invoice-dialogs";
import {
  HOLD_REASON_LABEL_KEYS,
  INVOICE_STATUS_HINT_KEYS,
  INVOICE_TYPE_LABEL_KEYS,
  LINE_KIND_LABEL_KEYS,
  RELEASE_REASON_LABEL_KEYS,
  RETRY_ACTION_KEYS,
  TAXPAYER_SOURCE_LABEL_KEYS,
} from "@/features/invoices/constants";
import {
  useDownloadInvoice,
  useFinalizeInvoice,
  useInvoice,
  useInvoicePreview,
  usePatchInvoice,
  useRetryInvoice,
  useValidateInvoice,
} from "@/features/invoices/hooks/use-invoices";
import type {
  InvoiceDetail,
  InvoiceLine,
  InvoiceLink,
} from "@/features/invoices/types";
import { IntegrationAttemptsTable } from "@/features/payments/components/integration-jobs";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const DATE = "d MMM yyyy HH:mm";

/** The go-ubltr HTML render in a sandboxed frame (no scripts, no loads). */
function PreviewCard({ invoice }: { invoice: InvoiceDetail }) {
  const { t } = useLocale();
  const preview = useInvoicePreview(invoice.uuid, invoice.updated_at);
  return (
    <EntitySectionCard title={t("invoices.sections.preview")}>
      <p className="text-muted-foreground mb-3 text-xs">
        {invoice.number
          ? t("invoices.preview.stored")
          : t("invoices.preview.draft")}
      </p>
      {preview.isLoading ? <Skeleton className="h-[480px] w-full" /> : null}
      {preview.isError ? (
        <Alert>
          <AlertDescription>
            {t("invoices.preview.unavailable")}
          </AlertDescription>
        </Alert>
      ) : null}
      {preview.data ? (
        <iframe
          title={t("invoices.sections.preview")}
          sandbox=""
          srcDoc={preview.data}
          className="h-[640px] w-full rounded-md border bg-white"
        />
      ) : null}
    </EntitySectionCard>
  );
}

function ProblemsCard({ invoice }: { invoice: InvoiceDetail }) {
  const { t, locale } = useLocale();
  const groups = groupProblems(invoice.validation_errors);
  if (groups.length === 0) return null;
  return (
    <Alert variant="destructive">
      <AlertTitle>
        {t("invoices.problems.title", {
          count: invoice.validation_errors.length,
        })}
      </AlertTitle>
      <AlertDescription>
        <p className="mb-2">{t("invoices.problems.hint")}</p>
        <ul className="space-y-1.5">
          {groups.map(([field, list]) => (
            <li key={field} className="text-sm">
              <span className="font-medium">{problemFieldText(t, field)}</span>
              <ul className="ms-4 list-disc">
                {list.map((p, i) => (
                  <li key={i} className="break-words">
                    {p.message}
                    <span className="text-muted-foreground ms-1 text-[11px] uppercase">
                      {p.layer}
                    </span>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
        {invoice.validated_at ? (
          <p className="text-muted-foreground mt-2 text-xs">
            {t("invoices.problems.checked_at", {
              at: datetime(invoice.validated_at, DATE, locale),
            })}
          </p>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}

function LinesCard({
  invoice,
  editable,
  onEdit,
}: {
  invoice: InvoiceDetail;
  editable: boolean;
  onEdit: (line: InvoiceLine) => void;
}) {
  const { t } = useLocale();
  const patch = usePatchInvoice();
  const columns = useMemo(
    () =>
      [
        createColumn<InvoiceLine>({
          accessorKey: "position",
          labelKey: "invoices.columns.position",
          enableSorting: false,
          cell: ({ row }) => row.original.position,
        }),
        createColumn<InvoiceLine>({
          accessorKey: "name",
          labelKey: "invoices.columns.description",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="text-sm">{row.original.name}</span>
              <span className="text-muted-foreground text-[11px]">
                {t(LINE_KIND_LABEL_KEYS[row.original.kind])}
                {row.original.days > 0
                  ? ` · ${t("invoices.days", { count: row.original.days })}`
                  : ""}
                {row.original.exemption_code
                  ? ` · ${t("invoices.exemption", { code: row.original.exemption_code })}`
                  : ""}
              </span>
            </span>
          ),
        }),
        createColumn<InvoiceLine>({
          accessorKey: "net_amount",
          labelKey: "invoices.columns.net_amount",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.net_amount}
              currency={invoice.currency}
            />
          ),
        }),
        createColumn<InvoiceLine>({
          accessorKey: "vat_rate",
          labelKey: "invoices.columns.vat_rate",
          enableSorting: false,
          cell: ({ row }) => `%${Number(row.original.vat_rate)}`,
        }),
        createColumn<InvoiceLine>({
          accessorKey: "vat_amount",
          labelKey: "invoices.columns.vat_amount",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.vat_amount}
              currency={invoice.currency}
            />
          ),
        }),
        createColumn<InvoiceLine>({
          accessorKey: "total_amount",
          labelKey: "invoices.columns.total_amount",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.total_amount}
              currency={invoice.currency}
              className="font-medium"
            />
          ),
        }),
        ...(editable
          ? [
              createColumn<InvoiceLine>({
                id: "actions",
                labelKey: "invoices.columns.actions",
                enableSorting: false,
                enableHiding: false,
                cell: ({ row }) => (
                  <EntityRowActions
                    actions={[
                      {
                        id: "edit",
                        label: t("invoices.actions.edit_line"),
                        icon: Pencil,
                        onSelect: () => onEdit(row.original),
                      },
                      ...(row.original.entry_uuid &&
                      invoice.links.filter((l) => !l.released).length > 1
                        ? [
                            {
                              id: "remove",
                              label: t("invoices.actions.remove_item"),
                              icon: Trash2,
                              onSelect: () =>
                                patch.mutate(
                                  {
                                    uuid: invoice.uuid,
                                    body: {
                                      remove_entry_uuids: [
                                        row.original.entry_uuid!,
                                      ],
                                    },
                                  },
                                  {
                                    onSuccess: () =>
                                      appToast.success(
                                        t("invoices.toast.item_removed"),
                                      ),
                                    onError: (e) =>
                                      appToast.error(invoiceProblemText(t, e)),
                                  },
                                ),
                            },
                          ]
                        : []),
                    ]}
                  />
                ),
              }),
            ]
          : []),
      ] as ColumnDef<InvoiceLine, unknown>[],
    [editable, invoice, onEdit, patch, t],
  );
  return (
    <EntitySectionCard
      title={t("invoices.sections.lines")}
      badge={invoice.lines.length}
    >
      <EntityTable
        columns={columns}
        data={invoice.lines}
        getRowId={(row) => row.uuid}
        emptyTitle={t("invoices.lines_empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-invoice-lines-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
        }}
      />
      <dl className="ms-auto mt-4 grid max-w-sm grid-cols-2 gap-x-4 gap-y-1 text-sm">
        <dt className="text-muted-foreground">{t("invoices.totals.net")}</dt>
        <dd className="text-end">
          <Amount
            value={invoice.line_extension_amount}
            currency={invoice.currency}
          />
        </dd>
        <dt className="text-muted-foreground">{t("invoices.totals.vat")}</dt>
        <dd className="text-end">
          <Amount value={invoice.tax_amount} currency={invoice.currency} />
        </dd>
        <dt className="font-medium">{t("invoices.totals.payable")}</dt>
        <dd className="text-end font-semibold">
          <Amount value={invoice.payable_amount} currency={invoice.currency} />
        </dd>
        {invoice.invoice_type === "SATIS" ? (
          <>
            <dt className="text-muted-foreground">
              {t("invoices.totals.linked")}
            </dt>
            <dd className="text-end">
              <Amount
                value={invoice.linked_amount}
                currency={invoice.currency}
                muted
              />
            </dd>
          </>
        ) : null}
        {Number(invoice.rounding_amount) !== 0 ? (
          <>
            <dt className="text-muted-foreground">
              {t("invoices.totals.rounding")}
            </dt>
            <dd className="text-end">
              <Amount
                value={invoice.rounding_amount}
                currency={invoice.currency}
                muted
              />
            </dd>
          </>
        ) : null}
      </dl>
    </EntitySectionCard>
  );
}

function LinksCard({ invoice }: { invoice: InvoiceDetail }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const columns = useMemo(
    () =>
      [
        createColumn<InvoiceLink>({
          accessorKey: "entry_date",
          labelKey: "invoices.columns.entry_date",
          enableSorting: false,
          cell: ({ row }) =>
            date(row.original.entry_date, "d MMM yyyy", locale),
        }),
        createColumn<InvoiceLink>({
          accessorKey: "description",
          labelKey: "invoices.columns.description",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="text-sm">{row.original.description || "—"}</span>
              {row.original.rental ? (
                <Link
                  href={routes.tenant.rentals.detail(
                    slug,
                    row.original.rental.uuid,
                  )}
                  className="text-muted-foreground font-mono text-xs underline-offset-4 hover:underline"
                >
                  {row.original.rental.reference}
                </Link>
              ) : null}
            </span>
          ),
        }),
        createColumn<InvoiceLink>({
          accessorKey: "amount",
          labelKey: "invoices.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount value={row.original.amount} currency={invoice.currency} />
          ),
        }),
        createColumn<InvoiceLink>({
          accessorKey: "released",
          labelKey: "invoices.columns.link_state",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.released ? (
              <Badge variant="outline" className="font-normal">
                {t(
                  RELEASE_REASON_LABEL_KEYS[row.original.release_reason] ??
                    "invoices.release.cancelled",
                )}
              </Badge>
            ) : (
              <Badge variant="secondary" className="font-normal">
                {t("invoices.link_live")}
              </Badge>
            ),
        }),
      ] as ColumnDef<InvoiceLink, unknown>[],
    [invoice.currency, locale, slug, t],
  );
  if (invoice.invoice_type !== "SATIS") return null;
  return (
    <EntitySectionCard
      title={t("invoices.sections.links")}
      badge={invoice.links.length}
    >
      <p className="text-muted-foreground mb-3 text-xs">
        {t("invoices.links_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={invoice.links}
        getRowId={(row) => row.entry_uuid}
        emptyTitle={t("invoices.links_empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-invoice-links-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
        }}
      />
    </EntitySectionCard>
  );
}

/**
 * One invoice: state and what it needs, the validation problems per field,
 * receiver, lines and totals, the ledger items it covers, the go-ubltr
 * preview, the downloads and every integrator call.
 */
export function InvoiceDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useInvoice(uuid);
  const validate = useValidateInvoice();
  const finalize = useFinalizeInvoice();
  const retry = useRetryInvoice();
  const downloadFile = useDownloadInvoice();
  const [editDraft, setEditDraft] = useState(false);
  const [editLine, setEditLine] = useState<InvoiceLine | null>(null);
  const [reason, setReason] = useState<"cancel" | "return" | null>(null);
  const n = query.data;
  const canWrite = can(permissions.tenant.invoices.write);
  const canCancel = can(permissions.tenant.invoices.cancel);

  const runValidate = (id: string) =>
    validate.mutate(id, {
      onSuccess: (res) =>
        res.valid
          ? appToast.success(t("invoices.toast.valid"))
          : appToast.error(
              t("invoices.problems.document_invalid", {
                count: res.problems.length,
              }),
            ),
      onError: (e) => appToast.error(invoiceProblemText(t, e)),
    });
  const runFinalize = (id: string) =>
    finalize.mutate(id, {
      onSuccess: (d) =>
        appToast.success(
          d.status === "succeeded"
            ? t("invoices.toast.issued")
            : d.status === "failed"
              ? t("invoices.toast.send_failed")
              : t("invoices.toast.finalized"),
        ),
      onError: (e) => appToast.error(invoiceProblemText(t, e)),
    });
  const runRetry = (id: string) =>
    retry.mutate(id, {
      onSuccess: (res) =>
        appToast.success(
          t(RETRY_ACTION_KEYS[res.action] ?? "invoices.toast.retried"),
        ),
      onError: (e) => appToast.error(invoiceProblemText(t, e)),
    });
  const fileName = (format: "xml" | "pdf") =>
    `${n?.number ?? "fatura"}.${format}`;

  return (
    <EntityPage
      title={t("invoices.detail.title")}
      permission={permissions.tenant.invoices.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("invoices.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("invoices.title"), href: routes.tenant.invoices.root(slug) },
        { label: n?.number ?? t("invoices.draft_number") },
      ]}
      actions={
        n ? (
          <div className="flex flex-wrap gap-2">
            {canWrite && n.actions.edit ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setEditDraft(true)}
              >
                <Pencil className="size-4" />
                {t("invoices.actions.edit")}
              </Button>
            ) : null}
            {canWrite && n.actions.validate ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={validate.isPending}
                onClick={() => runValidate(n.uuid)}
              >
                <CheckCircle2 className="size-4" />
                {t("invoices.actions.validate")}
              </Button>
            ) : null}
            {canWrite && n.actions.finalize ? (
              <Button
                type="button"
                size="sm"
                disabled={finalize.isPending}
                onClick={() => runFinalize(n.uuid)}
              >
                <Send className="size-4" />
                {t("invoices.actions.finalize")}
              </Button>
            ) : null}
            {canWrite && n.actions.retry ? (
              <Button
                type="button"
                size="sm"
                disabled={retry.isPending}
                onClick={() => runRetry(n.uuid)}
              >
                <RotateCcw className="size-4" />
                {t("invoices.actions.retry")}
              </Button>
            ) : null}
            {n.actions.download_pdf ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={downloadFile.isPending}
                onClick={() =>
                  downloadFile.mutate(
                    {
                      uuid: n.uuid,
                      format: "pdf",
                      fallbackName: fileName("pdf"),
                    },
                    {
                      onError: () =>
                        appToast.error(t("invoices.problems.pdf_unavailable")),
                    },
                  )
                }
              >
                <FileDown className="size-4" />
                PDF
              </Button>
            ) : null}
            {n.actions.download_xml ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={downloadFile.isPending}
                onClick={() =>
                  downloadFile.mutate({
                    uuid: n.uuid,
                    format: "xml",
                    fallbackName: fileName("xml"),
                  })
                }
              >
                <FileCode2 className="size-4" />
                XML
              </Button>
            ) : null}
            {canCancel && n.actions.return ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setReason("return")}
              >
                <Undo2 className="size-4" />
                {t("invoices.actions.return")}
              </Button>
            ) : null}
            {canCancel && n.actions.cancel ? (
              <Button
                type="button"
                size="sm"
                variant="destructive"
                onClick={() => setReason("cancel")}
              >
                <Ban className="size-4" />
                {t("invoices.actions.cancel")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {query.isLoading ? <Loading /> : null}
      {query.isError ? (
        <ErrorState
          title={t("invoices.detail.not_found")}
          description={t("invoices.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {n ? (
        <div className="space-y-6">
          <EntityHeader
            title={n.number ?? t("invoices.draft_number")}
            subtitle={
              <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span>{n.receiver_name || "—"}</span>
                <span className="tabular-nums">
                  {date(n.issue_date, "d MMM yyyy", locale)}
                </span>
                <span>{n.branch.name}</span>
              </span>
            }
            badges={
              <>
                <InvoiceStatusChip status={n.status} />
                <InvoiceProfileBadge profile={n.profile} />
                {n.invoice_type === "IADE" ? (
                  <Badge variant="outline" className="font-normal">
                    {t(INVOICE_TYPE_LABEL_KEYS.IADE!)}
                  </Badge>
                ) : null}
                {n.returned ? (
                  <Badge variant="outline" className="font-normal">
                    {t("invoices.returned")}
                  </Badge>
                ) : null}
              </>
            }
          />
          <Alert variant={n.status === "failed" ? "destructive" : "default"}>
            <AlertTitle>{t(INVOICE_STATUS_HINT_KEYS[n.status])}</AlertTitle>
            <AlertDescription>
              {n.hold_reason ? (
                <p>
                  {t(HOLD_REASON_LABEL_KEYS[n.hold_reason] ?? n.hold_reason)}
                </p>
              ) : null}
              {n.status === "failed" && n.last_error ? (
                <p className="break-words">{n.last_error}</p>
              ) : null}
              {n.cancel_job && n.status === "succeeded" ? (
                <p>
                  {t("invoices.cancel.pending", {
                    status: n.cancel_job.status,
                  })}
                </p>
              ) : null}
              <p>{t("invoices.ledger_note")}</p>
            </AlertDescription>
          </Alert>
          <ProblemsCard invoice={n} />
          <EntitySectionCard title={t("invoices.sections.details")}>
            <EntityDetail
              sections={[
                {
                  id: "details",
                  fields: [
                    {
                      key: "issue_date",
                      label: t("invoices.fields.issue_date"),
                      value: date(n.issue_date, "d MMM yyyy", locale),
                    },
                    {
                      key: "branch",
                      label: t("invoices.columns.branch"),
                      value: n.branch.name,
                    },
                    {
                      key: "account",
                      label: t("invoices.fields.account"),
                      value: n.account ? (
                        <Link
                          href={routes.tenant.finance.account(
                            slug,
                            n.account.uuid,
                          )}
                          className="underline-offset-4 hover:underline"
                        >
                          {n.account.code} · {n.account.name}
                        </Link>
                      ) : (
                        "—"
                      ),
                    },
                    {
                      key: "rental",
                      label: t("invoices.columns.rental"),
                      value: n.rental ? (
                        <Link
                          href={routes.tenant.rentals.detail(
                            slug,
                            n.rental.uuid,
                          )}
                          className="font-mono underline-offset-4 hover:underline"
                        >
                          {n.rental.reference}
                        </Link>
                      ) : (
                        "—"
                      ),
                    },
                    {
                      key: "ettn",
                      label: t("invoices.fields.ettn"),
                      value: (
                        <span className="font-mono text-xs break-all">
                          {n.uuid}
                        </span>
                      ),
                    },
                    {
                      key: "external_ref",
                      label: t("invoices.columns.external_ref"),
                      value: (
                        <span className="font-mono">
                          {n.external_ref || "—"}
                        </span>
                      ),
                    },
                    ...(n.returns
                      ? [
                          {
                            key: "returns",
                            label: t("invoices.fields.returns"),
                            value: (
                              <Link
                                href={routes.tenant.invoices.detail(
                                  slug,
                                  n.returns.uuid,
                                )}
                                className="font-mono underline-offset-4 hover:underline"
                              >
                                {n.returns.number ?? "—"}
                              </Link>
                            ),
                          },
                        ]
                      : []),
                    ...(n.returned_by
                      ? [
                          {
                            key: "returned_by",
                            label: t("invoices.fields.returned_by"),
                            value: (
                              <Link
                                href={routes.tenant.invoices.detail(
                                  slug,
                                  n.returned_by.uuid,
                                )}
                                className="font-mono underline-offset-4 hover:underline"
                              >
                                {n.returned_by.number ??
                                  t("invoices.draft_number")}
                              </Link>
                            ),
                          },
                        ]
                      : []),
                    {
                      key: "sent",
                      label: t("invoices.fields.sent_at"),
                      value: n.sent_at
                        ? datetime(n.sent_at, DATE, locale)
                        : "—",
                    },
                    {
                      key: "succeeded",
                      label: t("invoices.fields.succeeded_at"),
                      value: n.succeeded_at
                        ? datetime(n.succeeded_at, DATE, locale)
                        : "—",
                    },
                    ...(n.cancelled_at
                      ? [
                          {
                            key: "cancelled",
                            label: t("invoices.fields.cancelled"),
                            value: `${datetime(n.cancelled_at, DATE, locale)} · ${n.cancel_reason || "—"}`,
                          },
                        ]
                      : []),
                    {
                      key: "note",
                      label: t("invoices.fields.note"),
                      value: n.note || "—",
                    },
                  ],
                },
                {
                  id: "receiver",
                  title: t("invoices.sections.receiver"),
                  fields: [
                    {
                      key: "name",
                      label: t("invoices.fields.receiver_name"),
                      value: n.receiver.name || "—",
                    },
                    {
                      key: "identity",
                      label: t("invoices.fields.identity_number"),
                      value: (
                        <span className="font-mono">
                          {(n.receiver.identity_type ?? "").toUpperCase()}{" "}
                          {n.receiver.identity_hint || "—"}
                        </span>
                      ),
                    },
                    {
                      key: "tax_office",
                      label: t("invoices.fields.tax_office"),
                      value: n.receiver.tax_office || "—",
                    },
                    {
                      key: "address",
                      label: t("invoices.fields.address"),
                      value:
                        [
                          n.receiver.address,
                          n.receiver.district,
                          n.receiver.city,
                          n.receiver.postal_code,
                          n.receiver.country,
                        ]
                          .filter(Boolean)
                          .join(" · ") || "—",
                    },
                    {
                      key: "email",
                      label: t("invoices.fields.email"),
                      value: n.receiver.email || "—",
                    },
                    {
                      key: "einvoice",
                      label: t("invoices.fields.taxpayer"),
                      value: `${n.receiver.einvoice ? t("invoices.taxpayer.einvoice") : t("invoices.taxpayer.earsiv")} · ${t(
                        TAXPAYER_SOURCE_LABEL_KEYS[
                          n.receiver.taxpayer_source
                        ] ?? "invoices.taxpayer_source.customer_flag",
                      )}`,
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>
          <LinesCard
            invoice={n}
            editable={canWrite && n.actions.edit}
            onEdit={setEditLine}
          />
          <LinksCard invoice={n} />
          <PreviewCard invoice={n} />
          <EntitySectionCard
            title={t("invoices.sections.attempts")}
            badge={n.attempt_log.length}
          >
            <p className="text-muted-foreground mb-3 text-xs">
              {t("invoices.attempts_hint")}
            </p>
            <IntegrationAttemptsTable
              attempts={n.attempt_log}
              persistKey="tenant-invoice-attempts-v1"
            />
          </EntitySectionCard>
          {n.cancel_attempt_log.length ? (
            <EntitySectionCard
              title={t("invoices.sections.cancel_attempts")}
              badge={n.cancel_attempt_log.length}
            >
              <IntegrationAttemptsTable
                attempts={n.cancel_attempt_log}
                persistKey="tenant-invoice-cancel-attempts-v1"
              />
            </EntitySectionCard>
          ) : null}
          {n.actions.edit ? (
            <InvoiceDraftDialog
              invoice={n}
              open={editDraft}
              onClose={() => setEditDraft(false)}
            />
          ) : null}
          <InvoiceLineDialog
            invoice={n}
            line={editLine}
            onClose={() => setEditLine(null)}
          />
          <InvoiceReasonDialog
            invoice={n}
            mode={reason}
            onClose={() => setReason(null)}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
