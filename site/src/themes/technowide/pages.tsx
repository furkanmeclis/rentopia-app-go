import {
  ArrowRight,
  ArrowUpRight,
  Check,
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
import { Paragraphs } from "@/components/site/text";
import { VehicleCard } from "@/components/site/vehicle-card";
import { Alert } from "@/components/ui/card";
import { loadLocations, loadSubgroups } from "@/lib/catalog";
import { mailHref, telHref, whatsappHref } from "@/lib/links";
import type { Ctx } from "@/lib/server";

import type { ContentPage } from "../index";
import { Eyebrow } from "./parts";
import { AboutImage, aboutHighlights } from "./sections/about";
import { OfficeCard, isAirport } from "./sections/branches";
import { FaqList } from "./sections/faq";
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

function Contact({ ctx }: { ctx: Ctx }) {
  const { t, site } = ctx;
  const c = site.config.contact;
  const tel = telHref(c.phone);
  const wa = whatsappHref(c.whatsapp);
  const mail = mailHref(c.email);
  const has = Boolean(
    (c.phone && tel) || (c.whatsapp && wa) || (c.email && mail) || c.address,
  );
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("page.contact")}
        lead={t("tw.contact.lead")}
      />
      <PageSection>
        {!has ? (
          <EmptyState title={t("page.empty")} />
        ) : (
          <div className="tw-inquiry-grid">
            <div className="tw-inquiry-copy">
              <span className="tw-page-icon" aria-hidden>
                <MessageCircle size={26} />
              </span>
              <h2>{t("tw.contact.title")}</h2>
              <p>{t("tw.contact.text")}</p>
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
            </div>
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
          </div>
        )}
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
export function Content({ ctx, page }: { ctx: Ctx; page: ContentPage }) {
  switch (page) {
    case "faq":
      return <Faq ctx={ctx} />;
    case "about":
      return <AboutPage ctx={ctx} />;
    case "contact":
      return <Contact ctx={ctx} />;
    default:
      return <Legal ctx={ctx} page={page} />;
  }
}
