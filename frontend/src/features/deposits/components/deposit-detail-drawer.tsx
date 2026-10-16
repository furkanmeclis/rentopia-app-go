"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Check,
  Copy,
  Download,
  ExternalLink,
  FileUp,
  RefreshCw,
  Scissors,
  Trash2,
  Undo2,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppTextarea } from "@/components/forms";
import {
  EntityDrawer,
  EntityRowActions,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { FileUploader } from "@/components/upload/file-uploader";
import { Loading } from "@/components/common/loading";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  DepositMethodBadge,
  DepositStatusChip,
} from "@/features/deposits/components/deposit-cells";
import { ResolveDepositDialog } from "@/features/deposits/components/resolve-deposit-dialog";
import {
  CANCEL_REASON_KEYS,
  DEDUCTION_TARGET_LABEL_KEYS,
} from "@/features/deposits/constants";
import {
  useCancelDeposit,
  useDeposit,
  useRemoveDeductionDocument,
  useUploadDeductionDocument,
  useVerifyDeposit,
} from "@/features/deposits/hooks/use-deposits";
import { depositsService } from "@/features/deposits/services/deposits.service";
import type {
  DepositDeduction,
  SecurityDeposit,
} from "@/features/deposits/types";
import {
  Amount,
  AttachmentBadge,
} from "@/features/finance/components/finance-cells";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import { errorText } from "@/features/finance/lib/format";
import { CHARGE_KIND_LABEL_KEYS } from "@/features/rentals/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime, download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function Fact({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate text-sm">{children}</dd>
    </div>
  );
}

/** The provider's hosted pre-auth page of a pending online deposit. */
function PreAuthLink({ url }: { url: string }) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-2 rounded-md border p-3">
      <p className="text-sm font-medium">{t("deposits.detail.preauth_page")}</p>
      <div className="flex gap-2">
        <Input
          readOnly
          value={url}
          aria-label={t("deposits.detail.preauth_page")}
        />
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("deposits.actions.copy")}
          onClick={() => {
            void navigator.clipboard.writeText(url).then(() => {
              setCopied(true);
              window.setTimeout(() => setCopied(false), 2000);
            });
          }}
        >
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("deposits.actions.open_page")}
          onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
        >
          <ExternalLink className="size-4" />
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {t("deposits.detail.preauth_hint")}
      </p>
    </div>
  );
}

function CancelDialog({
  deposit,
  open,
  onOpenChange,
}: {
  deposit: SecurityDeposit;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const cancel = useCancelDeposit(deposit.uuid);
  return (
    <FinanceFormDialog<{ reason: string }>
      open={open}
      onOpenChange={(next) => {
        if (!next) setProblem(null);
        onOpenChange(next);
      }}
      formId="deposit-cancel-form"
      title={t("deposits.cancel.title")}
      description={t("deposits.cancel.description")}
      submitLabel={t("deposits.actions.cancel")}
      destructive
      pending={cancel.isPending}
      problem={problem}
      schema={z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("finance.validation.reason"))
          .max(500, t("finance.validation.reason")),
      })}
      defaultValues={{ reason: "" }}
      onSubmit={async (v) => {
        try {
          await cancel.mutateAsync(v.reason.trim());
          onOpenChange(false);
        } catch (error) {
          setProblem(errorText(error, t("deposits.errors.save_failed")));
        }
      }}
    >
      <AppTextarea name="reason" label={t("deposits.fields.reason")} />
    </FinanceFormDialog>
  );
}

