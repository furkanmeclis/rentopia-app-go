/**
 * Article body ↔ Plate value (TEC-785). The API accepts Plate JSON with a
 * strict allowlist (docs/design/branded-site.md §13): elements `p`, `h2`–`h4`,
 * `blockquote`, `ul`, `ol`, `li`, `lic`, `hr`, `img`, `a`, `code_block`,
 * `code_line`; text leaves with `text` and boolean `bold`, `italic`,
 * `underline`, `strikethrough`, `code`; images carry `media_uuid` + `alt`,
 * links `href`. Unknown keys are rejected, so everything the editor adds
 * (ids, `url`, `target`, `lang`, other plugins' nodes) is dropped here
 * before saving.
 */

export const ARTICLE_BLOCK_TYPES = [
  "p",
  "h2",
  "h3",
  "h4",
  "blockquote",
  "ul",
  "ol",
  "li",
  "lic",
  "hr",
  "img",
  "a",
  "code_block",
  "code_line",
] as const;
export type ArticleNodeType = (typeof ARTICLE_BLOCK_TYPES)[number];

export const ARTICLE_MARKS = [
  "bold",
  "italic",
  "underline",
  "strikethrough",
  "code",
] as const;
export type ArticleMark = (typeof ARTICLE_MARKS)[number];

export const ARTICLE_LIMITS = {
  bytes: 256 * 1024,
  nodes: 5000,
  depth: 32,
  href: 2048,
  alt: 300,
} as const;

export type ArticleText = { text: string } & Partial<
  Record<ArticleMark, boolean>
>;
export type ArticleElement =
  | { type: Exclude<ArticleNodeType, "img" | "a">; children: ArticleNode[] }
  | { type: "img"; media_uuid: string; alt: string; children: ArticleNode[] }
  | { type: "a"; href: string; children: ArticleNode[] };
export type ArticleNode = ArticleText | ArticleElement;

/** Plate's loose node shape (the editor value). */
export type PlateNode = { [key: string]: unknown };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const SCHEME = /^([a-zA-Z][a-zA-Z0-9+.-]*):/;
const MAILBOX = /^[^\s@<>()[\]\\,;:"]+@[^\s@<>()[\]\\,;:"]+$/;

/**
 * The API's link rule: a relative path (incl. root-relative), an `https:`
 * URL with a host and no credentials, or a single `mailto:` address.
 * Protocol-relative, `javascript:`, `data:`, `http:`, backslashes,
 * whitespace and control characters are rejected.
 */
