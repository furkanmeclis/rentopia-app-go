"use client";

import { useMutation } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  ArrowLeft,
  CreditCard,
  Download,
  FileSignature,
  Loader2,
  PencilLine,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { KIND_LABEL_KEYS } from "@/features/contracts/constants";
import { PortalRequestDialog } from "@/features/customer-portal/components/portal-request-dialog";
import { PortalShell } from "@/features/customer-portal/components/portal-shell";
import { PortalTable } from "@/features/customer-portal/components/portal-table";
import {
  REQUEST_STATUS_LABEL_KEYS,
  REQUEST_STATUS_TONES,
  REQUEST_TYPE_LABEL_KEYS,
  portalRoutes,
} from "@/features/customer-portal/constants";
import {
  isSignedOut,
  usePortalInfo,
  usePortalReservation,
  usePortalSignOut,
} from "@/features/customer-portal/hooks/use-portal";
import { portalService } from "@/features/customer-portal/services/portal.service";
import type {
  CustomerRequestType,
  PortalDocument,
  PortalInvoice,
  PortalRequest,
  PortalReservation,
} from "@/features/customer-portal/types";
import { moneyText } from "@/features/pricing/lib/format";
import {
  PaymentStatusChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-cells";
import type {
  PaymentStatus,
  ReservationStatus,
} from "@/features/reservations/types";
import { ApiError } from "@/lib/api";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 text-sm">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="text-end font-medium break-words">{value}</span>
    </div>
  );
}

function SummaryCard({ r }: { r: PortalReservation }) {
  const { t, locale } = useLocale();
  const place = (branch: string, point: string) =>
    point ? `${branch} · ${point}` : branch;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 text-lg">
          <span className="font-mono">{r.reference}</span>
          <ReservationStatusChip status={r.status as ReservationStatus} />
        </CardTitle>
        <CardDescription>{r.product}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Row
          label={t("customer_portal.detail.pickup")}
          value={
            <>
              {datetime(r.starts_at, "d MMM yyyy HH:mm", locale)}
              <br />
              <span className="text-muted-foreground font-normal">
                {place(r.pickup_branch, r.pickup_point)}
              </span>
            </>
          }
        />
        <Row
          label={t("customer_portal.detail.return")}
          value={
            <>
              {datetime(r.ends_at, "d MMM yyyy HH:mm", locale)}
              <br />
              <span className="text-muted-foreground font-normal">
                {place(r.return_branch, r.return_point)}
              </span>
            </>
          }
        />
        <Row
          label={t("customer_portal.detail.vehicle")}
          value={r.vehicle || t("customer_portal.detail.vehicle_pending")}
        />
        {r.drivers.length > 0 ? (
          <Row
            label={t("customer_portal.detail.drivers")}
            value={r.drivers.join(", ")}
          />
        ) : null}
      </CardContent>
    </Card>
  );
}

