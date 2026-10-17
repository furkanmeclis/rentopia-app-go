"use client";

import Image from "next/image";

import { ErrorState } from "@/components/common/error-state";
import { Skeleton } from "@/components/ui/skeleton";
import { problemText } from "@/features/notification-engine/lib/format";
import { useWhatsAppQR } from "@/features/whatsapp/hooks/use-whatsapp";
import { useLocale } from "@/providers/locale-provider";

/**
 * The pairing QR code (PNG data URL from the API), refreshed every three
 * seconds until the phone is linked.
 */
export function QRPanel() {
  const { t } = useLocale();
  const qr = useWhatsAppQR(true);
  return (
    <div className="flex flex-col items-center gap-4 rounded-lg border p-4 sm:flex-row sm:items-start">
      <div className="flex size-56 shrink-0 items-center justify-center rounded-md border bg-white p-2">
        {qr.data?.qr_code ? (
          <Image
            src={qr.data.qr_code}
            alt={t("whatsapp.qr.alt")}
            width={208}
            height={208}
            unoptimized
          />
        ) : qr.isError ? (
          <ErrorState
            title={t("whatsapp.qr.failed")}
            description={problemText(qr.error, t("whatsapp.errors.qr"))}
          />
        ) : (
          <Skeleton className="size-52" />
        )}
      </div>
      <div className="space-y-2 text-sm">
        <p className="font-medium">{t("whatsapp.qr.title")}</p>
        <ol className="text-muted-foreground list-decimal space-y-1 pl-4">
          <li>{t("whatsapp.qr.step_open")}</li>
          <li>{t("whatsapp.qr.step_devices")}</li>
          <li>{t("whatsapp.qr.step_scan")}</li>
        </ol>
        <p className="text-muted-foreground text-xs">
          {t("whatsapp.qr.refresh_hint")}
        </p>
      </div>
    </div>
  );
}
