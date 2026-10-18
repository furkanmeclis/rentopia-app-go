-- Data-loss note: all site articles are removed. Stored media bytes remain.
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

DROP TABLE site_articles;
DROP FUNCTION site_article_media_guard();
DROP FUNCTION site_article_media_ids(jsonb);
DROP FUNCTION site_article_slug_valid(text);
