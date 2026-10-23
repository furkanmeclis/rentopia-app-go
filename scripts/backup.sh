#!/usr/bin/env bash
# Encrypted backup of a production stack (docs/operations/backup-restore.md).
#
#   scripts/backup.sh [run] [--env-file FILE] [--dry-run] [--no-offsite]
#       One backup set, in this order (see the runbook for why):
#         postgres.dump  pg_dump -Fc of DB_NAME
#         redis.rdb      Redis snapshot: Asynq queues (pending / scheduled / retry)
#         objects.tar    every object of S3_BUCKET (SeaweedFS), copied last
#         manifest.txt   UTC stamp, schema version, row counts, queue sizes, checksums
#       packed and encrypted with age to BACKUP_AGE_RECIPIENT:
#         $BACKUP_DIR/<NAME_PREFIX>-<YYYYmmddTHHMMSSZ>.tar.age (+ .sha256, + .manifest.txt)
#       then pruned to BACKUP_KEEP_DAILY / BACKUP_KEEP_WEEKLY and, when
#       BACKUP_OFFSITE_BUCKET is set, uploaded off-site (never deleted there).
#
#   scripts/backup.sh schedule [--env-file FILE]
#       Runs `run` every day at BACKUP_TIME (HH:MM, container TZ). Entry
#       point of the compose `backup` service (profile "backup").
#
#   scripts/backup.sh status [--env-file FILE] [--max-age-hours N]
#       Lists the local backup sets; exit 1 if the newest is older than N
#       hours (default 26) or missing. Meant for monitoring.
#
# Env (from the environment, else from --env-file, default .env.server):
#   NAME_PREFIX, S3_BUCKET, S3_ACCESS_KEY, S3_SECRET_KEY, BACKUP_AGE_RECIPIENT
#   (age public key(s), comma separated), BACKUP_DIR, BACKUP_KEEP_DAILY,
#   BACKUP_KEEP_WEEKLY, BACKUP_TIME, BACKUP_OFFSITE_{ENDPOINT,REGION,BUCKET,ACCESS_KEY,SECRET_KEY}.
#
# Needs bash 4+, docker (daemon of the stack), age, sha256sum, tar.
# Single-quoted commands expand inside the containers on purpose.
# shellcheck disable=SC2016
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
# shellcheck source=scripts/backup-common.sh
. "$(dirname "$SELF")/backup-common.sh"

usage() { sed -n '2,27p' "$SELF"; }

cmd="run"
case "${1:-}" in run|schedule|status) cmd="$1"; shift ;; esac
ENV_FILE="${ENV_FILE:-}"; DRY_RUN=false; OFFSITE=true; MAX_AGE_HOURS=26
while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) ENV_FILE="${2:?--env-file needs a file}"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    --no-offsite) OFFSITE=false; shift ;;
    --max-age-hours) MAX_AGE_HOURS="${2:?--max-age-hours needs a number}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (see --help)" ;;
  esac
done

if [[ -n "$ENV_FILE" ]]; then load_env_file "$ENV_FILE"
elif [[ -f "$ROOT/.env.server" ]]; then ENV_FILE="$ROOT/.env.server"; load_env_file "$ENV_FILE"; fi

stack_init
BACKUP_DIR="${BACKUP_DIR:-/var/backups/${NAME_PREFIX}}"
BACKUP_KEEP_DAILY="${BACKUP_KEEP_DAILY:-7}"
BACKUP_KEEP_WEEKLY="${BACKUP_KEEP_WEEKLY:-4}"
BACKUP_TIME="${BACKUP_TIME:-03:15}"
[[ "$BACKUP_KEEP_DAILY" =~ ^[0-9]+$ && "$BACKUP_KEEP_WEEKLY" =~ ^[0-9]+$ ]] || die "BACKUP_KEEP_DAILY / BACKUP_KEEP_WEEKLY must be numbers"

