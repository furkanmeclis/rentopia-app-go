"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Menu } from "lucide-react";
import {
  motion,
  useMotionValueEvent,
  useReducedMotion,
  useScroll,
} from "motion/react";

import { AppWordmark } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { LANDING_LOGIN_HREF, demoMailto } from "../config";

export const LANDING_SECTIONS = {
  product: "product",
  site: "booking-site",
  control: "control",
  faq: "faq",
} as const;

const NAV = [
  { id: LANDING_SECTIONS.product, key: "landing.nav.product" },
  { id: LANDING_SECTIONS.site, key: "landing.nav.site" },
  { id: LANDING_SECTIONS.control, key: "landing.nav.control" },
  { id: LANDING_SECTIONS.faq, key: "landing.nav.faq" },
] as const;

const SECTION_IDS = NAV.map((item) => item.id);

/** Id of the section crossing the middle band of the viewport, if any. */
function useActiveSection(ids: readonly string[]) {
  const [active, setActive] = useState<string | null>(null);

  useEffect(() => {
    const visible = new Set<string>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target.id);
          else visible.delete(entry.target.id);
        }
        setActive(ids.find((id) => visible.has(id)) ?? null);
      },
      { rootMargin: "-45% 0px -50% 0px" },
    );
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, [ids]);

  return active;
}

const EASE = [0.22, 1, 0.36, 1] as const;

/**
 * Floating frosted bar. Wide and roomy over the hero, it condenses into a
 * tighter pill once the page scrolls; the pill behind the links follows the
 * section in view. Always dark, so it reads in both colour modes.
 */
export function LandingHeader() {
  const { t } = useLocale();
  const reduceMotion = useReducedMotion();
  const { scrollY } = useScroll();
  const [condensed, setCondensed] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const active = useActiveSection(SECTION_IDS);

  useMotionValueEvent(scrollY, "change", (y) => setCondensed(y > 24));

  const demoHref = demoMailto(t("landing.cta.demo_subject"));

  return (
    <header className="dark pointer-events-none fixed inset-x-0 top-0 z-50 px-3 pt-3 sm:px-4 sm:pt-4">
      <a
        href="#main"
        className="focus-visible:ring-ring pointer-events-auto sr-only rounded-md bg-white px-3 py-2 text-sm font-medium text-[#141527] focus:not-sr-only focus:absolute focus:top-4 focus:left-4 focus:z-50 focus-visible:ring-2"
      >
        {t("landing.skip")}
      </a>
      <motion.div
        initial={reduceMotion ? false : { y: -24, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ duration: 0.6, ease: EASE }}
        className={cn(
          "pointer-events-auto mx-auto flex items-center gap-4 rounded-2xl border text-white transition-[max-width,height,background-color,border-color,box-shadow] duration-500 ease-[cubic-bezier(0.22,1,0.36,1)]",
          condensed
            ? "h-14 max-w-[62rem] border-white/12 bg-[#141527]/80 px-3 shadow-[0_16px_40px_-16px_rgba(10,11,30,0.6)] backdrop-blur-xl sm:px-4"
            : "h-16 max-w-[78rem] border-transparent bg-transparent px-2 sm:px-4",
        )}
      >
        <Link
          href="/"
          className="focus-visible:ring-ring/60 shrink-0 rounded-md outline-none focus-visible:ring-[3px]"
          aria-label={t("landing.nav.home")}
        >
          <AppWordmark
            tone="dark"
            className={cn(
              "w-auto transition-[height] duration-500",
              condensed ? "h-6" : "h-7",
            )}
          />
        </Link>

        <nav
          aria-label={t("landing.nav.label")}
          className="mx-auto hidden md:block"
        >
          <ul className="flex items-center gap-0.5 text-sm">
            {NAV.map((item) => {
              const isActive = active === item.id;
              return (
                <li key={item.id} className="relative">
                  {isActive ? (
                    <motion.span
                      layoutId="landing-nav-active"
                      aria-hidden
                      className="absolute inset-0 rounded-full bg-white/10 ring-1 ring-white/10"
                      transition={{
                        type: "spring",
                        stiffness: 420,
                        damping: 36,
                      }}
                    />
                  ) : null}
                  <a
                    href={`#${item.id}`}
                    aria-current={isActive ? "location" : undefined}
                    className={cn(
                      "focus-visible:ring-ring/60 relative block rounded-full px-3.5 py-1.5 transition-colors outline-none focus-visible:ring-[3px]",
                      isActive
                        ? "text-white"
                        : "text-white/65 hover:text-white",
                    )}
                  >
                    {t(item.key)}
                  </a>
                </li>
              );
            })}
          </ul>
        </nav>

        <div className="ms-auto flex items-center gap-1.5 md:ms-0">
          <LocaleSwitch />
          <Button
            asChild
            variant="ghost"
            size="sm"
            className="hidden rounded-full text-white/80 hover:bg-white/10 hover:text-white sm:inline-flex"
          >
            <Link href={LANDING_LOGIN_HREF}>{t("landing.cta.login")}</Link>
          </Button>
          <Button
            asChild
            size="sm"
            className="hidden rounded-full px-4 sm:inline-flex"
          >
            <a href={demoHref}>{t("landing.cta.demo")}</a>
          </Button>

          <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
            <SheetTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="rounded-full text-white hover:bg-white/10 hover:text-white md:hidden"
              >
                <Menu className="size-5" />
                <span className="sr-only">{t("landing.nav.menu")}</span>
              </Button>
            </SheetTrigger>
            <SheetContent
              side="right"
              className="dark w-[min(22rem,88vw)] border-white/10 bg-[#141527] p-6 text-white"
            >
              <SheetTitle className="sr-only">
                {t("landing.nav.menu_title")}
              </SheetTitle>
              <SheetDescription className="sr-only">
                {t("landing.nav.label")}
              </SheetDescription>
              <AppWordmark tone="dark" className="h-7 w-auto" />
              <nav aria-label={t("landing.nav.label")} className="mt-10">
                <ul className="grid gap-1">
                  {NAV.map((item, index) => (
                    <motion.li
                      key={item.id}
                      initial={reduceMotion ? false : { opacity: 0, x: 24 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{
                        delay: 0.08 + index * 0.05,
                        duration: 0.4,
                        ease: EASE,
                      }}
                    >
                      <a
                        href={`#${item.id}`}
                        onClick={() => setMenuOpen(false)}
                        className={cn(
                          "focus-visible:ring-ring/60 block rounded-xl px-3 py-3 text-2xl font-semibold tracking-tight outline-none focus-visible:ring-[3px]",
                          active === item.id
                            ? "bg-white/10 text-white"
                            : "text-white/75 hover:text-white",
                        )}
                      >
                        {t(item.key)}
                      </a>
                    </motion.li>
                  ))}
                </ul>
              </nav>
              <motion.div
                initial={reduceMotion ? false : { opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.32, duration: 0.4, ease: EASE }}
                className="mt-10 grid gap-2"
              >
                <Button
                  asChild
                  size="lg"
                  className="h-12 rounded-full text-base"
                >
                  <a href={demoHref}>{t("landing.cta.demo")}</a>
                </Button>
                <Button
                  asChild
                  size="lg"
                  variant="outline"
                  className="h-12 rounded-full border-white/20 bg-transparent text-base text-white hover:bg-white/10 hover:text-white dark:bg-transparent"
                >
                  <Link href={LANDING_LOGIN_HREF}>
                    {t("landing.cta.login")}
                  </Link>
                </Button>
              </motion.div>
            </SheetContent>
          </Sheet>
        </div>
      </motion.div>
    </header>
  );
}
