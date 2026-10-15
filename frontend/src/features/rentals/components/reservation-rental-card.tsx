"use client";

import { KeyRound } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { RentalStatusChip } from "@/features/rentals/components/rental-cells";
import {
  useCreateRental,
  useRentalsList,
} from "@/features/rentals/hooks/use-rentals";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Rental of a reservation on its detail page: a link to the live (or
 * last) rental, or "Kiralama başlat" for a confirmed reservation (the
 * plate is fixed when the rental is created).
 */
export function ReservationRentalCard({
  reservationUuid,
  reservationStatus,
  plateLabel,
}: {
  reservationUuid: string;
  reservationStatus: string;
  plateLabel: string | null;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const readable = can(permissions.tenant.rentals.read);
  const query = useRentalsList(
    { limit: 5, offset: 0, sort: "-created_at", reservation: reservationUuid },
    readable,
  );
  const create = useCreateRental();
  const [confirming, setConfirming] = useState(false);
  if (!readable) return null;
  const items = query.data?.items ?? [];
  const rental = items.find((r) => r.status !== "cancelled") ?? items[0];
  const canStart =
    !items.some((r) => r.status !== "cancelled") &&
    reservationStatus === "confirmed" &&
    can(permissions.tenant.rentals.write);

  return (
    <EntitySectionCard
      title={t("rentals.reservation_card.title")}
      actions={
        canStart ? (
          <Button
            type="button"
            size="sm"
            onClick={() => setConfirming(true)}
            disabled={create.isPending}
          >
            <KeyRound className="size-4" />
            {t("rentals.actions.start")}
          </Button>
        ) : null
      }
    >
      {rental ? (
        <EntityDetail
          sections={[
            {
              id: "rental",
              fields: [
                {
                  key: "reference",
                  label: t("rentals.columns.reference"),
                  value: (
                    <Link
                      className="text-primary font-medium hover:underline"
                      href={routes.tenant.rentals.detail(slug, rental.uuid)}
                    >
                      {rental.reference}
                    </Link>
                  ),
                },
                {
                  key: "status",
                  label: t("rentals.columns.status"),
                  value: <RentalStatusChip status={rental.status} />,
                },
                {
                  key: "vehicle",
                  label: t("rentals.columns.plate"),
                  value: rental.vehicle.plate,
                },
                {
                  key: "handover",
                  label: t("rentals.columns.actual_pickup_at"),
                  value: rental.actual_pickup_at
                    ? datetime(rental.actual_pickup_at, "d MMM HH:mm", locale)
                    : "—",
                },
              ],
            },
          ]}
        />
      ) : (
        <p className="text-muted-foreground text-sm">
          {reservationStatus === "confirmed"
            ? t("rentals.reservation_card.ready")
            : t("rentals.reservation_card.not_ready")}
        </p>
      )}
      <ConfirmDialog
        open={confirming}
        title={t("rentals.create.title")}
        description={t("rentals.create.description", {
          plate: plateLabel ?? "—",
        })}
        confirmLabel={t("rentals.actions.start")}
        isPending={create.isPending}
        onCancel={() => setConfirming(false)}
        onConfirm={() =>
          create.mutate(
            { reservation_uuid: reservationUuid },
            {
              onSuccess: (created) => {
                setConfirming(false);
                router.push(routes.tenant.rentals.detail(slug, created.uuid));
              },
            },
          )
        }
      />
    </EntitySectionCard>
  );
}
