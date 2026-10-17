import {
  ArrowRight,
  ArrowUpRight,
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

/**
 * Pages that exist for this organization (no dead links): campaigns only
 * while one is live, the contact page with contact details or its form.
 * Corporate and franchise applications are linked from the footer while
 * their forms are on.
 */
function defaultHeaderLinks(ctx: Ctx): NavLink[] {
  const { t, content, site } = ctx;
  const c = site.config.contact;
  const links: NavLink[] = [];
  const add = (href: string, label: string) =>
    links.push({ href, label, external: false });
  if (site.booking_enabled) add("/vehicles", t("nav.vehicles"));
  if (campaignCards(site, ctx.locale).length)
    add("/campaigns", t("nav.campaigns"));
  add("/branches", t("nav.branches"));
  add("/lookup", t("nav.reservations"));
  if (content.faq.length) add("/faq", t("nav.faq"));
  if (content.about) add("/about", t("nav.about"));
  if (c.phone || c.email || c.address || c.whatsapp || leadFlags(site).contact)
    add("/contact", t("nav.contact"));
  return links;
}

async function currentPath(): Promise<string> {
  const raw = (await headers()).get(PATH_HEADER) ?? "/";
  return raw.split("?")[0] || "/";
}

function isActive(href: string, path: string): boolean {
  if (href.startsWith("https://")) return false;
  const target = href.split(/[?#]/)[0] || "/";
  return target === "/"
    ? path === "/"
    : path === target || path.startsWith(`${target}/`);
}

function NavAnchor({
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
        {link.label}
        {children}
      </a>
    );
  }
  return (
    <Link
      href={link.href}
      className={className}
      aria-current={isActive(link.href, path) ? "page" : undefined}
    >
      {link.label}
      {children}
    </Link>
  );
}

function Brand({ ctx }: { ctx: Ctx }) {
  const logo = mediaUrl(ctx.site.assets.logo);
  const name = ctx.site.organization.name;
  const tagline = localized(ctx.site.config.brand?.tagline, ctx.locale);
  return (
    <Link
      href="/"
      className="tw-brand"
      aria-label={`${name} · ${ctx.t("nav.home")}`}
    >
      {logo ? (
        // Images come from the API through /media (same origin).
        // eslint-disable-next-line @next/next/no-img-element
        <img src={logo} alt={name} />
      ) : (
        <span className="tw-brand-words">
          <b>{name}</b>
          {tagline ? <small>{tagline}</small> : null}
        </span>
      )}
    </Link>
  );
}

function LanguageSwitch({ ctx, path }: { ctx: Ctx; path: string }) {
  if (ctx.site.locales.length < 2) return null;
  return (
    <nav aria-label={ctx.t("nav.language")} className="tw-header-locale">
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
    <header className="tw-header">
      <div className="tw-header-top">
        <div className="rt-container tw-header-inner">
          <Brand ctx={ctx} />
          <div className="tw-header-tools">
            <LanguageSwitch ctx={ctx} path={raw} />
            {portal ? (
              <a href={portal} className="tw-login">
                {t("nav.login")}
                <ArrowUpRight aria-hidden size={15} />
              </a>
            ) : null}
            <Link href="/lookup" className="tw-register">
              {t("nav.lookup")}
            </Link>
            {/* Mobile menu without JavaScript. */}
            <details className="tw-menu">
              <summary className="tw-menu-toggle" aria-label={t("nav.menu")}>
                <Menu aria-hidden className="tw-menu-open" size={24} />
                <X aria-hidden className="tw-menu-close" size={24} />
              </summary>
              <div className="tw-mobile-menu">
                <nav aria-label={t("nav.menu")}>
                  <NavAnchor
                    link={{ href: "/", label: t("nav.home"), external: false }}
                    path={path}
                  >
                    <ArrowUpRight aria-hidden size={16} />
                  </NavAnchor>
                  {links.map((l) => (
                    <NavAnchor
                      key={`${l.href}-${l.label}`}
                      link={l}
                      path={path}
                    >
                      <ArrowUpRight aria-hidden size={16} />
                    </NavAnchor>
                  ))}
                </nav>
                {portal ? (
                  <a href={portal} className="tw-drawer-login">
                    {t("nav.login")}
                  </a>
                ) : null}
                <Link href="/lookup" className="tw-register">
                  {t("nav.lookup")}
                </Link>
              </div>
            </details>
          </div>
        </div>
      </div>
      <nav className="tw-desktop-nav rt-container" aria-label={t("nav.main")}>
        {links.map((l) => (
          <NavAnchor key={`${l.href}-${l.label}`} link={l} path={path} />
        ))}
      </nav>
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
            <NavAnchor link={l} path={path} />
          </li>
        ))}
      </ul>
    </div>
  );
}

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
  const hasContact = Boolean(
    c.phone || c.email || c.address || c.whatsapp || forms.contact,
  );
  const support = [
    ...(content.faq.length ? [link("/faq", t("nav.faq"))] : []),
    ...(hasContact ? [link("/contact", t("nav.contact"))] : []),
    link("/lookup", t("nav.lookup")),
  ];
  const company = [
    ...(content.about ? [link("/about", t("nav.about"))] : []),
    ...(site.booking_enabled ? [link("/vehicles", t("nav.vehicles"))] : []),
    ...(campaignCards(site, ctx.locale).length
      ? [link("/campaigns", t("nav.campaigns"))]
      : []),
    link("/branches", t("nav.branches")),
    ...(forms.corporate ? [link("/corporate", t("nav.corporate"))] : []),
    ...(forms.franchise ? [link("/franchise", t("nav.franchise"))] : []),
  ];
  const configured = menuLinks(site.config.menus?.footer, ctx.locale, []);
  const policies = [
    link("/terms", t("footer.terms")),
    link("/privacy", t("footer.privacy")),
  ];
  const locations = site.booking_enabled ? await loadLocations() : null;
  const offices = (locations ?? [])
    .slice(0, 5)
    .map((l) => link(`/branches/${l.uuid}`, l.name));

  return (
    <footer className="tw-footer">
      <div className="rt-container">
        <div className="tw-footer-main">
          <div className="tw-footer-brand">
            <Brand ctx={ctx} />
            {tagline ? <p>{tagline}</p> : null}
            {c.phone && tel ? (
              <a className="tw-footer-phone" href={tel}>
                <Phone aria-hidden size={18} />
                {c.phone}
              </a>
            ) : null}
            {c.email && mail ? (
              <a href={mail}>
                <Mail aria-hidden size={14} />
                {c.email}
              </a>
            ) : null}
            {c.address ? (
              <p className="tw-footer-address">
                <MapPin aria-hidden size={14} />
                <span>{c.address}</span>
              </p>
            ) : null}
            {socials.length ? (
              <ul className="tw-socials" aria-label={t("footer.social")}>
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
          {configured.length ? (
            <FooterColumn
              title={t("footer.links")}
              links={configured}
              path={path}
            />
          ) : (
            <>
              <FooterColumn
                title={t("footer.support")}
                links={support}
                path={path}
              />
              <FooterColumn
                title={t("footer.company")}
                links={company}
                path={path}
              />
            </>
          )}
          <FooterColumn
            title={t("footer.policies")}
            links={policies}
            path={path}
          />
          {offices.length ? (
            <div>
              <h2>{t("footer.locations")}</h2>
              <ul>
                {offices.map((o) => (
                  <li key={o.href}>
                    <NavAnchor link={o} path={path} />
                  </li>
                ))}
              </ul>
              <Link href="/branches" className="tw-footer-all">
                {t("branches.all")}
                <ArrowRight aria-hidden size={14} />
              </Link>
            </div>
          ) : null}
        </div>
        <div className="tw-footer-bottom">
          <small>
            © {new Date().getFullYear()} {site.organization.name}.{" "}
            {t("footer.rights")}
          </small>
          <span className="tw-footer-credit">Rentopia</span>
        </div>
      </div>
    </footer>
  );
}
