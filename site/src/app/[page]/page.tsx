import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { Container, PageTitle, Paragraphs } from "@/components/site/text";
import { Alert, Card } from "@/components/ui/card";
import { getCtx, type Ctx } from "@/lib/server";
import type { MessageKey } from "@/messages/tr";

/** Content pages of the site settings (about, FAQ, contact, terms, KVKK). */
const PAGES = ["about", "faq", "contact", "terms", "privacy"] as const;
type PageName = (typeof PAGES)[number];

const isPage = (v: string): v is PageName =>
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

function Contact({ ctx }: { ctx: Ctx }) {
  const c = ctx.site.config.contact;
  const rows = [
    c.phone && {
      label: ctx.t("contact.phone"),
      value: c.phone,
      href: `tel:${c.phone.replace(/[^+0-9]/g, "")}`,
    },
    c.whatsapp && {
      label: ctx.t("contact.whatsapp"),
      value: c.whatsapp,
      href: `https://wa.me/${c.whatsapp.replace(/[^0-9]/g, "")}`,
    },
    c.email && {
      label: ctx.t("contact.email"),
      value: c.email,
      href: `mailto:${c.email}`,
    },
    c.address && {
      label: ctx.t("contact.address"),
      value: c.address,
      href: "",
    },
  ].filter(Boolean) as { label: string; value: string; href: string }[];
  if (!rows.length) return <Alert>{ctx.t("page.empty")}</Alert>;
  return (
    <Card>
      <dl className="grid gap-4 sm:grid-cols-2">
        {rows.map((r) => (
          <div key={r.label}>
            <dt className="text-muted-foreground text-sm">{r.label}</dt>
            <dd className="whitespace-pre-line">
              {r.href ? (
                <a
                  href={r.href}
                  className="text-primary hover:underline"
                  rel="noopener noreferrer"
                >
                  {r.value}
                </a>
              ) : (
                r.value
              )}
            </dd>
          </div>
        ))}
      </dl>
    </Card>
  );
}

export default async function ContentPage({
  params,
}: {
  params: Promise<{ page: string }>;
}) {
  const { page } = await params;
  if (!isPage(page)) notFound();
  const ctx = await getCtx();
  const { t, content } = ctx;

  let body: React.ReactNode;
  switch (page) {
    case "faq":
      body = content.faq.length ? (
        <div className="space-y-3">
          {content.faq.map((f, i) => (
            <details
              key={i}
              className="border-border rounded-[var(--radius)] border p-4"
            >
              <summary className="cursor-pointer font-medium">
                {f.question}
              </summary>
              <Paragraphs
                text={f.answer}
                className="text-muted-foreground pt-3"
              />
            </details>
          ))}
        </div>
      ) : (
        <Alert>{t("page.empty")}</Alert>
      );
      break;
    case "contact":
      body = <Contact ctx={ctx} />;
      break;
    default: {
      const text = content[page];
      body = text ? (
        <Paragraphs text={text} />
      ) : (
        <Alert>{t("page.empty")}</Alert>
      );
    }
  }

  return (
    <Container className="max-w-3xl space-y-6 py-10">
      <PageTitle>{t(`page.${page}` as MessageKey)}</PageTitle>
      {body}
    </Container>
  );
}
