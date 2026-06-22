.PHONY: test vet fmt-check ci docker-test docker-build docker-smoke image-review ops-audit-verify ops-db-migrate ops-db-status ops-seed ops-reset ops-backup ops-restore ops-smoke dev-up dev-down dev-reset dev-seed

DEV_HEALTH_URL ?= http://127.0.0.1:8097/healthz
DEV_BASE_URL ?= http://127.0.0.1:8097
PSC_DB ?= data/project-scientist.db
PSC_BACKUP ?= var/backups/project-scientist.db
COMPOSE ?= docker compose

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

# Local operator commands. These are lab-test/dev-only and default to data/project-scientist.db.
ops-audit-verify:
	go run ./cmd/project-scientist audit verify --db $(PSC_DB)

ops-db-migrate:
	go run ./cmd/project-scientist db migrate --db $(PSC_DB)

ops-db-status:
	go run ./cmd/project-scientist db status --db $(PSC_DB)

ops-seed:
	go run ./cmd/project-scientist seed --db $(PSC_DB)

ops-reset:
	go run ./cmd/project-scientist reset --db $(PSC_DB) --force

ops-backup:
	go run ./cmd/project-scientist backup --db $(PSC_DB) --out $(PSC_BACKUP)

ops-restore:
	go run ./cmd/project-scientist restore --db $(PSC_DB) --backup $(PSC_BACKUP) --force

ops-smoke:
	go run ./cmd/project-scientist smoke --base-url $(DEV_BASE_URL)

dev-up:
	$(COMPOSE) up --build -d project-scientist
	@./scripts/wait-health.sh $(DEV_HEALTH_URL)

dev-down:
	$(COMPOSE) down --remove-orphans

dev-reset:
	$(COMPOSE) down --volumes --remove-orphans

dev-seed:
	@./scripts/dev-seed.sh $(DEV_BASE_URL)
