"use client";

import Link from "next/link";

import { AppLogo } from "@/components/brand";
import { useLocale } from "@/providers/locale-provider";

import {
  LANDING_CONTACT_EMAIL,
  LANDING_LOGIN_HREF,
  demoMailto,
} from "../config";

export function LandingFooter() {
  const { t } = useLocale();
  const year = new Date().getFullYear();

  return (
    <footer className="border-t">
      <div className="mx-auto flex max-w-[78rem] flex-col gap-8 px-4 py-12 sm:px-6 md:flex-row md:items-end md:justify-between lg:px-8">
        <div className="grid gap-3">
          <AppLogo variant="logo" className="h-7 w-auto" />
          <p className="text-muted-foreground text-sm">
            {t("landing.footer.tagline")}
          </p>
        </div>
        <nav aria-label={t("landing.footer.contact")}>
          <ul className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
            <li>
              <Link
                href={LANDING_LOGIN_HREF}
                className="hover:text-primary underline-offset-4 hover:underline"
              >
                {t("landing.cta.login")}
              </Link>
            </li>
            <li>
              <a
                href={demoMailto(t("landing.cta.demo_subject"))}
                className="hover:text-primary underline-offset-4 hover:underline"
              >
                {t("landing.cta.demo")}
              </a>
            </li>
            <li>
              <a
                href={`mailto:${LANDING_CONTACT_EMAIL}`}
                className="hover:text-primary underline-offset-4 hover:underline"
              >
                {LANDING_CONTACT_EMAIL}
              </a>
            </li>
          </ul>
          <p className="text-muted-foreground mt-4 text-xs md:text-right">
            {t("landing.footer.rights", { year })}
          </p>
        </nav>
      </div>
    </footer>
  );
}
