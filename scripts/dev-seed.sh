#!/bin/sh
set -eu

base_url="${1:-http://127.0.0.1:8097}"
session_token="${PSC_INTERNAL_SESSION_TOKEN:-local-dev-session}"

summary="$(curl -fsS \
  -H 'Accept: application/json' \
  -H 'X-PSC-Request-ID: dev-demo-reset' \
  -H "Cookie: psc_internal_session=$session_token" \
  -X POST \
  "$base_url/api/demo/reset")"

printf '%s' "$summary" | grep -q '"fixture_id":"psc-mvp-synthetic-lab-v1"'
printf '%s' "$summary" | grep -q '"client_id":"C-00001"'
printf '%s' "$summary" | grep -q '"sample_id":"S-000001"'
printf '%s' "$summary" | grep -q '"analysis_count":4'

printf 'seeded deterministic synthetic demo data at %s: %s\n' "$base_url" "$summary"