function ExtrasCard({ r }: { r: PortalReservation }) {
  const { t, locale } = useLocale();
  type Extra = PortalReservation["extras"][number];
  const columns = useMemo(
    () =>
      [
        createColumn<Extra>({
          accessorKey: "name",
          labelKey: "customer_portal.detail.extra",
          gridPrimary: true,
        }),
        createColumn<Extra>({
          accessorKey: "quantity",
          labelKey: "customer_portal.detail.quantity",
        }),
        createColumn<Extra>({
          accessorKey: "amount",
          labelKey: "customer_portal.detail.amount",
          cell: ({ row }) => moneyText(row.original.amount, locale),
        }),
      ] as ColumnDef<Extra, unknown>[],
    [locale],
  );
  if (r.extras.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          {t("customer_portal.detail.extras")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <PortalTable
          columns={columns}
          data={r.extras}
          getRowId={(row) => `${row.name}-${row.quantity}`}
        />
      </CardContent>
    </Card>
  );
}

function PaymentCard({
  orgSlug,
  r,
}: {
  orgSlug: string;
  r: PortalReservation;
}) {
  const { t, locale } = useLocale();
  const pay = useMutation({
    mutationFn: () => portalService.pay(orgSlug, r.uuid),
    onSuccess: (out) => {
      window.location.assign(out.url);
    },
  });
  const payErrorKeys: Record<string, string> = {
    PAYMENT_UNAVAILABLE: "customer_portal.payment.errors.payment_unavailable",
    NOTHING_DUE: "customer_portal.payment.errors.nothing_due",
  };
  const payError = pay.error
    ? t(
        (pay.error instanceof ApiError && payErrorKeys[pay.error.code]) ||
          "customer_portal.payment.errors.generic",
      )
    : null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 text-base">
          {t("customer_portal.payment.title")}
          <PaymentStatusChip status={r.payment_status as PaymentStatus} />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <Row
          label={t("customer_portal.payment.total")}
          value={moneyText(r.price.total, locale)}
        />
        <Row
          label={t("customer_portal.payment.paid")}
          value={moneyText(r.price.paid, locale)}
        />
        <Separator />
        <Row
          label={t("customer_portal.payment.balance")}
          value={moneyText(r.price.balance, locale)}
        />
        {r.payment_deadline && r.status === "awaiting_payment" ? (
          <p className="text-muted-foreground text-xs">
            {t("customer_portal.payment.deadline", {
              at: datetime(r.payment_deadline, "d MMM HH:mm", locale),
            })}
          </p>
        ) : null}
        {payError ? (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertDescription>{payError}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
      {r.actions.pay && r.actions.pay_amount ? (
        <CardFooter>
          <Button
            type="button"
            className="w-full"
            disabled={pay.isPending}
            onClick={() => pay.mutate()}
          >
            {pay.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <CreditCard className="size-4" />
            )}
            {t("customer_portal.payment.pay", {
              amount: moneyText(r.actions.pay_amount, locale),
            })}
          </Button>
        </CardFooter>
      ) : null}
    </Card>
  );
}

function DocumentsCard({ r }: { r: PortalReservation }) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<PortalDocument>({
          accessorKey: "kind",
          labelKey: "customer_portal.documents.kind",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {t(KIND_LABEL_KEYS[row.original.kind])} · v
              {row.original.version_number}
            </span>
          ),
        }),
        createColumn<PortalDocument>({
          accessorKey: "status",
          labelKey: "customer_portal.documents.status",
          cell: ({ row }) =>
            row.original.status === "approved" ? (
              <Badge variant="success">
                {t("customer_portal.documents.approved")}
              </Badge>
            ) : row.original.approval_url ? (
              <Badge variant="warning">
                {t("customer_portal.documents.awaiting")}
              </Badge>
            ) : (
              <Badge variant="secondary">
                {t("customer_portal.documents.issued")}
              </Badge>
            ),
        }),
        createColumn<PortalDocument>({
          id: "actions",
          labelKey: "customer_portal.documents.actions",
          enableHiding: false,
          cell: ({ row }) => {
            const d = row.original;
            return (
              <div className="flex flex-wrap gap-2">
                {d.download_path ? (
                  <Button asChild size="sm" variant="outline">
                    <a
                      href={portalService.downloadUrl(d.download_path)}
                      download
                      rel="noreferrer"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Download className="size-4" />
                      {t("customer_portal.documents.download")}
                    </a>
                  </Button>
                ) : null}
                {d.approval_url ? (
                  <Button asChild size="sm">
                    <a href={d.approval_url} rel="noreferrer">
                      <FileSignature className="size-4" />
                      {t("customer_portal.documents.approve")}
                    </a>
                  </Button>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<PortalDocument>({
          accessorKey: "updated_at",
          labelKey: "customer_portal.documents.updated_at",
          cell: ({ row }) =>
            datetime(row.original.updated_at, "d MMM yyyy HH:mm", locale),
        }),
      ] as ColumnDef<PortalDocument, unknown>[],
    [locale, t],
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          {t("customer_portal.documents.title")}
        </CardTitle>
        <CardDescription>
          {t("customer_portal.documents.description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <PortalTable
          columns={columns}
          data={r.documents}
          getRowId={(row) => row.uuid}
          emptyTitle={t("customer_portal.documents.empty")}
        />
      </CardContent>
    </Card>
  );
}

function InvoicesCard({ r }: { r: PortalReservation }) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<PortalInvoice>({
          accessorKey: "number",
          labelKey: "customer_portal.invoices.number",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs font-medium">
              {row.original.number}
            </span>
          ),
        }),
        createColumn<PortalInvoice>({
          accessorKey: "issue_date",
          labelKey: "customer_portal.invoices.issue_date",
          cell: ({ row }) =>
            date(row.original.issue_date, "d MMM yyyy", locale),
        }),
        createColumn<PortalInvoice>({
          accessorKey: "invoice_type",
          labelKey: "customer_portal.invoices.type",
          cell: ({ row }) =>
            row.original.invoice_type === "IADE" ? (
              <Badge variant="secondary">
                {t("customer_portal.invoices.return")}
              </Badge>
            ) : (
              <Badge variant="outline">
                {row.original.profile === "EARSIVFATURA"
                  ? t("customer_portal.invoices.earsiv")
                  : t("customer_portal.invoices.efatura")}
              </Badge>
            ),
        }),
        createColumn<PortalInvoice>({
          accessorKey: "payable_amount",
          labelKey: "customer_portal.invoices.amount",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.payable_amount, locale)}
            </span>
          ),
        }),
        createColumn<PortalInvoice>({
          id: "actions",
          labelKey: "customer_portal.invoices.actions",
          enableHiding: false,
          cell: ({ row }) => (
            <Button asChild size="sm" variant="outline">
              <a
                href={portalService.downloadUrl(row.original.download_path)}
                download
                rel="noreferrer"
                onClick={(e) => e.stopPropagation()}
              >
                <Download className="size-4" />
                {t("customer_portal.invoices.download")}
              </a>
            </Button>
          ),
        }),
      ] as ColumnDef<PortalInvoice, unknown>[],
    [locale, t],
  );
  if (r.invoices.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          {t("customer_portal.invoices.title")}
        </CardTitle>
        <CardDescription>
          {t("customer_portal.invoices.description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <PortalTable
          columns={columns}
          data={r.invoices}
          getRowId={(row) => row.uuid}
          emptyTitle={t("customer_portal.invoices.empty")}
        />
      </CardContent>
    </Card>
  );
}

function RequestsCard({
  r,
  onRequest,
}: {
  r: PortalReservation;
  onRequest: (type: CustomerRequestType) => void;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<PortalRequest>({
          accessorKey: "type",
          labelKey: "customer_portal.requests.type",
          gridPrimary: true,
          cell: ({ row }) => t(REQUEST_TYPE_LABEL_KEYS[row.original.type]),
        }),
        createColumn<PortalRequest>({
          accessorKey: "status",
          labelKey: "customer_portal.requests.status",
          cell: ({ row }) => (
            <StatusChip
              label={t(REQUEST_STATUS_LABEL_KEYS[row.original.status])}
              tone={REQUEST_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<PortalRequest>({
          accessorKey: "resolution_note",
          labelKey: "customer_portal.requests.answer",
          cell: ({ row }) =>
            row.original.outcome === "self_cancelled"
              ? t("customer_portal.requests.self_cancelled")
              : row.original.resolution_note || "—",
        }),
        createColumn<PortalRequest>({
          accessorKey: "created_at",
          labelKey: "customer_portal.requests.created_at",
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
      ] as ColumnDef<PortalRequest, unknown>[],
    [locale, t],
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          {t("customer_portal.requests.title")}
        </CardTitle>
        <CardDescription>
          {t("customer_portal.requests.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <PortalTable
          columns={columns}
          data={r.requests}
          getRowId={(row) => row.uuid}
          emptyTitle={t("customer_portal.requests.empty")}
        />
      </CardContent>
      {r.actions.request_change || r.actions.request_cancel ? (
        <CardFooter className="flex flex-col gap-2 sm:flex-row">
          {r.actions.request_change ? (
            <Button
              type="button"
              variant="outline"
              className="w-full sm:w-auto"
              onClick={() => onRequest("change")}
            >
              <PencilLine className="size-4" />
              {t("customer_portal.requests.request_change")}
            </Button>
          ) : null}
          {r.actions.request_cancel ? (
            <Button
              type="button"
              variant="outline"
              className="text-destructive w-full sm:w-auto"
              onClick={() => onRequest("cancel")}
            >
              <XCircle className="size-4" />
              {t("customer_portal.requests.request_cancel")}
            </Button>
          ) : null}
        </CardFooter>
      ) : null}
    </Card>
  );
}

/** `/r/{orgSlug}/reservations/{uuid}`: one reservation of the customer. */
export function PortalReservationPage({
  orgSlug,
  uuid,
}: {
  orgSlug: string;
  uuid: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const info = usePortalInfo(orgSlug);
  const query = usePortalReservation(orgSlug, uuid);
  const signOut = usePortalSignOut(orgSlug);
  const [request, setRequest] = useState<CustomerRequestType | null>(null);
  const signedOut = query.isError && isSignedOut(query.error);

  useEffect(() => {
    if (signedOut) router.replace(portalRoutes.home(orgSlug));
  }, [orgSlug, router, signedOut]);

  let body: React.ReactNode;
  if (query.isLoading || signedOut) {
    body = <Loading />;
  } else if (query.isError || !query.data) {
    body = (
      <Alert variant="destructive">
        <AlertTriangle className="size-4" />
        <AlertDescription>
          {t("customer_portal.detail.not_found")}
        </AlertDescription>
      </Alert>
    );
  } else {
    const r = query.data;
    body = (
      <>
        <SummaryCard r={r} />
        <PaymentCard orgSlug={orgSlug} r={r} />
        <ExtrasCard r={r} />
        <DocumentsCard r={r} />
        <InvoicesCard r={r} />
        <RequestsCard r={r} onRequest={setRequest} />
        <PortalRequestDialog
          orgSlug={orgSlug}
          reservation={r}
          type={request}
          onOpenChange={(open) => {
            if (!open) setRequest(null);
          }}
        />
      </>
    );
  }

  return (
    <PortalShell
      organizationName={info.data?.organization.name}
      orgSlug={orgSlug}
      onSignOut={() => signOut.mutate()}
      signingOut={signOut.isPending}
    >
      <Button asChild variant="ghost" size="sm" className="self-start">
        <Link href={portalRoutes.home(orgSlug)}>
          <ArrowLeft className="size-4" />
          {t("customer_portal.detail.back")}
        </Link>
      </Button>
      {body}
    </PortalShell>
  );
}
