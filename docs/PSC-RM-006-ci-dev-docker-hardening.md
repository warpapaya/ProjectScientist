# PSC-RM-006 CI/dev/Docker hardening

Status: review-required. Lab-test lane only; not a production/customer deployment path.

## Scope boundary

This work hardens local development, Docker repeatability, and CI candidate gates. It does not deploy, publish images, expose the app publicly, mutate customer/prod systems, or claim production readiness.

## Changes

- `Dockerfile`
  - Pinned Go and Alpine base images by digest.
  - Named the runtime stage `runtime` and kept `test` as the container test stage.
  - Added CGO-capable build/test toolchain (`gcc`, `musl-dev`) because the current storage lane uses `github.com/mattn/go-sqlite3`.
  - Builds a static stripped Linux binary and copies only the binary/web assets into the runtime image.
  - Preserves non-root runtime user, `/data` volume path, and `/healthz` health check.
- `docker-compose.yml`
  - Added deterministic Compose project name, local image tags, runtime target, loopback-only port binding, named dev volume, and service health check.
- `Makefile`
  - Added aggregate `ci`, `docker-build`, `docker-smoke`, `image-review`, `dev-reset`, and `dev-seed` targets.
  - `docker-smoke` starts the container, waits for health, seeds synthetic demo data through the local API, and verifies `/api/state`.
- `scripts/wait-health.sh`
  - Bounded health polling for `make dev-up` instead of a single race-prone curl.
- `scripts/dev-seed.sh`
  - API-level synthetic seed fixture for local-only demo state.
- `.github/workflows/ci.yml`
  - Candidate CI workflow: host fmt/test/vet, Compose config validation, Docker test stage, runtime image build.
- `README.md` and `docs/dev.md`
  - Documented commands, reset caveats, CI candidate, image review, deterministic Docker posture, and stop-lines.

## Verification evidence

Commands run from `/Users/citadel/Projects/ProjectScientist`:

```bash
make fmt-check
make test
make vet
docker compose config --quiet
make docker-test
```

Result: passed.

Observed summary:

```text
go test -mod=readonly ./...
?    github.com/warpapaya/ProjectScientist/cmd/project-scientist [no test files]
ok   github.com/warpapaya/ProjectScientist/internal/lab (cached)
go vet ./...
docker compose run --build --rm project-scientist-test
ok   github.com/warpapaya/ProjectScientist/internal/lab 0.045s
```

Docker/HTTP smoke and image review:

```bash
make dev-reset
make docker-smoke
make image-review
make dev-down
```

Result: passed.

Observed summary:

```text
ok
seeded synthetic client C-00002 and sample data at http://127.0.0.1:8097
docker smoke ok
Image=[project-scientist:dev-local] Size=8246032 User=scientist Entrypoint=[] Cmd=[/app/project-scientist]
```

## Dependency review

No new dependency was added by PSC-RM-006 itself. The working tree already contained the SQLite storage lane using `github.com/mattn/go-sqlite3`; this task adjusted Docker build/test stages so that dependency works under Compose instead of failing with `CGO_ENABLED=0`. The build toolchain remains confined to build/test stages and is not copied into runtime.

## Risks / follow-up

- Current repo has concurrent uncommitted storage/security changes (`go.mod`, `internal/lab/store.go`, `internal/lab/store_sqlite_test.go`, etc.). PSC-RM-006 verified against that live tree, but review should separate those changes from the Docker/dev hardening diff before merge.
- CI workflow is a candidate only; it does not deploy or publish images.
- `make dev-reset` is intentionally destructive to the local named dev volume only.
