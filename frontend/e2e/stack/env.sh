# Shared settings of the acceptance e2e stack (sourced by up.sh / down.sh;
# playwright.config.ts uses the same defaults). Ports avoid the local-dev
# ones (5432 / 6379 / 8080 / 3000 / 3100).
E2E_STATE="${E2E_STATE:-${TMPDIR:-/tmp}/rentopya-e2e}"
E2E_PG_PORT="${E2E_PG_PORT:-55433}"
E2E_REDIS_PORT="${E2E_REDIS_PORT:-56380}"
E2E_API_PORT="${E2E_API_PORT:-18080}"
E2E_PANEL_PORT="${E2E_PANEL_PORT:-13000}"
E2E_SITE_PORT="${E2E_SITE_PORT:-13100}"
E2E_S3_PORT="${E2E_S3_PORT:-59000}"
E2E_PG_CONTAINER="${E2E_PG_CONTAINER:-rentopya-e2e-postgres}"
E2E_REDIS_CONTAINER="${E2E_REDIS_CONTAINER:-rentopya-e2e-redis}"
E2E_S3_CONTAINER="${E2E_S3_CONTAINER:-rentopya-e2e-s3}"
E2E_DATABASE_URL="${E2E_DATABASE_URL:-postgres://app:app@127.0.0.1:$E2E_PG_PORT/app_e2e?sslmode=disable}"
E2E_ADMIN_EMAIL="${E2E_ADMIN_EMAIL:-kabul-admin@rentopya.test}"
E2E_PASSWORD="${E2E_PASSWORD:-Kabul2026Test}"
export E2E_STATE E2E_PG_PORT E2E_REDIS_PORT E2E_API_PORT E2E_PANEL_PORT E2E_SITE_PORT \
  E2E_S3_PORT E2E_PG_CONTAINER E2E_REDIS_CONTAINER E2E_S3_CONTAINER E2E_DATABASE_URL E2E_ADMIN_EMAIL E2E_PASSWORD

# Go API environment (development mode: in-process fakes allowed, local file
# SeaweedFS S3, no Meilisearch / Centrifugo / SMTP).
e2e_api_env() {
  local db_port db_host
  db_host=127.0.0.1
  db_port="$E2E_PG_PORT"
  printf '%s ' \
    APP_ENV=development APP_NAME=api-e2e "APP_HTTP_ADDR=127.0.0.1:$E2E_API_PORT" LOG_FORMAT=json LOG_LEVEL=info \
    "DB_HOST=$db_host" "DB_PORT=$db_port" DB_USER=app DB_PASSWORD=app DB_NAME=app_e2e DB_SSLMODE=disable \
    "REDIS_ADDR=127.0.0.1:$E2E_REDIS_PORT" \
    STORAGE_DRIVER=minio "S3_ENDPOINT=http://127.0.0.1:$E2E_S3_PORT" S3_BUCKET=e2e S3_ACCESS_KEY=e2e S3_SECRET_KEY=e2e-secret S3_REGION=us-east-1 S3_USE_PATH_STYLE=true \
    SEARCH_ENABLED=false CENTRIFUGO_ENABLED=false QUEUE_ENABLED=true QUEUE_WORKER_INPROCESS=true \
    TENANCY_ENABLED=true TENANCY_CACHE_TTL=1s SMS_LOG_PROVIDER=true \
    "PUBLIC_FRONTEND_URL=http://localhost:$E2E_PANEL_PORT" "SITE_BASE_URL=http://localhost:$E2E_SITE_PORT" \
    "CORS_ALLOWED_ORIGINS=http://localhost:$E2E_PANEL_PORT" AUTH_WEBAUTHN_RP_ID=localhost \
    SMTP_HOST=127.0.0.1 SMTP_PORT=9
}

# Panel / site environment (server side: API_URL is the Go /v1 base).
e2e_web_env() {
  printf '%s ' \
    "API_URL=http://127.0.0.1:$E2E_API_PORT/v1" "PUBLIC_API_URL=http://127.0.0.1:$E2E_API_PORT" \
    AUTH_SECRET=e2e-auth-secret-0123456789abcdef0123456789 "AUTH_URL=http://localhost:$E2E_PANEL_PORT" AUTH_TRUST_HOST=true \
    AUTH_ADAPTER_SECRET=app-dev-auth-adapter-secret-change-me \
    NEXT_PUBLIC_BFF_BASE_URL=/api NEXT_PUBLIC_REALTIME_ENABLED=false NEXT_PUBLIC_DEFAULT_LOCALE=tr NEXT_PUBLIC_FALLBACK_LOCALE=en \
    SITE_EMBED_SECRET=e2e-site-embed-secret-0123456789abcdef NEXT_TELEMETRY_DISABLED=1
}
