#!/usr/bin/env bash
# Restore a backup set written by scripts/backup.sh into a running stack
# (docs/operations/backup-restore.md).
#
#   scripts/restore.sh ARCHIVE --identity FILE --env-file FILE [options]
#       ARCHIVE    <NAME_PREFIX>-<stamp>.tar.age (its .sha256 is checked when present)
#       --identity age private key file (kept off the server; see the runbook)
#       --env-file env of the TARGET stack (NAME_PREFIX, S3_*); required
#     Order: stop backend + worker -> Postgres (database recreated, then
#     pg_restore --clean --if-exists) -> bucket (rclone sync) -> Redis
#     (dump.rdb, AOF rebuilt) -> Meilisearch emptied -> migrations (migrate
#     service) -> verify (row counts vs manifest, PDF keys in the bucket,
#     Asynq queues) -> start worker + backend -> checklist.
#   Options:
#     --target staging|prod  default staging. A set restored into the stack it
#                            came from (same NAME_PREFIX) counts as prod.
#     --yes-i-mean-prod      required for a prod target
#     --yes                  no interactive confirmation
#     --dry-run              decrypt + check only; the stack is not touched
#     --no-start             leave backend + worker stopped (review jobs first)
#     --skip-objects / --skip-redis   keep the target's bucket / Redis as is
#     --keep-search          keep Meilisearch data (default: emptied, so the
#                            worker rebuilds every index from the restored DB)
#     --work-dir DIR         where the set is decrypted (default: mktemp under /tmp)
#
#   scripts/restore.sh verify --env-file FILE [--manifest FILE]
#       RT-A32 checks against a running stack: row counts (compared with the
#       set's manifest when given), PDF object keys present in the bucket,
#       integration_jobs / Asynq state.
#
# Needs bash 4+, docker, age, sha256sum, tar.
# Single-quoted commands expand inside the containers on purpose.
# shellcheck disable=SC2016
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
# shellcheck source=scripts/backup-common.sh
. "$(dirname "$SELF")/backup-common.sh"

usage() { sed -n '2,31p' "$SELF"; }

cmd="restore"
[[ "${1:-}" == "verify" ]] && { cmd="verify"; shift; }
ARCHIVE=""; IDENTITY="${BACKUP_AGE_IDENTITY_FILE:-}"; ENV_FILE=""; TARGET="staging"; PROD_OK=false
ASSUME_YES=false; DRY_RUN=false; START=true; DO_OBJECTS=true; DO_REDIS=true; DO_SEARCH=true; WORK_DIR=""; MANIFEST=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --identity|-i) IDENTITY="${2:?--identity needs a file}"; shift 2 ;;
    --env-file) ENV_FILE="${2:?--env-file needs a file}"; shift 2 ;;
    --target) TARGET="${2:?--target needs staging|prod}"; shift 2 ;;
    --yes-i-mean-prod) PROD_OK=true; shift ;;
    --yes|-y) ASSUME_YES=true; shift ;;
    --dry-run) DRY_RUN=true; shift ;;
    --no-start) START=false; shift ;;
    --skip-objects) DO_OBJECTS=false; shift ;;
    --skip-redis) DO_REDIS=false; shift ;;
    --keep-search) DO_SEARCH=false; shift ;;
    --work-dir) WORK_DIR="${2:?--work-dir needs a directory}"; shift 2 ;;
    --manifest) MANIFEST="${2:?--manifest needs a file}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*) die "unknown option: $1 (see --help)" ;;
    *) [[ -z "$ARCHIVE" ]] || die "unexpected argument: $1"; ARCHIVE="$1"; shift ;;
  esac
done
[[ "$TARGET" == staging || "$TARGET" == prod ]] || die "--target must be staging or prod"
[[ -n "$ENV_FILE" ]] || die "--env-file of the target stack is required"
load_env_file "$ENV_FILE"
stack_init
need_cmd docker

BACKEND_C="${NAME_PREFIX}-backend"; WORKER_C="${NAME_PREFIX}-worker"
MIGRATE_C="${NAME_PREFIX}-migrate"; BACKUP_C="${NAME_PREFIX}-backup"; MEILI_C="${NAME_PREFIX}-meilisearch"
exists() { docker inspect "$1" >/dev/null 2>&1; }

