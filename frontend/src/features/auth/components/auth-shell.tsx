"use client";

import { useSyncExternalStore, type ReactNode } from "react";
import Link from "next/link";
import { CalendarCheck, CarFront, Globe } from "lucide-react";

import { AppWordmark } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { AuthBackdrop } from "./auth-backdrop";

const FEATURES = [
  { icon: CarFront, key: "auth.shell.feature_fleet" },
  { icon: CalendarCheck, key: "auth.shell.feature_reservations" },
  { icon: Globe, key: "auth.shell.feature_site" },
] as const;

const DESKTOP = "(min-width: 64rem)";

function subscribeDesktop(onChange: () => void) {
  const query = window.matchMedia(DESKTOP);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

/** Matches Tailwind's `lg`; the server render assumes a phone. */
function useIsDesktop() {
  return useSyncExternalStore(
    subscribeDesktop,
    () => window.matchMedia(DESKTOP).matches,
    () => false,
  );
}

/**
 * Below `lg` the guest pages always use the dark palette, because the frosted
 * card is drawn over the animated ink/indigo backdrop; the theme switch only
 * shows on desktop, where the form sits on the page background.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  const isDesktop = useIsDesktop();

  return (
    <div
      className={cn(
        "bg-background text-foreground grid min-h-svh lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]",
        !isDesktop && "dark",
      )}
    >
      <aside className="relative hidden overflow-hidden text-white lg:flex lg:flex-col lg:justify-between lg:p-12">
        <AuthBackdrop className="absolute inset-0" />
        <div className="pointer-events-none absolute inset-0 bg-linear-to-t from-[#141527]/85 via-[#141527]/20 to-[#141527]/40" />

        <Link
          href={routes.guest.login}
          className="relative w-fit"
          aria-label={t("common.app_product")}
        >
          <AppWordmark tone="dark" className="h-9" />
        </Link>

        <div className="relative max-w-lg space-y-8">
          <div className="space-y-4">
            <p className="text-sm font-medium tracking-[0.2em] text-white/60 uppercase">
              {t("auth.shell.eyebrow")}
            </p>
            <h2 className="text-4xl leading-tight font-semibold tracking-tight text-balance xl:text-5xl">
              {t("auth.shell.headline")}
            </h2>
            <p className="max-w-md text-base text-pretty text-white/70">
              {t("auth.shell.subline")}
            </p>
          </div>
          <ul className="space-y-3">
            {FEATURES.map(({ icon: Icon, key }) => (
              <li key={key} className="flex items-center gap-3 text-white/85">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white/10 ring-1 ring-white/15 backdrop-blur-sm">
                  <Icon className="size-4" aria-hidden />
                </span>
                <span className="text-sm">{t(key)}</span>
              </li>
            ))}
          </ul>
        </div>

        <p className="relative text-xs text-white/50">
          {t("auth.shell.footer", { year: new Date().getFullYear() })}
        </p>
      </aside>

      <main className="relative flex min-h-svh flex-col items-center justify-center overflow-hidden p-6 md:p-10">
        <AuthBackdrop className="absolute inset-0 lg:hidden" />
        <div className="absolute top-4 right-4 z-10 flex items-center gap-1 rounded-full bg-black/30 p-0.5 backdrop-blur lg:bg-transparent lg:backdrop-blur-none">
          <LocaleSwitch />
          {isDesktop ? <ThemeSwitch /> : null}
        </div>
        <div className="relative flex w-full max-w-sm flex-col gap-6">
          <Link
            href={routes.guest.login}
            className="self-center lg:hidden"
            aria-label={t("common.app_product")}
          >
            <AppWordmark tone="dark" />
          </Link>
          <div className="auth-glass lg:[&_[data-slot=card]]:border-0 lg:[&_[data-slot=card]]:bg-transparent lg:[&_[data-slot=card]]:shadow-none">
            {children}
          </div>
        </div>
      </main>
    </div>
  );
}
