package usecase

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
)

const MaxArticleBodyBytes = 256 << 10
const maxArticleNodes = 5000
const maxArticleDepth = 32

var articleSlug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var reservedArticleSlugs = strings.Split("api,admin,platform,t,media,assets,lang,preview,embed,book,search,reservation,lookup,vehicles,branches,about,faq,contact,terms,privacy,corporate,franchise,campaigns,guide,articles,sitemap,sitemap.xml,robots,robots.txt,www,app", ",")
var articleNodeTypes = []string{"p", "h2", "h3", "h4", "blockquote", "ul", "ol", "li", "lic", "hr", "img", "a", "code_block", "code_line"}
var articleMarks = []string{"bold", "italic", "underline", "strikethrough", "code"}

func articlePlainText(ve *ValidationError, field, value string, limit int) {
	checkLen(ve, field, value, limit)
	if strings.ContainsAny(value, "<>") || strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
		ve.add(field, "plain_text", "plain text without markup or control characters required")
	}
}

func articleTextMap(ve *ValidationError, field string, value map[string]string, limit int) map[string]string {
	out := localizedText(ve, field, value, limit)
	for _, loc := range model.Locales {
		articlePlainText(ve, field+"."+loc, out[loc], limit)
	}
	return out
}

func validArticleHref(href string) bool {
	if href == "" || strings.ContainsAny(href, "\\") || strings.IndexFunc(href, unicode.IsSpace) >= 0 || strings.IndexFunc(href, unicode.IsControl) >= 0 {
		return false
	}
	decoded, err := url.PathUnescape(href)
	if err != nil || strings.ContainsAny(decoded, "\\\r\n\x00") || strings.HasPrefix(decoded, "//") {
		return false
	}
	u, err := url.Parse(href)
	if err != nil || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Hostname() != ""
	case "mailto":
		if u.RawQuery != "" || u.Fragment != "" {
			return false
		}
		a, err := mail.ParseAddress(u.Opaque)
		return err == nil && a.Address == u.Opaque
	case "":
		return u.Host == "" && u.Opaque == "" && u.Path != "" && !strings.HasPrefix(href, "//")
	default:
		return false
	}
}

type articleBodyValidator struct {
	ve    *ValidationError
	nodes int
	media map[string]string
}

func (v *articleBodyValidator) node(value any, path string, depth int) {
	v.nodes++
	if v.nodes > maxArticleNodes {
		if v.nodes == maxArticleNodes+1 {
			v.ve.add(path, "too_many", "at most 5000 nodes per locale")
		}
		return
	}
	if depth > maxArticleDepth {
		v.ve.add(path, "too_deep", "at most 32 nested levels")
		return
	}
	node, ok := value.(map[string]any)
	if !ok {
		v.ve.add(path, "invalid", "node object required")
		return
	}
	text, leaf := node["text"]
	if leaf {
		if value, ok := text.(string); !ok {
			v.ve.add(path+".text", "invalid", "text must be a string")
		} else if strings.ContainsRune(value, 0) {
			v.ve.add(path+".text", "invalid", "text must not contain NUL")
		}
		for key, value := range node {
			if key == "text" {
				continue
			}
			if !slices.Contains(articleMarks, key) {
				v.ve.add(path+"."+key, "unknown", "unsupported leaf key or mark")
				continue
			}
			if _, ok := value.(bool); !ok {
				v.ve.add(path+"."+key, "invalid", "mark must be a boolean")
			}
		}
		return
	}
	typ, _ := node["type"].(string)
	if !slices.Contains(articleNodeTypes, typ) {
		v.ve.add(path+".type", "invalid", "unsupported node type")
	}
	for key := range node {
		allowed := key == "type" || key == "children" || (typ == "img" && (key == "media_uuid" || key == "alt")) || (typ == "a" && key == "href")
		if !allowed {
			v.ve.add(path+"."+key, "unknown", "unsupported node key")
		}
	}
	if typ == "a" {
		href, ok := node["href"].(string)
		if !ok || len(href) > 2048 || !validArticleHref(href) {
			v.ve.add(path+".href", "invalid", "relative path, HTTPS or mailto required")
		}
	}
	if typ == "img" {
		id, _ := node["media_uuid"].(string)
		mediaID(v.ve, path+".media_uuid", id)
		if id == "" {
			v.ve.add(path+".media_uuid", "required", "media UUID required")
		} else {
			v.media[path+".media_uuid"] = id
		}
		alt, ok := node["alt"].(string)
		if !ok {
			v.ve.add(path+".alt", "required", "plain text alt required")
		} else {
			articlePlainText(v.ve, path+".alt", alt, 300)
		}
	}
	children, ok := node["children"].([]any)
	if !ok || len(children) == 0 {
		v.ve.add(path+".children", "required", "nonempty children array required")
		return
	}
	for i, child := range children {
		if v.nodes > maxArticleNodes {
			break
		}
		v.node(child, fmt.Sprintf("%s.children.%d", path, i), depth+1)
	}
}