# --- status ----------------------------------------------------------------------
cmd_status() {
  local files=() f newest age
  shopt -s nullglob
  files=("$BACKUP_DIR"/"$NAME_PREFIX"-????????T??????Z.tar.age)
  shopt -u nullglob
  if [[ ${#files[@]} -eq 0 ]]; then echo "no backup sets in $BACKUP_DIR"; exit 1; fi
  for f in "${files[@]}"; do printf '%s  %s\n' "$(du -h "$f" | cut -f1)" "$(basename "$f")"; done
  newest="$(printf '%s\n' "${files[@]}" | sort | tail -n1)"
  age=$(( ($(date +%s) - $(stat -c %Y "$newest")) / 3600 ))
  echo "newest: $(basename "$newest") (${age}h old, limit ${MAX_AGE_HOURS}h)"
  [[ "$age" -lt "$MAX_AGE_HOURS" ]] || { echo "STALE"; exit 1; }
  echo "ok"
}

# --- schedule --------------------------------------------------------------------
seconds_until() { # HH:MM in local time -> seconds from now (next occurrence)
  local hh mm now target
  [[ "$1" =~ ^([01][0-9]|2[0-3]):([0-5][0-9])$ ]] || die "BACKUP_TIME must be HH:MM, got: $1"
  hh=$((10#${BASH_REMATCH[1]})); mm=$((10#${BASH_REMATCH[2]}))
  read -r h m s <<<"$(date '+%H %M %S')"
  now=$((10#$h * 3600 + 10#$m * 60 + 10#$s)); target=$((hh * 3600 + mm * 60))
  if [[ $target -le $now ]]; then target=$((target + 86400)); fi
  echo $((target - now))
}

cmd_schedule() {
  local secs args=(run)
  [[ -n "$ENV_FILE" ]] && args+=(--env-file "$ENV_FILE")
  "$OFFSITE" || args+=(--no-offsite)
  seconds_until "$BACKUP_TIME" >/dev/null
  log "backup schedule: daily at $BACKUP_TIME ($(date +%Z)), sets in $BACKUP_DIR"
  while true; do
    secs="$(seconds_until "$BACKUP_TIME")"
    log "next backup in ${secs}s"
    sleep "$secs"
    if "$SELF" "${args[@]}"; then log "backup finished"; else log "BACKUP FAILED (exit $?)"; fi
    sleep 61 # never twice in the same minute
  done
}

# --- run -------------------------------------------------------------------------
WORK=""; LOCK_DIR=""
cleanup() {
  [[ -n "$WORK" && -d "$WORK" ]] && rm -rf "${WORK:?}"
  [[ -n "$LOCK_DIR" && -d "$LOCK_DIR" ]] && rmdir "$LOCK_DIR" 2>/dev/null
  return 0
}

age_args() {
  local r out=()
  IFS=', ' read -r -a rs <<<"$BACKUP_AGE_RECIPIENT"
  for r in "${rs[@]}"; do [[ -n "$r" ]] && out+=(-r "$r"); done
  [[ ${#out[@]} -gt 0 ]] || die "BACKUP_AGE_RECIPIENT is empty"
  printf '%s\n' "${out[@]}"
}

prune() {
  local files=() f name day wk keep ndays=0 nweeks=0
  declare -A days=() weeks=() keepset=()
  shopt -s nullglob
  files=("$BACKUP_DIR"/"$NAME_PREFIX"-????????T??????Z.tar.age)
  shopt -u nullglob
  [[ ${#files[@]} -gt 0 ]] || return 0
  # Newest first: keep the newest set of each of the last N days and of the
  # last M weeks (weeks start on Monday, UTC). The newest set is always kept.
  while IFS= read -r f; do
    name="$(basename "$f")"; day="${name#"${NAME_PREFIX}"-}"; day="${day:0:8}"
    wk=$(( ( $(date -u -d "${day:0:4}-${day:4:2}-${day:6:2}" +%s) / 86400 + 3 ) / 7 ))
    keep=false
    [[ ${#keepset[@]} -eq 0 ]] && keep=true
    if [[ ! -v days["$day"] && $ndays -lt $BACKUP_KEEP_DAILY ]]; then days["$day"]=1; ndays=$((ndays + 1)); keep=true; fi
    if [[ ! -v weeks["$wk"] && $nweeks -lt $BACKUP_KEEP_WEEKLY ]]; then weeks["$wk"]=1; nweeks=$((nweeks + 1)); keep=true; fi
    if "$keep"; then keepset["$f"]=1; continue; fi
    if "$DRY_RUN"; then log "would prune $name"; continue; fi
    rm -f "$f" "$f.sha256" "${f%.tar.age}.manifest.txt"
    log "pruned $name"
  done < <(printf '%s\n' "${files[@]}" | sort -r)
}

cmd_run() {
  require_vars S3_BUCKET S3_ACCESS_KEY S3_SECRET_KEY BACKUP_AGE_RECIPIENT
  need_cmd docker; need_cmd age "apt install age / apk add age"; need_cmd sha256sum; need_cmd tar
  local recipients stamp name out
  mapfile -t recipients < <(age_args)
  require_running "$PG_C" "$REDIS_C" "$S3_C"
  docker network inspect "$APP_NETWORK_NAME" >/dev/null 2>&1 || die "network not found: $APP_NETWORK_NAME"

  stamp="$(date -u +%Y%m%dT%H%M%SZ)"; name="${NAME_PREFIX}-${stamp}"
  out="$BACKUP_DIR/$name.tar.age"
  if "$DRY_RUN"; then
    log "dry run: would write $out"
    log "  postgres  $PG_C ($(printf 'SELECT pg_size_pretty(pg_database_size(current_database()));' | pg_sql))"
    log "  redis     $REDIS_C ($(redis_cli DBSIZE) keys)"
    log "  bucket    $S3_BUCKET via $BACKUP_S3_ENDPOINT"
    log "  recipients: $(( ${#recipients[@]} / 2 )) age key(s)"
    if [[ -n "${BACKUP_OFFSITE_BUCKET:-}" ]] && "$OFFSITE"; then log "  off-site  ${BACKUP_OFFSITE_BUCKET}/${NAME_PREFIX}/"; else log "  off-site  off"; fi
    prune
    return 0
  fi

  umask 077
  mkdir -p "$BACKUP_DIR"
  LOCK_DIR="$BACKUP_DIR/.lock"
  mkdir "$LOCK_DIR" 2>/dev/null || die "another backup is running (remove $LOCK_DIR if stale)"
  trap cleanup EXIT
  WORK="$BACKUP_DIR/.work-$stamp"; mkdir -p "$WORK"

  # 1. Postgres first: rows written after this point are simply not in the set.
  log "postgres: pg_dump $PG_C"
  db_stats > "$WORK/stats-db.txt"
  pg_exec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$WORK/postgres.dump"
  [[ -s "$WORK/postgres.dump" ]] || die "pg_dump wrote nothing"

  # 2. Redis second: Asynq tasks enqueued after the dump may point at rows
  #    the dump does not have (they fail as not found); the reverse (rows
  #    whose task is missing) is what the DB sweeps recover.
  log "redis: snapshot $REDIS_C"
  asynq_stats > "$WORK/stats-redis.txt"
  docker exec "$REDIS_C" sh -c '
    [ -n "${REDIS_PASSWORD:-}" ] && export REDISCLI_AUTH="$REDIS_PASSWORD"
    f=/tmp/backup-$$.rdb
    redis-cli --no-auth-warning --rdb "$f" >/tmp/backup-rdb.log 2>&1 || { cat /tmp/backup-rdb.log >&2; exit 1; }
    cat "$f"; rc=$?; rm -f "$f"; exit $rc' > "$WORK/redis.rdb"
  [[ -s "$WORK/redis.rdb" ]] || die "redis snapshot is empty"

  # 3. Objects last: every key the dump references was uploaded before its
  #    row was written, so it is in the copy (newer objects are orphans).
  log "objects: sync s3://$S3_BUCKET"
  export_s3_remote_src
  local mirror="${NAME_PREFIX}-backup-mirror"
  docker volume create "$mirror" >/dev/null
  rclone_src -v "$mirror:/mirror" -- sync "src:$S3_BUCKET" /mirror --fast-list --checksum --transfers 8 --log-level ERROR
  rclone_src -- size "src:$S3_BUCKET" --fast-list > "$WORK/stats-objects.txt"
  docker run --rm -v "$mirror:/mirror:ro" --entrypoint tar "$RCLONE_IMAGE" -C /mirror -cf - . > "$WORK/objects.tar"

  {
    echo "set=$name"
    echo "name_prefix=$NAME_PREFIX"
    echo "created_utc=$stamp"
    echo "host=$(hostname)"
    echo "bucket=$S3_BUCKET"
    echo "order=postgres,redis,objects"
    echo
    echo "# sha256"
    (cd "$WORK" && sha256sum postgres.dump redis.rdb objects.tar)
    echo
    echo "# postgres (at dump time)"
    cat "$WORK/stats-db.txt"
    echo
    echo "# redis / asynq (at snapshot time)"
    cat "$WORK/stats-redis.txt"
    echo
    echo "# objects"
    cat "$WORK/stats-objects.txt"
    echo "object_keys_in_db=$(printf '%s\n' "$OBJECT_KEYS_SQL" | pg_sql | grep -c . || true)"
  } > "$WORK/manifest.txt"

  log "encrypt: $(basename "$out")"
  tar -C "$WORK" -cf - manifest.txt postgres.dump redis.rdb objects.tar | age "${recipients[@]}" -o "$out.partial"
  mv "$out.partial" "$out"
  (cd "$BACKUP_DIR" && sha256sum "$(basename "$out")" > "$(basename "$out").sha256")
  cp "$WORK/manifest.txt" "$BACKUP_DIR/$name.manifest.txt"
  log "wrote $out ($(du -h "$out" | cut -f1))"

  prune

  if [[ -n "${BACKUP_OFFSITE_BUCKET:-}" ]] && "$OFFSITE"; then
    require_vars BACKUP_OFFSITE_ENDPOINT BACKUP_OFFSITE_ACCESS_KEY BACKUP_OFFSITE_SECRET_KEY
    export_s3_remote_offsite
    local dest="offsite:${BACKUP_OFFSITE_BUCKET}/${NAME_PREFIX}"
    log "off-site: upload to ${BACKUP_OFFSITE_BUCKET}/${NAME_PREFIX}/"
    # rcat streams stdin: no host path is mounted into the helper container.
    # shellcheck disable=SC2094 # reads the local file, writes the remote copy
    rclone_offsite rcat "$dest/$(basename "$out")" --s3-no-check-bucket < "$out"
    rclone_offsite rcat "$dest/$(basename "$out").sha256" --s3-no-check-bucket < "$out.sha256"
    rclone_offsite rcat "$dest/$name.manifest.txt" --s3-no-check-bucket < "$BACKUP_DIR/$name.manifest.txt"
  fi
  log "done: $name"
}

case "$cmd" in
  run) cmd_run ;;
  schedule) cmd_schedule ;;
  status) cmd_status ;;
esac
