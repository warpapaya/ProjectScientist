#!/bin/sh
set -eu

base_url="${1:-http://127.0.0.1:8097}"
actor="psc-dev-seed"

client_json="$(curl -fsS -H 'Accept: application/json' -H "X-PSC-Actor: $actor"   -X POST   --data-urlencode 'name=Clearline Synthetic Lab'   --data-urlencode 'email=synthetic@example.test'   "$base_url/api/clients")"

client_id="$(printf '%s' "$client_json" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [ -z "$client_id" ]; then
  printf 'could not parse client id from response: %s
' "$client_json" >&2
  exit 1
fi

curl -fsS -H 'Accept: application/json' -H "X-PSC-Actor: $actor"   -X POST   --data-urlencode "client_id=$client_id"   --data-urlencode 'project=Synthetic Drinking Water Compliance'   --data-urlencode 'matrix=Water'   --data-urlencode 'tests=pH,Turbidity,Lead'   "$base_url/api/samples" >/dev/null

printf 'seeded synthetic client %s and sample data at %s
' "$client_id" "$base_url"