// ValidateArticle rejects unknown keys instead of persisting editor/plugin metadata.
// The returned references must also be resolved inside the organization before writing.
func ValidateArticle(in *model.ArticleInput, publishing bool) (map[string]string, error) {
	ve := &ValidationError{}
	in.Slug = articleTextMap(ve, "slug", in.Slug, 160)
	in.Title = articleTextMap(ve, "title", in.Title, 160)
	in.Summary = articleTextMap(ve, "summary", in.Summary, 1000)
	in.SEOTitle = articleTextMap(ve, "seo_title", in.SEOTitle, 70)
	in.SEODescription = articleTextMap(ve, "seo_description", in.SEODescription, 300)
	refs := map[string]string{}
	if in.CoverMediaUUID != nil {
		refs["cover_media_uuid"] = in.CoverMediaUUID.String()
		mediaID(ve, "cover_media_uuid", in.CoverMediaUUID.String())
	}
	if in.Body == nil {
		in.Body = map[string]json.RawMessage{}
	}
	for loc := range in.Body {
		if !slices.Contains(model.Locales, loc) {
			ve.add("body."+loc, "invalid", "unsupported locale")
		}
	}
	hasLocale := false
	for _, loc := range model.Locales {
		slug := in.Slug[loc]
		if slug != "" {
			hasLocale = true
			if !articleSlug.MatchString(slug) || slices.Contains(reservedArticleSlugs, slug) {
				ve.add("slug."+loc, "invalid", "lowercase a-z0-9- and non-reserved slug required")
			}
			if in.Title[loc] == "" {
				ve.add("title."+loc, "required", "title required for a localized slug")
			}
		} else if publishing && in.Title[loc] != "" {
			ve.add("slug."+loc, "required", "slug required for a localized title")
		}
		raw := in.Body[loc]
		if len(raw) == 0 {
			raw = json.RawMessage(`[]`)
			in.Body[loc] = raw
		}
		if len(raw) > MaxArticleBodyBytes {
			ve.add("body."+loc, "too_large", "at most 256 KB per locale")
			continue
		}
		nodes := []any{}
		if err := json.Unmarshal(raw, &nodes); err != nil || nodes == nil {
			ve.add("body."+loc, "invalid", "Plate value must be an array of nodes")
			continue
		}
		if publishing && slug != "" && len(nodes) == 0 {
			ve.add("body."+loc, "required", "body required to publish")
		}
		v := articleBodyValidator{ve: ve, media: refs}
		for i, n := range nodes {
			if v.nodes > maxArticleNodes {
				break
			}
			v.node(n, fmt.Sprintf("body.%s.%d", loc, i), 1)
		}
	}
	if !hasLocale {
		ve.add("slug", "required", "at least one localized slug and title required")
	}
	if len(in.Tags) > 10 {
		ve.add("tags", "too_many", "at most 10 tags")
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	for i, tag := range in.Tags {
		in.Tags[i] = strings.TrimSpace(tag)
		articlePlainText(ve, fmt.Sprintf("tags.%d", i), in.Tags[i], 60)
		if utf8.RuneCountInString(in.Tags[i]) == 0 {
			ve.add(fmt.Sprintf("tags.%d", i), "required", "tag must not be empty")
		}
	}
	return refs, ve.orNil()
}
