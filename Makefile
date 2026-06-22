.PHONY: test vet fmt-check ci docker-test docker-build docker-smoke backup-restore-proof image-review dev-up dev-down dev-clean-projects dev-clean-by-name dev-reset dev-seed demo-reset

WORKTREE_SLUG ?= $(shell basename "$$(pwd)" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]_.-' '-' | sed 's/^-//;s/-$$//')
COMPOSE_PROJECT_NAME ?= project-scientist-$(WORKTREE_SLUG)
DEV_PORT ?= 8097
SMOKE_PORT ?= 18097
DEV_HEALTH_URL ?= http://127.0.0.1:$(DEV_PORT)/healthz
DEV_BASE_URL ?= http://127.0.0.1:$(DEV_PORT)
SMOKE_BASE_URL ?= http://127.0.0.1:$(SMOKE_PORT)
PSC_IMAGE_TAG ?= project-scientist:dev-local
PSC_TEST_IMAGE_TAG ?= project-scientist:test-local
DOCKER_GO_PARALLEL ?= 2
COMPOSE ?= docker compose
COMPOSE_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" PSC_DEV_PORT="$(DEV_PORT)" PSC_DATA_DIR="$(PSC_DATA_DIR)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" $(COMPOSE)
COMPOSE_TEST_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)-test" PSC_DEV_PORT="$(DEV_PORT)" PSC_DATA_DIR="$(PSC_DATA_DIR)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" $(COMPOSE)
COMPOSE_SMOKE_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)-smoke" PSC_DEV_PORT="$(SMOKE_PORT)" PSC_DATA_DIR="/tmp/project-scientist-smoke-data" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" $(COMPOSE)

# Host gates.
test:
	go test -mod=readonly ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l $$(find . -path ./.git -prune -o -name '*.go' -print))" || \
		(echo "gofmt required:"; gofmt -l $$(find . -path ./.git -prune -o -name '*.go' -print); exit 1)

ci: fmt-check test vet docker-test docker-build

# Docker gates.
docker-test:
	@set -e; \
		trap '$(COMPOSE_TEST_RUN) down --remove-orphans >/dev/null 2>&1 || true' EXIT; \
		$(COMPOSE_TEST_RUN) down --remove-orphans >/dev/null 2>&1 || true; \
		$(COMPOSE_TEST_RUN) run --build --rm project-scientist-test

docker-build:
	$(COMPOSE_RUN) build project-scientist

docker-smoke:
	@set -e; \
		trap '$(COMPOSE_SMOKE_RUN) down --remove-orphans >/dev/null 2>&1 || true' EXIT; \
		$(COMPOSE_SMOKE_RUN) down --remove-orphans >/dev/null 2>&1 || true; \
		$(COMPOSE_SMOKE_RUN) up --build -d project-scientist; \
		./scripts/wait-health.sh $(SMOKE_BASE_URL)/healthz; \
		./scripts/dev-seed.sh $(SMOKE_BASE_URL); \
		curl -fsS $(SMOKE_BASE_URL)/api/state | grep -q 'Okefenokee Synthetic Water Authority'; \
		curl -fsS $(SMOKE_BASE_URL)/api/state | grep -q 'S-000001'; \
		printf 'docker smoke ok\n'

backup-restore-proof:
	@./scripts/backup-restore-proof.sh

image-review: docker-build
	@printf 'Image size/dependency review for $(PSC_IMAGE_TAG)\n'
	@docker image inspect $(PSC_IMAGE_TAG) --format 'Image={{.RepoTags}} Size={{.Size}} User={{.Config.User}} Entrypoint={{.Config.Entrypoint}} Cmd={{.Config.Cmd}}'
	@docker history $(PSC_IMAGE_TAG) --no-trunc | head -20

dev-up:
	$(COMPOSE_RUN) up --build -d project-scientist
	@./scripts/wait-health.sh $(DEV_HEALTH_URL)

dev-down:
	$(COMPOSE_RUN) down --remove-orphans
	@$(COMPOSE_TEST_RUN) down --remove-orphans >/dev/null 2>&1 || true
	@$(COMPOSE_SMOKE_RUN) down --remove-orphans >/dev/null 2>&1 || true
	@./scripts/dev-clean-containers.sh "$(COMPOSE_PROJECT_NAME)" "$(COMPOSE_PROJECT_NAME)-test" "$(COMPOSE_PROJECT_NAME)-smoke"

# Explicit scoped cleanup for known Compose project names; preserves named volumes.
dev-clean-projects:
	@./scripts/dev-clean-containers.sh "$(COMPOSE_PROJECT_NAME)" "$(COMPOSE_PROJECT_NAME)-test" "$(COMPOSE_PROJECT_NAME)-smoke"

# Diagnostic/admin-only stale cleanup. Requires an explicit Docker name pattern and
# is intentionally not used by normal dev-down because concurrent workers may share
# the project-scientist name prefix.
dev-clean-by-name:
	@test -n "$(NAME_PATTERN)" || (echo 'NAME_PATTERN is required, e.g. make dev-clean-by-name NAME_PATTERN=project-scientist-my-worktree' >&2; exit 2)
	@ids="$$(docker ps -aq --filter "name=$(NAME_PATTERN)")"; test -z "$$ids" || docker rm -f $$ids
	@ids="$$(docker network ls -q --filter "name=$(NAME_PATTERN)")"; test -z "$$ids" || docker network rm $$ids

dev-reset:
	$(COMPOSE_RUN) down --volumes --remove-orphans

dev-seed:
	@./scripts/dev-seed.sh $(DEV_BASE_URL)

demo-reset: dev-up dev-seed
