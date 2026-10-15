"use client";

import { cn } from "@/lib/utils";

/** Print-like frame around a rendered body (fonts and table borders). */
const FRAME_STYLE = `<style>
html,body{margin:0;background:#fff;color:#111;}
body{font-family:"DejaVu Sans",Arial,sans-serif;font-size:12px;line-height:1.5;padding:24px;}
table{border-collapse:collapse;width:100%;}
th,td{border:1px solid #999;padding:4px 6px;text-align:left;vertical-align:top;}
img{max-width:100%;}
.ph-unknown{background:#fee2e2;color:#991b1b;}
</style>`;

/**
 * Rendered contract HTML in a fully sandboxed iframe (no scripts, no
 * same-origin access): what the PDF will show.
 */
export function HtmlPreview({
  html,
  title,
  className,
}: {
  html: string;
  title: string;
  className?: string;
}) {
  const doc = /<html[\s>]/i.test(html)
    ? html
    : `<!doctype html><html><head><meta charset="utf-8">${FRAME_STYLE}</head><body>${html}</body></html>`;
  return (
    <iframe
      title={title}
      sandbox=""
      srcDoc={doc}
      className={cn("h-[70vh] w-full rounded-md border bg-white", className)}
    />
  );
}
