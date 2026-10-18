package usecase

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
)

func (s *Service) publishedSite(ctx context.Context, rawHost string) (resolvedRow, error) {
	host, ok := NormalizeHost(rawHost)
	if !ok {
		return resolvedRow{}, ErrNotFound
	}
	row, _, err := s.lookup(ctx, host)
	if err != nil {
		return resolvedRow{}, err
	}
	now := s.now()
	accessible := row.OrganizationStatus == orgctx.StatusActive && (!row.AccessStartsAt.Valid || !now.Before(row.AccessStartsAt.Time)) && (!row.AccessEndsAt.Valid || now.Before(row.AccessEndsAt.Time))
	if !accessible || !row.Enabled || !row.PublishedRevision.Valid {
		return resolvedRow{}, ErrNotFound
	}
	return row, nil
}

func articleLocale(locale, fallback string) (string, error) {
	if locale == "" {
		locale = fallback
	}
	if locale != "tr" && locale != "en" {
		ve := &ValidationError{}
		ve.add("locale", "invalid", "tr or en required")
		return "", ve
	}
	return locale, nil
}

func (s *Service) PublicArticles(ctx context.Context, host, locale string, limit, offset int32) (apiquery.Page[model.ArticleCard], error) {
	row, err := s.publishedSite(ctx, host)
	if err != nil {
		return apiquery.Page[model.ArticleCard]{}, err
	}
	locale, err = articleLocale(locale, row.DefaultLocale)
	if err != nil {
		return apiquery.Page[model.ArticleCard]{}, err
	}
	return s.publicArticleList(ctx, row.OrganizationID, row.Uuid, locale, limit, offset)
}

func (s *Service) PublicArticle(ctx context.Context, host, locale, slug string) (model.PublicArticle, error) {
	row, err := s.publishedSite(ctx, host)
	if err != nil {
		return model.PublicArticle{}, err
	}
	locale, err = articleLocale(locale, row.DefaultLocale)
	if err != nil {
		return model.PublicArticle{}, err
	}
	if !articleSlug.MatchString(slug) || slug == "" {
		return model.PublicArticle{}, ErrNotFound
	}
	article, err := s.q.GetPublicSiteArticle(ctx, db.GetPublicSiteArticleParams{OrganizationID: row.OrganizationID, Locale: locale, Slug: slug})
	if err != nil {
		return model.PublicArticle{}, articleDBError(err)
	}
	return publicArticle(article, row.Uuid, locale), nil
}

// Sitemap returns relative paths. Vehicle segments use the existing fleet page;
// the site currently has no individual vehicle detail route.
func (s *Service) Sitemap(ctx context.Context, host string) ([]model.SitemapEntry, error) {
	row, err := s.publishedSite(ctx, host)
	if err != nil {
		return nil, err
	}
	cfg := decodeConfig(row.PublishedConfig)
	modified := row.PublishedAt.Time.UTC()
	out := []model.SitemapEntry{{Path: "/", Lastmod: modified}}
	for _, path := range []string{"about", "faq", "terms", "privacy", "contact"} {
		exists := false
		for _, loc := range model.Locales {
			c := cfg.Content[loc]
			switch path {
			case "about":
				exists = exists || strings.TrimSpace(c.About) != ""
			case "faq":
				exists = exists || len(c.FAQ) > 0
			case "terms":
				exists = exists || strings.TrimSpace(c.Terms) != ""
			case "privacy":
				exists = exists || strings.TrimSpace(c.Privacy) != ""
			case "contact":
				exists = cfg.Contact.Phone != "" || cfg.Contact.Email != "" || cfg.Contact.Address != "" || cfg.Contact.WhatsApp != ""
			}
		}
		if exists {
			out = append(out, model.SitemapEntry{Path: "/" + path, Lastmod: modified})
		}
	}
	catalog, err := s.q.ListSiteSitemapCatalog(ctx, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	active, err := s.channelActive(ctx, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	booking := active && s.keyUsable(row.ApiKeyStatus, row.ApiKeyExpiresAt)
	branches, vehicles := false, false
	for _, item := range catalog {
		path := item.Path
		if strings.HasPrefix(path, "/vehicles?") {
			if !booking {
				continue
			}
			path = "/vehicles?segment=" + url.QueryEscape(strings.TrimPrefix(path, "/vehicles?segment="))
			vehicles = true
		} else {
			branches = true
		}
		out = append(out, model.SitemapEntry{Path: path, Lastmod: item.UpdatedAt.Time.UTC()})
	}
	if branches {
		out = append(out, model.SitemapEntry{Path: "/branches", Lastmod: modified})
	}
	if vehicles {
		out = append(out, model.SitemapEntry{Path: "/vehicles", Lastmod: modified})
	}
	articles, err := s.q.ListSiteSitemapArticles(ctx, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	for _, a := range articles {
		slugs := map[string]string{}
		if err := json.Unmarshal(a.Slug, &slugs); err != nil {
			return nil, err
		}
		for _, loc := range model.Locales {
			if slugs[loc] != "" {
				out = append(out, model.SitemapEntry{Path: "/guide/" + loc + "/" + slugs[loc], Locale: loc, Lastmod: a.UpdatedAt.Time.UTC()})
			}
		}
	}
	return out, nil
}