function DocumentDialog({
  deposit,
  deduction,
  onOpenChange,
}: {
  deposit: SecurityDeposit;
  deduction: DepositDeduction | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const upload = useUploadDeductionDocument(deposit.uuid);
  return (
    <AppDialog
      open={Boolean(deduction)}
      onOpenChange={onOpenChange}
      title={t("deposits.document.title")}
      description={t("deposits.document.description")}
      className="sm:max-w-lg"
    >
      <div className="space-y-2">
        <FileUploader
          multiple={false}
          accept="application/pdf,image/jpeg,image/png,image/webp"
          maxSizeBytes={10 * 1024 * 1024}
          onUpload={async (files) => {
            const file = files[0];
            if (!file || !deduction) return;
            try {
              await upload.mutateAsync({ deduction: deduction.uuid, file });
              onOpenChange(false);
            } catch (error) {
              appToast.error(
                errorText(error, t("deposits.errors.save_failed")),
              );
            }
          }}
        />
        <p className="text-muted-foreground text-xs">
          {t("deposits.document.hint")}
        </p>
      </div>
    </AppDialog>
  );
}

/** Deductions of a deposit: a bounded client-side DataTable. */
function DeductionsTable({
  deposit,
  onDocument,
}: {
  deposit: SecurityDeposit;
  onDocument: (d: DepositDeduction) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const remove = useRemoveDeductionDocument(deposit.uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<DepositDeduction>({
          accessorKey: "amount",
          labelKey: "deposits.columns.amount",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.amount}
              currency={deposit.currency}
              className="font-semibold"
            />
          ),
        }),
        createColumn<DepositDeduction>({
          accessorKey: "reason",
          labelKey: "deposits.columns.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="max-w-[18rem] space-y-0.5">
              <div className="truncate">{row.original.reason}</div>
              <div className="text-muted-foreground text-xs">
                {t(DEDUCTION_TARGET_LABEL_KEYS[row.original.applies_to])}
              </div>
            </div>
          ),
        }),
        createColumn<DepositDeduction>({
          id: "link",
          accessorFn: (row) => row.charge?.code ?? "",
          labelKey: "deposits.columns.link",
          enableSorting: false,
          cell: ({ row }) => {
            const c = row.original.charge;
            const d = row.original.damage;
            if (!c && !d)
              return <span className="text-muted-foreground">—</span>;
            return (
              <div className="text-xs">
                {c ? (
                  <div>
                    {t(
                      CHARGE_KIND_LABEL_KEYS[
                        c.code as keyof typeof CHARGE_KIND_LABEL_KEYS
                      ] ??
                        c.code ??
                        "",
                    )}
                  </div>
                ) : null}
                {d ? (
                  <div className="text-muted-foreground">
                    {t("deposits.detail.damage_link")}
                  </div>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<DepositDeduction>({
          id: "document",
          accessorFn: (row) => row.document?.name ?? "",
          labelKey: "deposits.columns.document",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.document ? (
              <AttachmentBadge name={row.original.document.name} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<DepositDeduction>({
          accessorKey: "created_at",
          labelKey: "deposits.columns.created_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "dd.MM.yyyy HH:mm", locale),
        }),
        createColumn<DepositDeduction>({
          id: "actions",
          labelKey: "deposits.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const x = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "upload",
                    label: t("deposits.actions.attach_document"),
                    icon: FileUp,
                    permission: permissions.tenant.finance.deposits.deduct,
                    onSelect: () => onDocument(x),
                  },
                  ...(x.document
                    ? [
                        {
                          id: "download",
                          label: t("deposits.actions.download"),
                          icon: Download,
                          onSelect: async () => {
                            const out = await depositsService.downloadDocument(
                              slug,
                              deposit.uuid,
                              x.uuid,
                            );
                            download(
                              out.blob,
                              out.filename ?? x.document?.name ?? "belge",
                            );
                          },
                        },
                        {
                          id: "remove",
                          label: t("deposits.actions.remove_document"),
                          icon: Trash2,
                          variant: "destructive" as const,
                          permission:
                            permissions.tenant.finance.deposits.deduct,
                          onSelect: () => remove.mutate(x.uuid),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<DepositDeduction, unknown>[],
    [deposit.currency, deposit.uuid, locale, onDocument, remove, slug, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={deposit.deductions}
      getRowId={(row) => row.uuid}
      emptyTitle={t("deposits.detail.no_deductions")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-deposit-deductions-v1",
        rowSelection: false,
      }}
    />
  );
}

/**
 * One deposit: its figures, the online pre-auth page while pending, the
 * resolution actions (release / deduct), cancel, and the deductions with
 * their documents.
 */
export function DepositDetailDrawer({
  uuid,
  onOpenChange,
  autoResolve = false,
}: {
  uuid: string | null;
  onOpenChange: (open: boolean) => void;
  /** Open the resolution dialog right away (deposit task link). */
  autoResolve?: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useDeposit(uuid);
  const verify = useVerifyDeposit();
  const [resolve, setResolve] = useState<"release" | "deduct" | null>(null);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [document, setDocument] = useState<DepositDeduction | null>(null);
  const [autoOpened, setAutoOpened] = useState<string | null>(null);
  const d = query.data;
  if (
    autoResolve &&
    d &&
    autoOpened !== d.uuid &&
    d.capabilities.release &&
    resolve === null
  ) {
    setAutoOpened(d.uuid);
    setResolve(d.capabilities.deduct ? "deduct" : "release");
  }
  const cashHeld = d?.method === "cash" || d?.method === "bank_transfer";
  return (
    <EntityDrawer
      open={Boolean(uuid)}
      onOpenChange={onOpenChange}
      title={t("deposits.detail.title")}
      description={
        d ? `${d.rental.code ?? ""} · ${d.customer.name ?? ""}` : undefined
      }
      size="lg"
    >
      {query.isLoading || !d ? (
        <Loading label={t("common.loading")} />
      ) : (
        <div className="space-y-5">
          <div className="flex flex-wrap items-center gap-2">
            <DepositStatusChip status={d.status} />
            <DepositMethodBadge method={d.method} />
            {d.status === "held" && d.held_days > 0 ? (
              <span className="text-muted-foreground text-xs">
                {t("deposits.detail.held_days", { days: d.held_days })}
              </span>
            ) : null}
          </div>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <Fact label={t("deposits.fields.amount")}>
              <Amount
                value={d.amount}
                currency={d.currency}
                className="font-semibold"
              />
            </Fact>
            <Fact label={t("deposits.columns.captured")}>
              <Amount value={d.captured_amount} currency={d.currency} />
            </Fact>
            <Fact
              label={t(
                d.status === "held" || d.status === "pending"
                  ? "deposits.columns.open"
                  : cashHeld
                    ? "deposits.resolve.paid_back"
                    : "deposits.resolve.released",
              )}
            >
              <Amount
                value={
                  d.status === "held" || d.status === "pending"
                    ? d.open_amount
                    : d.returned_amount
                }
                currency={d.currency}
              />
            </Fact>
            <Fact label={t("deposits.columns.branch")}>{d.branch.name}</Fact>
            <Fact label={t("deposits.columns.held_at")}>
              {d.held_at
                ? datetime(d.held_at, "dd.MM.yyyy HH:mm", locale)
                : "—"}
            </Fact>
            {d.cash_account ? (
              <Fact label={t("deposits.columns.cash_account")}>
                {d.cash_account.code} · {d.cash_account.name}
              </Fact>
            ) : null}
            {d.pos_reference ? (
              <Fact label={t("deposits.fields.pos_reference")}>
                <span className="font-mono">{d.pos_reference}</span>
              </Fact>
            ) : null}
            {d.provider_payment_id ? (
              <Fact label={t("deposits.detail.provider_reference")}>
                <span className="font-mono">{d.provider_payment_id}</span>
              </Fact>
            ) : null}
            {d.authorization_expires_at ? (
              <Fact label={t("deposits.fields.hold_until")}>
                {datetime(
                  d.authorization_expires_at,
                  "dd.MM.yyyy HH:mm",
                  locale,
                )}
              </Fact>
            ) : null}
            {d.resolved_at ? (
              <Fact label={t("deposits.columns.resolved_at")}>
                {datetime(d.resolved_at, "dd.MM.yyyy HH:mm", locale)}
              </Fact>
            ) : null}
          </dl>
          {d.note || d.resolution_note || d.cancel_reason ? (
            <p className="text-muted-foreground text-sm whitespace-pre-line">
              {[
                d.note,
                d.resolution_note,
                CANCEL_REASON_KEYS[d.cancel_reason]
                  ? t(CANCEL_REASON_KEYS[d.cancel_reason])
                  : d.cancel_reason,
              ]
                .filter(Boolean)
                .join("\n")}
            </p>
          ) : null}
          {d.status === "pending" && d.payment_page_url ? (
            <PreAuthLink url={d.payment_page_url} />
          ) : null}
          {d.customer_notice_status === "pending" ? (
            <Alert>
              <AlertDescription>
                {t("deposits.detail.notice_pending")}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {d.capabilities.verify ? (
              <Button
                type="button"
                size="sm"
                disabled={verify.isPending}
                onClick={async () => {
                  try {
                    const out = await verify.mutateAsync(d.uuid);
                    appToast.success(
                      t(
                        out.status === "held"
                          ? "deposits.toast.verified"
                          : out.status === "cancelled"
                            ? "deposits.toast.declined"
                            : "deposits.toast.still_pending",
                      ),
                    );
                  } catch (error) {
                    appToast.error(
                      errorText(error, t("deposits.errors.verify_failed")),
                    );
                  }
                }}
              >
                <RefreshCw className="size-4" />
                {t("deposits.actions.verify")}
              </Button>
            ) : null}
            {d.capabilities.release ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setResolve("release")}
              >
                <Undo2 className="size-4" />
                {t(
                  cashHeld
                    ? "deposits.actions.refund_full"
                    : "deposits.actions.release_full",
                )}
              </Button>
            ) : null}
            {d.capabilities.deduct ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setResolve("deduct")}
              >
                <Scissors className="size-4" />
                {t("deposits.actions.deduct")}
              </Button>
            ) : null}
            {d.capabilities.cancel ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="text-destructive"
                onClick={() => setCancelOpen(true)}
              >
                <XCircle className="size-4" />
                {t("deposits.actions.cancel")}
              </Button>
            ) : null}
            <Button asChild size="sm" variant="ghost">
              <Link href={routes.tenant.rentals.detail(slug, d.rental.uuid)}>
                <ExternalLink className="size-4" />
                {t("deposits.actions.open_rental")}
              </Link>
            </Button>
          </div>
          <Separator />
          <div className="space-y-2">
            <h3 className="text-sm font-semibold">
              {t("deposits.detail.deductions")}
            </h3>
            <DeductionsTable deposit={d} onDocument={setDocument} />
          </div>
          {resolve ? (
            <ResolveDepositDialog
              deposit={d}
              open
              initialMode={resolve}
              onOpenChange={(next) => {
                if (!next) setResolve(null);
              }}
            />
          ) : null}
          <CancelDialog
            deposit={d}
            open={cancelOpen}
            onOpenChange={setCancelOpen}
          />
          <DocumentDialog
            deposit={d}
            deduction={document}
            onOpenChange={(next) => {
              if (!next) setDocument(null);
            }}
          />
        </div>
      )}
    </EntityDrawer>
  );
}
