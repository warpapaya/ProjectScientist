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

## Concurrent clones / Kanban workers

The default Make workflow derives a Compose project name from the repo directory:

```text
COMPOSE_PROJECT_NAME ?= project-scientist-<repo-dir>
DEV_PORT ?= 8097
PSC_IMAGE_TAG ?= <COMPOSE_PROJECT_NAME>:dev-local
PSC_TEST_IMAGE_TAG ?= <COMPOSE_PROJECT_NAME>:test-local
DOCKER_GO_PARALLEL ?= 1
```

Compose service names are project-scoped and the workflow intentionally does not set `container_name`, so multiple worktrees can use separate Compose projects. When two local clones may run the dev HTTP service at the same time, give each one a unique project and loopback port:

```bash
make dev-up COMPOSE_PROJECT_NAME=project-scientist-$USER-a DEV_PORT=18097
make docker-smoke COMPOSE_PROJECT_NAME=project-scientist-$USER-a DEV_PORT=18097
make dev-down COMPOSE_PROJECT_NAME=project-scientist-$USER-a DEV_PORT=18097
```

`make docker-test` automatically runs under a unique `<COMPOSE_PROJECT_NAME>-test-<timestamp>-<pid>` project, so one-off test containers/networks do not share the long-running dev project and back-to-back runs do not race a just-removed test network.

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

This runs the Compose `project-scientist-test` profile/service in an isolated `<COMPOSE_PROJECT_NAME>-test-<timestamp>-<pid>` project. The Dockerfile `test` target installs the CGO toolchain and pre-downloads modules, then Compose bind-mounts the current worktree read-only at `/src` and executes:

```bash
go test -mod=readonly ./...
```

The source bind mount keeps repeated Docker test runs from rebuilding/exporting a source-copy image layer while still testing the live worktree. Make also defaults image tags to the Compose project (`<COMPOSE_PROJECT_NAME>:dev-local` and `<COMPOSE_PROJECT_NAME>:test-local`) so concurrent worktrees do not overwrite each other's local images. The test image carries a dependency SHA label derived from `Dockerfile`, `go.mod`, and `go.sum`; `make docker-test` rebuilds that image only when the SHA changes, then runs without `--build` to avoid repeated BuildKit export/unpack pressure. The Dockerfile/Compose test lane sets `GOMAXPROCS`/`GOFLAGS=-p=<DOCKER_GO_PARALLEL>` and uses an ephemeral container Go cache. The default cap is `1` to reduce CGO/sqlite compile pressure on local concurrent workers; raise it only when the host is idle.

Validate the development container, health endpoint, seed path, and API state smoke:

```bash
docker compose config --quiet
make docker-smoke
make dev-down
```

`make docker-smoke` starts the local container, waits for `/healthz`, seeds synthetic demo data through the public local API, verifies `/api/state` contains the synthetic lab, and then stops the Compose project through an exit trap. The seed path is intentionally API-level so it exercises the running container instead of mutating files directly.

## Data persistence and cleanup

The development app stores runtime files under `/data` inside the container, backed by the project-scoped named Docker volume `project-scientist-data`.

Normal stop:

```bash
make dev-down
```

This removes the project container/network and preserves the project-scoped `project-scientist-data` volume.

Use Compose project labels for cleanup proof, not broad container-name filters. Example:

```bash
docker ps -a --filter label=com.docker.compose.project=${COMPOSE_PROJECT_NAME:-project-scientist-$(basename "$PWD")}
docker ps -a --filter label=com.docker.compose.project --format '{{.Label "com.docker.compose.project"}} {{.Names}}' | grep "^${COMPOSE_PROJECT_NAME:-project-scientist-$(basename "$PWD")}-test-" || true
docker network ls --filter label=com.docker.compose.project=${COMPOSE_PROJECT_NAME:-project-scientist-$(basename "$PWD")}
docker network ls --filter label=com.docker.compose.project --format '{{.Label "com.docker.compose.project"}} {{.Name}}' | grep "^${COMPOSE_PROJECT_NAME:-project-scientist-$(basename "$PWD")}-test-" || true
```

Avoid using `docker ps -a --filter name=project-scientist` as cleanup evidence on shared developer hosts. It can match unrelated legacy/default-project containers from other worktrees, including old dev or test runs that are outside the current Compose project.

Local-only reset, destructive to this dev volume:

```bash
make dev-reset
```

`make dev-reset` expands to `docker compose down --volumes --remove-orphans`. Only use it when you intentionally want to discard local prototype data. Do not run it against anything containing customer or production data; this lane should never have that data in the first place.

Repo-local runtime directories are ignored and excluded from Docker context: `data/`, `var/`, `tmp/`, build outputs, logs, editor folders, and `.DS_Store`.

## Determinism and image review

- Compose uses a directory-derived project name by default, supports explicit `COMPOSE_PROJECT_NAME`/`DEV_PORT` overrides for concurrent workers, uses project-scoped local image tags (`<COMPOSE_PROJECT_NAME>:dev-local`, `<COMPOSE_PROJECT_NAME>:test-local`), loopback-only port binding, and a named dev data volume scoped by Compose project.
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
make docker-test  # Docker/Compose test lane in a unique <COMPOSE_PROJECT_NAME>-test-* project; cleans up on exit
make docker-build # build runtime container image
make docker-smoke # start local app, seed synthetic data, verify API state, stop container
make image-review # print local image size/user/cmd/layers
make dev-up       # build/start local dev container and health check 127.0.0.1:8097
make dev-seed     # seed synthetic local-only demo data via API
make dev-down     # stop local dev container, preserve named volume
make dev-reset    # destructive local reset of the named dev volume
```
