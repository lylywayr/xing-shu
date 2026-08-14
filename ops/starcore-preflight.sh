#!/bin/sh
set -eu
BASE_URL=${STARCORE_URL:-http://127.0.0.1:12100}
DATA_DIR=${STARCORE_DATA_DIR:-./data}
COMPOSE=${STARCORE_COMPOSE_FILE:-./docker-compose.yml}
fail=0
check() {
  name=$1
  url=$2
  body=$(curl -fsS --max-time 10 "$url" 2>/dev/null) || { printf '%s: failed\n' "$name"; fail=1; return; }
  printf '%s: %s\n' "$name" "$body"
}
check starcore-health "$BASE_URL/health"
check starcore-ready "$BASE_URL/health/ready"
[ -f "$COMPOSE" ] || { printf 'compose: missing\n'; fail=1; }
[ -d "$DATA_DIR" ] || { printf 'data-dir: missing\n'; fail=1; }
if grep -n -E 'V1_UPSTREAM|V1_HEALTH_URL|LEGACY_DATA_DIR|/v1-data|ROUTER_V1_DIR|ROUTER_V2_DIR|freellmapi_freellmapi-data' "$COMPOSE" >/dev/null 2>&1; then
  printf 'boundary: forbidden external runtime dependency found\n'
  fail=1
else
  printf 'boundary: compose has only starcore runtime dependencies\n'
fi
exit "$fail"
