import { Card } from "@/components/ui/card";
import { cn } from "@/lib/cn";
import { mediaUrl, type Ctx } from "@/lib/server";

import { Container } from "./text";

/**
 * The ready-made hero variants (site layout.hero): centered, split, image
 * (cover image behind the search) and minimal.
 */
export function Hero({
  ctx,
  children,
}: {
  ctx: Ctx;
  children: React.ReactNode;
}) {
  const { t, content, site } = ctx;
  const title = content.hero_title || t("hero.title");
  const subtitle = content.hero_subtitle || t("hero.subtitle");
  const cover = mediaUrl(site.assets.cover);
  const variant = site.config.layout.hero;

  const heading = (center: boolean, inverted = false) => (
    <div className={cn("space-y-3", center && "mx-auto max-w-2xl text-center")}>
      <h1 className="text-3xl font-bold tracking-tight sm:text-5xl">{title}</h1>
      <p
        className={cn(
          "text-lg",
          inverted ? "text-white/85" : "text-muted-foreground",
        )}
      >
        {subtitle}
      </p>
    </div>
  );

  if (variant === "split") {
    return (
      <section className="bg-muted">
        <Container className="grid items-center gap-8 py-12 lg:grid-cols-2 lg:py-20">
          {heading(false)}
          <Card className="p-6">{children}</Card>
        </Container>
      </section>
    );
  }
  if (variant === "image") {
    return (
      <section
        className="relative bg-cover bg-center"
        style={cover ? { backgroundImage: `url("${cover}")` } : undefined}
      >
        <div
          className={cn(
            "absolute inset-0",
            cover ? "bg-black/55" : "bg-primary",
          )}
          aria-hidden
        />
        <Container className="relative space-y-8 py-16 text-white lg:py-28">
          {heading(true, true)}
          <Card className="text-foreground mx-auto max-w-5xl p-6">
            {children}
          </Card>
        </Container>
      </section>
    );
  }
  if (variant === "minimal") {
    return (
      <section>
        <Container className="space-y-6 py-10">
          <h1 className="text-2xl font-bold tracking-tight sm:text-3xl">
            {title}
          </h1>
          {children}
        </Container>
      </section>
    );
  }
  return (
    <section className="from-primary/10 bg-gradient-to-b to-transparent">
      <Container className="space-y-8 py-14 lg:py-24">
        {heading(true)}
        <Card className="mx-auto max-w-5xl p-6">{children}</Card>
      </Container>
    </section>
  );
}
