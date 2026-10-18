import {
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  BriefcaseBusiness,
  Check,
  Handshake,
  Headphones,
  HelpCircle,
  Mail,
  MapPin,
  MessageCircle,
  Phone,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";
import { notFound } from "next/navigation";

import {
  EmptyState,
  PageHeading,
  PageSection,
} from "@/components/site/page-heading";
import { LeadForm } from "@/components/site/lead-form";
import { Paragraphs } from "@/components/site/text";
import { VehicleCard } from "@/components/site/vehicle-card";
import { Alert } from "@/components/ui/card";
import { campaignCard, campaignCards } from "@/lib/campaigns";
import { loadLocations, loadSubgroups } from "@/lib/catalog";
import { leadFlags, readLeadResult } from "@/lib/leads";
import { mailHref, telHref, whatsappHref } from "@/lib/links";
import type { Ctx } from "@/lib/server";

import type { ContentPage, InquiryKind, Query } from "../index";
import { Eyebrow } from "./parts";
import { AboutImage, aboutHighlights } from "./sections/about";
import { OfficeCard, isAirport } from "./sections/branches";
import { FaqList } from "./sections/faq";
import {
  CampaignCardView,
  CampaignCode,
  CampaignVisual,
  validityText,
} from "./sections/campaigns";
import { FleetActions, SegmentTabs } from "./sections/fleet";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** /vehicles: the live fleet, filterable by segment through links. */
export async function Vehicles({
  ctx,
  segment,
}: {
  ctx: Ctx;
  segment?: string;
}) {
  const { t, site } = ctx;
  const subgroups = site.booking_enabled ? await loadSubgroups() : null;
  const active =
    segment && subgroups?.some((s) => s.segment === segment)
      ? segment
      : undefined;
  const list = (subgroups ?? []).filter((s) => !active || s.segment === active);
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("fleet.title")}
        lead={t("tw.fleet.lead")}
      />
      <PageSection>
        {!subgroups ? (
          <Alert>{t("search.booking_off")}</Alert>
        ) : !subgroups.length ? (
          <EmptyState title={t("fleet.empty")} />
        ) : (
          <>
            <SegmentTabs ctx={ctx} subgroups={subgroups} active={active} />
            <p className="tw-result-count" role="status">
              {t("fleet.count", { count: list.length })}
            </p>
            <ul className="tw-vehicle-grid">
              {list.map((s) => (
                <li key={s.uuid}>
                  <VehicleCard ctx={ctx} subgroup={s} heading="h2">
                    <FleetActions ctx={ctx} onFleetPage />
                  </VehicleCard>
                </li>
              ))}
            </ul>
            <div className="rt-notice">
              <ShieldCheck aria-hidden size={19} />
              <p>{t("fleet.notice")}</p>
            </div>
          </>
        )}
      </PageSection>
    </>
  );
}

/** /branches: offices, filterable by city through links. */
export async function BranchList({ ctx, city }: { ctx: Ctx; city?: string }) {
  const { t, site } = ctx;
  const locations = site.booking_enabled ? await loadLocations() : null;
  const cities = [
    ...new Set((locations ?? []).map((l) => l.city).filter(Boolean)),
  ];
  const active = city && cities.includes(city) ? city : undefined;
  const list = (locations ?? []).filter((l) => !active || l.city === active);
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("branches.title")}
        lead={t("tw.branches.lead")}
      />
      <PageSection>
        {!locations || locations.length === 0 ? (
          <EmptyState title={t("branches.empty")} />
        ) : (
          <>
            {cities.length > 1 ? (
              <nav className="tw-fleet-tabs" aria-label={t("branches.cities")}>
                <Link
                  href="/branches"
                  aria-current={!active ? "page" : undefined}
                >
                  {t("branches.all_cities")}
                </Link>
                {cities.map((c) => (
                  <Link
                    key={c}
                    href={`/branches?city=${encodeURIComponent(c)}`}
                    aria-current={active === c ? "page" : undefined}
                  >
                    {c}
                  </Link>
                ))}
              </nav>
            ) : null}
            <p className="tw-result-count" role="status">
              {t("branches.count", { count: list.length })}
            </p>
            <ul className="tw-office-grid">
              {list.map((l) => (
                <li key={l.uuid}>
                  <OfficeCard ctx={ctx} location={l} />
                </li>
              ))}
            </ul>
          </>
        )}
      </PageSection>
    </>
  );
}

