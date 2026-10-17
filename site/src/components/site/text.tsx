import { cn } from "@/lib/cn";

/**
 * Organization texts are plain text: blank lines start a paragraph, single
 * newlines are kept. Nothing is rendered as HTML.
 */
export function Paragraphs({
  text,
  className,
  limit,
}: {
  text: string;
  className?: string;
  /** Only the first N paragraphs (teasers). */
  limit?: number;
}) {
  const parts = text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
    .slice(0, limit ?? undefined);
  return (
    <div className={cn("rt-paragraphs", className)}>
      {parts.map((p, i) => (
        <p key={i}>{p}</p>
      ))}
    </div>
  );
}

export function PageTitle({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <h1 className={cn("rt-page-title", className)}>{children}</h1>;
}

/** The theme's content width (`rt-container`). */
export function Container({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn("rt-container", className)}>{children}</div>;
}
