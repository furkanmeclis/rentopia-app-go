import { CarFront, Globe2, Headphones, Menu, Phone, X } from "lucide-react";
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

import { ClAnchor as AuAnchor } from "../classic/chrome";
import { hasContactPage } from "../horizon/chrome";
import { currentPath, defaultHeaderLinks } from "../technowide/chrome";

/** The car mark tile and the name, or the organization's logo. */
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
          <span className="au-mark" aria-hidden>
            <CarFront size={22} />
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
    <nav aria-label={ctx.t("nav.language")} className="au-locale">
      <Globe2 aria-hidden size={15} />
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
 * Aurum header: the car mark and name, the menu pushed right, the language
 * switch and the dark account action (the customer portal, else the
 * reservation lookup). Below 850 px the menu is a native <details> panel.
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
  return (
    <header className="au-header">
      <div className="au-wrap au-header-inner">
        <Brand ctx={ctx} className="au-brand" />
        <nav className="au-nav" aria-label={t("nav.main")}>
          {links.map((l) => (
            <AuAnchor key={`${l.href}-${l.label}`} link={l} path={path} />
          ))}
        </nav>
        <div className="au-header-actions">
          <LanguageSwitch ctx={ctx} path={raw} />
          {portal ? (
            <a href={portal} className="rt-button au-dark">
              {t("au.account")}
            </a>
          ) : (
            <Link href="/lookup" className="rt-button au-dark">
              {t("nav.lookup")}
            </Link>
          )}
          <details className="au-menu">
            <summary className="au-menu-toggle" aria-label={t("nav.menu")}>
              <Menu aria-hidden className="au-menu-open" size={22} />
              <X aria-hidden className="au-menu-close" size={22} />
            </summary>
            <div className="au-menu-panel">
              <strong className="au-menu-title">
                {site.organization.name}
              </strong>
              <nav aria-label={t("nav.menu")}>
                <AuAnchor
                  link={{ href: "/", label: t("nav.home"), external: false }}
                  path={path}
                />
                {links.map((l) => (
                  <AuAnchor key={`${l.href}-${l.label}`} link={l} path={path} />
                ))}
                {portal ? <a href={portal}>{t("au.account")}</a> : null}
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
            <AuAnchor link={l} path={path} />
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * The dark support band ("Yolculuğunuzu birlikte planlayalım."), then the
 * pale footer: brand with tagline and address, explore, useful
 * information and contact columns, and the copyright line. Only pages that
 * exist are linked.
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
  const explore = configured.length
    ? configured
    : [
        ...(site.booking_enabled ? [link("/vehicles", t("nav.vehicles"))] : []),
        ...(campaignCards(site, ctx.locale).length
          ? [link("/campaigns", t("nav.campaigns"))]
          : []),
        link("/branches", t("nav.branches")),
        ...(content.about ? [link("/about", t("nav.about"))] : []),
        ...(forms.corporate ? [link("/corporate", t("nav.corporate"))] : []),
        ...(forms.franchise ? [link("/franchise", t("nav.franchise"))] : []),
      ];
  const info = [
    ...(content.faq.length ? [link("/faq", t("page.faq"))] : []),
    link("/lookup", t("nav.lookup")),
    link("/terms", t("footer.terms")),
    link("/privacy", t("footer.privacy")),
  ];
  return (
    <>
      {tel || contact ? (
        <aside className="au-support" aria-labelledby="au-support">
          <div className="au-wrap">
            <span className="au-support-icon" aria-hidden>
              <Headphones size={30} />
            </span>
            <div>
              <h2 id="au-support">{t("au.support.title")}</h2>
              {tagline ? <p>{tagline}</p> : null}
            </div>
            {tel && c.phone ? (
              <a href={tel}>
                <Phone aria-hidden size={18} />
                {c.phone}
              </a>
            ) : (
              <Link href="/contact">{t("au.support.contact")}</Link>
            )}
          </div>
        </aside>
      ) : null}
      <footer className="au-footer">
        <div className="au-wrap">
          <div className="au-footer-grid">
            <div className="au-footer-about">
              <strong className="au-footer-brand">
                {site.organization.name}
              </strong>
              {tagline ? <p>{tagline}</p> : null}
              {c.address ? <p>{c.address}</p> : null}
              {socials.length ? (
                <ul className="au-socials" aria-label={t("footer.social")}>
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
                configured.length ? t("footer.links") : t("au.footer.explore")
              }
              links={explore}
              path={path}
            />
            <FooterColumn
              title={t("au.footer.info")}
              links={info}
              path={path}
            />
            <div className="au-footer-contact">
              <h2>{t("au.footer.contact")}</h2>
              {c.email && mail ? <a href={mail}>{c.email}</a> : null}
              {c.phone && tel ? <a href={tel}>{c.phone}</a> : null}
              {contact ? (
                <Link href="/contact">{t("au.support.contact")}</Link>
              ) : null}
            </div>
          </div>
          <div className="au-footer-bottom">
            <small>
              © {new Date().getFullYear()} {site.organization.name}.{" "}
              {t("footer.rights")}
            </small>
            <span>Rentopya</span>
          </div>
        </div>
      </footer>
    </>
  );
}
