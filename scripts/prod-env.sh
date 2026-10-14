#!/usr/bin/env bash
# Production env generator and upgrader, driven by deploy/env/schema.
#
#   scripts/prod-env.sh init [-o FILE] [--app-url URL]
#       Asks only for the public app URL, derives URLs and generates every
#       secret. Writes FILE (default .env.server).
#
#   scripts/prod-env.sh upgrade FILE [-o OUT] [--app-url URL]
#       Brings an env written by an older version up to the current schema:
#       keeps every existing value, applies renames, generates only missing
#       keys, comments out removed ones. Writes in place (backup kept) unless
#       -o is given.
#
#   scripts/prod-env.sh check FILE
#       Lists missing managed keys and the schema version gap; exit 1 if any.
#
# Needs bash 4+, openssl and awk.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCHEMA="${PROD_ENV_SCHEMA:-$ROOT/deploy/env/schema}"

die() { echo "error: $*" >&2; exit 1; }
trim() { local s="$1"; s="${s#"${s%%[![:space:]]*}"}"; printf '%s' "${s%"${s##*[![:space:]]}"}"; }

# --- schema ------------------------------------------------------------------
SCHEMA_VERSION=0
declare -a ORDER=()      # keys and "#section" lines, in schema order
declare -A RULE=()
declare -a RENAMES=()    # "OLD NEW VERSION"
declare -a REMOVALS=()   # "KEY VERSION"

load_schema() {
  [[ -f "$SCHEMA" ]] || die "schema not found: $SCHEMA"
  local line key rule
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="$(trim "$line")"
    case "$line" in
      "") continue ;;
      "# ---"*) ORDER+=("#${line#\# }") ;;
      "#"*) continue ;;
      "@version "*) SCHEMA_VERSION="$(trim "${line#@version }")" ;;
      "@rename "*) RENAMES+=("$(trim "${line#@rename }")") ;;
      "@remove "*) REMOVALS+=("$(trim "${line#@remove }")") ;;
      *"|"*)
        key="$(trim "${line%%|*}")"; rule="$(trim "${line#*|}")"
        [[ "$key" =~ ^[A-Z][A-Z0-9_]*$ ]] || die "bad key in schema: $key"
        ORDER+=("$key"); RULE["$key"]="$rule" ;;
      *) die "cannot parse schema line: $line" ;;
    esac
  done < "$SCHEMA"
  [[ "$SCHEMA_VERSION" =~ ^[0-9]+$ && "$SCHEMA_VERSION" -gt 0 ]] || die "schema has no @version"
}

# --- existing env --------------------------------------------------------------
declare -A OLD=()
declare -a OLD_ORDER=()

load_env() {
  local file="$1" line key val
  [[ -f "$file" ]] || die "env file not found: $file"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
    key="${BASH_REMATCH[2]}"; val="${BASH_REMATCH[3]}"
    if [[ "$val" =~ ^\"(.*)\"$ ]]; then val="${BASH_REMATCH[1]//\\\"/\"}"
    elif [[ "$val" =~ ^\'(.*)\'$ ]]; then val="${BASH_REMATCH[1]}"; fi
    [[ -v OLD["$key"] ]] || OLD_ORDER+=("$key")
    OLD["$key"]="$val"
  done < "$file"
}

# --- generators ----------------------------------------------------------------
b64url() { base64 | tr -d '\n=' | tr '+/' '-_'; }

gen_secret() {
  local kind="$1" n="$2" out=""
  case "$kind" in
    hex) openssl rand -hex "$n" ;;
    b64) openssl rand -base64 "$n" | tr -d '\n' ;;
    alnum)
      while [[ ${#out} -lt $n ]]; do out+="$(openssl rand -base64 64 | tr -dc 'A-Za-z0-9')"; done
      printf '%s' "${out:0:$n}" ;;
    *) die "unknown secret kind: $kind" ;;
  esac
}

VAPID_PUB=""; VAPID_PRIV=""
gen_vapid() {
  [[ -n "$VAPID_PUB" ]] && return
  local der; der="$(mktemp)"
  openssl ecparam -name prime256v1 -genkey -noout 2>/dev/null | openssl ec -outform DER -out "$der" 2>/dev/null
  # SEC1 DER for P-256: 7-byte header, 32-byte private scalar, ..., 65-byte uncompressed point at the end.
  VAPID_PRIV="$(head -c 39 "$der" | tail -c 32 | b64url)"
  VAPID_PUB="$(tail -c 65 "$der" | b64url)"
  rm -f "$der"
  [[ ${#VAPID_PRIV} -eq 43 && ${#VAPID_PUB} -eq 87 ]] || die "VAPID key generation failed"
}

# --- resolution ----------------------------------------------------------------
declare -A NEW=()
APP_URL_ARG=""

normalize_url() {
  local u; u="$(trim "$1")"; u="${u%/}"
  [[ "$u" =~ ^https?://[^/[:space:]]+$ ]] || return 1
  printf '%s' "$u"
}

url_host() { local h="${1#*://}"; h="${h%%/*}"; printf '%s' "${h%%:*}"; }

ask_app_url() {
  local u
  if [[ -n "$APP_URL_ARG" ]]; then
    normalize_url "$APP_URL_ARG" || die "invalid --app-url: $APP_URL_ARG (expected https://host)"
    return
  fi
  [[ -t 0 ]] || die "APP_URL is required: pass --app-url https://your.domain"
  while true; do
    read -rp "Uygulama adresi (ör. https://rentopia.example.com): " u
    if u="$(normalize_url "$u")"; then printf '%s' "$u"; return; fi
    echo "  Geçersiz adres: https:// ile başlamalı, yol içermemeli." >&2
  done
}

render() {
  local t="$1" k
  [[ ! -v NEW[APP_URL] ]] || t="${t//\{APP_HOST\}/$(url_host "${NEW[APP_URL]}")}"
  for k in "${!NEW[@]}"; do t="${t//\{$k\}/${NEW[$k]}}"; done
  [[ "$t" != *"{"*"}"* ]] || die "unresolved placeholder in template: $1"
  printf '%s' "$t"
}

generate() {
  local key="$1" rule="${RULE[$1]}" kind n
  case "$rule" in
    ask:APP_URL) ask_app_url ;;
    derive:*) render "${rule#derive:}" ;;
    const:*) printf '%s' "${rule#const:}" ;;
    empty) printf '' ;;
    secret:*) IFS=: read -r _ kind n <<<"$rule"; gen_secret "$kind" "$n" ;;
    vapid:public) gen_vapid; printf '%s' "$VAPID_PUB" ;;
    vapid:private) gen_vapid; printf '%s' "$VAPID_PRIV" ;;
    *) die "unknown rule for $key: $rule" ;;
  esac
}

declare -a ADDED=() RENAMED=() REMOVED=() KEPT=()

resolve() {
  local key old new ver item
  # Renames first, so a renamed key keeps its value instead of being regenerated.
  for item in "${RENAMES[@]+"${RENAMES[@]}"}"; do
    read -r old new ver <<<"$item"
    if [[ -v OLD["$old"] && ! -v OLD["$new"] ]]; then
      OLD["$new"]="${OLD[$old]}"; unset 'OLD[$old]'; RENAMED+=("$old -> $new (v$ver)")
    fi
  done
  # The vapid pair must stay a pair: regenerate both if either is missing.
  if [[ ! -v OLD[VAPID_PUBLIC_KEY] || ! -v OLD[VAPID_PRIVATE_KEY] || -z "${OLD[VAPID_PUBLIC_KEY]:-}" || -z "${OLD[VAPID_PRIVATE_KEY]:-}" ]]; then
    unset 'OLD[VAPID_PUBLIC_KEY]' 'OLD[VAPID_PRIVATE_KEY]'
    gen_vapid # here, not inside generate's $(...) subshell, so both halves share one key
  fi
  # APP_URL first: older files may only carry AUTH_URL / PUBLIC_FRONTEND_URL.
  if [[ ! -v OLD[APP_URL] ]]; then
    for k in PUBLIC_FRONTEND_URL AUTH_URL; do
      if [[ -n "${OLD[$k]:-}" ]] && normalize_url "${OLD[$k]}" >/dev/null; then
        OLD[APP_URL]="$(normalize_url "${OLD[$k]}")"; break
      fi
    done
  fi
  [[ -n "$APP_URL_ARG" && -v OLD[APP_URL] ]] && echo "note: --app-url ignored, APP_URL already set (${OLD[APP_URL]})" >&2
  for key in "${ORDER[@]}"; do
    [[ "$key" == "#"* ]] && continue
    if [[ -v OLD["$key"] ]]; then
      NEW["$key"]="${OLD[$key]}"; KEPT+=("$key")
    else
      NEW["$key"]="$(generate "$key")"; ADDED+=("$key")
    fi
  done
  for item in "${REMOVALS[@]+"${REMOVALS[@]}"}"; do
    read -r key ver <<<"$item"
    [[ -v OLD["$key"] ]] && REMOVED+=("$key")
  done
  return 0
}

# --- output --------------------------------------------------------------------
quote() {
  local v="$1"
  if [[ "$v" =~ [[:space:]\#\"\'\$] ]]; then v="${v//\\/\\\\}"; printf '"%s"' "${v//\"/\\\"}"; else printf '%s' "$v"; fi
}

is_removed() { local i k v; for i in "${REMOVALS[@]+"${REMOVALS[@]}"}"; do read -r k v <<<"$i"; [[ "$k" == "$1" ]] && return 0; done; return 1; }

write_env() {
  local out="$1" tmp key i k ver
  tmp="$(mktemp "${out}.XXXXXX")"
  {
    echo "# Generated by scripts/prod-env.sh from deploy/env/schema."
    echo "# Keep this file secret. Upgrade with: scripts/prod-env.sh upgrade <this file>"
    echo "ENV_SCHEMA_VERSION=$SCHEMA_VERSION"
    for key in "${ORDER[@]}"; do
      if [[ "$key" == "#"* ]]; then printf '\n# %s\n' "${key#\#}"; continue; fi
      printf '%s=%s\n' "$key" "$(quote "${NEW[$key]}")"
    done
    local extra=()
    for key in "${OLD_ORDER[@]+"${OLD_ORDER[@]}"}"; do
      [[ -v OLD["$key"] && ! -v RULE["$key"] && "$key" != ENV_SCHEMA_VERSION ]] || continue
      is_removed "$key" && continue
      extra+=("$key")
    done
    if [[ ${#extra[@]} -gt 0 ]]; then
      printf '\n# --- Unmanaged (kept as is) ---\n'
      for key in "${extra[@]}"; do printf '%s=%s\n' "$key" "$(quote "${OLD[$key]}")"; done
    fi
    if [[ ${#REMOVED[@]} -gt 0 ]]; then
      printf '\n# --- Removed from the schema (no longer read) ---\n'
      for key in "${REMOVED[@]}"; do printf '# %s=%s\n' "$key" "$(quote "${OLD[$key]}")"; done
    fi
  } > "$tmp"
  chmod 600 "$tmp"
  mv "$tmp" "$out"
}

summary() {
  local from="$1"
  echo "env schema: v$from -> v$SCHEMA_VERSION"
  echo "  kept:    ${#KEPT[@]}"
  [[ ${#ADDED[@]} -eq 0 ]] || printf '  added:   %s\n' "${ADDED[*]}"
  [[ ${#RENAMED[@]} -eq 0 ]] || printf '  renamed: %s\n' "${RENAMED[@]}"
  [[ ${#REMOVED[@]} -eq 0 ]] || printf '  removed: %s (commented out)\n' "${REMOVED[*]}"
  echo
  echo "Dokploy alan adları:"
  echo "  frontend   -> $(url_host "${NEW[APP_URL]}") (port 3000)"
  echo "  centrifugo -> ${NEW[REALTIME_HOST]} (port 8000)"
  [[ -n "${NEW[SMTP_HOST]:-}" ]] || echo "Not: SMTP_HOST boş; e-posta göndermek için SMTP_* alanlarını doldurun."
}

# --- commands --------------------------------------------------------------------
cmd="${1:-}"; shift || true
OUT=""; IN=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--out) OUT="${2:?-o needs a file}"; shift 2 ;;
    --app-url) APP_URL_ARG="${2:?--app-url needs a value}"; shift 2 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    -*) die "unknown option: $1" ;;
    *) [[ -z "$IN" ]] || die "unexpected argument: $1"; IN="$1"; shift ;;
  esac
done

command -v openssl >/dev/null || die "openssl is required"
load_schema

case "$cmd" in
  init)
    OUT="${OUT:-${IN:-.env.server}}"
    [[ ! -e "$OUT" ]] || die "$OUT already exists; use: $0 upgrade $OUT"
    resolve
    write_env "$OUT"
    echo "wrote $OUT"; summary 0 ;;
  upgrade)
    [[ -n "$IN" ]] || die "usage: $0 upgrade FILE [-o OUT]"
    load_env "$IN"
    from="${OLD[ENV_SCHEMA_VERSION]:-0}"
    [[ "$from" =~ ^[0-9]+$ ]] || die "bad ENV_SCHEMA_VERSION in $IN: $from"
    [[ "$from" -le "$SCHEMA_VERSION" ]] || die "$IN is v$from, newer than this schema (v$SCHEMA_VERSION); update the repo first"
    resolve
    if [[ -z "$OUT" ]]; then
      OUT="$IN"; backup="$IN.bak.$(date +%Y%m%d%H%M%S)"; cp -p "$IN" "$backup"; echo "backup: $backup"
    fi
    write_env "$OUT"
    echo "wrote $OUT"; summary "$from" ;;
  check)
    [[ -n "$IN" ]] || die "usage: $0 check FILE"
    load_env "$IN"
    from="${OLD[ENV_SCHEMA_VERSION]:-0}"; missing=()
    for key in "${ORDER[@]}"; do [[ "$key" == "#"* || -v OLD["$key"] ]] || missing+=("$key"); done
    echo "env schema: file v$from, repo v$SCHEMA_VERSION"
    if [[ ${#missing[@]} -gt 0 ]]; then echo "missing: ${missing[*]}"; fi
    [[ ${#missing[@]} -eq 0 && "$from" -eq "$SCHEMA_VERSION" ]] || { echo "run: $0 upgrade $IN"; exit 1; }
    echo "ok" ;;
  *) sed -n '2,20p' "$0"; exit 2 ;;
esac
