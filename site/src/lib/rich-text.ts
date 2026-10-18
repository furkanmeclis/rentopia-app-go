import { createElement, Fragment, type ReactNode } from "react";

/**
 * Guide article bodies (TEC-785; pure: node --test imports it). The API
 * stores Plate JSON with a strict allowlist and validates it on every
 * write; the site renders it again with its own allowlist, as React
 * elements only (text is escaped by React, nothing is injected as HTML):
 *
 * - elements: p, h2–h4, blockquote, ul / ol / li / lic, hr, img, a,
 *   code_block / code_line; anything else is dropped with its content;
 * - marks: bold, italic, underline, strikethrough, code (booleans);
 * - links are checked again (relative path, https:// with a host and no
 *   credentials, or one mailto: address); external links get
 *   rel="noopener noreferrer"; an invalid link keeps only its text;
 * - images resolve their media UUID through the caller (the API's
 *   uuid → stream path map, served by the site's /media route); an image
 *   without a URL is dropped;
 * - at most 5000 nodes and 32 levels are rendered.
 */

export const MAX_NODES = 5000;
export const MAX_DEPTH = 32;

const SCHEME = /^([a-zA-Z][a-zA-Z0-9+.-]*):/;
const MAILBOX = /^[^\s@<>()[\]\\,;:"]+@[^\s@<>()[\]\\,;:"]+$/;

export type ArticleLink = { href: string; external: boolean };

