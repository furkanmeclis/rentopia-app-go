import {
  ArrowUpRight,
  Asterisk,
  Globe2,
  Mail,
  MapPin,
  Menu,
  Phone,
  X,
} from "lucide-react";
import { headers } from "next/headers";
import Link from "next/link";

import { campaignCards } from "@/lib/campaigns";
import { leadFlags } from "@/lib/leads";
import {
  localized,
  mailHref,
  menuLinks,
  socialLinks,
  telHref,
  type NavLink,
} from "@/lib/links";
import { PATH_HEADER } from "@/lib/request";
import { mediaUrl, type Ctx } from "@/lib/server";

import {
  currentPath,
  defaultHeaderLinks,
  isActive,
} from "../technowide/chrome";

/** Whether /contact has something to show (details or its form). */
export function hasContactPage(ctx: Ctx): boolean {
  const c = ctx.site.config.contact;
  return Boolean(
    c.phone ||
    c.email ||
    c.address ||
    c.whatsapp ||
    leadFlags(ctx.site).contact,
  );
}

/** A menu link; external ones open with noopener noreferrer. */
function HzAnchor({
  link,
  path,
  className,
  before,
  after,
}: {
  link: NavLink;
  path: string;
  className?: string;
  before?: React.ReactNode;
  after?: React.ReactNode;
}) {
  if (link.external) {
    return (
      <a href={link.href} rel="noopener noreferrer" className={className}>
        {before}
        <span>{link.label}</span>
        {after}
      </a>
    );
  }
  return (
    <Link
      href={link.href}
      className={className}
      aria-current={isActive(link.href, path) ? "page" : undefined}
    >
      {before}
      <span>{link.label}</span>
      {after}
    </Link>
  );
}

/** The italic serif wordmark (or the organization's logo) with the asterisk. */
function Wordmark({ ctx, className }: { ctx: Ctx; className: string }) {
  const logo = mediaUrl(ctx.site.assets.logo);
  const name = ctx.site.organization.name;
  return (
    <Link
      href="/"
      className={className}
      aria-label={`${name} · ${ctx.t("nav.home")}`}
    >
      {logo ? (
        // Images come from the API through /media (same origin).
        // eslint-disable-next-line @next/next/no-img-element
        <img src={logo} alt={name} />
      ) : (
        <span>{name}</span>
      )}
      <Asterisk aria-hidden />
    </Link>
  );
}

function LanguageSwitch({ ctx, path }: { ctx: Ctx; path: string }) {
  if (ctx.site.locales.length < 2) return null;
  return (
    <nav aria-label={ctx.t("nav.language")} className="hz-locale">
      <Globe2 aria-hidden size={14} />
      {ctx.site.locales.map((l) => (
        <a
          key={l}
          href={`/lang?l=${l}&next=${encodeURIComponent(path)}`}
          hrefLang={l}
          lang={l}
          aria-current={l === ctx.locale ? "true" : undefined}
        >
          {l.toUpperCase()}
        </a>
      ))}
    </nav>
  );
}

const pad = (n: number) => String(n).padStart(2, "0");

/**
 * Horizon header: a thin slogan line (with the address), the serif
 * wordmark, numbered menu links and the reservation lookup as an
 * underlined link. The mobile menu is a native <details> (no JavaScript).
 */
export async function Header({ ctx }: { ctx: Ctx }) {
  const { t, site } = ctx;
  const raw = (await headers()).get(PATH_HEADER) ?? "/";
  const path = await currentPath();
  const links = menuLinks(
    site.config.menus?.header,
    ctx.locale,
    defaultHeaderLinks(ctx),
  );
  const portal = site.portal_url;
  const address = site.config.contact.address;
  return (
    <header className="hz-header-wrap">
      <div className="hz-container hz-topline">
        <span>{t("hz.topline")}</span>
        {address ? (
          <span className="hz-topline-address">
            <MapPin aria-hidden size={12} />
            {address}
          </span>
        ) : null}
      </div>
      <div className="hz-container hz-header">
        <Wordmark ctx={ctx} className="hz-wordmark" />
        <nav className="hz-nav" aria-label={t("nav.main")}>
          {links.map((l, i) => (
            <HzAnchor
              key={`${l.href}-${l.label}`}
              link={l}
              path={path}
              before={<sup aria-hidden>{pad(i + 1)}</sup>}
            />
          ))}
        </nav>
        <div className="hz-header-actions">
          <LanguageSwitch ctx={ctx} path={raw} />
          {portal ? (
            <a href={portal} className="hz-header-login">
              {t("nav.login")}
            </a>
          ) : null}
          <Link href="/lookup" className="hz-link hz-header-booking">
            {t("nav.lookup")}
            <ArrowUpRight aria-hidden size={16} />
          </Link>
          <details className="hz-menu">
            <summary className="hz-menu-toggle" aria-label={t("nav.menu")}>
              <Menu aria-hidden className="hz-menu-open" size={22} />
              <X aria-hidden className="hz-menu-close" size={22} />
            </summary>
            <div className="hz-menu-panel">
              <nav aria-label={t("nav.menu")}>
                <HzAnchor
                  link={{ href: "/", label: t("nav.home"), external: false }}
                  path={path}
                  after={<ArrowUpRight aria-hidden size={18} />}
                />
                {links.map((l) => (
                  <HzAnchor
                    key={`${l.href}-${l.label}`}
                    link={l}
                    path={path}
                    after={<ArrowUpRight aria-hidden size={18} />}
                  />
                ))}
              </nav>
              {portal ? (
                <a href={portal} className="hz-menu-secondary">
                  {t("nav.login")}
                </a>
              ) : null}
              <Link href="/lookup" className="hz-menu-secondary">
                {t("nav.lookup")}
              </Link>
            </div>
          </details>
        </div>
      </div>
    </header>
  );
}

