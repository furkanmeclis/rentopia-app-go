import { ArrowLeft, ArrowRight, ArrowUpRight, BookOpen } from "lucide-react";
import Link from "next/link";

import {
  EmptyState,
  PageHeading,
  PageSection,
} from "@/components/site/page-heading";
import {
  articleDate,
  articleMediaUrl,
  homeArticleCards,
  toArticleCards,
  type ArticleCard,
} from "@/lib/articles";
import { siteMediaUrl } from "@/lib/media";
import { renderArticleBody } from "@/lib/rich-text";
import type { Ctx } from "@/lib/server";
import type { ArticlePage, PublicArticle } from "@/lib/types";

import { SectionHeading } from "../parts";

/** The cover photo, or the brand-token gradient (no stock image). */
function ArticleImage({ card }: { card: Pick<ArticleCard, "image"> }) {
  return card.image ? (
    // Library media comes from the API through /media (same origin).
    // eslint-disable-next-line @next/next/no-img-element
    <img src={card.image} alt="" loading="lazy" decoding="async" />
  ) : (
    <span className="tw-article-plain" aria-hidden>
      <BookOpen size={34} />
    </span>
  );
}

/** One guide card (home section and /guide). */
export function ArticleCardView({
  ctx,
  card,
  heading = "h3",
}: {
  ctx: Ctx;
  card: ArticleCard;
  heading?: "h2" | "h3";
}) {
  const Title = heading;
  return (
    <article className="tw-article">
      <Link
        href={card.href}
        className="tw-article-image"
        tabIndex={-1}
        aria-hidden
      >
        <ArticleImage card={card} />
        <span>
          <ArrowUpRight size={20} />
        </span>
      </Link>
      <div className="tw-article-meta">
        <span>{ctx.t("tw.guide.label")}</span>
        {card.publishedAt ? (
          <time dateTime={card.publishedAt}>
            {articleDate(card.publishedAt, ctx.locale)}
          </time>
        ) : null}
      </div>
      <Title>
        <Link href={card.href}>{card.title}</Link>
      </Title>
      {card.summary ? (
        <p className="tw-article-summary">{card.summary}</p>
      ) : null}
    </article>
  );
}

/**
 * Home guide: the three newest published articles in the visitor's
 * language (from the resolve answer); lib/sections.ts drops the section
 * when there is none.
 */
export function Guide({ ctx }: { ctx: Ctx }) {
  const { t } = ctx;
  const cards = homeArticleCards(ctx.site, ctx.locale);
  if (!cards.length) return null;
  return (
    <section className="tw-section tw-guide" aria-labelledby="guide-title">
      <div className="rt-container">
        <SectionHeading
          id="guide-title"
          eyebrow={t("tw.guide.eyebrow")}
          title={t("tw.guide.title")}
          action={
            <Link href="/guide" className="tw-text-button">
              {t("tw.guide.all")}
              <ArrowUpRight aria-hidden size={17} />
            </Link>
          }
        />
        <div className="tw-article-grid">
          {cards.map((card) => (
            <ArticleCardView key={card.href} ctx={ctx} card={card} />
          ))}
        </div>
      </div>
    </section>
  );
}

/** /guide: published articles of the visitor's language, 12 per page. */
export function GuideList({
  ctx,
  page,
  data,
}: {
  ctx: Ctx;
  page: number;
  data: ArticlePage | null;
}) {
  const { t } = ctx;
  const cards = toArticleCards(ctx.site, data?.items, ctx.locale);
  const pages = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1;
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("page.guide")}
        lead={t("tw.guide.lead")}
      />
      <PageSection>
        {cards.length ? (
          <>
            <div className="tw-article-grid tw-article-grid--page">
              {cards.map((card) => (
                <ArticleCardView
                  key={card.href}
                  ctx={ctx}
                  card={card}
                  heading="h2"
                />
              ))}
            </div>
            {pages > 1 ? (
              <nav className="tw-pager" aria-label={t("tw.guide.pages")}>
                {page > 1 ? (
                  <Link
                    href={page === 2 ? "/guide" : `/guide?page=${page - 1}`}
                    className="tw-text-button"
                    rel="prev"
                  >
                    <ArrowLeft aria-hidden size={15} />
                    {t("tw.guide.newer")}
                  </Link>
                ) : (
                  <span />
                )}
                <span className="tw-pager-count">
                  {t("tw.guide.page_of", { page, total: pages })}
                </span>
                {page < pages ? (
                  <Link
                    href={`/guide?page=${page + 1}`}
                    className="tw-text-button"
                    rel="next"
                  >
                    {t("tw.guide.older")}
                    <ArrowRight aria-hidden size={15} />
                  </Link>
                ) : (
                  <span />
                )}
              </nav>
            ) : null}
          </>
        ) : (
          <EmptyState
            title={t("tw.guide.empty")}
            text={t("tw.guide.empty_text")}
            action={
              <Link href="/" className="rt-button rt-button--primary">
                {t("nav.home")}
                <ArrowRight aria-hidden size={17} />
              </Link>
            }
          />
        )}
      </PageSection>
    </>
  );
}

/** /guide/{locale}/{slug}: one published article. */
export function ArticleDetail({
  ctx,
  article,
}: {
  ctx: Ctx;
  article: PublicArticle;
}) {
  const { t, site } = ctx;
  const cover = articleMediaUrl(site, article.cover_url);
  const body = renderArticleBody(article.body, {
    mediaUrl: (id) =>
      siteMediaUrl({ site_uuid: site.site_uuid, media: article.media }, id),
  });
  const tags = (Array.isArray(article.tags) ? article.tags : [])
    .filter((x) => typeof x === "string" && x.trim())
    .slice(0, 10);
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={article.title}
        trail={[{ href: "/guide", label: t("page.guide") }]}
      />
      <PageSection>
        <article className="tw-article-page" lang={article.locale}>
          {cover ? (
            <div className="tw-article-cover">
              {/* eslint-disable-next-line @next/next/no-img-element -- API-owned library media via /media. */}
              <img src={cover} alt="" decoding="async" />
            </div>
          ) : null}
          <p className="tw-mini-label">
            {t("tw.guide.label")}
            {article.published_at ? (
              <>
                {" · "}
                <time dateTime={article.published_at}>
                  {articleDate(article.published_at, ctx.locale)}
                </time>
              </>
            ) : null}
          </p>
          {article.summary ? (
            <p className="tw-article-lead">{article.summary}</p>
          ) : null}
          <div className="tw-prose tw-article-body">{body}</div>
          {tags.length ? (
            <ul className="tw-article-tags" aria-label={t("tw.guide.tags")}>
              {tags.map((tag) => (
                <li key={tag}>{tag}</li>
              ))}
            </ul>
          ) : null}
          <Link href="/guide" className="tw-text-button">
            <ArrowLeft aria-hidden size={15} />
            {t("tw.guide.all")}
          </Link>
        </article>
      </PageSection>
    </>
  );
}