/** The API's link rule, checked again before rendering an href. */
export function articleHref(raw: unknown): ArticleLink | null {
  if (typeof raw !== "string") return null;
  const href = raw;
  if (!href || href.length > 2048) return null;
  if (/[\\\s\u0000-\u001f\u007f]/.test(href)) return null;
  let decoded: string;
  try {
    decoded = decodeURIComponent(href);
  } catch {
    return null;
  }
  if (/[\\\r\n\u0000]/.test(decoded) || decoded.startsWith("//")) return null;
  const scheme = SCHEME.exec(href)?.[1]?.toLowerCase();
  if (scheme === "https") {
    if (!/^https:\/\//i.test(href)) return null;
    try {
      const u = new URL(href);
      if (!u.hostname || u.username || u.password) return null;
      return { href: u.toString(), external: true };
    } catch {
      return null;
    }
  }
  if (scheme === "mailto") {
    const address = href.slice("mailto:".length);
    return !/[?#]/.test(address) && MAILBOX.test(address)
      ? { href: `mailto:${address}`, external: false }
      : null;
  }
  if (scheme || href.startsWith("//")) return null;
  const path = href.split(/[?#]/)[0];
  if (!path || path.split("/")[0].includes(":")) return null;
  return { href, external: false };
}

type Node = Record<string, unknown>;

export type RenderOptions = {
  /** Same-origin URL of a library image, or null (the image is dropped). */
  mediaUrl: (uuid: string) => string | null;
};

const BLOCKS: Record<string, string> = {
  p: "p",
  h2: "h2",
  h3: "h3",
  h4: "h4",
  blockquote: "blockquote",
  ul: "ul",
  ol: "ol",
  li: "li",
};

/** Where a node sits: blocks may hold blocks, inline content only text. */
type Mode = "block" | "inline" | "link";

type State = { count: number; opts: RenderOptions };

function children(
  n: Node,
  state: State,
  depth: number,
  mode: Mode,
): ReactNode[] {
  const list = Array.isArray(n.children) ? n.children : [];
  const out: ReactNode[] = [];
  list.forEach((child, i) => {
    const el = renderNode(child, state, depth + 1, mode, i);
    if (el !== null) out.push(el);
  });
  return out;
}

function leaf(n: Node, key: number): ReactNode {
  const text = n.text as string;
  if (!text) return null;
  let el: ReactNode = text;
  if (n.code === true) el = createElement("code", null, el);
  if (n.bold === true) el = createElement("strong", null, el);
  if (n.italic === true) el = createElement("em", null, el);
  if (n.underline === true) el = createElement("u", null, el);
  if (n.strikethrough === true) el = createElement("s", null, el);
  return createElement(Fragment, { key }, el);
}

function codeText(n: Node, state: State, depth: number): string {
  if (depth > MAX_DEPTH || ++state.count > MAX_NODES) return "";
  if (typeof n.text === "string") return n.text;
  const list = Array.isArray(n.children) ? n.children : [];
  return list
    .map((c) =>
      c && typeof c === "object" ? codeText(c as Node, state, depth + 1) : "",
    )
    .join("");
}

function renderNode(
  raw: unknown,
  state: State,
  depth: number,
  mode: Mode,
  key: number,
): ReactNode {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  if (depth > MAX_DEPTH || ++state.count > MAX_NODES) return null;
  const n = raw as Node;
  if ("text" in n) return typeof n.text === "string" ? leaf(n, key) : null;
  const type = typeof n.type === "string" ? n.type : "";

  if (type === "a") {
    if (mode === "block") return null;
    const kids = children(n, state, depth, "link");
    const link = mode === "link" ? null : articleHref(n.href);
    if (!link) return createElement(Fragment, { key }, ...kids);
    return createElement(
      "a",
      {
        key,
        href: link.href,
        ...(link.external ? { rel: "noopener noreferrer" } : {}),
      },
      ...kids,
    );
  }
  // Everything below is a block: never inside a paragraph or a link.
  if (mode !== "block") return null;

  switch (type) {
    case "p":
    case "h2":
    case "h3":
    case "h4": {
      const kids = children(n, state, depth, "inline");
      return kids.length ? createElement(BLOCKS[type], { key }, ...kids) : null;
    }
    case "blockquote":
    case "ul":
    case "ol": {
      // Quotes and lists hold blocks; loose text in a quote is a paragraph.
      const kids =
        type === "blockquote" && hasText(n)
          ? children(n, state, depth, "inline")
          : children(n, state, depth, "block");
      return kids.length ? createElement(BLOCKS[type], { key }, ...kids) : null;
    }
    case "li": {
      const kids = hasText(n)
        ? children(n, state, depth, "inline")
        : children(n, state, depth, "block");
      return createElement("li", { key }, ...kids);
    }
    case "lic":
      // A list item's text line.
      return createElement(
        Fragment,
        { key },
        ...children(n, state, depth, "inline"),
      );
    case "hr":
      return createElement("hr", { key });
    case "img": {
      const id = typeof n.media_uuid === "string" ? n.media_uuid : "";
      const src = id ? state.opts.mediaUrl(id) : null;
      if (!src) return null;
      const alt = typeof n.alt === "string" ? n.alt.slice(0, 300) : "";
      return createElement(
        "figure",
        { key, className: "rt-article-figure" },
        createElement("img", { src, alt, loading: "lazy", decoding: "async" }),
      );
    }
    case "code_block": {
      const lines = (Array.isArray(n.children) ? n.children : [])
        .filter(
          (c): c is Node =>
            Boolean(c) &&
            typeof c === "object" &&
            (c as Node).type === "code_line",
        )
        .map((c) => codeText(c, state, depth + 1));
      return createElement(
        "pre",
        { key },
        createElement("code", null, lines.join("\n")),
      );
    }
    default:
      return null;
  }
}

function hasText(n: Node): boolean {
  const list = Array.isArray(n.children) ? n.children : [];
  return list.some(
    (c) =>
      c &&
      typeof c === "object" &&
      ("text" in (c as Node) || (c as Node).type === "a"),
  );
}

/** The body as React nodes (an invalid body renders nothing). */
export function renderArticleBody(
  body: unknown,
  opts: RenderOptions,
): ReactNode[] {
  const state: State = { count: 0, opts };
  const list = Array.isArray(body) ? body : [];
  const out: ReactNode[] = [];
  list.forEach((n, i) => {
    const el = renderNode(n, state, 1, "block", i);
    if (el !== null) out.push(el);
  });
  return out;
}