# --- verify ----------------------------------------------------------------------
cmd_verify() {
  require_vars S3_BUCKET S3_ACCESS_KEY S3_SECRET_KEY
  require_running "$PG_C" "$REDIS_C" "$S3_C"
  local tmp rc=0; tmp="$(mktemp -d)"

  echo "== (a) records: row counts (backup set -> now)"
  { db_stats; asynq_stats; } > "$tmp/now.txt"
  if [[ -n "$MANIFEST" ]]; then
    [[ -f "$MANIFEST" ]] || die "manifest not found: $MANIFEST"
    awk -F'|' '
      NR == FNR { if ($0 ~ /^[a-z_]+[.|]/ && NF == 2) before[$1] = $2; next }
      NF == 2 { now[$1] = $2; if (!($1 in before)) before[$1] = "-" }
      END {
        bad = 0
        for (k in before) {
          n = (k in now) ? now[k] : "-"
          flag = (k ~ /^rows\./ && before[k] != n) ? "  <-- differs" : ""
          if (flag != "") bad++
          printf "  %-44s %10s %10s%s\n", k, before[k], n, flag
        }
        if (bad) printf "  %d table(s) differ: expected only when the stack was used after the restore\n", bad
      }' "$MANIFEST" "$tmp/now.txt" | sort
  else
    sed 's/^/  /' "$tmp/now.txt"
  fi

  echo "== (a) documents: PDF object keys referenced by the database"
  export_s3_remote_src
  printf '%s\n' "$OBJECT_KEYS_SQL" | pg_sql | sort -u > "$tmp/keys-db.txt"
  rclone_src -- lsf -R --files-only --fast-list "src:$S3_BUCKET" | sort -u > "$tmp/keys-bucket.txt"
  comm -23 "$tmp/keys-db.txt" "$tmp/keys-bucket.txt" > "$tmp/missing.txt"
  echo "  referenced: $(grep -c . "$tmp/keys-db.txt" || true)  in bucket: $(grep -c . "$tmp/keys-bucket.txt" || true)  missing: $(grep -c . "$tmp/missing.txt" || true)"
  if [[ -s "$tmp/missing.txt" ]]; then head -n 20 "$tmp/missing.txt" | sed 's/^/  missing: /'; fi

  echo "== (b) work to resume"
  grep -E '^(integration_jobs|notifications|notification_dispatches|document_versions|asynq|redis)\.' "$tmp/now.txt" | sed 's/^/  /' || true
  echo "  integration jobs due now (the minute sweep picks these up):"
  printf '%s\n' "SELECT '    ' || provider || ' ' || kind || ' ' || status || ' x' || count(*) FROM integration_jobs
    WHERE status IN ('pending', 'submitted', 'needs_verification') AND attempts < max_attempts
    GROUP BY provider, kind, status ORDER BY 1;" | pg_sql

  cat <<'EOF'
== checklist (docs/operations/backup-restore.md, "RT-A32 doğrulama")
  [ ] (a) rows.* counts equal the manifest (before any new traffic); missing PDF keys = 0
  [ ] (a) open a contract PDF and an inspection photo in the panel
  [ ] (b) worker log: integration_job sweep runs; pending/submitted jobs move on (Entegrasyon işleri ekranı)
  [ ] (b) Asynq pending/scheduled/retry drain; document_versions.pdf_pending goes to 0
  [ ] (c) no second collection / invoice / ledger entry for the same payment (SQL in the runbook)
  [ ] (c) provider side: SMS / e-mail / WhatsApp sent after the backup time are reviewed (not deduplicated)
EOF
  [[ ! -s "$tmp/missing.txt" ]] || rc=1
  rm -rf "${tmp:?}"
  return "$rc"
}

if [[ "$cmd" == verify ]]; then cmd_verify; exit $?; fi

# --- restore ---------------------------------------------------------------------
[[ -n "$ARCHIVE" ]] || die "usage: $0 ARCHIVE --identity FILE --env-file FILE (see --help)"
[[ -f "$ARCHIVE" ]] || die "archive not found: $ARCHIVE"
[[ -n "$IDENTITY" ]] || die "--identity (age private key file) is required"
[[ -f "$IDENTITY" ]] || die "identity file not found: $IDENTITY"
need_cmd age; need_cmd sha256sum; need_cmd tar
require_vars S3_BUCKET S3_ACCESS_KEY S3_SECRET_KEY

# Prod guard before the (slow) decrypt; checked again against the manifest.
ARCHIVE_PREFIX="$(basename "$ARCHIVE")"; ARCHIVE_PREFIX="${ARCHIVE_PREFIX%-????????T??????Z.tar.age}"
if [[ ( "$TARGET" == prod || "$ARCHIVE_PREFIX" == "$NAME_PREFIX" ) ]] && ! "$PROD_OK"; then
  die "target '$NAME_PREFIX' is prod (or the stack the set came from): pass --yes-i-mean-prod"
fi

if [[ -f "$ARCHIVE.sha256" ]]; then
  log "checksum: $(basename "$ARCHIVE")"
  (cd "$(dirname "$ARCHIVE")" && sha256sum -c --quiet "$(basename "$ARCHIVE").sha256") || die "archive checksum mismatch"
