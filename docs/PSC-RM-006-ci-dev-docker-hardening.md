# PSC-RM-006 CI/dev/Docker hardening

Status: review-required. Lab-test lane only; not a production/customer deployment path.

## Scope boundary

This work hardens local development, Docker repeatability, and CI candidate gates. It does not deploy, publish images, expose the app publicly, mutate customer/prod systems, or claim production readiness.

## Changes

- `Dockerfile`
  - Pinned Go and Alpine base images by digest.
  - Named the runtime stage `runtime` and kept `test` as the container test stage.
  - Added CGO-capable build/test toolchain (`gcc`, `musl-dev`) because the current storage lane uses `github.com/mattn/go-sqlite3`.
  - Pre-downloads Go modules in the Docker test target, labels that test image with a dependency SHA derived from `Dockerfile`/`go.mod`/`go.sum`, bind-mounts the current worktree read-only for test execution, and caps Docker build/test package parallelism with `GOFLAGS=-p=<DOCKER_GO_PARALLEL>` plus `GOMAXPROCS` to reduce local CGO/sqlite compile pressure.
  - Builds a static stripped Linux binary and copies only the binary/web assets into the runtime image.
  - Preserves non-root runtime user, `/data` volume path, and `/healthz` health check.
- `docker-compose.yml`
  - Added env-overridable Compose project name, project-scoped local image tags, runtime target, loopback-only port binding, named dev volume, build parallelism args, and service health check.
  - Removed fixed `container_name`; Compose now scopes service/container/network/volume names by project.
- `Makefile`
  - Added aggregate `ci`, `docker-build`, `docker-smoke`, `image-review`, `dev-reset`, and `dev-seed` targets.
  - Added `COMPOSE_PROJECT_NAME`, `DEV_PORT`, image tag, and `DOCKER_GO_PARALLEL` overrides. Default `COMPOSE_PROJECT_NAME` derives from the repo/worktree directory.
  - `docker-test` runs under a unique `<COMPOSE_PROJECT_NAME>-test-<timestamp>-<pid>` project, rebuilds the test image only when the dependency SHA changes, runs tests without forcing another build, uses ephemeral container Go cache/test temp space, and cleans the test project on exit.
  - `docker-smoke` starts the container, waits for health, seeds synthetic demo data through the local API, verifies `/api/state`, and stops the Compose project on exit.
- `scripts/wait-health.sh`
  - Bounded health polling for `make dev-up` instead of a single race-prone curl.
- `scripts/dev-seed.sh`
  - API-level synthetic seed fixture for local-only demo state.
- `.github/workflows/ci.yml`
  - Candidate CI workflow: host fmt/test/vet, Compose config validation, Docker test stage, runtime image build.
- `README.md` and `docs/dev.md`
  - Documented commands, reset caveats, CI candidate, image review, deterministic Docker posture, and stop-lines.

## Verification evidence

Commands run from `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_f8ce13cf/verify-psc-rm-006`:

```bash
make fmt-check
make test
make vet
docker compose config --quiet
make docker-test
make docker-test
make dev-reset && make docker-smoke && make dev-down
make image-review
docker ps -a --filter label=com.docker.compose.project=project-scientist-verify-psc-rm-006

docker ps -a --filter label=com.docker.compose.project=project-scientist-verify-psc-rm-006-test
```

Result: passed.

Observed summary:

```text
go test -mod=readonly ./...
?    github.com/warpapaya/ProjectScientist/cmd/project-scientist [no test files]
ok   github.com/warpapaya/ProjectScientist/internal/lab (cached)
go vet ./...
project-scientist-verify-psc-rm-006-test project built and removed twice
ok   github.com/warpapaya/ProjectScientist/internal/lab 0.032s
ok   github.com/warpapaya/ProjectScientist/internal/lab 0.033s
ok
seeded synthetic client C-00001 and sample data at http://127.0.0.1:8097
docker smoke ok
Image=[project-scientist:dev-local] Size=8246038 User=scientist Entrypoint=[] Cmd=[/app/project-scientist]
cleanup proof: no containers for project-scientist-verify-psc-rm-006 or project-scientist-verify-psc-rm-006-test; name filter also empty
```

## Dependency review

No new dependency was added by PSC-RM-006 itself. The working tree already contained the SQLite storage lane using `github.com/mattn/go-sqlite3`; this task adjusted Docker build/test stages so that dependency works under Compose instead of failing with `CGO_ENABLED=0`. The build toolchain remains confined to build/test stages and is not copied into runtime.

## Risks / follow-up

- Current repo has concurrent uncommitted storage/security changes (`go.mod`, `internal/lab/store.go`, `internal/lab/store_sqlite_test.go`, etc.). PSC-RM-006 verified against that live tree, but review should separate those changes from the Docker/dev hardening diff before merge.
- CI workflow is a candidate only; it does not deploy or publish images.
- `make dev-reset` is intentionally destructive to the local named dev volume only.
