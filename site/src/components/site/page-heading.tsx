import { ChevronRight } from "lucide-react";
import Link from "next/link";

import { embedPath } from "@/lib/embed";
import type { Ctx } from "@/lib/server";

import { Container } from "./text";

/**
 * The heading band of every sub page: breadcrumb, title and an optional
 * lead. In the embedded widget it is only the title (no site navigation).
 */
export function PageHeading({
  ctx,
  title,
  lead,
  trail = [],
}: {
  ctx: Ctx;
  title: string;
  lead?: string;
  /** Intermediate breadcrumb steps between home and this page. */
  trail?: { href: string; label: string }[];
}) {
  if (ctx.embed) {
    return (
      <div className="rt-embed-heading">
        <h1>{title}</h1>
        {lead ? <p>{lead}</p> : null}
      </div>
    );
  }
  return (
    <section className="rt-page-heading">
      <Container>
        <nav className="rt-breadcrumb" aria-label={ctx.t("nav.breadcrumb")}>
          <Link href={embedPath("/", ctx.embed)}>{ctx.t("nav.home")}</Link>
          {trail.map((step) => (
            <span key={step.href} className="rt-breadcrumb-step">
              <ChevronRight aria-hidden size={13} />
              <Link href={step.href}>{step.label}</Link>
            </span>
          ))}
          <span className="rt-breadcrumb-step">
            <ChevronRight aria-hidden size={13} />
            <span aria-current="page">{title}</span>
          </span>
        </nav>
        <h1>{title}</h1>
        {lead ? <p>{lead}</p> : null}
      </Container>
    </section>
  );
}

/** The padded body of a sub page. */
export function PageSection({
  children,
  className,
  narrow = false,
}: {
  children: React.ReactNode;
  className?: string;
  narrow?: boolean;
}) {
  return (
    <section
      className={["rt-page-section", className].filter(Boolean).join(" ")}
    >
      <Container className={narrow ? "rt-narrow" : undefined}>
        {children}
      </Container>
    </section>
  );
}

/** Centered empty / not-found state. */
export function EmptyState({
  title,
  text,
  action,
  icon,
}: {
  title: string;
  text?: string;
  action?: React.ReactNode;
  icon?: React.ReactNode;
}) {
  return (
    <div className="rt-empty">
      {icon}
      <h2>{title}</h2>
      {text ? <p>{text}</p> : null}
      {action}
    </div>
  );
}