export function validArticleHref(raw: string): boolean {
  const href = raw;
  if (!href || href.length > ARTICLE_LIMITS.href) return false;
  if (/[\\\s\u0000-\u001f\u007f]/.test(href)) return false;
  let decoded: string;
  try {
    decoded = decodeURIComponent(href);
  } catch {
    return false;
  }
  if (/[\\\r\n\u0000]/.test(decoded) || decoded.startsWith("//")) return false;
  const scheme = SCHEME.exec(href)?.[1]?.toLowerCase();
  if (scheme === "https") {
    if (!/^https:\/\//i.test(href)) return false;
    try {
      const u = new URL(href);
      return Boolean(u.hostname) && !u.username && !u.password;
    } catch {
      return false;
    }
  }
  if (scheme === "mailto") {
    const address = href.slice("mailto:".length);
    return !/[?#]/.test(address) && MAILBOX.test(address);
  }
  if (scheme) return false;
  if (href.startsWith("//")) return false;
  const path = href.split(/[?#]/)[0];
  if (!path) return false;
  // A colon in the first segment would read as a scheme.
  return !path.split("/")[0].includes(":");
}

/** Normalizes what a person typed into a link field ("" when empty). */
export function normalizeHrefInput(raw: string): string {
  const v = raw.trim();
  if (!v) return "";
  if (/^[^\s@/:]+@[^\s@/:]+\.[^\s@/:]+$/.test(v)) return `mailto:${v}`;
  if (/^www\./i.test(v)) return `https://${v}`;
  return v;
}

export type BodyIssue =
  | { kind: "invalid_link"; path: string; href: string }
  | { kind: "image_without_media"; path: string };

export type BodyStats = { nodes: number; depth: number; bytes: number };

const isText = (n: PlateNode): boolean => typeof n.text === "string";

function textLeaf(n: PlateNode): ArticleText {
  const out: ArticleText = {
    text: String(n.text ?? "").replace(/\u0000/g, ""),
  };
  for (const mark of ARTICLE_MARKS) if (n[mark] === true) out[mark] = true;
  return out;
}

const EMPTY: ArticleText[] = [{ text: "" }];

function serializeChildren(
  children: unknown,
  path: string,
  issues: BodyIssue[],
): ArticleNode[] {
  const out: ArticleNode[] = [];
  const list = Array.isArray(children) ? children : [];
  list.forEach((child, i) => {
    if (!child || typeof child !== "object") return;
    out.push(...serializeNode(child as PlateNode, `${path}.${i}`, issues));
  });
  return out;
}

/** One editor node → zero or more allowlisted nodes (unknown → unwrapped). */
function serializeNode(
  n: PlateNode,
  path: string,
  issues: BodyIssue[],
): ArticleNode[] {
  if (isText(n)) return [textLeaf(n)];
  const type = String(n.type ?? "");
  const kids = serializeChildren(n.children, `${path}.children`, issues);
  const children = kids.length ? kids : EMPTY;
  switch (type) {
    case "img": {
      const media = typeof n.media_uuid === "string" ? n.media_uuid : "";
      if (!UUID.test(media)) {
        issues.push({ kind: "image_without_media", path });
        return [];
      }
      const alt = typeof n.alt === "string" ? n.alt : "";
      return [
        {
          type: "img",
          media_uuid: media,
          alt: plainText(alt).slice(0, ARTICLE_LIMITS.alt),
          children: [{ text: "" }],
        },
      ];
    }
    case "a": {
      const raw =
        typeof n.href === "string"
          ? n.href
          : typeof n.url === "string"
            ? n.url
            : "";
      const href = raw.trim();
      if (!validArticleHref(href)) {
        issues.push({ kind: "invalid_link", path, href });
      }
      return [{ type: "a", href, children }];
    }
    case "hr":
      return [{ type: "hr", children: [{ text: "" }] }];
    default:
      if ((ARTICLE_BLOCK_TYPES as readonly string[]).includes(type)) {
        return [
          {
            type: type as Exclude<ArticleNodeType, "img" | "a">,
            children,
          },
        ];
      }
      // Another plugin's node (paste): keep its content, drop the node.
      return kids;
  }
}

/** Plain text rule of the API (no markup or control characters). */
export function plainText(value: string): string {
  return value
    .replace(/[<>]/g, "")
    .replace(/[\u0000-\u0008\u000b-\u001f\u007f]/g, "");
}

function isEmptyParagraph(n: ArticleNode): boolean {
  return (
    "type" in n &&
    n.type === "p" &&
    n.children.every((c) => "text" in c && c.text.trim() === "")
  );
}

/**
 * The editor value as the API body: allowlisted nodes only, top-level
 * loose text wrapped in paragraphs, and an empty document as `[]`.
 */
export function serializeBody(value: unknown): {
  nodes: ArticleNode[];
  issues: BodyIssue[];
} {
  const issues: BodyIssue[] = [];
  const top = Array.isArray(value) ? value : [];
  const nodes: ArticleNode[] = [];
  let loose: ArticleNode[] = [];
  const flush = () => {
    if (loose.length) nodes.push({ type: "p", children: loose });
    loose = [];
  };
  top.forEach((child, i) => {
    if (!child || typeof child !== "object") return;
    for (const n of serializeNode(child as PlateNode, String(i), issues)) {
      if ("text" in n || n.type === "a") {
        loose.push(n);
      } else {
        flush();
        nodes.push(n);
      }
    }
  });
  flush();
  // Trailing empty paragraphs (the editor always keeps one) are not content.
  while (nodes.length && isEmptyParagraph(nodes[nodes.length - 1])) {
    nodes.pop();
  }
  return { nodes, issues };
}

/** The API body as an editor value (`a.href` → Plate's `url`). */
export function deserializeBody(nodes: unknown): PlateNode[] {
  const convert = (n: unknown): PlateNode | null => {
    if (!n || typeof n !== "object") return null;
    const node = n as PlateNode;
    if (isText(node)) return { ...node };
    const children = (Array.isArray(node.children) ? node.children : [])
      .map(convert)
      .filter((c): c is PlateNode => c !== null);
    const out: PlateNode = {
      ...node,
      children: children.length ? children : [{ text: "" }],
    };
    if (node.type === "a") {
      out.url = node.href;
      delete out.href;
    }
    return out;
  };
  const list = (Array.isArray(nodes) ? nodes : [])
    .map(convert)
    .filter((c): c is PlateNode => c !== null);
  return list.length ? list : [{ type: "p", children: [{ text: "" }] }];
}

/** Node count (text leaves included), nesting depth and UTF-8 JSON size. */
export function bodyStats(nodes: ArticleNode[]): BodyStats {
  let count = 0;
  let depth = 0;
  const walk = (list: ArticleNode[], level: number) => {
    for (const n of list) {
      count += 1;
      depth = Math.max(depth, level);
      if ("children" in n) walk(n.children, level + 1);
    }
  };
  walk(nodes, 1);
  const bytes = new TextEncoder().encode(JSON.stringify(nodes)).length;
  return { nodes: count, depth, bytes };
}

export function overLimits(stats: BodyStats): boolean {
  return (
    stats.nodes > ARTICLE_LIMITS.nodes ||
    stats.depth > ARTICLE_LIMITS.depth ||
    stats.bytes > ARTICLE_LIMITS.bytes
  );
}

/** Whether a body has visible content (text or an image / rule). */
export function bodyHasContent(nodes: ArticleNode[]): boolean {
  const walk = (list: ArticleNode[]): boolean =>
    list.some((n) =>
      "text" in n
        ? n.text.trim() !== ""
        : n.type === "img" || n.type === "hr" || walk(n.children),
    );
  return walk(nodes);
}
