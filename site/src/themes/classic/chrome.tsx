import {
  CarFront,
  Globe2,
  Mail,
  MapPin,
  Menu,
  Phone,
  Search,
  X,
} from "lucide-react";
import { headers } from "next/headers";
import Link from "next/link";

import { campaignCards } from "@/lib/campaigns";
import { loadLocations } from "@/lib/catalog";
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

import { hasContactPage } from "../horizon/chrome";
import { monogram } from "./helpers";
import {
  currentPath,
  defaultHeaderLinks,
  isActive,
} from "../technowide/chrome";

/** A menu link; external ones open with noopener noreferrer. */
export function ClAnchor({
  link,
  path,
  className,
  children,
}: {
  link: NavLink;
  path: string;
  className?: string;
  children?: React.ReactNode;
}) {
  if (link.external) {
    return (
      <a href={link.href} rel="noopener noreferrer" className={className}>
        {children}
        {link.label}
      </a>
    );
  }
  return (
    <Link
      href={link.href}
      className={className}
      aria-current={isActive(link.href, path) ? "page" : undefined}
    >
      {children}
      {link.label}
    </Link>
  );
}

/** Monogram tile and the name, or the organization's logo. */
function Brand({ ctx, className }: { ctx: Ctx; className: string }) {
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
        <>
          <span className="cl-monogram" aria-hidden>
            {monogram(name, ctx.locale)}
          </span>
          <strong>{name}</strong>
        </>
      )}
    </Link>
  );
}

