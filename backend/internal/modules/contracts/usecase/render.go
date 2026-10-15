package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/microcosm-cc/bluemonday"
)

// tokenRE matches a placeholder token: {{group.key}} (spaces allowed).
var tokenRE = regexp.MustCompile(`\{\{\s*([a-z_]+\.[a-z_]+)\s*\}\}`)

// policy sanitizes template HTML: the rich text the editor produces
// (paragraphs, headings, lists, tables, links, inline styles of the editor,
// data: images); scripts, event handlers and remote loads are dropped.
var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").Globally()
	p.AllowStyles(
		"text-align", "font-weight", "font-style", "text-decoration", "color", "background-color", "font-size",
		"font-family", "white-space", "margin-left", "padding-left", "padding-inline-start", "width", "border",
		"vertical-align",
	).Globally()
	p.AllowDataURIImages()
	p.AllowElements("u", "s", "mark", "span", "figure", "figcaption", "colgroup", "col")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	return p
}()

// Sanitize cleans template HTML (saved and previewed bodies).
func Sanitize(body string) string { return policy.Sanitize(body) }

// Tokens returns the distinct placeholder keys of a body (sorted) and the
// ones the catalog does not know.
func Tokens(body string) (known, unknown []string) {
	seen := map[string]bool{}
	known, unknown = []string{}, []string{}
	for _, m := range tokenRE.FindAllStringSubmatch(body, -1) {
		key := m[1]
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, ok := model.LookupPlaceholder(key); ok {
			known = append(known, key)
		} else {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(known)
	sort.Strings(unknown)
	return known, unknown
}

// fill replaces every token with its value (text escaped, HTML blocks as
// built by the server). Unknown tokens and empty values are marked so a
// preview shows them; a final document never contains unknown tokens
// (templates are validated on save).
func fill(body string, values map[string]string) (string, []string) {
	empty := map[string]bool{}
	out := tokenRE.ReplaceAllStringFunc(body, func(tok string) string {
		key := tokenRE.FindStringSubmatch(tok)[1]
		p, ok := model.LookupPlaceholder(key)
		if !ok {
			return `<span class="ph-unknown">` + html.EscapeString(tok) + `</span>`
		}
		v, has := values[key]
		if !has || strings.TrimSpace(v) == "" {
			empty[key] = true
			return `<span class="ph-empty">—</span>`
		}
		if p.HTML {
			return v
		}
		return html.EscapeString(v)
	})
	keys := make([]string, 0, len(empty))
	for k := range empty {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return out, keys
}

// kindTitles are the document titles (Turkish first: documents are printed
// for the customer).
var kindTitles = map[string]string{
	model.KindRentalContract: "Araç Kiralama Sözleşmesi",
	model.KindHandoverForm:   "Araç Teslim Formu",
	model.KindReturnForm:     "Araç İade Formu",
	model.KindTerms:          "Kiralama Koşulları",
}

const documentCSS = `@page{size:A4;margin:15mm}
*{box-sizing:border-box}
body{font-family:"DejaVu Sans","Noto Sans",Arial,sans-serif;font-size:10.5pt;line-height:1.45;color:#111;margin:0}
.doc-header{display:flex;justify-content:space-between;gap:16px;border-bottom:1px solid #999;padding-bottom:6px;margin-bottom:14px}
.doc-header .doc-meta{text-align:right;font-size:9pt;color:#333}
h1,h2,h3{margin:0.6em 0 0.3em}
p{margin:0.35em 0}
table{border-collapse:collapse;width:100%;margin:6px 0}
td,th{border:1px solid #bbb;padding:4px 6px;text-align:left;vertical-align:top}
th{background:#f3f3f3}
.doc-footer{margin-top:24px;font-size:8.5pt;color:#555;border-top:1px solid #ccc;padding-top:6px}
.ph-empty{color:#888}
.ph-unknown{background:#fde68a}`

// documentHTML wraps a filled body into the printable document.
func documentHTML(kind, company, reference string, version int32, at time.Time, body string) string {
	title := kindTitles[kind]
	date := rentaltime.Local(at).Format("02.01.2006")
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="tr"><head><meta charset="utf-8"><title>`)
	b.WriteString(html.EscapeString(title + " " + reference))
	b.WriteString(`</title><style>`)
	b.WriteString(documentCSS)
	b.WriteString(`</style></head><body><header class="doc-header"><div><strong>`)
	b.WriteString(html.EscapeString(company))
	b.WriteString(`</strong></div><div class="doc-meta"><strong>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</strong><br>`)
	b.WriteString(html.EscapeString(reference))
	b.WriteString(` · Sürüm `)
	b.WriteString(strconv.Itoa(int(version)))
	b.WriteString(`<br>`)
	b.WriteString(date)
	b.WriteString(`</div></header><main>`)
	b.WriteString(body)
	b.WriteString(`</main><footer class="doc-footer">`)
	b.WriteString(html.EscapeString(title + " · " + reference + " · Sürüm " + strconv.Itoa(int(version)) + " · " + date))
	b.WriteString(`</footer></body></html>`)
	return b.String()
}

// contentHash is the SHA-256 (hex) of the rendered document.
func contentHash(doc string) string {
	sum := sha256.Sum256([]byte(doc))
	return hex.EncodeToString(sum[:])
}

// table builds an HTML table (escaped cells).
func table(head []string, rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<table><thead><tr>")
	for _, h := range head {
		b.WriteString("<th>" + html.EscapeString(h) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for _, r := range rows {
		b.WriteString("<tr>")
		for _, c := range r {
			b.WriteString("<td>" + html.EscapeString(c) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}
