"use client";

import { Check, Copy, ExternalLink } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { moneyText } from "@/features/pricing/lib/format";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** A created pay-by-link link: URL with copy / open, amount and expiry. */
export function PaymentLinkBox({
  url,
  amount,
  expiresAt,
}: {
  url: string;
  amount: string;
  expiresAt: string;
}) {
  const { t, locale } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-2 rounded-md border p-3">
      <p className="text-sm font-medium">{t("payments.links.created_title")}</p>
      <div className="flex gap-2">
        <Input readOnly value={url} aria-label={t("payments.links.url")} />
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("payments.links.copy")}
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
          aria-label={t("payments.links.open")}
          onClick={() => window.open(url, "_blank", "noopener,noreferrer")}
        >
          <ExternalLink className="size-4" />
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        {t("payments.links.summary", {
          amount: moneyText(amount, locale),
          expires: datetime(expiresAt, "d MMM yyyy HH:mm", locale),
        })}
      </p>
    </div>
  );
}
