-- Branded site (TEC-729, docs/design/branded-site.md): one site per
-- organization, served by the single `site` image. The site resolves the
-- organization from the request host and books through the public API
-- (TEC-728) with the publishable key linked here.
--
-- site_settings: the site of an organization.
--   enabled         the host resolves only while the site is switched on.
--   subdomain       {subdomain}.{SITE_BASE_URL host}: the default address and
--                   the preview of every site. Globally unique: the resolver
--                   looks the host up before it knows the organization.
--   default_locale  tr | en.
--   config          the typed theme / layout / content / SEO / analytics
--                   document (backend/internal/modules/site/model); validated
--                   by the API, never trusted by the site either.
--   api_key_id      the publishable `web` key the site mints booking sessions
--                   with. The API keeps the site's origins in its
--                   allowed_origins (subdomain now, verified custom domains
--                   with TEC-731).
--   *_object_key    uploaded images (TEC-730 adds the uploads); the API
--                   streams them, the browser never sees a storage key.
--   version         +1 on every save: optimistic concurrency for the panel
--                   and the cache key of the site.
CREATE TABLE site_settings (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    subdomain VARCHAR(63) NOT NULL
        CHECK (subdomain ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'),
    default_locale VARCHAR(5) NOT NULL DEFAULT 'tr' CHECK (default_locale IN ('tr', 'en')),
    config JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config) = 'object'),
    api_key_id BIGINT NULL,
    logo_object_key TEXT NULL,
    favicon_object_key TEXT NULL,
    cover_object_key TEXT NULL,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_site_settings_organization UNIQUE (organization_id),
    CONSTRAINT uq_site_settings_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT fk_site_settings_api_key
        FOREIGN KEY (organization_id, api_key_id)
        REFERENCES public_api_keys (organization_id, id) ON DELETE SET NULL (api_key_id)
);

-- Deliberately global: the host resolves the organization (like
-- public_api_keys.key_hash).
CREATE UNIQUE INDEX uq_site_settings_subdomain ON site_settings (subdomain);
CREATE UNIQUE INDEX uq_site_settings_uuid ON site_settings (uuid);

CREATE TRIGGER trg_site_settings_updated_at
    BEFORE UPDATE ON site_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
