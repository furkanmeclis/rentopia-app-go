"use client";

import { ShieldCheck } from "lucide-react";

import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useLocale } from "@/providers/locale-provider";

/** Neutral page after the provider's pre-authorization form. */
export function PreAuthDonePage() {
  const { t } = useLocale();
  return (
    <main className="bg-background flex min-h-dvh items-center justify-center px-4 py-10">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <ShieldCheck className="text-primary mb-2 size-10" />
          <CardTitle>{t("deposits.preauth_done.title")}</CardTitle>
          <CardDescription>
            {t("deposits.preauth_done.description")}
          </CardDescription>
        </CardHeader>
      </Card>
    </main>
  );
}
