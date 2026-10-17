import { Suspense } from "react";

import { WidgetBridge } from "@/components/site/widget-bridge";
import { getCtx } from "@/lib/server";

export default async function EmbedLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { site } = await getCtx();
  return (
    <div data-widget-root className="min-w-0">
      <Suspense>
        <WidgetBridge domains={site.config.widget?.allowed_domains ?? []} />
      </Suspense>
      {children}
      <footer className="text-muted-foreground px-4 py-3 text-center text-xs">
        Rentopia
      </footer>
    </div>
  );
}
