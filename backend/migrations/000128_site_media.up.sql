CREATE TABLE site_media (
 id BIGSERIAL PRIMARY KEY,
 uuid uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 kind text NOT NULL DEFAULT 'image' CHECK (kind = 'image'),
 mime text NOT NULL CHECK (mime IN ('image/png','image/jpeg','image/webp')),
 size bigint NOT NULL CHECK (size > 0 AND size <= 10485760),
 width integer NOT NULL CHECK (width > 0),
 height integer NOT NULL CHECK (height > 0),
 alt jsonb NOT NULL DEFAULT '{"tr":"","en":""}',
 object_key text NOT NULL UNIQUE,
 created_by bigint REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_site_media_org ON site_media(organization_id, id DESC);
CREATE TRIGGER trg_site_media_updated BEFORE UPDATE ON site_media FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Only designated media fields count as references; arbitrary text never does.
CREATE FUNCTION site_config_media_ids(c jsonb) RETURNS SETOF text LANGUAGE sql IMMUTABLE AS $$
 SELECT DISTINCT value FROM (
  SELECT c#>>'{about,image}' AS value
  UNION ALL
  SELECT slide->>'media' FROM jsonb_each(COALESCE(c->'content','{}')) AS locale,
   LATERAL jsonb_array_elements(COALESCE(locale.value->'hero_slides','[]')) AS slide
 ) refs WHERE value IS NOT NULL AND value <> '';
$$;

-- All media and document mutations serialize on the organization row. This
-- also covers first saves and avoids a save/delete race or quota write skew.
CREATE FUNCTION site_media_guard() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER trg_site_media_guard BEFORE INSERT OR DELETE ON site_media FOR EACH ROW EXECUTE FUNCTION site_media_guard();

CREATE FUNCTION site_config_media_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM organizations WHERE id = NEW.organization_id FOR UPDATE;
 IF EXISTS (SELECT 1 FROM site_config_media_ids(NEW.config) ref WHERE NOT EXISTS (
  SELECT 1 FROM site_media m WHERE m.organization_id = NEW.organization_id AND m.uuid::text = ref
 )) THEN
  RAISE EXCEPTION 'unknown or foreign media reference' USING ERRCODE = '23514', CONSTRAINT = 'site_media_reference';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER trg_site_settings_media BEFORE INSERT OR UPDATE OF config ON site_settings FOR EACH ROW EXECUTE FUNCTION site_config_media_guard();
CREATE TRIGGER trg_site_versions_media BEFORE INSERT OR UPDATE OF config ON site_setting_versions FOR EACH ROW EXECUTE FUNCTION site_config_media_guard();
