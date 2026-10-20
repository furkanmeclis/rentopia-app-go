"use client";

import { Check, Minus } from "lucide-react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const ROLES = [
  "landing.control.role_manager",
  "landing.control.role_field",
  "landing.control.role_accounting",
] as const;

const PERMISSIONS: { key: string; allowed: [boolean, boolean, boolean] }[] = [
  { key: "landing.control.perm_booking", allowed: [true, false, false] },
  { key: "landing.control.perm_handover", allowed: [true, true, false] },
  { key: "landing.control.perm_deposit", allowed: [true, false, true] },
  { key: "landing.control.perm_invoice", allowed: [false, false, true] },
];

/** Three roles against four actions. Decorative; the caption describes it. */
export function PermissionsMock({ className }: { className?: string }) {
  const { t } = useLocale();

  return (
    <div
      aria-hidden
      className={cn(
        "bg-card text-card-foreground w-full rounded-2xl border p-5 shadow-[0_1px_0_rgba(20,21,39,0.04),0_24px_48px_-24px_rgba(20,21,39,0.25)] sm:p-6",
        className,
      )}
    >
      <p className="font-semibold">{t("landing.control.mock_title")}</p>
      <div className="mt-4 grid grid-cols-[minmax(0,1.6fr)_repeat(3,minmax(0,1fr))] text-xs sm:text-sm">
        <span />
        {ROLES.map((role) => (
          <span
            key={role}
            className="text-muted-foreground px-1 pb-3 text-center text-[11px] leading-tight font-medium sm:text-xs"
          >
            {t(role)}
          </span>
        ))}
        {PERMISSIONS.map((perm) => (
          <div key={perm.key} className="contents">
            <span className="border-t py-3 pr-2 font-medium">
              {t(perm.key)}
            </span>
            {perm.allowed.map((allowed, i) => (
              <span key={i} className="grid place-items-center border-t py-3">
                <span
                  className={cn(
                    "grid size-6 place-items-center rounded-md",
                    allowed
                      ? "bg-[#ECEDF8] text-[#2E358C] dark:bg-[#22264A] dark:text-[#C7CAF6]"
                      : "text-muted-foreground/60",
                  )}
                >
                  {allowed ? (
                    <Check className="size-3.5" strokeWidth={3} />
                  ) : (
                    <Minus className="size-3.5" />
                  )}
                </span>
              </span>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
