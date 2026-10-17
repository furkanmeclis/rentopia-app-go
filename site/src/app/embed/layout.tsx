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
    <div data-widget-root className="rt-embed">
      <Suspense>
        <WidgetBridge domains={site.config.widget?.allowed_domains ?? []} />
      </Suspense>
      {children}
      <footer className="rt-embed-credit">Rentopia</footer>
    </div>
  );
}
