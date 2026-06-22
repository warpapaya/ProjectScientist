#!/bin/sh
set -eu

url="${1:-http://127.0.0.1:8097/healthz}"
attempts="${PSC_HEALTH_ATTEMPTS:-30}"
sleep_seconds="${PSC_HEALTH_SLEEP_SECONDS:-1}"

i=1
while [ "$i" -le "$attempts" ]; do
  if body="$(curl -fsS "$url" 2>/dev/null)" && [ "$body" = "ok" ]; then
    printf 'ok
'
    exit 0
  fi
  i=$((i + 1))
  sleep "$sleep_seconds"
done

printf 'health check failed for %s after %s attempts
' "$url" "$attempts" >&2
exit 1