else
  log "warning: no $(basename "$ARCHIVE").sha256 next to the archive; skipping the checksum"
fi

if [[ -z "$WORK_DIR" ]]; then WORK_DIR="$(mktemp -d)"; CLEAN_WORK=true; else mkdir -p "$WORK_DIR"; CLEAN_WORK=false; fi
chmod 700 "$WORK_DIR"
MIRROR="${NAME_PREFIX}-restore-mirror"
# shellcheck disable=SC2329 # run by the EXIT trap
cleanup() {
  "$CLEAN_WORK" && rm -rf "${WORK_DIR:?}"
  docker volume rm -f "$MIRROR" >/dev/null 2>&1 || true
  return 0
}
trap cleanup EXIT

log "decrypt: $(basename "$ARCHIVE") -> $WORK_DIR"
age -d -i "$IDENTITY" < "$ARCHIVE" | tar -xf - -C "$WORK_DIR"
for f in manifest.txt postgres.dump redis.rdb objects.tar; do [[ -f "$WORK_DIR/$f" ]] || die "set is missing $f"; done
(cd "$WORK_DIR" && grep -E '^[0-9a-f]{64}  ' manifest.txt | sha256sum -c --quiet) || die "set content checksum mismatch"
SRC_PREFIX="$(sed -n 's/^name_prefix=//p' "$WORK_DIR/manifest.txt")"
SET_NAME="$(sed -n 's/^set=//p' "$WORK_DIR/manifest.txt")"
log "set $SET_NAME (from $SRC_PREFIX) -> target $NAME_PREFIX ($TARGET)"

if [[ "$SRC_PREFIX" == "$NAME_PREFIX" && "$TARGET" != prod ]]; then
  log "target NAME_PREFIX equals the set's: this is the stack the backup came from -> prod"
  TARGET=prod
fi
if [[ "$TARGET" == prod ]] && ! "$PROD_OK"; then
  die "refusing to restore into prod without --yes-i-mean-prod"
fi

if "$DRY_RUN"; then
  sed -n '/^# postgres/,/^$/p' "$WORK_DIR/manifest.txt"
  log "dry run: set is readable and consistent; nothing restored"
  exit 0
fi

require_running "$PG_C" "$S3_C"
exists "$REDIS_C" || die "container not found: $REDIS_C"
if ! "$ASSUME_YES"; then
  [[ -t 0 ]] || die "not a terminal: pass --yes to confirm"
  what="database"; "$DO_OBJECTS" && what+=", bucket"; "$DO_REDIS" && what+=", Redis"
  echo "This REPLACES the $what of stack '$NAME_PREFIX' ($TARGET)"
  echo "with set $SET_NAME. Staging must not reach real providers (runbook: 'Staging yalıtımı')."
  read -rp "Type the target NAME_PREFIX to continue: " answer
  [[ "$answer" == "$NAME_PREFIX" ]] || die "aborted"
fi

# 1. Stop the writers.
BACKUP_WAS_RUNNING=false
container_running "$BACKUP_C" && BACKUP_WAS_RUNNING=true
for c in "$BACKEND_C" "$WORKER_C" "$BACKUP_C"; do
  if container_running "$c"; then log "stop $c"; docker stop -t 40 "$c" >/dev/null; fi
done

# 2. Postgres: a fresh database, so tables of newer migrations do not linger.
log "postgres: recreate database and pg_restore"
docker exec "$PG_C" sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"
  psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
    -c "DROP DATABASE IF EXISTS \"$POSTGRES_DB\" WITH (FORCE)" \
    -c "CREATE DATABASE \"$POSTGRES_DB\" OWNER \"$POSTGRES_USER\""'
pg_exec 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-acl --exit-on-error' < "$WORK_DIR/postgres.dump"

# 3. Bucket: the set's objects, extra objects removed (sync).
if "$DO_OBJECTS"; then
  log "objects: sync into s3://$S3_BUCKET"
  export_s3_remote_src
  docker volume rm -f "$MIRROR" >/dev/null 2>&1 || true
  docker volume create "$MIRROR" >/dev/null
  docker run --rm -i -v "$MIRROR:/mirror" --entrypoint tar "$RCLONE_IMAGE" -xf - -C /mirror < "$WORK_DIR/objects.tar"
  rclone_src -v "$MIRROR:/mirror:ro" -- sync /mirror "src:$S3_BUCKET" --fast-list --checksum --transfers 8 --log-level ERROR
fi

