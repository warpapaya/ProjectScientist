.PHONY: test vet fmt-check ci docker-test docker-build docker-smoke image-review dev-up dev-down dev-reset dev-seed

DEV_HEALTH_URL ?= http://127.0.0.1:8097/healthz
DEV_BASE_URL ?= http://127.0.0.1:8097
COMPOSE ?= docker compose
COMPOSE_PROJECT_NAME ?= project-scientist-$(shell printf '%s' '$(abspath $(CURDIR))' | cksum | cut -d ' ' -f 1)
export COMPOSE_PROJECT_NAME

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
	$(COMPOSE) run --build --rm project-scientist-test

docker-build:
	$(COMPOSE) build project-scientist

docker-smoke: dev-up dev-seed
	@curl -fsS $(DEV_BASE_URL)/api/state | grep -q 'Clearline Synthetic Lab'
	@printf 'docker smoke ok\n'

image-review: docker-build
	@printf 'Image size/dependency review for project-scientist:dev-local\n'
	@docker image inspect project-scientist:dev-local --format 'Image={{.RepoTags}} Size={{.Size}} User={{.Config.User}} Entrypoint={{.Config.Entrypoint}} Cmd={{.Config.Cmd}}'
	@docker history project-scientist:dev-local --no-trunc | head -20

dev-up:
	$(COMPOSE) up --build -d project-scientist
	@./scripts/wait-health.sh $(DEV_HEALTH_URL)

dev-down:
	$(COMPOSE) down --remove-orphans

dev-reset:
	$(COMPOSE) down --volumes --remove-orphans

dev-seed:
	@./scripts/dev-seed.sh $(DEV_BASE_URL)
