import assert from "node:assert/strict";
import { test } from "node:test";

import { createElement, Fragment } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { articleHref, renderArticleBody } from "./rich-text.ts";

const MEDIA = "123e4567-e89b-12d3-a456-426614174000";
const html = (body: unknown, known: Record<string, string> = {}) =>
  renderToStaticMarkup(
    createElement(
      Fragment,
      null,
      ...renderArticleBody(body, { mediaUrl: (id) => known[id] ?? null }),
    ),
  );
const p = (...children: unknown[]) => ({ type: "p", children });

test("allowlisted blocks render as their elements", () => {
  assert.equal(
    html([
      { type: "h2", children: [{ text: "Title" }] },
      { type: "h3", children: [{ text: "Sub" }] },
      { type: "h4", children: [{ text: "Small" }] },
      p({ text: "Text" }),
      { type: "blockquote", children: [{ text: "Quote" }] },
      { type: "hr", children: [{ text: "" }] },
    ]),
    "<h2>Title</h2><h3>Sub</h3><h4>Small</h4><p>Text</p><blockquote>Quote</blockquote><hr/>",
  );
  assert.equal(
    html([
      {
        type: "ul",
        children: [
          {
            type: "li",
            children: [
              { type: "lic", children: [{ text: "One" }] },
              {
                type: "ol",
                children: [
                  {
                    type: "li",
                    children: [{ type: "lic", children: [{ text: "1a" }] }],
                  },
                ],
              },
            ],
          },
        ],
      },
    ]),
    "<ul><li>One<ol><li>1a</li></ol></li></ul>",
  );
  assert.equal(
    html([
      {
        type: "code_block",
        children: [
          { type: "code_line", children: [{ text: "a < b" }] },
          { type: "code_line", children: [{ text: "c" }] },
        ],
      },
    ]),
    "<pre><code>a &lt; b\nc</code></pre>",
  );
});

test("marks wrap text and text is escaped", () => {
  assert.equal(
    html([
      p(
        { text: "<b>x</b>", bold: true },
        { text: "i", italic: true, underline: true },
        { text: "s", strikethrough: true, code: true },
        { text: "n", bold: "yes" },
      ),
    ]),
    "<p><strong>&lt;b&gt;x&lt;/b&gt;</strong><u><em>i</em></u><s><code>s</code></s>n</p>",
  );
});

test("unknown nodes, keys and misplaced blocks are dropped", () => {
  assert.equal(
    html([
      { type: "script", children: [{ text: "alert(1)" }] },
      { type: "iframe", src: "https://evil", children: [{ text: "" }] },
      { type: "h1", children: [{ text: "H1" }] },
      p({ text: "a" }, { type: "h2", children: [{ text: "nested" }] }),
      { type: "p", children: [{ text: 5 }] },
      "loose",
      null,
      [p({ text: "array" })],
    ]),
    "<p>a</p>",
  );
  assert.equal(html("not an array"), "");
  assert.equal(html({ type: "p" }), "");
});

test("links are validated again; external ones get rel", () => {
  const link = (href: unknown) =>
    html([p({ type: "a", href, children: [{ text: "go" }] })]);
  assert.equal(link("/vehicles"), '<p><a href="/vehicles">go</a></p>');
  assert.equal(link("guide/x?y=1#z"), '<p><a href="guide/x?y=1#z">go</a></p>');
  assert.equal(
    link("https://example.com/a"),
    '<p><a href="https://example.com/a" rel="noopener noreferrer">go</a></p>',
  );
  assert.equal(
    link("mailto:info@example.com"),
    '<p><a href="mailto:info@example.com">go</a></p>',
  );
  for (const bad of [
    "javascript:alert(1)",
    "JavaScript:alert(1)",
    "data:text/html,x",
    "http://example.com",
    "//evil.com",
    "/%2F%2Fevil.com",
    "/\\evil",
    "https://user:pw@example.com",
    "https:evil.com",
    "mailto:a@b.com?bcc=x@y.z",
    " /x",
    "a:b",
    "?q",
    "",
    42,
  ]) {
    assert.equal(link(bad), "<p>go</p>", String(bad));
  }
  // No link inside a link.
  assert.equal(
    html([
      p({
        type: "a",
        href: "/a",
        children: [{ type: "a", href: "/b", children: [{ text: "x" }] }],
      }),
    ]),
    '<p><a href="/a">x</a></p>',
  );
  assert.equal(articleHref("https://example.com")?.external, true);
  assert.equal(articleHref("/x")?.external, false);
});

test("images need a resolved media URL", () => {
  const img = {
    type: "img",
    media_uuid: MEDIA,
    alt: "Car",
    children: [{ text: "" }],
  };
  assert.equal(
    html([img], { [MEDIA]: "/media/site/media/s/m" }),
    '<figure class="rt-article-figure"><img src="/media/site/media/s/m" alt="Car" loading="lazy" decoding="async"/></figure>',
  );
  assert.equal(html([img]), "");
  assert.equal(
    html([{ ...img, media_uuid: undefined, url: "https://evil/x.png" }]),
    "",
  );
});

test("depth and node limits stop rendering", () => {
  let deep: unknown = { text: "deep" };
  for (let i = 0; i < 40; i++) deep = { type: "blockquote", children: [deep] };
  assert.equal(html([deep]), "");
  const many = Array.from({ length: 6000 }, () => p({ text: "x" }));
  const out = html(many);
  assert.ok(out.split("<p>").length - 1 < 6000);
});