function FooterColumn({
  title,
  links,
  path,
}: {
  title: string;
  links: NavLink[];
  path: string;
}) {
  if (!links.length) return null;
  return (
    <div>
      <h2 className="hz-kicker">{title}</h2>
      <ul>
        {links.map((l) => (
          <li key={`${l.href}-${l.label}`}>
            <HzAnchor link={l} path={path} />
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Horizon footer: the large italic invitation with a round arrow, then the
 * brand, explore and "good to know" columns and a short note, and the
 * copyright line. Only pages that exist are linked.
 */
export async function Footer({ ctx }: { ctx: Ctx }) {
  const { t, site, content } = ctx;
  const path = await currentPath();
  const c = site.config.contact;
  const tagline = localized(site.config.brand?.tagline, ctx.locale);
  const socials = socialLinks(site.config.brand?.socials);
  const tel = telHref(c.phone);
  const mail = mailHref(c.email);
  const link = (href: string, label: string): NavLink => ({
    href,
    label,
    external: false,
  });
  const forms = leadFlags(site);
  const configured = menuLinks(site.config.menus?.footer, ctx.locale, []);
  const explore = configured.length
    ? configured
    : [
        ...(site.booking_enabled ? [link("/vehicles", t("nav.vehicles"))] : []),
        ...(campaignCards(site, ctx.locale).length
          ? [link("/campaigns", t("nav.campaigns"))]
          : []),
        ...(content.about ? [link("/about", t("nav.about"))] : []),
        link("/branches", t("nav.branches")),
        ...(forms.corporate ? [link("/corporate", t("nav.corporate"))] : []),
        ...(forms.franchise ? [link("/franchise", t("nav.franchise"))] : []),
      ];
  const know = [
    ...(content.faq.length ? [link("/faq", t("nav.faq"))] : []),
    link("/lookup", t("nav.lookup")),
    link("/terms", t("footer.terms")),
    link("/privacy", t("footer.privacy")),
    ...(hasContactPage(ctx) ? [link("/contact", t("nav.contact"))] : []),
  ];
  const next = site.booking_enabled ? "/search" : "/branches";
  return (
    <footer className="hz-footer">
      <div className="hz-container">
        <div className="hz-footer-invitation">
          <div>
            <span className="hz-kicker">{t("hz.footer.kicker")}</span>
            <p>{t("hz.footer.invite")}</p>
          </div>
          <Link
            href={next}
            className="hz-round hz-footer-arrow"
            aria-label={t("hz.footer.invite_action")}
          >
            <ArrowUpRight aria-hidden strokeWidth={1} />
          </Link>
        </div>
        <div className="hz-footer-grid">
          <div className="hz-footer-brand">
            <Wordmark ctx={ctx} className="hz-footer-wordmark" />
            {tagline ? <p>{tagline}</p> : null}
            {c.phone && tel ? (
              <a href={tel}>
                <Phone aria-hidden size={13} />
                {c.phone}
              </a>
            ) : null}
            {c.email && mail ? (
              <a href={mail}>
                <Mail aria-hidden size={13} />
                {c.email}
              </a>
            ) : null}
            {socials.length ? (
              <ul className="hz-socials" aria-label={t("footer.social")}>
                {socials.map((s) => (
                  <li key={s.network}>
                    <a
                      href={s.href}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={s.name}
                      title={s.name}
                    >
                      <span aria-hidden>{s.short}</span>
                    </a>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
          <FooterColumn
            title={
              configured.length ? t("footer.links") : t("hz.footer.explore")
            }
            links={explore}
            path={path}
          />
          <FooterColumn title={t("hz.footer.know")} links={know} path={path} />
          <div className="hz-footer-note">
            <Asterisk aria-hidden size={50} strokeWidth={0.9} />
            <p>{t("hz.footer.note")}</p>
            {c.address ? <span>{c.address}</span> : null}
          </div>
        </div>
        <div className="hz-copyright">
          <small>
            © {new Date().getFullYear()} {site.organization.name}.{" "}
            {t("footer.rights")}
          </small>
          <span>Rentopya</span>
        </div>
      </div>
    </footer>
  );
}
