#!/usr/bin/env bash
# Pull the deploy branch and redeploy the production stack when it moved.
# Meant for cron on a plain Docker host, e.g. every 5 minutes:
#
#   */5 * * * * /opt/rentopya/scripts/server-sync.sh >> /var/log/rentopya-sync.log 2>&1
#
#   scripts/server-sync.sh [--force] [--dry-run]
#     --force    rebuild and restart even if the branch did not move
#     --dry-run  only report whether an update is pending
#
# Environment (all optional):
#   BRANCH=main            branch to follow (only CI-green commits reach main)
#   ENV_FILE=.env.server   production env file (see scripts/prod-env.sh)
#   HEALTH_TIMEOUT=300     seconds to wait for the backend to become healthy
#   SYNC_LOCK=/tmp/<repo>-sync.lock
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BRANCH="${BRANCH:-main}"
ENV_FILE="${ENV_FILE:-.env.server}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-300}"
SYNC_LOCK="${SYNC_LOCK:-/tmp/$(basename "$ROOT")-sync.lock}"
FORCE=false; DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --force) FORCE=true ;;
    --dry-run) DRY_RUN=true ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

log() { printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*"; }
compose() { docker compose --env-file "$ENV_FILE" -f compose.prod.yml "$@"; }

# One sync at a time; a long build must not be started twice by cron.
exec 9>"$SYNC_LOCK"
flock -n 9 || { log "another sync is running; skipping"; exit 0; }

PAUSED=false
if [[ -f .sync-paused ]]; then
  PAUSED=true
  [[ "$FORCE" == true ]] || { log "paused (.sync-paused exists); skipping"; exit 0; }
fi

[[ -f "$ENV_FILE" ]] || { log "missing $ENV_FILE; create it with: scripts/prod-env.sh init -o $ENV_FILE"; exit 1; }

current="$(git rev-parse HEAD)"
if [[ "$PAUSED" == true ]]; then
  target="$current"; log "paused: rebuilding ${current:0:7} without pulling"
else
  git fetch --quiet origin "$BRANCH"
  target="$(git rev-parse "origin/$BRANCH")"
fi

if [[ "$current" == "$target" && "$FORCE" == false ]]; then
  log "up to date at ${current:0:7}"
  exit 0
fi
if [[ "$DRY_RUN" == true ]]; then
  log "update pending: ${current:0:7} -> ${target:0:7}"
  git --no-pager log --oneline "$current..$target" | head -20
  exit 0
fi

if [[ "$current" != "$target" ]]; then
  if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
    log "tracked files changed locally; refusing to pull. Inspect with: git -C $ROOT status"
    exit 1
  fi
  log "updating ${current:0:7} -> ${target:0:7}"
  git --no-pager log --oneline "$current..$target" | head -20
  git checkout --quiet "$BRANCH" 2>/dev/null || git checkout --quiet -B "$BRANCH" "origin/$BRANCH"
  git merge --quiet --ff-only "origin/$BRANCH"
fi

# A release may add env keys; upgrade keeps every existing value.
if ! scripts/prod-env.sh check "$ENV_FILE" >/dev/null 2>&1; then
  log "env schema changed; upgrading $ENV_FILE (backup kept)"
  scripts/prod-env.sh upgrade "$ENV_FILE"
fi

compose config -q
log "building and starting"
compose up -d --build --remove-orphans

prefix="$(grep -E '^NAME_PREFIX=' "$ENV_FILE" | head -1 | cut -d= -f2- | tr -d '"')"
backend="${prefix:-rentopya}-backend"
deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
status=""
while (( $(date +%s) < deadline )); do
  status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$backend" 2>/dev/null || true)"
  [[ "$status" == "healthy" ]] && break
  sleep 5
done
if [[ "$status" != "healthy" ]]; then
  log "backend is '$status' after ${HEALTH_TIMEOUT}s; check: docker logs $backend"
  log "previous commit was ${current:0:7}. To roll back code: touch $ROOT/.sync-paused && git -C $ROOT checkout $current && $0 --force (migrations are not reverted)"
  exit 1
fi

docker image prune -f >/dev/null 2>&1 || true
log "deployed ${target:0:7}"
