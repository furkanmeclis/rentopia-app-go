package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrArticleSlugTaken = errors.New("article slug already exists in this locale")

func articleDBError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if pe.Code == "23505" && strings.HasPrefix(pe.ConstraintName, "uq_site_articles_slug_") {
			return ErrArticleSlugTaken
		}
		if (pe.Code == "23503" && pe.ConstraintName == "fk_site_article_cover") || pe.ConstraintName == "site_article_media_reference" {
			ve := &ValidationError{}
			field := "body"
			if pe.ConstraintName == "fk_site_article_cover" {
				field = "cover_media_uuid"
			}
			ve.add(field, "not_found", "unknown, deleted or foreign media")
			return ve
		}
	}
	return mediaDBError(err)
}

func articleModel(row db.SiteArticle) model.Article {
	out := model.Article{
		UUID: row.Uuid, Status: row.Status, Sort: row.Sort, Featured: row.Featured,
		Version: int(row.Version), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), Tags: row.Tags,
	}
	_ = json.Unmarshal(row.Slug, &out.Slug)
	_ = json.Unmarshal(row.Title, &out.Title)
	_ = json.Unmarshal(row.Summary, &out.Summary)
	_ = json.Unmarshal(row.Body, &out.Body)
	_ = json.Unmarshal(row.SeoTitle, &out.SEOTitle)
	_ = json.Unmarshal(row.SeoDescription, &out.SEODescription)
	if row.CoverMediaUuid.Valid {
		id := uuid.UUID(row.CoverMediaUuid.Bytes)
		out.CoverMediaUUID = &id
	}
	if row.PublishedAt.Valid {
		t := row.PublishedAt.Time.UTC()
		out.PublishedAt = &t
	}
	if row.CreatedBy.Valid {
		out.CreatedBy = &row.CreatedBy.Int64
	}
	if row.UpdatedBy.Valid {
		out.UpdatedBy = &row.UpdatedBy.Int64
	}
	return out
}

func (s *Service) GetArticle(ctx context.Context, orgID int64, id uuid.UUID) (model.Article, error) {
	row, err := s.q.GetSiteArticle(ctx, db.GetSiteArticleParams{OrganizationID: orgID, Uuid: id})
	return articleModel(row), articleDBError(err)
}

func (s *Service) validateArticle(ctx context.Context, orgID int64, in *model.ArticleInput, publishing bool) error {
	refs, err := ValidateArticle(in, publishing)
	if err != nil {
		return err
	}
	ve := &ValidationError{}
	checked := map[string]bool{}
	for field, ref := range refs {
		if found, ok := checked[ref]; ok {
			if !found {
				ve.add(field, "not_found", "unknown, deleted or foreign media")
			}
			continue
		}
		id, err := uuid.Parse(ref)
		if err != nil {
			ve.add(field, "invalid", "canonical media UUID required")
			continue
		}
		_, err = s.q.GetSiteMedia(ctx, db.GetSiteMediaParams{OrganizationID: orgID, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			checked[ref] = false
			ve.add(field, "not_found", "unknown, deleted or foreign media")
			continue
		}
		if err != nil {
			return err
		}
		checked[ref] = true
	}
	return ve.orNil()
}

func articleParams(orgID, userID int64, in model.ArticleInput) db.InsertSiteArticleParams {
	// All raw bodies were parsed by ValidateArticle; marshaling cannot fail.
	slug, _ := json.Marshal(in.Slug)
	title, _ := json.Marshal(in.Title)
	summary, _ := json.Marshal(in.Summary)
	body, _ := json.Marshal(in.Body)
	seoTitle, _ := json.Marshal(in.SEOTitle)
	seoDescription, _ := json.Marshal(in.SEODescription)
	p := db.InsertSiteArticleParams{OrganizationID: orgID, Actor: actor(userID), Slug: slug, Title: title, Summary: summary,
		Body: body, SeoTitle: seoTitle, SeoDescription: seoDescription, Tags: in.Tags, Sort: in.Sort, Featured: in.Featured}
	if in.CoverMediaUUID != nil {
		p.CoverMediaUuid = pgtype.UUID{Bytes: *in.CoverMediaUUID, Valid: true}
	}
	return p
}

func (s *Service) CreateArticle(ctx context.Context, orgID, userID int64, in model.ArticleInput) (model.Article, error) {
	if err := s.validateArticle(ctx, orgID, &in, false); err != nil {
		return model.Article{}, err
	}
	row, err := s.q.InsertSiteArticle(ctx, articleParams(orgID, userID, in))
	return articleModel(row), articleDBError(err)
}

func checkArticleVersion(version *int, current int) error {
	if version == nil || *version < 1 {
		ve := &ValidationError{}
		ve.add("version", "required", "send the article version you edited")
		return ve
	}
	if *version != current {
		return ErrVersionConflict
	}
	return nil
}

func (s *Service) UpdateArticle(ctx context.Context, orgID int64, id uuid.UUID, userID int64, in model.ArticleInput) (model.Article, error) {
	current, err := s.GetArticle(ctx, orgID, id)
	if err != nil {
		return model.Article{}, err
	}
	if err = checkArticleVersion(in.Version, current.Version); err != nil {
		return model.Article{}, err
	}
	if err = s.validateArticle(ctx, orgID, &in, current.Status == "published"); err != nil {
		return model.Article{}, err
	}
	p := articleParams(orgID, userID, in)
	row, err := s.q.UpdateSiteArticle(ctx, db.UpdateSiteArticleParams{OrganizationID: orgID, Uuid: id, Version: int32(*in.Version),
		Slug: p.Slug, Title: p.Title, Summary: p.Summary, Body: p.Body, SeoTitle: p.SeoTitle, SeoDescription: p.SeoDescription,
		CoverMediaUuid: p.CoverMediaUuid, Tags: p.Tags, Sort: p.Sort, Featured: p.Featured, Actor: p.Actor})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Article{}, ErrVersionConflict
	}
	return articleModel(row), articleDBError(err)
}