# 4. Redis: dump.rdb in place and the AOF rebuilt from it (the server runs
#    with appendonly yes and would otherwise load its old AOF).
if "$DO_REDIS"; then
  log "redis: load snapshot into $REDIS_C"
  redis_image="$(docker inspect -f '{{.Config.Image}}' "$REDIS_C")"
  if container_running "$REDIS_C"; then docker stop "$REDIS_C" >/dev/null; fi
  docker run --rm -i --network none --volumes-from "$REDIS_C" --entrypoint sh "$redis_image" -c '
    set -eu
    owner="$(stat -c %u:%g /data)"
    cd /data
    rm -rf appendonlydir appendonly.aof dump.rdb
    cat > dump.rdb
    redis-server --dir /data --dbfilename dump.rdb --appendonly no --save "" --port 6399 --daemonize yes >/dev/null
    r() { redis-cli -p 6399 "$@"; }
    i=0; until r PING 2>/dev/null | grep -q PONG && r INFO persistence | grep -q "^loading:0"; do i=$((i+1)); [ $i -lt 600 ] || exit 1; sleep 0.2; done
    r CONFIG SET appendonly yes >/dev/null
    i=0; until r INFO persistence | grep -q "^aof_enabled:1" && r INFO persistence | grep -q "^aof_rewrite_in_progress:0" && r INFO persistence | grep -q "^aof_rewrite_scheduled:0"; do i=$((i+1)); [ $i -lt 1500 ] || exit 1; sleep 0.2; done
    echo "keys: $(r DBSIZE)"
    r SHUTDOWN >/dev/null 2>&1 || true
    chown -R "$owner" /data' < "$WORK_DIR/redis.rdb"
fi
container_running "$REDIS_C" || docker start "$REDIS_C" >/dev/null

# 5. Search: Meilisearch is not in the set and would keep documents of
#    records the restored DB does not have. Emptied, the worker's start-up
#    bootstrap reindexes every empty index (searchengine.Indexer.Bootstrap).
if "$DO_SEARCH" && exists "$MEILI_C"; then
  log "search: empty $MEILI_C (rebuilt by the worker on start)"
  docker stop "$MEILI_C" >/dev/null
  docker run --rm --network none --volumes-from "$MEILI_C" --entrypoint sh "$RCLONE_IMAGE" -c 'find /meili_data -mindepth 1 -delete'
  docker start "$MEILI_C" >/dev/null
fi

# 6. Migrations: the image may be newer than the set.
if exists "$MIGRATE_C"; then
  log "migrate: $MIGRATE_C"
  docker start -a "$MIGRATE_C"
elif [[ -f "$ROOT/compose.prod.yml" ]]; then
  log "migrate: compose run migrate"
  docker compose --env-file "$ENV_FILE" -f "$ROOT/compose.prod.yml" run --rm --no-deps migrate
else
  die "no $MIGRATE_C container and no compose.prod.yml: run the migrate service, then start backend + worker"
fi

# 7. Verify before any new traffic: counts must equal the manifest.
cp "$WORK_DIR/manifest.txt" "${TMPDIR:-/tmp}/$SET_NAME.manifest.txt"
MANIFEST="${TMPDIR:-/tmp}/$SET_NAME.manifest.txt"
log "restored $SET_NAME into $NAME_PREFIX; verifying (manifest kept at $MANIFEST)"
VERIFY_RC=0
cmd_verify || VERIFY_RC=$?

# 8. Start: the worker resumes the queued work (runbook, RT-A32 (b)).
wait_healthy() { # container, tries (3 s apart)
  local i st
  for ((i = 0; i < $2; i++)); do
    st="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$1")"
    case "$st" in healthy|none) return 0 ;; esac
    sleep 3
  done
  return 1
}
if "$START"; then
  # The worker bootstrap skips indexes it cannot reach.
  if exists "$MEILI_C"; then wait_healthy "$MEILI_C" 40 || log "warning: $MEILI_C not healthy; run a reindex later"; fi
  for c in "$WORKER_C" "$BACKEND_C"; do
    if exists "$c"; then log "start $c"; docker start "$c" >/dev/null; fi
  done
  if exists "$BACKEND_C"; then
    wait_healthy "$BACKEND_C" 60 || true
    log "backend: $(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$BACKEND_C")"
  fi
  if "$BACKUP_WAS_RUNNING"; then docker start "$BACKUP_C" >/dev/null; fi
else
  log "--no-start: backend and worker stay stopped; start them with: docker start $WORKER_C $BACKEND_C"
fi

[[ "$VERIFY_RC" -eq 0 ]] || log "verify: some PDF keys are missing from the bucket (see above)"
log "re-run later: $0 verify --env-file $ENV_FILE --manifest $MANIFEST"
exit "$VERIFY_RC"
