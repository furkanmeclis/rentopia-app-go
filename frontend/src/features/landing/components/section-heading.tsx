import { cn } from "@/lib/utils";

/** Left-aligned section title with an optional lead paragraph. */
export function SectionHeading({
  id,
  title,
  lead,
  className,
}: {
  id: string;
  title: string;
  lead?: string;
  className?: string;
}) {
  return (
    <div className={cn("max-w-3xl", className)}>
      <h2
        id={id}
        className="text-3xl leading-[1.08] font-bold tracking-[-0.03em] text-balance sm:text-[2.75rem]"
      >
        {title}
      </h2>
      {lead ? (
        <p className="text-muted-foreground mt-5 max-w-[62ch] text-lg leading-relaxed text-pretty">
          {lead}
        </p>
      ) : null}
    </div>
  );
}
