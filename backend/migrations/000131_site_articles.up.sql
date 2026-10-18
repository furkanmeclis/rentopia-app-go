CREATE FUNCTION site_article_slug_valid(value text) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT value = '' OR (length(value) <= 160 AND value ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND value <> ALL (ARRAY['api','admin','platform','t','media','assets','lang','preview','embed','book','search','reservation','lookup','vehicles','branches','about','faq','contact','terms','privacy','corporate','franchise','campaigns','guide','articles','sitemap','sitemap.xml','robots','robots.txt','www','app']));
$$;
CREATE TABLE site_articles (
 id BIGSERIAL PRIMARY KEY,
 organization_id bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 uuid uuid NOT NULL DEFAULT gen_random_uuid(),
 status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
 published_at timestamptz,
 slug jsonb NOT NULL DEFAULT '{"tr":"","en":""}',
 title jsonb NOT NULL DEFAULT '{"tr":"","en":""}',
 summary jsonb NOT NULL DEFAULT '{"tr":"","en":""}',
 cover_media_uuid uuid,
 body jsonb NOT NULL DEFAULT '{"tr":[],"en":[]}',
 seo_title jsonb NOT NULL DEFAULT '{"tr":"","en":""}',
 seo_description jsonb NOT NULL DEFAULT '{"tr":"","en":""}',
 tags text[] NOT NULL DEFAULT '{}',
 sort integer NOT NULL DEFAULT 0,
 featured boolean NOT NULL DEFAULT false,
 created_by bigint REFERENCES users(id) ON DELETE SET NULL,
 updated_by bigint REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 deleted_at timestamptz,
 version integer NOT NULL DEFAULT 1 CHECK (version > 0),
 UNIQUE (organization_id, uuid),
 CONSTRAINT fk_site_article_cover FOREIGN KEY (organization_id, cover_media_uuid)
  REFERENCES site_media(organization_id, uuid) ON DELETE SET NULL (cover_media_uuid),
 CHECK (jsonb_typeof(slug) = 'object' AND slug - 'tr' - 'en' = '{}'::jsonb
  AND site_article_slug_valid(COALESCE(slug->>'tr','')) AND site_article_slug_valid(COALESCE(slug->>'en',''))),
 CHECK (jsonb_typeof(body->'tr') = 'array' AND jsonb_typeof(body->'en') = 'array'),
 CHECK (cardinality(tags) <= 10),
 CHECK (status <> 'published' OR published_at IS NOT NULL)
);
CREATE UNIQUE INDEX uq_site_articles_slug_tr ON site_articles(organization_id, (slug->>'tr')) WHERE deleted_at IS NULL AND slug->>'tr' <> '';
CREATE UNIQUE INDEX uq_site_articles_slug_en ON site_articles(organization_id, (slug->>'en')) WHERE deleted_at IS NULL AND slug->>'en' <> '';
CREATE INDEX idx_site_articles_public ON site_articles(organization_id, published_at DESC, id DESC) WHERE deleted_at IS NULL AND status = 'published';
CREATE TRIGGER trg_site_articles_updated BEFORE UPDATE ON site_articles FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- Only image elements contribute references, never UUID-like article text.
CREATE FUNCTION site_article_media_ids(document jsonb) RETURNS SETOF text LANGUAGE sql IMMUTABLE AS $$
 SELECT DISTINCT value #>> '{}' FROM jsonb_path_query(document, '$.** ? (@.type == "img").media_uuid') AS value;
$$;
CREATE FUNCTION site_article_media_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM organizations WHERE id = NEW.organization_id FOR UPDATE;
 IF NEW.deleted_at IS NULL AND EXISTS (SELECT 1 FROM site_article_media_ids(NEW.body) ref WHERE NOT EXISTS (
  SELECT 1 FROM site_media m WHERE m.organization_id = NEW.organization_id AND m.uuid::text = ref
 )) THEN
  RAISE EXCEPTION 'unknown or foreign article media' USING ERRCODE = '23514', CONSTRAINT = 'site_article_media_reference';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER trg_site_article_media_guard BEFORE INSERT OR UPDATE OF body, cover_media_uuid, status ON site_articles FOR EACH ROW EXECUTE FUNCTION site_article_media_guard();
CREATE OR REPLACE FUNCTION site_media_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE org bigint; total_files bigint; total_bytes bigint;
BEGIN
 IF TG_OP = 'DELETE' THEN org := OLD.organization_id; ELSE org := NEW.organization_id; END IF;
 PERFORM id FROM organizations WHERE id = org FOR UPDATE;
 IF TG_OP = 'INSERT' THEN
  SELECT count(*), COALESCE(sum(size),0) INTO total_files,total_bytes FROM site_media WHERE organization_id = org;
  IF total_files >= 200 OR total_bytes + NEW.size > 524288000 THEN
   RAISE EXCEPTION 'site media quota exceeded' USING ERRCODE = '23514', CONSTRAINT = 'site_media_quota';
  END IF;
 ELSIF TG_OP = 'DELETE' THEN
  -- Parent cascading deletion must be allowed.
  IF EXISTS (SELECT 1 FROM organizations WHERE id = org) AND (
   EXISTS (SELECT 1 FROM site_articles a WHERE a.organization_id = org AND a.status = 'published' AND a.deleted_at IS NULL
    AND (a.cover_media_uuid = OLD.uuid OR EXISTS (SELECT 1 FROM site_article_media_ids(a.body) ref WHERE ref = OLD.uuid::text))) OR
   EXISTS (SELECT 1 FROM campaigns c WHERE c.organization_id = org AND c.site_media_uuid = OLD.uuid AND c.deleted_at IS NULL) OR
   EXISTS (SELECT 1 FROM site_settings s, LATERAL site_config_media_ids(s.config) ref WHERE s.organization_id = org AND ref = OLD.uuid::text) OR
   EXISTS (SELECT 1 FROM site_setting_versions v, LATERAL site_config_media_ids(v.config) ref WHERE v.organization_id = org AND ref = OLD.uuid::text)
  ) THEN
   RAISE EXCEPTION 'media is referenced by site content' USING ERRCODE = '23514', CONSTRAINT = 'site_media_in_use';
  END IF;
  RETURN OLD;
 END IF;
 RETURN NEW;
END;
$$;
