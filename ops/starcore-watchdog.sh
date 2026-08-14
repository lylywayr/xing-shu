#!/bin/sh
set -eu
BASE_URL=${STARCORE_URL:-http://127.0.0.1:12100}
STATE=${STARCORE_WATCHDOG_STATE_DIR:-/var/run/starcore-watchdog}
MAX_FAILURES=${STARCORE_WATCHDOG_MAX_FAILURES:-5}
DRY_RUN=${STARCORE_WATCHDOG_DRY_RUN:-0}
LOCK=$STATE/lock
FAIL=$STATE/failures
LAST_FAILURE=$STATE/last_failure_at
mkdir -p "$STATE"
exec 9>"$LOCK"
flock -n 9 || exit 0
if wget -qO- --timeout=5 "$BASE_URL/health/ready" 2>/dev/null | grep -q '"ready":true'; then
  printf '0' > "$FAIL"
  rm -f "$LAST_FAILURE" "$STATE/fault"
  exit 0
fi
printf '%s\n' "$(date -Iseconds)" > "$LAST_FAILURE"
n=0
[ -f "$FAIL" ] && n=$(cat "$FAIL" 2>/dev/null || printf '0')
n=$((n+1))
printf '%s' "$n" > "$FAIL"
[ "$n" -lt "$MAX_FAILURES" ] && exit 1
printf '%s\n' "$(date -Iseconds)" > "$STATE/fault"
if [ "$DRY_RUN" = "1" ]; then
  logger -t starcore-watchdog 'starcore failed threshold; dry-run fault marker written'
  exit 0
fi
logger -t starcore-watchdog 'starcore failed threshold; no historical service fallback is configured'
exit 1
