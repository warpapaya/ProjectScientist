#!/bin/sh
set -eu

# Local lab-only cleanup: remove Project Scientist containers and networks while preserving named data volumes.
# Volumes are intentionally listed by the caller/reviewer and are never deleted here.

remove_containers() {
  ids="$(docker ps -aq --filter 'name=project-scientist' 2>/dev/null || true)"
  [ -z "$ids" ] || docker rm -f $ids >/dev/null 2>&1 || true
}

remove_networks() {
  ids="$(docker network ls -q --filter 'name=project-scientist' 2>/dev/null || true)"
  [ -z "$ids" ] || docker network rm $ids >/dev/null 2>&1 || true
}

attempt=1
while [ "$attempt" -le 12 ]; do
  remove_containers
  remove_networks
  containers="$(docker ps -aq --filter 'name=project-scientist' 2>/dev/null || true)"
  networks="$(docker network ls -q --filter 'name=project-scientist' 2>/dev/null || true)"
  if [ -z "$containers" ] && [ -z "$networks" ]; then
    exit 0
  fi
  attempt=$((attempt + 1))
  sleep 1
done

printf 'project-scientist cleanup incomplete\n' >&2
docker ps -a --filter 'name=project-scientist' --format '{{.ID}} {{.Names}} {{.Status}}' >&2 || true
docker network ls --filter 'name=project-scientist' --format '{{.ID}} {{.Name}}' >&2 || true
exit 1
