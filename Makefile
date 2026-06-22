.PHONY: test vet fmt-check ci docker-test docker-build docker-smoke backup-restore-proof image-review dev-up dev-down dev-reset dev-seed demo-reset

WORKTREE_SLUG ?= $(shell basename "$$(pwd)" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]_.-' '-' | sed 's/^-//;s/-$$//')
COMPOSE_PROJECT_NAME ?= project-scientist-$(WORKTREE_SLUG)
DEV_PORT ?= 8097
DEV_HEALTH_URL ?= http://127.0.0.1:$(DEV_PORT)/healthz
DEV_BASE_URL ?= http://127.0.0.1:$(DEV_PORT)
PSC_IMAGE_TAG ?= project-scientist:$(WORKTREE_SLUG)-dev-local
PSC_TEST_IMAGE_TAG ?= project-scientist:$(WORKTREE_SLUG)-test-local
DOCKER_GO_PARALLEL ?= 1
COMPOSE ?= docker compose
COMPOSE_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" PSC_DEV_PORT="$(DEV_PORT)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" $(COMPOSE)
COMPOSE_TEST_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)-test" PSC_DEV_PORT="$(DEV_PORT)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" $(COMPOSE)

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
		$(COMPOSE_TEST_RUN) run --build --rm project-scientist-test

docker-build:
	$(COMPOSE_RUN) build project-scientist

docker-smoke:
	@set -e; \
		trap '$(COMPOSE_RUN) down --remove-orphans >/dev/null 2>&1 || true' EXIT; \
		$(MAKE) --no-print-directory dev-up; \
		$(MAKE) --no-print-directory dev-seed; \
		curl -fsS $(DEV_BASE_URL)/api/state | grep -q 'Okefenokee Synthetic Water Authority'; \
		curl -fsS $(DEV_BASE_URL)/api/state | grep -q 'S-000001'; \
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

dev-reset:
	$(COMPOSE_RUN) down --volumes --remove-orphans

dev-seed:
	@./scripts/dev-seed.sh $(DEV_BASE_URL)

demo-reset: dev-up dev-seed
