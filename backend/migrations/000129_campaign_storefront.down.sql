-- Data-loss note: storefront fields are removed; campaigns remain.
DROP TRIGGER trg_campaign_site_media_lock ON campaigns;
DROP FUNCTION campaign_site_media_lock();
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
ALTER TABLE campaigns DROP CONSTRAINT fk_campaign_site_media,
 DROP COLUMN show_on_site, DROP COLUMN site_sort, DROP COLUMN site_title,
 DROP COLUMN site_summary, DROP COLUMN site_cta_label, DROP COLUMN site_media_uuid;
ALTER TABLE site_media DROP CONSTRAINT uq_site_media_org_uuid;
