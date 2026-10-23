# shellcheck shell=bash disable=SC2016,SC2034
# Shared helpers of scripts/backup.sh and scripts/restore.sh (sourced, not run).
#
# Containers are addressed as <NAME_PREFIX>-<service>: compose.prod.yml pins
# container_name, so `docker exec <NAME_PREFIX>-postgres` is the same as
# `docker compose exec postgres` and also works under Dokploy (where the
# compose file lives elsewhere) and inside the `backup` service (Docker socket).
#
# Secrets never reach argv or the log: containers read their own env
# (POSTGRES_PASSWORD, REDIS_PASSWORD; the single-quoted commands below expand
# inside the container) and helper containers get S3 keys with
# `docker run -e NAME` (value taken from this process's environment).

# Pinned helper image: rclone (S3 copy) + busybox tar (alpine based).
RCLONE_IMAGE="${BACKUP_RCLONE_IMAGE:-rclone/rclone:1.68.2}"

die() { echo "error: $*" >&2; exit 1; }
log() { printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*"; }
trim() { local s="$1"; s="${s#"${s%%[![:space:]]*}"}"; printf '%s' "${s%"${s##*[![:space:]]}"}"; }

# load_env_file FILE: KEY=VALUE lines (same parsing as scripts/prod-env.sh).
# A variable already set in the environment wins, so one-off overrides work:
#   BACKUP_DIR=/mnt/x scripts/backup.sh
load_env_file() {
  local file="$1" line key val
  [[ -f "$file" ]] || die "env file not found: $file"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
    key="${BASH_REMATCH[2]}"; val="${BASH_REMATCH[3]}"
    if [[ "$val" =~ ^\"(.*)\"$ ]]; then val="${BASH_REMATCH[1]//\\\"/\"}"
    elif [[ "$val" =~ ^\'(.*)\'$ ]]; then val="${BASH_REMATCH[1]}"; fi
    [[ -v "$key" ]] && continue
    printf -v "$key" '%s' "$val"
    export "${key?}"
  done < "$file"
}

require_vars() {
  local v missing=()
  for v in "$@"; do [[ -n "${!v:-}" ]] || missing+=("$v"); done
  [[ ${#missing[@]} -eq 0 ]] || die "missing env: ${missing[*]}"
}

# Stack naming (compose.prod.yml).
stack_init() {
  require_vars NAME_PREFIX
  APP_NETWORK_NAME="${APP_NETWORK_NAME:-${NAME_PREFIX}-app}"
  PG_C="${NAME_PREFIX}-postgres"
  REDIS_C="${NAME_PREFIX}-redis"
  S3_C="${NAME_PREFIX}-seaweedfs"
  S3_REGION="${S3_REGION:-us-east-1}"
  # The API reaches SeaweedFS the same way (S3_ENDPOINT in compose.prod.yml).
  BACKUP_S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-http://seaweedfs.${APP_NETWORK_NAME}:9000}"
}

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "$1 is required${2:+ ($2)}"; }

container_running() {
  [[ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" == "true" ]]
}

require_running() {
  local c
  for c in "$@"; do container_running "$c" || die "container not running: $c"; done
}

# psql / pg_* inside the postgres container, as its own superuser.
pg_exec() {
  local cmd="$1"; shift
  docker exec -i "$PG_C" sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec '"$cmd"' "$@"' sh "$@"
}

pg_sql() { # stdin = SQL, prints unaligned rows "col|col"
  pg_exec 'psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tA'
}

redis_cli() {
  docker exec -i "$REDIS_C" sh -c '[ -n "${REDIS_PASSWORD:-}" ] && export REDISCLI_AUTH="$REDIS_PASSWORD"; exec redis-cli --no-auth-warning "$@"' sh "$@"
}

# S3 remotes for rclone, configured through env (RCLONE_CONFIG_<NAME>_*).
export_s3_remote_src() {
  export RCLONE_CONFIG_SRC_TYPE=s3 RCLONE_CONFIG_SRC_PROVIDER=Other \
    RCLONE_CONFIG_SRC_ENDPOINT="$BACKUP_S3_ENDPOINT" RCLONE_CONFIG_SRC_REGION="$S3_REGION" \
    RCLONE_CONFIG_SRC_FORCE_PATH_STYLE=true
  export RCLONE_CONFIG_SRC_ACCESS_KEY_ID="$S3_ACCESS_KEY" RCLONE_CONFIG_SRC_SECRET_ACCESS_KEY="$S3_SECRET_KEY"
}

export_s3_remote_offsite() {
  export RCLONE_CONFIG_OFFSITE_TYPE=s3 RCLONE_CONFIG_OFFSITE_PROVIDER=Other \
    RCLONE_CONFIG_OFFSITE_ENDPOINT="$BACKUP_OFFSITE_ENDPOINT" \
    RCLONE_CONFIG_OFFSITE_REGION="${BACKUP_OFFSITE_REGION:-us-east-1}"
  export RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID="$BACKUP_OFFSITE_ACCESS_KEY" \
    RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY="$BACKUP_OFFSITE_SECRET_KEY"
}

# rclone_src [docker run args...] -- rclone args...  (on the app network)
rclone_src() {
  local dargs=()
  while [[ $# -gt 0 && "$1" != "--" ]]; do dargs+=("$1"); shift; done
  shift
  docker run --rm --network "$APP_NETWORK_NAME" "${dargs[@]}" -e RCLONE_LOG_LEVEL=ERROR \
    -e RCLONE_CONFIG_SRC_TYPE -e RCLONE_CONFIG_SRC_PROVIDER -e RCLONE_CONFIG_SRC_ENDPOINT \
    -e RCLONE_CONFIG_SRC_REGION -e RCLONE_CONFIG_SRC_FORCE_PATH_STYLE \
    -e RCLONE_CONFIG_SRC_ACCESS_KEY_ID -e RCLONE_CONFIG_SRC_SECRET_ACCESS_KEY \
    "$RCLONE_IMAGE" "$@"
}

rclone_offsite() {
  docker run --rm -i -e RCLONE_LOG_LEVEL=ERROR \
    -e RCLONE_CONFIG_OFFSITE_TYPE -e RCLONE_CONFIG_OFFSITE_PROVIDER -e RCLONE_CONFIG_OFFSITE_ENDPOINT \
    -e RCLONE_CONFIG_OFFSITE_REGION \
    -e RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID -e RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY \
    "$RCLONE_IMAGE" "$@"
}

# Row counts of the tables RT-A32 compares before / after a restore, plus
# the state of the work that has to resume. Missing tables are skipped.
STATS_SQL="$(cat <<'SQL'
SELECT 'schema_version|' || version || CASE WHEN dirty THEN ' (dirty)' ELSE '' END FROM schema_migrations;
SELECT 'rows.' || table_name || '|' ||
       (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM public.%I', table_name), false, true, '')))[1]::text
FROM information_schema.tables
WHERE table_schema = 'public' AND table_name IN (
  'organizations', 'users', 'customers', 'vehicles', 'reservations', 'rentals',
  'documents', 'document_versions', 'document_approvals', 'inspections',
  'payments', 'payment_refunds', 'collections', 'ledger_entries', 'invoices', 'security_deposits',
  'notifications', 'notification_dispatches', 'notification_deliveries',
  'sms_messages', 'whatsapp_messages', 'kabis_notifications', 'toll_passes',
  'integration_jobs', 'integration_attempts', 'outbox_events')
ORDER BY table_name;
SELECT 'integration_jobs.' || status || '|' || count(*) FROM integration_jobs GROUP BY status ORDER BY status;
SELECT 'integration_jobs.locked|' || count(*) FROM integration_jobs WHERE locked_until > NOW();
SELECT 'notifications.' || status || '|' || count(*) FROM notifications
  WHERE status IN ('queued', 'processing', 'failed') GROUP BY status ORDER BY status;
SELECT 'notification_dispatches.' || status || '|' || count(*) FROM notification_dispatches
  WHERE status IN ('pending', 'sending') GROUP BY status ORDER BY status;
SELECT 'document_versions.pdf_' || pdf_status || '|' || count(*) FROM document_versions GROUP BY pdf_status ORDER BY pdf_status;
SQL
)"

# Object keys the database points at (contract / approval PDFs), one per line.
OBJECT_KEYS_SQL="$(cat <<'SQL'
SELECT pdf_object_key FROM document_versions WHERE pdf_status = 'ready' AND pdf_object_key IS NOT NULL
UNION SELECT pdf_object_key FROM document_approvals WHERE pdf_object_key IS NOT NULL
ORDER BY 1;
SQL
)"

db_stats() { printf '%s\n' "$STATS_SQL" | pg_sql; }

# Asynq queues (Redis): pending / active / scheduled / retry / archived per queue.
asynq_stats() {
  docker exec -i "$REDIS_C" sh -c '
    [ -n "${REDIS_PASSWORD:-}" ] && export REDISCLI_AUTH="$REDIS_PASSWORD"
    r() { redis-cli --no-auth-warning "$@"; }
    for q in $(r SMEMBERS asynq:queues | sort); do
      echo "asynq.$q.pending|$(r LLEN "asynq:{$q}:pending")"
      echo "asynq.$q.active|$(r LLEN "asynq:{$q}:active")"
      echo "asynq.$q.scheduled|$(r ZCARD "asynq:{$q}:scheduled")"
      echo "asynq.$q.retry|$(r ZCARD "asynq:{$q}:retry")"
      echo "asynq.$q.archived|$(r ZCARD "asynq:{$q}:archived")"
    done
    echo "redis.dbsize|$(r DBSIZE)"'
}