func articleInput(a model.Article) model.ArticleInput {
	return model.ArticleInput{Slug: a.Slug, Title: a.Title, Summary: a.Summary, Body: a.Body, SEOTitle: a.SEOTitle,
		SEODescription: a.SEODescription, CoverMediaUUID: a.CoverMediaUUID, Tags: a.Tags, Sort: a.Sort, Featured: a.Featured}
}

func (s *Service) SetArticleStatus(ctx context.Context, orgID int64, id uuid.UUID, userID int64, in model.VersionInput, published bool) (model.Article, error) {
	current, err := s.GetArticle(ctx, orgID, id)
	if err != nil {
		return model.Article{}, err
	}
	if err = checkArticleVersion(in.Version, current.Version); err != nil {
		return model.Article{}, err
	}
	status := "draft"
	if published {
		status = "published"
		document := articleInput(current)
		if err = s.validateArticle(ctx, orgID, &document, true); err != nil {
			return model.Article{}, err
		}
	}
	if status == current.Status {
		return current, nil
	}
	row, err := s.q.SetSiteArticleStatus(ctx, db.SetSiteArticleStatusParams{OrganizationID: orgID, Uuid: id, Actor: actor(userID), Version: int32(*in.Version), Status: status})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Article{}, ErrVersionConflict
	}
	return articleModel(row), articleDBError(err)
}

func (s *Service) DeleteArticle(ctx context.Context, orgID int64, id uuid.UUID, userID int64, in model.VersionInput) error {
	current, err := s.GetArticle(ctx, orgID, id)
	if err != nil {
		return err
	}
	if err = checkArticleVersion(in.Version, current.Version); err != nil {
		return err
	}
	n, err := s.q.DeleteSiteArticle(ctx, db.DeleteSiteArticleParams{OrganizationID: orgID, Uuid: id, Actor: actor(userID), Version: int32(*in.Version)})
	if err != nil {
		return articleDBError(err)
	}
	if n == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Service) ListArticles(ctx context.Context, p db.ListSiteArticlesParams) (apiquery.Page[model.Article], error) {
	rows, err := s.q.ListSiteArticles(ctx, p)
	if err != nil {
		return apiquery.Page[model.Article]{}, err
	}
	total, err := s.q.CountSiteArticles(ctx, db.CountSiteArticlesParams{OrganizationID: p.OrganizationID, Status: p.Status, Locale: p.Locale, Q: p.Q})
	out := []model.Article{}
	for _, row := range rows {
		out = append(out, articleModel(row))
	}
	return apiquery.NewPage(out, total, p.PageLimit, p.PageOffset), err
}

func articleCard(row db.SiteArticle, site uuid.UUID, locale string) model.ArticleCard {
	slug, title, summary := map[string]string{}, map[string]string{}, map[string]string{}
	_ = json.Unmarshal(row.Slug, &slug)
	_ = json.Unmarshal(row.Title, &title)
	_ = json.Unmarshal(row.Summary, &summary)
	out := model.ArticleCard{Title: title[locale], Summary: summary[locale], Slug: slug[locale], PublishedAt: row.PublishedAt.Time.UTC()}
	if row.CoverMediaUuid.Valid {
		p := mediaPath(site, uuid.UUID(row.CoverMediaUuid.Bytes).String(), "")
		out.CoverURL = &p
	}
	return out
}

func publicArticle(row db.SiteArticle, site uuid.UUID, locale string) model.PublicArticle {
	a := articleModel(row)
	out := model.PublicArticle{Locale: locale, Body: a.Body[locale], SEOTitle: a.SEOTitle[locale], SEODescription: a.SEODescription[locale],
		Tags: a.Tags, Featured: a.Featured, UpdatedAt: a.UpdatedAt, Media: map[string]string{}, ArticleCard: articleCard(row, site, locale)}
	input := articleInput(a)
	input.Body = map[string]json.RawMessage{locale: a.Body[locale]}
	refs, _ := ValidateArticle(&input, false)
	for _, id := range refs {
		out.Media[id] = mediaPath(site, id, "")
	}
	return out
}

func (s *Service) publicArticleList(ctx context.Context, orgID int64, site uuid.UUID, locale string, limit, offset int32) (apiquery.Page[model.ArticleCard], error) {
	rows, err := s.q.ListPublicSiteArticles(ctx, db.ListPublicSiteArticlesParams{OrganizationID: orgID, Locale: locale, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return apiquery.Page[model.ArticleCard]{}, err
	}
	total, err := s.q.CountPublicSiteArticles(ctx, db.CountPublicSiteArticlesParams{OrganizationID: orgID, Locale: locale})
	cards := []model.ArticleCard{}
	for _, row := range rows {
		cards = append(cards, articleCard(row, site, locale))
	}
	return apiquery.NewPage(cards, total, limit, offset), err
}
