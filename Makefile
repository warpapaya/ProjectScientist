.PHONY: test vet fmt-check ci docker-test docker-build docker-smoke image-review dev-up dev-down dev-reset dev-seed

WORKTREE_SLUG ?= $(shell basename "$$(pwd)" | tr -cs '[:alnum:]_.-' '-' | sed 's/^-//;s/-$$//')
COMPOSE_PROJECT_NAME ?= project-scientist-$(WORKTREE_SLUG)
DEV_PORT ?= 8097
DEV_HEALTH_URL ?= http://127.0.0.1:$(DEV_PORT)/healthz
DEV_BASE_URL ?= http://127.0.0.1:$(DEV_PORT)
PSC_IMAGE_TAG ?= $(COMPOSE_PROJECT_NAME):dev-local
PSC_TEST_IMAGE_TAG ?= $(COMPOSE_PROJECT_NAME):test-local
DOCKER_GO_PARALLEL ?= 1
PSC_TEST_DEPS_SHA ?= $(shell shasum -a 256 Dockerfile go.mod go.sum | shasum -a 256 | cut -d' ' -f1)
COMPOSE ?= docker compose
COMPOSE_RUN = env COMPOSE_PROJECT_NAME="$(COMPOSE_PROJECT_NAME)" PSC_DEV_PORT="$(DEV_PORT)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" PSC_TEST_DEPS_SHA="$(PSC_TEST_DEPS_SHA)" $(COMPOSE)


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
		test_project="$(COMPOSE_PROJECT_NAME)-test-$$(date +%s)-$$$$"; \
		compose_test_run='env COMPOSE_PROJECT_NAME="'"$$test_project"'" PSC_DEV_PORT="$(DEV_PORT)" PSC_IMAGE_TAG="$(PSC_IMAGE_TAG)" PSC_TEST_IMAGE_TAG="$(PSC_TEST_IMAGE_TAG)" PSC_DOCKER_GO_PARALLEL="$(DOCKER_GO_PARALLEL)" PSC_TEST_DEPS_SHA="$(PSC_TEST_DEPS_SHA)" $(COMPOSE)'; \
		trap "eval \"$$compose_test_run\" down --remove-orphans >/dev/null 2>&1 || true" EXIT; \
		actual_sha="$$(docker image inspect "$(PSC_TEST_IMAGE_TAG)" --format '{{ index .Config.Labels "org.projectscientist.test-deps-sha" }}' 2>/dev/null || true)"; \
		if [ "$$actual_sha" != "$(PSC_TEST_DEPS_SHA)" ]; then eval "$$compose_test_run" build project-scientist-test; fi; \
		eval "$$compose_test_run" run --rm project-scientist-test

docker-build:
	$(COMPOSE_RUN) build project-scientist

docker-smoke:
	@set -e; \
		trap '$(COMPOSE_RUN) down --remove-orphans >/dev/null 2>&1 || true' EXIT; \
		$(MAKE) --no-print-directory dev-up; \
		$(MAKE) --no-print-directory dev-seed; \
		curl -fsS $(DEV_BASE_URL)/api/state | grep -q 'Clearline Synthetic Lab'; \
		printf 'docker smoke ok\n'

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
