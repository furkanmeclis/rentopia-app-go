"use client";

import { useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { Loading } from "@/components/common/loading";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PortalShell } from "@/features/customer-portal/components/portal-shell";
import { PortalSignIn } from "@/features/customer-portal/components/portal-sign-in";
import { PortalTable } from "@/features/customer-portal/components/portal-table";
import { portalRoutes } from "@/features/customer-portal/constants";
import {
  isSignedOut,
  portalKeys,
  usePortalInfo,
  usePortalReservations,
  usePortalSession,
  usePortalSignOut,
} from "@/features/customer-portal/hooks/use-portal";
import type { PortalReservationSummary } from "@/features/customer-portal/types";
import { moneyText } from "@/features/pricing/lib/format";
import {
  PaymentStatusChip,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-cells";
import type {
  PaymentStatus,
  ReservationStatus,
} from "@/features/reservations/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function ReservationList({ orgSlug }: { orgSlug: string }) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const query = usePortalReservations(orgSlug, true);
  const columns = useMemo(
    () =>
      [
        createColumn<PortalReservationSummary>({
          accessorKey: "reference",
          labelKey: "customer_portal.list.reference",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-sm font-medium">
              {row.original.reference}
            </span>
          ),
        }),
        createColumn<PortalReservationSummary>({
          accessorKey: "status",
          labelKey: "customer_portal.list.status",
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              <ReservationStatusChip
                status={row.original.status as ReservationStatus}
              />
              <PaymentStatusChip
                status={row.original.payment_status as PaymentStatus}
              />
            </div>
          ),
        }),
        createColumn<PortalReservationSummary>({
          accessorKey: "starts_at",
          labelKey: "customer_portal.list.dates",
          gridSecondary: true,
          cell: ({ row }) =>
            `${datetime(row.original.starts_at, "d MMM yyyy HH:mm", locale)} – ${datetime(
              row.original.ends_at,
              "d MMM yyyy HH:mm",
              locale,
            )}`,
        }),
        createColumn<PortalReservationSummary>({
          accessorKey: "product",
          labelKey: "customer_portal.list.product",
        }),
        createColumn<PortalReservationSummary>({
          id: "balance",
          accessorFn: (row) => row.price.balance,
          labelKey: "customer_portal.list.balance",
          cell: ({ row }) => moneyText(row.original.price.balance, locale),
        }),
      ] as ColumnDef<PortalReservationSummary, unknown>[],
    [locale],
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">
          {t("customer_portal.list.title")}
        </CardTitle>
        <CardDescription>
          {t("customer_portal.list.description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <PortalTable
          columns={columns}
          data={query.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={query.isLoading}
          emptyTitle={t("customer_portal.list.empty")}
          onRowClick={(row) =>
            router.push(portalRoutes.reservation(orgSlug, row.uuid))
          }
        />
      </CardContent>
    </Card>
  );
}

/**
 * `/r/{orgSlug}`: the sign-in form, or the signed-in customer's
 * reservations in this organization.
 */
export function PortalHome({ orgSlug }: { orgSlug: string }) {
  const { t } = useLocale();
  const client = useQueryClient();
  const info = usePortalInfo(orgSlug);
  const session = usePortalSession(orgSlug);
  const signOut = usePortalSignOut(orgSlug);

  let body: React.ReactNode;
  if (info.isError) {
    body = (
      <Alert variant="destructive">
        <AlertTitle>{t("customer_portal.not_found_title")}</AlertTitle>
        <AlertDescription>
          {t("customer_portal.not_found_body")}
        </AlertDescription>
      </Alert>
    );
  } else if (session.isLoading || info.isLoading) {
    body = <Loading />;
  } else if (session.data) {
    body = (
      <>
        <div>
          <h1 className="text-xl font-semibold">
            {t("customer_portal.greeting", {
              name: session.data.display_name,
            })}
          </h1>
          <p className="text-muted-foreground text-sm">
            {t(
              session.data.role === "driver"
                ? "customer_portal.roles_hint.driver"
                : "customer_portal.roles_hint.renter",
            )}
          </p>
        </div>
        <ReservationList orgSlug={orgSlug} />
      </>
    );
  } else if (session.isError && !isSignedOut(session.error)) {
    body = (
      <Alert variant="destructive">
        <AlertDescription>{t("customer_portal.load_failed")}</AlertDescription>
      </Alert>
    );
  } else {
    body = (
      <PortalSignIn
        orgSlug={orgSlug}
        info={info.data}
        onSignedIn={() =>
          void client.invalidateQueries({ queryKey: portalKeys.all(orgSlug) })
        }
      />
    );
  }

  return (
    <PortalShell
      organizationName={info.data?.organization.name}
      onSignOut={session.data ? () => signOut.mutate() : undefined}
      signingOut={signOut.isPending}
    >
      {body}
    </PortalShell>
  );
}
