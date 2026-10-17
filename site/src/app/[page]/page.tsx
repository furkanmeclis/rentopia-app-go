import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { getCtx } from "@/lib/server";
import { themeFor, type ContentPage, type Query } from "@/themes";
import type { MessageKey } from "@/messages/tr";

/** Content pages of the site settings (about, FAQ, contact, terms, KVKK). */
const PAGES = ["about", "faq", "contact", "terms", "privacy"] as const;

const isPage = (v: string): v is ContentPage =>
  (PAGES as readonly string[]).includes(v);

export async function generateMetadata({
  params,
}: {
  params: Promise<{ page: string }>;
}): Promise<Metadata> {
  const { page } = await params;
  if (!isPage(page)) return {};
  const { t } = await getCtx();
  return { title: t(`page.${page}` as MessageKey) };
}

export default async function ContentPageRoute({
  params,
  searchParams,
}: {
  params: Promise<{ page: string }>;
  searchParams: Promise<Query>;
}) {
  const { page } = await params;
  if (!isPage(page)) notFound();
  const ctx = await getCtx();
  const theme = themeFor(ctx.site);
  return <theme.Content ctx={ctx} page={page} query={await searchParams} />;
}
