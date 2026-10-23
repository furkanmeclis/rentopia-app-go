#!/usr/bin/env bash
# Acceptance e2e stack (docs/acceptance/beta.md): Postgres, Redis and SeaweedFS (S3) in Docker,
# the Go API (in-process worker), the panel (Next.js) and the branded site,
# all on dedicated ports so a running `make local-dev` is left alone.
#
#   frontend/e2e/stack/up.sh     start (idempotent; waits until healthy)
#   frontend/e2e/stack/down.sh   stop everything this script started
#
# Nothing here talks to a real provider: payments, SMS, e-invoice, KABİS
# and channel accounts are never connected in the seeded organization.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck source=frontend/e2e/stack/env.sh
source "$ROOT/frontend/e2e/stack/env.sh"
mkdir -p "$E2E_STATE"

log() { printf '[e2e-stack] %s\n' "$*"; }

wait_for() { # name, command
  local name="$1"; shift
  for _ in $(seq 1 "${E2E_WAIT_SECONDS:-180}"); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  log "timeout waiting for $name"
  return 1
}

start_bg() { # name, logfile, command...
  local name="$1" logfile="$2"; shift 2
  if [ -f "$E2E_STATE/$name.pid" ] && kill -0 "$(cat "$E2E_STATE/$name.pid")" 2>/dev/null; then
    log "$name already running"
    return 0
  fi
  log "starting $name (log: $logfile)"
  setsid "$@" >"$logfile" 2>&1 < /dev/null &
  echo $! >"$E2E_STATE/$name.pid"
}

# --- Infra -------------------------------------------------------------------
if [ "${E2E_SKIP_INFRA:-0}" != "1" ]; then
  if ! docker start "$E2E_PG_CONTAINER" >/dev/null 2>&1; then
    log "postgres :$E2E_PG_PORT"
    docker run -d --name "$E2E_PG_CONTAINER" -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app \
      -e POSTGRES_DB=app_e2e -p "$E2E_PG_PORT:5432" postgres:18-alpine >/dev/null
  fi
  if ! docker start "$E2E_REDIS_CONTAINER" >/dev/null 2>&1; then
    log "redis :$E2E_REDIS_PORT"
    docker run -d --name "$E2E_REDIS_CONTAINER" -p "$E2E_REDIS_PORT:6379" redis:8-alpine >/dev/null
  fi
  if ! docker start "$E2E_S3_CONTAINER" >/dev/null 2>&1; then
    log "seaweedfs (S3) :$E2E_S3_PORT"
    docker run -d --name "$E2E_S3_CONTAINER" -p "$E2E_S3_PORT:9000" --entrypoint /bin/sh chrislusf/seaweedfs:4.47 -c \
      'printf '"'"'{"identities":[{"name":"app","credentials":[{"accessKey":"e2e","secretKey":"e2e-secret"}],"actions":["Admin","Read","List","Tagging","Write"]}]}'"'"' > /tmp/s3.json && exec weed server -dir=/data -ip.bind=0.0.0.0 -volume.max=0 -master.volumeSizeLimitMB=256 -s3 -s3.port=9000 -s3.config=/tmp/s3.json' >/dev/null
  fi
  wait_for postgres docker exec "$E2E_PG_CONTAINER" pg_isready -U app -d app_e2e
  wait_for redis docker exec "$E2E_REDIS_CONTAINER" redis-cli ping
  wait_for seaweedfs sh -c "docker exec '$E2E_S3_CONTAINER' wget -q -O - http://127.0.0.1:9333/cluster/status | grep -q '\"IsLeader\":true'"
fi
wait_for "postgres (sql)" psql "$E2E_DATABASE_URL" -c 'SELECT 1'

log "migrations"
docker run --rm --network host -v "$ROOT/backend/migrations:/migrations:ro" migrate/migrate:v4.19.1 \
  -path /migrations -database "$E2E_DATABASE_URL" up 2>&1 | tail -1

# --- Go API --------------------------------------------------------------------
if [ "${E2E_SKIP_BUILD:-0}" != "1" ] || [ ! -x "$E2E_STATE/server" ]; then
  log "building API binaries"
  (cd "$ROOT/backend" && go build -o "$E2E_STATE/server" ./cmd/server && go build -o "$E2E_STATE/create-super-admin" ./cmd/create-super-admin)
fi
(cd "$E2E_STATE" && env $(e2e_api_env) ./create-super-admin -email "$E2E_ADMIN_EMAIL" -password "$E2E_PASSWORD" \
  -name Kabul -surname Yonetici >/dev/null 2>&1 || true)
# Run from the state dir: config.Load reads ../.env and .env relative to the
# working directory, so a developer's .env never leaks into the test stack.
start_bg api "$E2E_STATE/api.log" bash -c "cd '$E2E_STATE' && exec env $(e2e_api_env) ./server"
wait_for api curl -fsS "http://127.0.0.1:$E2E_API_PORT/healthz"

# --- Panel (Next.js) -----------------------------------------------------------
# `next dev`: a production build only accepts the __Secure- session cookie
# (HTTPS behind the proxy in production), which a plain-http test stack
# never gets. Routes compile on first use; the specs allow for that.
FE="${E2E_FRONTEND_DIR:-$ROOT/frontend}"
start_bg panel "$E2E_STATE/panel.log" bash -c "cd '$FE' && exec env $(e2e_web_env) pnpm exec next dev -p $E2E_PANEL_PORT"
wait_for panel curl -fsS -o /dev/null "http://localhost:$E2E_PANEL_PORT/login"

# --- Branded site + widget -------------------------------------------------------
SITE="${E2E_SITE_DIR:-$ROOT/site}"
if [ "${E2E_SKIP_SITE:-0}" != "1" ]; then
  if [ "${E2E_SKIP_BUILD:-0}" != "1" ] || [ ! -d "$SITE/.next" ]; then
    log "building site ($SITE)"
    (cd "$SITE" && env $(e2e_web_env) pnpm exec next build >"$E2E_STATE/site-build.log" 2>&1) || {
      tail -40 "$E2E_STATE/site-build.log"; exit 1; }
  fi
  start_bg site "$E2E_STATE/site.log" bash -c "cd '$SITE' && exec env $(e2e_web_env) pnpm exec next start -p $E2E_SITE_PORT"
  wait_for site curl -fsS -o /dev/null "http://127.0.0.1:$E2E_SITE_PORT/health"
fi
log "ready: api :$E2E_API_PORT, panel :$E2E_PANEL_PORT, site :$E2E_SITE_PORT"
