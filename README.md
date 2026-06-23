# Project Scientist

Greenfield Clearline LIMS lab-test lane.

Goal: explore whether we can build a focused, low-click, defensible-audit LIMS with eventual SENAITE feature parity for Clearline customers, without inheriting SENAITE/Bika/Plone complexity.

Current status: foundation prototype only. Useful for local architecture pressure-testing; not production-ready; not customer-ready.

## Principles

- Few external dependencies: current app adds only the SQLite driver needed for transactional persistence.
- SQLite-first local persistence with domain state and audit events committed in one transaction.
- Tenant/lab boundary is part of the domain, storage, audit, API query scope, and UI surface from the local prototype onward.
- Local-first development in Docker.
- Dense, low-click UI for daily lab work.
- Defensible audit logging: every mutation writes an append-only hash-chained audit event with actor, entity, action, timestamp, previous hash, and hash.
- Every mutable object and audit event carries `tenant_id` and `lab_id`; reads and mutations are scoped by `X-PSC-Tenant-ID`/`X-PSC-Lab-ID` headers or hidden local-dev form scope.
- Workflow transitions are enforced by the domain layer, not just UI buttons.

## Run locally

This is a lab-only local development workflow, not a public/customer production path.

```bash
make dev-up
open http://127.0.0.1:8097
```

The Docker workflow derives a clone/worktree-specific Compose project name by default (`project-scientist-<repo-dir>`). `make docker-smoke` uses a separate `<COMPOSE_PROJECT_NAME>-smoke` Compose project, loopback port `18097`, and an in-container temp data directory so the smoke gate does not mutate or delete preserved local development volumes. For concurrent local clones or Kanban workers, override the project and loopback port explicitly:

```bash
make dev-up COMPOSE_PROJECT_NAME=project-scientist-$USER-1 DEV_PORT=18097
open http://127.0.0.1:18097
```

Health check:

```bash
curl -fsS http://127.0.0.1:8097/healthz
```

Run gates:

```bash
make fmt-check
make test
make vet
make docker-test
make docker-smoke
make prospect-trial-smoke
```

Local prospect trial browser-route smoke:

```bash
make prospect-trial-smoke COMPOSE_PROJECT_NAME=project-scientist-trial DEV_PORT=8108
```

That command rebuilds/starts the local dev container, signs in with the local browser credentials, loads the deterministic demo workspace, and verifies the prospect path: login → dashboard → samples with `S-000001` → results → reports. It is local lab-test evidence only, not a production/customer readiness claim.

`make docker-test` uses an isolated `<COMPOSE_PROJECT_NAME>-test` Compose project and clone-specific default image tags, then cleans the Compose project up on exit. `make docker-smoke` uses an isolated `<COMPOSE_PROJECT_NAME>-smoke` Compose project on loopback port `18097`, verifies the seeded API state against an in-container temp data directory, runs the MVP vertical-slice command against the smoke project volume, then removes that smoke-only container/network/volume so immediate reruns start cleanly. Preserved development volumes are left in place and are not silently deleted.

Browser trial login for local development:

```bash
make dev-up
open http://127.0.0.1:8097/login
```

Default local credentials are `lab-dev` / `project-scientist-dev`. The checked-in Compose/Makefile defaults create only a non-secret local dev session token (`psc-local-dev-session-token`) and enable the fixture-backed demo reset for loopback development so the dashboard can load realistic Tindall/CENLA-style sample data from the browser. For a different local token, use `PSC_INTERNAL_SESSION_TOKEN=$(openssl rand -hex 24) make dev-up`. The reset is safe to rerun only for the local prototype volume and recreates the fixture-backed `C-00001` / `S-000001` demo state from `fixtures/mvp_synthetic_lab.json`; do not enable it for shared/customer/prod deployments.

Seed/reset deterministic synthetic local-only data from the CLI:

```bash
make demo-reset
```

Stop only this checkout's dev/test/smoke Compose projects while preserving named development data volumes:

```bash
make dev-down
```

Reset the local dev container and named volume when you intentionally want a clean prototype state:

```bash
make dev-reset
```

Full workflow, persistence/reset caveats, CI candidate notes, image review, and stop-lines are documented in [docs/dev.md](docs/dev.md). Local operator commands for audit verification, DB migrate/status, seed/reset, backup/restore, HTTP smoke, and the logs/metrics plan are documented in [docs/operations.md](docs/operations.md). The local/private public-debut review packet, artifact capture path, cleanup, and later approval checklist live in [docs/public-debut-local-review-runbook.md](docs/public-debut-local-review-runbook.md). The lab-test MVP acceptance contract and operator demo script live in [docs/mvp-acceptance-contract-demo-script.md](docs/mvp-acceptance-contract-demo-script.md). The MVP synthetic lab fixture/scenario pack lives in [docs/mvp-synthetic-lab-fixture-pack.md](docs/mvp-synthetic-lab-fixture-pack.md) with machine-readable definitions in [fixtures/mvp_synthetic_lab.json](fixtures/mvp_synthetic_lab.json). The MVP critical-path click budget lives in [docs/mvp-critical-path-ux-click-budget.md](docs/mvp-critical-path-ux-click-budget.md).

## Implemented in bootstrap

- Client creation.
- Sample receiving/intake.
- Analysis list attached to sample.
- Legal workflow path: received -> in_prep -> in_analysis -> in_review -> released.
- Illegal transition rejection.
- Append-only hash-chained audit events stored in SQLite with domain state transactionally.
- Tenant/lab scoped client/sample creation, sample lookup/listing, transition, audit tail, and `/api/state` reads.
- Single-screen local UI with quick intake and audit tail.
- Dockerfile + docker-compose for local Citadel development.

## Major parity gaps

Everything important, basically:

- Auth/RBAC/e-signatures.
- Results entry, QC, worksheets, instruments, calculations, specs/limits.
- COA/report rendering beyond the initial dependency-free synthetic text renderer.
- COC package generation.
- Client/contact/project hierarchy.
- Attachments and custody chain.
- Imports/exports and migration tooling.
- Multi-tenant hosting model.
- Real tamper-evident audit hardening and backup policy.

That gap is the point of the side lane: prove the architecture before pretending this can replace anything live.
