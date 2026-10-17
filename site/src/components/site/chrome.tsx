import { Menu } from "lucide-react";
import { headers } from "next/headers";
import Link from "next/link";

import { cn } from "@/lib/cn";
import { PATH_HEADER } from "@/lib/request";
import { mediaUrl, type Ctx } from "@/lib/server";

import { Container } from "./text";

function navLinks(ctx: Ctx) {
  const { t, content, site } = ctx;
  const links = [{ href: "/branches", label: t("nav.branches") }];
  if (content.about) links.push({ href: "/about", label: t("nav.about") });
  if (content.faq.length) links.push({ href: "/faq", label: t("nav.faq") });
  const c = site.config.contact;
  if (c.phone || c.email || c.address || c.whatsapp) {
    links.push({ href: "/contact", label: t("nav.contact") });
  }
  links.push({ href: "/lookup", label: t("nav.lookup") });
  return links;
}

function Brand({ ctx }: { ctx: Ctx }) {
  const logo = mediaUrl(ctx.site.assets.logo);
  return (
    <Link href="/" className="flex min-w-0 items-center gap-2 font-semibold">
      {logo ? (
        // Images come from the API through /media (same origin).
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={logo}
          alt={ctx.site.organization.name}
          className="h-9 w-auto max-w-[160px] object-contain"
        />
      ) : (
        <span className="truncate text-lg">{ctx.site.organization.name}</span>
      )}
    </Link>
  );
}

async function LanguageSwitch({ ctx }: { ctx: Ctx }) {
  if (ctx.site.locales.length < 2) return null;
  const path = (await headers()).get(PATH_HEADER) ?? "/";
  return (
    <nav
      aria-label={ctx.t("nav.language")}
      className="flex items-center gap-1 text-sm"
    >
      {ctx.site.locales.map((l) => (
        <a
          key={l}
          href={`/lang?l=${l}&next=${encodeURIComponent(path)}`}
          hrefLang={l}
          aria-current={l === ctx.locale ? "true" : undefined}
          className={cn(
            "rounded px-2 py-1 uppercase",
            l === ctx.locale
              ? "bg-muted font-semibold"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {l}
        </a>
      ))}
    </nav>
  );
}

export async function SiteHeader({ ctx }: { ctx: Ctx }) {
  const links = navLinks(ctx);
  return (
    <header className="border-border bg-background/95 sticky top-0 z-30 border-b backdrop-blur">
      <Container className="flex h-16 items-center justify-between gap-4">
        <Brand ctx={ctx} />
        <nav
          aria-label={ctx.t("nav.menu")}
          className="hidden items-center gap-5 text-sm md:flex"
        >
          {links.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className="text-muted-foreground hover:text-foreground"
            >
              {l.label}
            </Link>
          ))}
        </nav>
        <div className="flex items-center gap-2">
          <LanguageSwitch ctx={ctx} />
          {/* No-JS mobile menu. */}
          <details className="relative md:hidden">
            <summary
              className="hover:bg-muted flex size-10 cursor-pointer list-none items-center justify-center rounded-[var(--radius)] [&::-webkit-details-marker]:hidden"
              aria-label={ctx.t("nav.menu")}
            >
              <Menu aria-hidden />
            </summary>
            <nav
              aria-label={ctx.t("nav.menu")}
              className="border-border bg-background absolute right-0 mt-2 grid w-56 gap-1 rounded-[var(--radius)] border p-2 shadow-lg"
            >
              <Link href="/" className="hover:bg-muted rounded px-3 py-2">
                {ctx.t("nav.home")}
              </Link>
              {links.map((l) => (
                <Link
                  key={l.href}
                  href={l.href}
                  className="hover:bg-muted rounded px-3 py-2"
                >
                  {l.label}
                </Link>
              ))}
            </nav>
          </details>
        </div>
      </Container>
    </header>
  );
}

export function SiteFooter({ ctx }: { ctx: Ctx }) {
  const { t, site } = ctx;
  const c = site.config.contact;
  const variant = site.config.layout.footer;
  const year = new Date().getFullYear();
  const legal = (
    <ul className="flex flex-wrap gap-x-4 gap-y-2 text-sm">
      <li>
        <Link href="/terms" className="hover:underline">
          {t("footer.terms")}
        </Link>
      </li>
      <li>
        <Link href="/privacy" className="hover:underline">
          {t("footer.privacy")}
        </Link>
      </li>
      <li>
        <Link href="/lookup" className="hover:underline">
          {t("nav.lookup")}
        </Link>
      </li>
    </ul>
  );
  const contact = (
    <ul className="space-y-1 text-sm">
      {c.phone ? (
        <li>
          <a
            href={`tel:${c.phone.replace(/[^+0-9]/g, "")}`}
            className="hover:underline"
          >
            {c.phone}
          </a>
        </li>
      ) : null}
      {c.email ? (
        <li>
          <a href={`mailto:${c.email}`} className="hover:underline">
            {c.email}
          </a>
        </li>
      ) : null}
      {c.address ? <li className="whitespace-pre-line">{c.address}</li> : null}
    </ul>
  );
  const copyright = (
    <p className="text-muted-foreground text-xs">
      © {year} {site.organization.name}. {t("footer.rights")}
    </p>
  );

  if (variant === "columns") {
    return (
      <footer className="border-border bg-muted mt-16 border-t">
        <Container className="grid gap-8 py-10 sm:grid-cols-3">
          <div className="space-y-2">
            <p className="font-semibold">{site.organization.name}</p>
            {copyright}
          </div>
          <div className="space-y-2">
            <p className="font-semibold">{t("footer.contact")}</p>
            {contact}
          </div>
          <div className="space-y-2">
            <p className="font-semibold">{t("footer.links")}</p>
            {legal}
          </div>
        </Container>
      </footer>
    );
  }
  if (variant === "minimal") {
    return (
      <footer className="mt-16">
        <Container className="flex flex-col items-center gap-3 py-8 text-center">
          {legal}
          {copyright}
        </Container>
      </footer>
    );
  }
  return (
    <footer className="border-border mt-16 border-t">
      <Container className="flex flex-col gap-4 py-8 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-2">
          <p className="font-semibold">{site.organization.name}</p>
          {contact}
        </div>
        <div className="space-y-3 sm:text-right">
          {legal}
          {copyright}
        </div>
      </Container>
    </footer>
  );
}
