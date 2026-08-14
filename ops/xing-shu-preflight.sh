#!/bin/sh
set -eu
BASE_URL=${XING_SHU_URL:-http://127.0.0.1:12100}
DATA_DIR=${XING_SHU_DATA_DIR:-./data}
COMPOSE=${XING_SHU_COMPOSE_FILE:-./docker-compose.yml}
fail=0
check() {
  name=$1
  url=$2
  body=$(curl -fsS --max-time 10 "$url" 2>/dev/null) || { printf '%s: failed\n' "$name"; fail=1; return; }
  printf '%s: %s\n' "$name" "$body"
}
check xing-shu-health "$BASE_URL/health"
check xing-shu-ready "$BASE_URL/health/ready"
[ -f "$COMPOSE" ] || { printf 'compose: missing\n'; fail=1; }
[ -d "$DATA_DIR" ] || { printf 'data-dir: missing\n'; fail=1; }
if ! grep -q 'xing-shu' "$COMPOSE" || ! grep -q ':/data' "$COMPOSE"; then
  printf 'boundary: xing-shu service or data mount missing\n'
  fail=1
else
  printf 'boundary: compose contains only xing-shu product configuration\n'
fi
exit "$fail"
