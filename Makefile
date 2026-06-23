.PHONY: test vet fmt-check ci docker-test docker-build docker-smoke performance-concurrency-smoke backup-restore-proof image-review dev-up dev-down dev-clean-projects dev-clean-by-name dev-reset dev-seed demo-reset mvp-vertical-slice mvp-verify-suite

WORKTREE_SLUG ?= $(shell basename "$$(pwd)" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]_.-' '-' | sed 's/^-//;s/-$$//')
COMPOSE_PROJECT_NAME ?= project-scientist-$(WORKTREE_SLUG)
DEV_PORT ?= 8097
SMOKE_PORT ?= 18097
DEV_HEALTH_URL ?= http://127.0.0.1:$(DEV_PORT)/healthz
DEV_BASE_URL ?= http://127.0.0.1:$(DEV_PORT)
SMOKE_BASE_URL ?= http://127.0.0.1:$(SMOKE_PORT)
PSC_IMAGE_TAG ?= project-scientist:$(WORKTREE_SLUG)-dev-local
PSC_TEST_IMAGE_TAG ?= project-scientist:$(WORKTREE_SLUG)-test-local
DOCKER_GO_PARALLEL ?= 1
PSC_INTERNAL_SESSION_TOKEN ?= psc-local-dev-session-token
PSC_ENABLE_DEMO_RESET ?= true
COMPOSE ?= docker compose
COMPOSE_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" PSC_DEV_PORT="$(DEV_PORT)" PSC_DATA_DIR="$(PSC_DATA_DIR)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" PSC_INTERNAL_SESSION_TOKEN="$(PSC_INTERNAL_SESSION_TOKEN)" PSC_ENABLE_DEMO_RESET="$(PSC_ENABLE_DEMO_RESET)" $(COMPOSE)
COMPOSE_TEST_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)-test" PSC_DEV_PORT="$(DEV_PORT)" PSC_DATA_DIR="$(PSC_DATA_DIR)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" PSC_INTERNAL_SESSION_TOKEN="$(PSC_INTERNAL_SESSION_TOKEN)" PSC_ENABLE_DEMO_RESET="false" $(COMPOSE)
COMPOSE_SMOKE_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)-smoke" PSC_DEV_PORT="$(SMOKE_PORT)" PSC_DATA_DIR="/tmp/project-scientist-smoke-data" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" PSC_INTERNAL_SESSION_TOKEN="$(PSC_INTERNAL_SESSION_TOKEN)" PSC_ENABLE_DEMO_RESET="true" $(COMPOSE)

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
		trap '$(COMPOSE_SMOKE_RUN) down --volumes --remove-orphans >/dev/null 2>&1 || true' EXIT; \
		$(COMPOSE_SMOKE_RUN) down --volumes --remove-orphans >/dev/null 2>&1 || true; \
		$(COMPOSE_SMOKE_RUN) up --build -d project-scientist; \
		./scripts/wait-health.sh $(SMOKE_BASE_URL)/healthz; \
		PSC_INTERNAL_SESSION_TOKEN="$(PSC_INTERNAL_SESSION_TOKEN)" ./scripts/dev-seed.sh $(SMOKE_BASE_URL); \
		csrf_token="$$(printf '%s' "project-scientist-csrf-v1:$(PSC_INTERNAL_SESSION_TOKEN)" | shasum -a 256 | awk '{print $$1}')"; \
		curl -fsS -H 'Cookie: psc_internal_session=$(PSC_INTERNAL_SESSION_TOKEN)' $(SMOKE_BASE_URL)/api/state | grep -q 'Okefenokee Synthetic Water Authority'; \
		curl -fsS -H 'Cookie: psc_internal_session=$(PSC_INTERNAL_SESSION_TOKEN)' $(SMOKE_BASE_URL)/api/state | grep -q 'S-000001'; \
		curl -fsS -H 'Accept: application/json' -H "X-PSC-CSRF-Token: $$csrf_token" -H 'Cookie: psc_internal_session=$(PSC_INTERNAL_SESSION_TOKEN)' -X POST \
			-d tenant_id=lab-test -d lab_id=default-lab \
			-d package_format=application/vnd.project-scientist.coc+json \
			-d attachment_name=custody-history.json \
			-d attachment_media_type=application/json \
			--data-urlencode attachment_content_text='synthetic COC smoke attachment' \
			$(SMOKE_BASE_URL)/api/samples/S-000001/coc-package | grep -q '"content_hash":"sha256:'; \
		$(COMPOSE_SMOKE_RUN) exec -T project-scientist /app/project-scientist mvp vertical-slice --db /data/project-scientist-mvp.db | grep -q 'mvp vertical-slice ok'; \
		$(COMPOSE_SMOKE_RUN) exec -T project-scientist /app/project-scientist mvp verify-suite --db /data/project-scientist-mvp-suite.db --artifacts /tmp/mvp-verification | grep -q 'mvp verify-suite ok'; \
		printf 'docker smoke ok\n'

performance-concurrency-smoke:
	@./scripts/performance-concurrency-smoke.sh --json

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
	@PSC_INTERNAL_SESSION_TOKEN="$(PSC_INTERNAL_SESSION_TOKEN)" ./scripts/dev-seed.sh $(DEV_BASE_URL)

mvp-vertical-slice:
	@go run ./cmd/project-scientist mvp vertical-slice --db "$${PSC_MVP_DB:-data/project-scientist-mvp.db}"

mvp-verify-suite:
	@go run ./cmd/project-scientist mvp verify-suite --db "$${PSC_MVP_DB:-data/project-scientist-mvp.db}" --artifacts "$${PSC_MVP_ARTIFACTS:-artifacts/mvp-verification}"

demo-reset: PSC_ENABLE_DEMO_RESET=true
demo-reset: dev-up dev-seed
