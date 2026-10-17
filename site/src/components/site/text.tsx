import { cn } from "@/lib/cn";

/**
 * Organization texts are plain text: blank lines start a paragraph, single
 * newlines are kept. Nothing is rendered as HTML.
 */
export function Paragraphs({
  text,
  className,
}: {
  text: string;
  className?: string;
}) {
  const parts = text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean);
  return (
    <div className={cn("space-y-4 leading-relaxed", className)}>
      {parts.map((p, i) => (
        <p key={i} className="whitespace-pre-line">
          {p}
        </p>
      ))}
    </div>
  );
}

export function PageTitle({ children }: { children: React.ReactNode }) {
  return (
    <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">
      {children}
    </h1>
  );
}

export function Container({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("mx-auto w-full max-w-6xl px-4 sm:px-6", className)}>
      {children}
    </div>
  );
}
