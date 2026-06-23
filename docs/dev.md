# Project Scientist local development workflow

Project Scientist is currently a lab-only foundation prototype for local architecture pressure-testing. This workflow is durable for development and Friday review; it is not a customer, public, or production deployment path.

## Stop-lines

Do not use this lane for:

- Customer or production data.
- Tindall, CENLA, RJ Lee, or other client integrations.
- Public exposure, reverse proxies, Traefik, Authentik, shared hosting, or internet-accessible demos.
- Production-ready claims.
- Adding Redis/auth/external services without a later explicit task approving that scope.

The current app is intentionally small: Go plus one explicit SQLite driver dependency, a local SQLite data file, Docker for local repeatability, and no external services.

## Prerequisites

- Go matching `go.mod`.
- Docker with Compose v2 (`docker compose`).
- `make` for the checked-in workflow targets.

## First run

From the repo root:

```bash
make dev-up
```

This builds the local image, starts the Compose-managed `project-scientist` service, and verifies the documented health endpoint:

```bash
curl -fsS http://127.0.0.1:8097/healthz
```

Expected response:

```text
ok
```

The UI is available on loopback only:

```text
http://127.0.0.1:8097
```

Compose provisions a synthetic local development session by default, not a real credential:

```text
PSC_INTERNAL_SESSION_TOKEN=psc-local-dev-session-token
PSC_INTERNAL_SESSION_USER=lab-dev
```

Protected routes require the `psc_internal_session` cookie even in local Docker. Browser/manual API checks can use the default synthetic token, or an operator can override it without committing secrets:

```bash
export PSC_INTERNAL_SESSION_TOKEN="$(openssl rand -hex 24)"
make dev-up
curl -fsS -H "Cookie: psc_internal_session=$PSC_INTERNAL_SESSION_TOKEN" http://127.0.0.1:8097/api/state
```

## Concurrent clones / Kanban workers

The default Make workflow derives a lowercase Compose project name from the repo directory:

```text
COMPOSE_PROJECT_NAME ?= project-scientist-<repo-dir>
DEV_PORT ?= 8097
SMOKE_PORT ?= 18097
PSC_IMAGE_TAG ?= project-scientist:dev-local
PSC_TEST_IMAGE_TAG ?= project-scientist:test-local
DOCKER_GO_PARALLEL ?= 2
```

Compose service names are project-scoped and the workflow intentionally does not set `container_name`, so multiple worktrees can use separate Compose projects. When two local clones may run the dev HTTP service at the same time, give each one a unique project and loopback port:

```bash
make dev-up COMPOSE_PROJECT_NAME=project-scientist-$USER-a DEV_PORT=18097
make docker-smoke COMPOSE_PROJECT_NAME=project-scientist-$USER-a DEV_PORT=18097
make dev-down COMPOSE_PROJECT_NAME=project-scientist-$USER-a DEV_PORT=18097
```

`make docker-test` automatically runs under `<COMPOSE_PROJECT_NAME>-test`, so one-off test containers/networks do not share the long-running dev project. `make docker-smoke` runs under `<COMPOSE_PROJECT_NAME>-smoke`, defaults to loopback port `18097`, sets `PSC_DATA_DIR=/tmp/project-scientist-smoke-data` inside the container, and removes only the smoke project's named volume on start/exit. That makes repeat smoke runs independent from any preserved local named development volume, including a volume intentionally kept for forensic review after a failed audit-verification experiment.

Stop the local container without deleting the named data volume:

```bash
make dev-down
```

## Host gates

Run these before handing work to review:

```bash
make fmt-check
make test
make vet
```

The targets expand to:

- `gofmt` cleanliness check over tracked Go source paths.
- `go test -mod=readonly ./...`.
- `go vet ./...`.

`-mod=readonly` is intentional dependency discipline. If a command wants to edit `go.mod`, stop and review the dependency change instead of letting hidden state drift.

## Docker gates

Validate the Docker build/test lane without relying on host Go caches or a host-built binary:

```bash
make docker-test
```

This runs the Compose `project-scientist-test` profile/service in an isolated `<COMPOSE_PROJECT_NAME>-test` project, building the Dockerfile `test` target and executing:

```bash
go test -mod=readonly ./...
```

The Dockerfile pre-downloads Go modules before copying source and sets `GOMAXPROCS`/`GOFLAGS=-p=<DOCKER_GO_PARALLEL>` in build/test stages. The default cap is `2` to reduce CGO/sqlite compile pressure on local concurrent workers; raise it only when the host is idle.

Validate the development container, health endpoint, seed path, and API state smoke:

```bash
docker compose config --quiet
make docker-smoke
make dev-down
```

`make docker-smoke` starts an isolated smoke Compose project, waits for `/healthz`, enables the destructive demo reset only for that disposable smoke project, seeds synthetic demo data through the protected public local API with the synthetic session cookie, verifies `/api/state` contains the synthetic lab through the same session boundary, runs the MVP vertical-slice CLI smoke against the smoke project volume, and then removes the smoke-only container/network/volume through an exit trap. It intentionally uses container-local temp storage and a disposable smoke volume instead of the named development data volume so a preserved lab volume is never deleted or rewritten just to make smoke pass. The seed path is API-level so it exercises the running container instead of mutating files directly.

## Deterministic local demo seed/reset

Use the deterministic demo reset command when you need a known MVP fixture state from a clean clone or from an already-running local Docker stack:

