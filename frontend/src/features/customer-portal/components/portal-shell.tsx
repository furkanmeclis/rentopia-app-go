"use client";

import { LogOut } from "lucide-react";
import type { ReactNode } from "react";

import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Frame of the customer portal pages: the organization's name (the branded
 * site of M5 replaces this header), language and theme switches, and the
 * sign-out button of a signed-in customer. Mobile first (375 px).
 */
export function PortalShell({
  organizationName,
  children,
  onSignOut,
  signingOut,
}: {
  organizationName?: string;
  children: ReactNode;
  onSignOut?: () => void;
  signingOut?: boolean;
}) {
  const { t } = useLocale();
  return (
    <div className="bg-muted min-h-svh">
      <header className="bg-background/95 sticky top-0 z-10 border-b backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-3xl items-center justify-between gap-2 px-4">
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold">
              {organizationName ?? "…"}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("customer_portal.title")}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <LocaleSwitch />
            <ThemeSwitch />
            {onSignOut ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={onSignOut}
                disabled={signingOut}
                aria-label={t("customer_portal.sign_out")}
              >
                <LogOut className="size-4" />
                <span className="hidden sm:inline">
                  {t("customer_portal.sign_out")}
                </span>
              </Button>
            ) : null}
          </div>
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-3xl flex-col gap-4 px-4 py-6">
        {children}
      </main>
    </div>
  );
}