function LanguageSwitch({ ctx, path }: { ctx: Ctx; path: string }) {
  if (ctx.site.locales.length < 2) return null;
  return (
    <nav aria-label={ctx.t("nav.language")} className="cl-locale">
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

/**
 * classic header: a compact sticky white bar with the monogram, the menu,
 * the language switch and the accent "Araç bul" action. Below 1150 px the
 * menu moves into a native <details> panel (no JavaScript).
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
  const find = site.booking_enabled ? "/vehicles" : "/branches";
  return (
    <header className="cl-header">
      <div className="cl-container cl-header-inner">
        <Brand ctx={ctx} className="cl-brand" />
        <nav className="cl-nav" aria-label={t("nav.main")}>
          {links.map((l) => (
            <ClAnchor key={`${l.href}-${l.label}`} link={l} path={path} />
          ))}
        </nav>
        <div className="cl-header-actions">
          <LanguageSwitch ctx={ctx} path={raw} />
          {portal ? (
            <a href={portal} className="cl-login">
              {t("nav.login")}
            </a>
          ) : null}
          <Link href={find} className="rt-button cl-cta cl-header-find">
            <CarFront aria-hidden size={15} />
            {t("cl.find")}
          </Link>
          <details className="cl-menu">
            <summary className="cl-menu-toggle" aria-label={t("nav.menu")}>
              <Menu aria-hidden className="cl-menu-open" size={20} />
              <X aria-hidden className="cl-menu-close" size={20} />
            </summary>
            <div className="cl-menu-panel">
              <strong className="cl-menu-title">
                {site.organization.name}
              </strong>
              <nav aria-label={t("nav.menu")}>
                <ClAnchor
                  link={{ href: "/", label: t("nav.home"), external: false }}
                  path={path}
                />
                {links.map((l) => (
                  <ClAnchor key={`${l.href}-${l.label}`} link={l} path={path} />
                ))}
                {portal ? <a href={portal}>{t("nav.login")}</a> : null}
              </nav>
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
      <h2>{title}</h2>
      <ul>
        {links.map((l) => (
          <li key={`${l.href}-${l.label}`}>
            <ClAnchor link={l} path={path} />
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The support band ("Sorularınız için ekibimiz hazır."), the four column
 * footer (brand, live branches, corporate & legal, contact) and, on
 * phones, the fixed quick navigation. Only pages that exist are linked.
 */
export async function Footer({ ctx }: { ctx: Ctx }) {
  const { t, site, content } = ctx;
  const path = await currentPath();
  const c = site.config.contact;
  const tagline = localized(site.config.brand?.tagline, ctx.locale);
  const socials = socialLinks(site.config.brand?.socials);
  const tel = telHref(c.phone);
  const mail = mailHref(c.email);
  const contact = hasContactPage(ctx);
  const forms = leadFlags(site);
  const link = (href: string, label: string): NavLink => ({
    href,
    label,
    external: false,
  });
  const configured = menuLinks(site.config.menus?.footer, ctx.locale, []);
  const legal = configured.length
    ? configured
    : [
        ...(content.about ? [link("/about", t("nav.about"))] : []),
        ...(campaignCards(site, ctx.locale).length
          ? [link("/campaigns", t("nav.campaigns"))]
          : []),
        ...(forms.corporate ? [link("/corporate", t("nav.corporate"))] : []),
        ...(forms.franchise ? [link("/franchise", t("nav.franchise"))] : []),
        ...(content.faq.length ? [link("/faq", t("nav.faq"))] : []),
        link("/terms", t("footer.terms")),
        link("/privacy", t("footer.privacy")),
      ];
  const locations = site.booking_enabled ? await loadLocations() : null;
  const offices = [
    ...(locations ?? [])
      .slice(0, 4)
      .map((l) => link(`/branches/${l.uuid}`, l.name)),
    link("/branches", t("branches.all")),
  ];
  const quick = [
    {
      href: site.booking_enabled ? "/search" : "/branches",
      label: t("cl.quick.search"),
      icon: Search,
    },
    { href: "/lookup", label: t("cl.quick.booking"), icon: CarFront },
    ...(contact
      ? [{ href: "/contact", label: t("cl.quick.contact"), icon: Phone }]
      : []),
    { href: "/branches", label: t("cl.quick.branches"), icon: MapPin },
  ];
  return (
    <>
      {tel || contact ? (
        <aside className="cl-container cl-support" aria-labelledby="cl-support">
          <div>
            <span className="cl-badge">{t("cl.support.badge")}</span>
            <h2 id="cl-support">{t("cl.support.title")}</h2>
            <p>{t("cl.support.text")}</p>
          </div>
          {tel && c.phone ? (
            <a href={tel} className="rt-button cl-cta">
              <Phone aria-hidden size={16} />
              {c.phone}
            </a>
          ) : (
            <Link href="/contact" className="rt-button cl-cta">
              {t("cl.support.contact")}
            </Link>
          )}
        </aside>
      ) : null}
      <footer className="cl-footer">
        <div className="cl-container cl-footer-grid">
          <div className="cl-footer-brand">
            <strong>{site.organization.name}</strong>
            {tagline ? <p>{tagline}</p> : null}
            <span className="cl-badge cl-badge--outline">
              {t("cl.footer.kind")}
            </span>
            {socials.length ? (
              <ul className="cl-socials" aria-label={t("footer.social")}>
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
            title={t("cl.footer.branches")}
            links={offices}
            path={path}
          />
          <FooterColumn
            title={configured.length ? t("footer.links") : t("cl.footer.legal")}
            links={legal}
            path={path}
          />
          <div className="cl-footer-contact">
            <h2>{t("footer.contact")}</h2>
            {c.address ? <p>{c.address}</p> : null}
            {c.email && mail ? (
              <a href={mail}>
                <Mail aria-hidden size={13} />
                {c.email}
              </a>
            ) : null}
            {c.phone && tel ? (
              <a href={tel}>
                <Phone aria-hidden size={13} />
                {c.phone}
              </a>
            ) : null}
            <Link href="/lookup">{t("nav.lookup")}</Link>
          </div>
        </div>
        <div className="cl-container cl-copyright">
          <small>
            © {new Date().getFullYear()} {site.organization.name}.{" "}
            {t("footer.rights")}
          </small>
          <span>Rentopya</span>
        </div>
      </footer>
      <nav className="cl-bottom-nav" aria-label={t("cl.quick")}>
        {quick.map(({ href, label, icon: Icon }) => (
          <Link
            key={href}
            href={href}
            aria-current={isActive(href, path) ? "page" : undefined}
          >
            <Icon aria-hidden size={18} />
            <span>{label}</span>
          </Link>
        ))}
      </nav>
    </>
  );
}