/** /branches/{uuid}: one live office. */
export async function BranchDetail({ ctx, uuid }: { ctx: Ctx; uuid: string }) {
  if (!UUID.test(uuid)) notFound();
  const { t, site } = ctx;
  const locations = site.booking_enabled ? await loadLocations() : null;
  const l = locations?.find((x) => x.uuid === uuid);
  if (!l) notFound();
  const address = [l.address, l.district, l.city].filter(Boolean).join(", ");
  const tel = telHref(l.phone);
  const mail = mailHref(l.email);
  const map =
    l.latitude != null && l.longitude != null
      ? `https://www.openstreetmap.org/?mlat=${l.latitude}&mlon=${l.longitude}#map=16/${l.latitude}/${l.longitude}`
      : null;
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={l.name}
        trail={[{ href: "/branches", label: t("branches.title") }]}
      />
      <PageSection>
        <div className="tw-office-detail">
          <div className="tw-office-location">
            <MapPin aria-hidden size={42} strokeWidth={1.3} />
            {address ? <p>{address}</p> : null}
            {map ? (
              <a
                href={map}
                target="_blank"
                rel="noopener noreferrer"
                className="tw-text-button"
              >
                {t("branches.map")}
                <ArrowUpRight aria-hidden size={16} />
              </a>
            ) : null}
          </div>
          <div>
            <Eyebrow>
              {isAirport(l)
                ? t("branches.airport")
                : l.city || t("branches.office")}
            </Eyebrow>
            <h2>{l.name}</h2>
            {l.phone && tel ? (
              <a href={tel}>
                <Phone aria-hidden size={18} />
                {l.phone}
              </a>
            ) : null}
            {l.email && mail ? (
              <a href={mail}>
                <Mail aria-hidden size={18} />
                {l.email}
              </a>
            ) : null}
            {l.points.length ? (
              <div className="tw-office-points">
                <h3>{t("branches.points")}</h3>
                <ul className="tw-checklist">
                  {l.points.map((p) => (
                    <li key={p.uuid}>
                      <Check aria-hidden size={17} />
                      {p.name}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            <Link
              href={`/search?pickup=${encodeURIComponent(l.uuid)}`}
              className="rt-button rt-button--primary"
            >
              {t("branches.search_here")}
              <ArrowRight aria-hidden size={17} />
            </Link>
          </div>
        </div>
      </PageSection>
    </>
  );
}

function Faq({ ctx }: { ctx: Ctx }) {
  const { t, content } = ctx;
  return (
    <>
      <PageHeading ctx={ctx} title={t("page.faq")} lead={t("tw.faq.lead")} />
      <PageSection>
        {content.faq.length ? (
          <div className="tw-faq-page">
            <aside>
              <span className="tw-page-icon" aria-hidden>
                <HelpCircle size={26} />
              </span>
              <h2>{t("tw.faq.title")}</h2>
              <p>{t("tw.faq.aside")}</p>
              <Link href="/contact" className="tw-text-button">
                {t("nav.contact")}
                <ArrowUpRight aria-hidden size={16} />
              </Link>
            </aside>
            <FaqList
              items={content.faq}
              className="tw-faq-items tw-faq-items--page"
            />
          </div>
        ) : (
          <EmptyState title={t("page.empty")} />
        )}
      </PageSection>
    </>
  );
}

function AboutPage({ ctx }: { ctx: Ctx }) {
  const { t, content, site } = ctx;
  const highlights = aboutHighlights(ctx);
  return (
    <>
      <PageHeading ctx={ctx} title={t("page.about")} />
      <PageSection>
        {content.about ? (
          <div className="tw-about-page">
            <div>
              <Eyebrow>{t("tw.about.eyebrow")}</Eyebrow>
              <h2>{t("tw.about.title", { name: site.organization.name })}</h2>
              <Paragraphs text={content.about} className="tw-about-text" />
              {highlights.length ? (
                <ul className="tw-checklist">
                  {highlights.map((h, i) => (
                    <li key={i}>
                      <Check aria-hidden size={17} />
                      {h}
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
            <AboutImage ctx={ctx} />
          </div>
        ) : (
          <EmptyState title={t("page.empty")} />
        )}
      </PageSection>
    </>
  );
}

/** Phone, WhatsApp, e-mail and address of the site settings. */
function ContactList({ ctx }: { ctx: Ctx }) {
  const { t, site } = ctx;
  const c = site.config.contact;
  const tel = telHref(c.phone);
  const wa = whatsappHref(c.whatsapp);
  const mail = mailHref(c.email);
  if (!(
    (c.phone && tel) ||
    (c.whatsapp && wa) ||
    (c.email && mail) ||
    c.address
  ))
    return null;
  return (
    <ul className="tw-contact-list">
      {c.phone && tel ? (
        <li>
          <Phone aria-hidden size={20} />
          <span>
            <small>{t("contact.phone")}</small>
            <a href={tel}>{c.phone}</a>
          </span>
        </li>
      ) : null}
      {c.whatsapp && wa ? (
        <li>
          <MessageCircle aria-hidden size={20} />
          <span>
            <small>{t("contact.whatsapp")}</small>
            <a href={wa} target="_blank" rel="noopener noreferrer">
              {c.whatsapp}
            </a>
          </span>
        </li>
      ) : null}
      {c.email && mail ? (
        <li>
          <Mail aria-hidden size={20} />
          <span>
            <small>{t("contact.email")}</small>
            <a href={mail}>{c.email}</a>
          </span>
        </li>
      ) : null}
      {c.address ? (
        <li>
          <MapPin aria-hidden size={20} />
          <span>
            <small>{t("contact.address")}</small>
            <span className="tw-pre-line">{c.address}</span>
          </span>
        </li>
      ) : null}
    </ul>
  );
}

/**
 * /contact: the contact details and, while the published `leads.contact`
 * flag is on, the contact form (otherwise the quick-contact card).
 */
function Contact({ ctx, query }: { ctx: Ctx; query?: Query }) {
  const { t, site } = ctx;
  const c = site.config.contact;
  const tel = telHref(c.phone);
  const wa = whatsappHref(c.whatsapp);
  const mail = mailHref(c.email);
  const has = Boolean(
    (c.phone && tel) || (c.whatsapp && wa) || (c.email && mail) || c.address,
  );
  const form = leadFlags(site).contact;
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("page.contact")}
        lead={t("tw.contact.lead")}
      />
      <PageSection>
        {!has && !form ? (
          <EmptyState title={t("page.empty")} />
        ) : (
          <div className="tw-inquiry-grid">
            <div className="tw-inquiry-copy">
              <span className="tw-page-icon" aria-hidden>
                <MessageCircle size={26} />
              </span>
              <h2>{t("tw.contact.title")}</h2>
              <p>{t("tw.contact.text")}</p>
              <ContactList ctx={ctx} />
            </div>
            {form ? (
              <LeadForm
                ctx={ctx}
                kind="contact"
                result={readLeadResult(query, "contact")}
                title={t("tw.inquiry.contact_form")}
                className="rt-lead-form tw-page-form"
              />
            ) : (
              <div className="rt-card tw-contact-card">
                <Headphones aria-hidden size={24} />
                <h2>{t("tw.contact.card_title")}</h2>
                <p>{t("tw.contact.card_text")}</p>
                <div className="tw-contact-actions">
                  {wa ? (
                    <a
                      href={wa}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="rt-button rt-button--primary"
                    >
                      <MessageCircle aria-hidden size={18} />
                      {t("tw.contact.whatsapp")}
                    </a>
                  ) : null}
                  {tel ? (
                    <a href={tel} className="rt-button rt-button--outline">
                      <Phone aria-hidden size={18} />
                      {t("tw.contact.call")}
                    </a>
                  ) : null}
                  {mail ? (
                    <a href={mail} className="rt-button rt-button--outline">
                      <Mail aria-hidden size={18} />
                      {t("tw.contact.mail")}
                    </a>
                  ) : null}
                </div>
              </div>
            )}
          </div>
        )}
      </PageSection>
    </>
  );
}

/**
 * /corporate and /franchise: the application forms (404 while the
 * published flag is off or the site has no usable key).
 */
export function Inquiry({
  ctx,
  kind,
  query,
}: {
  ctx: Ctx;
  kind: InquiryKind;
  query?: Query;
}) {
  const { t, site } = ctx;
  if (!leadFlags(site)[kind]) notFound();
  const Icon = kind === "corporate" ? BriefcaseBusiness : Handshake;
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t(kind === "corporate" ? "page.corporate" : "page.franchise")}
        lead={t(
          kind === "corporate" ? "tw.corporate.lead" : "tw.franchise.lead",
        )}
      />
      <PageSection>
        <div className="tw-inquiry-grid">
          <div className="tw-inquiry-copy">
            <span className="tw-page-icon" aria-hidden>
              <Icon size={26} />
            </span>
            <h2>
              {t(
                kind === "corporate"
                  ? "tw.corporate.title"
                  : "tw.franchise.title",
              )}
            </h2>
            <p>
              {t(
                kind === "corporate"
                  ? "tw.corporate.text"
                  : "tw.franchise.text",
              )}
            </p>
            <ContactList ctx={ctx} />
          </div>
          <LeadForm
            ctx={ctx}
            kind={kind}
            result={readLeadResult(query, kind)}
            title={t("tw.inquiry.form_title")}
            className="rt-lead-form tw-page-form"
          />
        </div>
      </PageSection>
    </>
  );
}

/** /campaigns: every live campaign of the storefront. */
export function CampaignList({ ctx }: { ctx: Ctx }) {
  const { t, site } = ctx;
  const cards = campaignCards(site, ctx.locale);
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("page.campaigns")}
        lead={t("tw.campaigns.lead")}
      />
      <PageSection>
        {cards.length ? (
          <ul className="tw-campaign-list">
            {cards.map((card, i) => (
              <li key={card.uuid}>
                <CampaignCardView
                  ctx={ctx}
                  card={card}
                  heading="h2"
                  eyebrow={String(i + 1).padStart(2, "0")}
                />
              </li>
            ))}
          </ul>
        ) : (
          <EmptyState
            title={t("tw.campaigns.empty")}
            text={t("tw.campaigns.empty_text")}
            action={
              site.booking_enabled ? (
                <Link href="/vehicles" className="rt-button rt-button--primary">
                  {t("nav.vehicles")}
                  <ArrowRight aria-hidden size={17} />
                </Link>
              ) : undefined
            }
          />
        )}
      </PageSection>
    </>
  );
}

/** /campaigns/{uuid}: one live campaign (404 once it is no longer live). */
export function CampaignDetail({ ctx, uuid }: { ctx: Ctx; uuid: string }) {
  const { t, site } = ctx;
  const card = campaignCard(site, uuid, ctx.locale);
  if (!card) notFound();
  const title = card.title || t("tw.campaigns.default_title");
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={title}
        trail={[{ href: "/campaigns", label: t("page.campaigns") }]}
      />
      <PageSection>
        <div className="tw-editorial-grid">
          <article className="tw-prose">
            <CampaignVisual ctx={ctx} card={card} label={false} />
            <p className="tw-mini-label">{validityText(ctx, card)}</p>
            {card.summary ? (
              <Paragraphs text={card.summary} className="tw-campaign-summary" />
            ) : null}
            {card.code ? (
              <div className="tw-campaign-code-box">
                <CampaignCode ctx={ctx} code={card.code} />
                <p>{t("tw.campaigns.code_hint")}</p>
              </div>
            ) : null}
            <h2>{t("tw.campaigns.how")}</h2>
            <ol>
              <li>{t("tw.campaigns.step1")}</li>
              <li>{t("tw.campaigns.step2")}</li>
              <li>{t("tw.campaigns.step3")}</li>
            </ol>
            <div className="rt-notice">
              <ShieldCheck aria-hidden size={19} />
              <p>{t("tw.campaigns.terms")}</p>
            </div>
          </article>
          <aside
            className="tw-editorial-cta"
            aria-label={t("tw.campaigns.aside_title")}
          >
            <h2>{t("tw.campaigns.aside_title")}</h2>
            <p>{t("tw.campaigns.aside_text")}</p>
            {site.booking_enabled ? (
              <Link href="/vehicles" className="rt-button rt-button--primary">
                {t("nav.vehicles")}
                <ArrowRight aria-hidden size={17} />
              </Link>
            ) : (
              <Link href="/contact" className="rt-button rt-button--primary">
                {t("tw.campaigns.aside_contact")}
                <ArrowRight aria-hidden size={17} />
              </Link>
            )}
            <Link href="/campaigns" className="tw-text-button">
              <ArrowLeft aria-hidden size={15} />
              {t("tw.campaigns.all")}
            </Link>
          </aside>
        </div>
      </PageSection>
    </>
  );
}

function Legal({ ctx, page }: { ctx: Ctx; page: "terms" | "privacy" }) {
  const { t, content } = ctx;
  const text = content[page];
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t(page === "terms" ? "page.terms" : "page.privacy")}
      />
      <PageSection narrow>
        {text ? (
          <article className="rt-prose">
            <Paragraphs text={text} />
          </article>
        ) : (
          <EmptyState title={t("page.empty")} />
        )}
      </PageSection>
    </>
  );
}

/** The content pages of the settings document (about, FAQ, contact, legal). */
export function Content({
  ctx,
  page,
  query,
}: {
  ctx: Ctx;
  page: ContentPage;
  query?: Query;
}) {
  switch (page) {
    case "faq":
      return <Faq ctx={ctx} />;
    case "about":
      return <AboutPage ctx={ctx} />;
    case "contact":
      return <Contact ctx={ctx} query={query} />;
    default:
      return <Legal ctx={ctx} page={page} />;
  }
}
