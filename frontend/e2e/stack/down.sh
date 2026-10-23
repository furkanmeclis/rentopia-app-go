#!/usr/bin/env bash
# Stops what up.sh started (processes by pid file, the two containers).
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
# shellcheck source=frontend/e2e/stack/env.sh
source "$ROOT/frontend/e2e/stack/env.sh"
for name in site panel api; do
  f="$E2E_STATE/$name.pid"
  if [ -f "$f" ]; then
    pid="$(cat "$f")"
    # setsid made each process a group leader: stop the whole group.
    kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
    rm -f "$f"
    echo "[e2e-stack] stopped $name"
  fi
done
if [ "${E2E_SKIP_INFRA:-0}" != "1" ]; then
  docker rm -f "$E2E_PG_CONTAINER" "$E2E_REDIS_CONTAINER" "$E2E_S3_CONTAINER" >/dev/null 2>&1 || true
  echo "[e2e-stack] removed containers"
fi