```bash
make demo-reset
```

This command starts the loopback-only Docker development container if needed, calls the lab-test-only `POST /api/demo/reset` endpoint, and seeds from `fixtures/mvp_synthetic_lab.json`. It is safe to rerun: each run clears only the local prototype SQLite store in the `project-scientist-data` Docker volume and recreates exactly one synthetic client/sample pair:

```text
client_id=C-00001
sample_id=S-000001
fixture_id=psc-mvp-synthetic-lab-v1
client=Okefenokee Synthetic Water Authority
analyses=4
```

The reset endpoint is disabled unless `PSC_ENABLE_DEMO_RESET=true`; the checked-in Compose dev service defaults it to `false` so a browser-reachable shared stack cannot destructively reseed itself by default. Compose and Makefile local workflows use the non-secret synthetic `PSC_INTERNAL_SESSION_TOKEN` default `psc-local-dev-session-token` unless the operator exports a different local token. Local validation paths such as `make demo-reset` and `make docker-smoke` opt in explicitly, send the configured `psc_internal_session` cookie, and point `PSC_SYNTHETIC_FIXTURE_PATH` at `/app/fixtures/mvp_synthetic_lab.json` inside the runtime image. The reset operation is explicitly classified as a local-only admin configuration action: `ResetAndSeedSyntheticDemo` authorizes `admin.configure` against the authenticated internal session actor before clearing data, and unauthenticated requests are rejected before reseeding. Do not enable this endpoint on any customer, shared, or production-like deployment.

## Data persistence and cleanup

The development app stores runtime files under `/data` inside the container, backed by the project-scoped named Docker volume `project-scientist-data`.

Normal stop:

```bash
make dev-down
```

This removes only this checkout's Compose projects (`$(COMPOSE_PROJECT_NAME)`, `$(COMPOSE_PROJECT_NAME)-test`, and `$(COMPOSE_PROJECT_NAME)-smoke`) by exact Docker Compose project label and preserves the project-scoped `project-scientist-data` volume. It intentionally does not run a broad `name=project-scientist` cleanup because concurrent Kanban/dev workers may have legitimate Project Scientist containers and networks in other workspaces.

For diagnostic/admin cleanup of stale local lab resources outside the current project, use an explicit name pattern:

```bash
make dev-clean-by-name NAME_PATTERN=project-scientist-my-worktree
```

That target is not part of normal `dev-down`; inspect labels/working directories first and keep the pattern narrow. Named volumes are still preserved.

Local-only reset, destructive to this dev volume:

```bash
make dev-reset
```

`make dev-reset` expands to `docker compose down --volumes --remove-orphans`. Only use it when you intentionally want to discard the entire local prototype data volume. For the deterministic MVP demo fixture state, prefer `make demo-reset`; it leaves the container up and resets/reseeds through the local API. Do not run either reset path against anything containing customer or production data; this lane should never have that data in the first place.

Repo-local runtime directories are ignored and excluded from Docker context: `data/`, `var/`, `tmp/`, build outputs, logs, editor folders, and `.DS_Store`.

## Determinism and image review

- Compose uses a directory-derived project name by default, supports explicit `COMPOSE_PROJECT_NAME`/`DEV_PORT`/`SMOKE_PORT` overrides for concurrent workers, uses worktree-specific local image tags, loopback-only port binding, a named dev data volume scoped by Compose project, and an isolated smoke project that does not mutate preserved dev volumes.
- Dockerfile build/runtime bases are pinned by digest and should only be updated deliberately.
- Runtime stays dependency-light: static Go binary on Alpine, non-root `scientist` user, SQLite file persistence only, no Redis/auth/proxy services added in this lane.
- SQLite uses `github.com/mattn/go-sqlite3`, so Docker build/test stages install Alpine `gcc`/`musl-dev` and compile with CGO enabled; build tooling is not copied into the runtime stage.
- Run `make image-review` after Dockerfile changes to print image size, user, command, and top image layers.

## CI candidate

A GitHub Actions candidate workflow lives at `.github/workflows/ci.yml`. It is intentionally conservative:

1. Checkout.
2. Set up Go from `go.mod`.
3. Run `make fmt-check`, `make test`, and `make vet`.
4. Validate Compose config.
5. Run `make docker-test`.
6. Build the runtime image.

It does not deploy anything, publish images, touch customer/prod data, or make customer-facing claims.

## Workflow targets

```text
make test         # host go test -mod=readonly ./...
make vet          # host go vet ./...
make fmt-check    # verify gofmt cleanliness
make ci           # local aggregate: host gates + Docker test/build
make docker-test  # Docker/Compose test lane in <COMPOSE_PROJECT_NAME>-test; cleans up on exit
make docker-build # build runtime container image
make docker-smoke # start local app, seed synthetic data, verify API state, stop container
make image-review # print local image size/user/cmd/layers
make dev-up       # build/start local dev container and health check 127.0.0.1:8097
make dev-seed     # reset/seed deterministic synthetic local-only demo data via API
make demo-reset   # start Docker dev app if needed, then reset/seed fixture state
make dev-down     # stop only current dev/test/smoke Compose projects, preserve named volumes
make dev-clean-by-name NAME_PATTERN=... # admin-only stale cleanup with explicit narrow pattern
make dev-reset    # destructive local reset of the named dev volume
```
