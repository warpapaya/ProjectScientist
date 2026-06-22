#!/bin/sh
set -eu

# Local lab-only cleanup for Project Scientist Compose projects.
# Removes containers/networks that belong to explicitly named Compose projects only.
# Named volumes are intentionally preserved and listed by callers/reviewers; this script never deletes volumes.

if [ "$#" -lt 1 ]; then
  cat >&2 <<'USAGE'
usage: scripts/dev-clean-containers.sh COMPOSE_PROJECT_NAME [COMPOSE_PROJECT_NAME ...]

Refusing unscoped cleanup. Pass exact Compose project names; cleanup is performed
only by Docker Compose project labels (com.docker.compose.project=<name>).
Named volumes are preserved.
USAGE
  exit 2
fi

remove_project_containers() {
  project="$1"
  ids="$(docker ps -aq --filter "label=com.docker.compose.project=$project" 2>/dev/null || true)"
  [ -z "$ids" ] || docker rm -f $ids >/dev/null 2>&1 || true
}

remove_project_networks() {
  project="$1"
  ids="$(docker network ls -q --filter "label=com.docker.compose.project=$project" 2>/dev/null || true)"
  [ -z "$ids" ] || docker network rm $ids >/dev/null 2>&1 || true
}

remaining_project_resources() {
  project="$1"
  containers="$(docker ps -aq --filter "label=com.docker.compose.project=$project" 2>/dev/null || true)"
  networks="$(docker network ls -q --filter "label=com.docker.compose.project=$project" 2>/dev/null || true)"
  [ -z "$containers" ] && [ -z "$networks" ]
}

attempt=1
while [ "$attempt" -le 12 ]; do
  for project in "$@"; do
    remove_project_containers "$project"
    remove_project_networks "$project"
  done

  all_clean=1
  for project in "$@"; do
    if ! remaining_project_resources "$project"; then
      all_clean=0
    fi
  done

  if [ "$all_clean" -eq 1 ]; then
    exit 0
  fi

  attempt=$((attempt + 1))
  sleep 1
done

printf 'project-scientist scoped cleanup incomplete for Compose projects: %s\n' "$*" >&2
for project in "$@"; do
  docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.ID}} {{.Names}} {{.Status}}' >&2 || true
  docker network ls --filter "label=com.docker.compose.project=$project" --format '{{.ID}} {{.Name}}' >&2 || true
done
exit 1
