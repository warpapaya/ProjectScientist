# Project Scientist

Greenfield Clearline LIMS lab-test lane.

Goal: explore whether we can build a focused, low-click, defensible-audit LIMS with eventual SENAITE feature parity for Clearline customers, without inheriting SENAITE/Bika/Plone complexity.

Current status: foundation prototype only. Useful for local architecture pressure-testing; not production-ready; not customer-ready.

## Principles

- Few external dependencies: current app adds only the SQLite driver needed for transactional persistence.
- SQLite-first local persistence with domain state and audit events committed in one transaction.
- Local-first development in Docker.
- Dense, low-click UI for daily lab work.
- Defensible audit logging: every mutation writes an append-only hash-chained audit event with actor, entity, action, timestamp, previous hash, and hash.
- Workflow transitions are enforced by the domain layer, not just UI buttons.

## Run locally

This is a lab-only local development workflow, not a public/customer production path.

```bash
make dev-up
open http://127.0.0.1:8097
```

The Docker workflow derives a clone/worktree-specific Compose project name by default (`project-scientist-<repo-dir>`). For concurrent local clones or Kanban workers, override the project and loopback port explicitly:

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
```

`make docker-test` uses isolated Compose project/image names, bind-mounts the current worktree read-only into the test container, and cleans the test project up on exit. Use Compose project-label filters for cleanup proof; broad `name=project-scientist` filters can match unrelated legacy containers from other local worktrees. `make docker-smoke` starts the dev container, verifies the seeded API state, then stops the container so normal pass/fail runs do not leave a running smoke container behind.

Seed synthetic local-only data through the running dev API:

```bash
make dev-seed
```

Stop the local container while preserving the named development data volume:

```bash
make dev-down
```

Reset the local dev container and named volume when you intentionally want a clean prototype state:

```bash
make dev-reset
```

Full workflow, persistence/reset caveats, CI candidate notes, image review, and stop-lines are documented in [docs/dev.md](docs/dev.md). The lab-test MVP acceptance contract and operator demo script live in [docs/mvp-acceptance-contract-demo-script.md](docs/mvp-acceptance-contract-demo-script.md). The MVP critical-path click budget lives in [docs/mvp-critical-path-ux-click-budget.md](docs/mvp-critical-path-ux-click-budget.md).

## Implemented in bootstrap

- Client creation.
- Sample receiving/intake.
- Analysis list attached to sample.
- Legal workflow path: received -> in_prep -> in_analysis -> in_review -> released.
- Illegal transition rejection.
- Append-only hash-chained audit events stored in SQLite with domain state transactionally.
- Single-screen local UI with quick intake and audit tail.
- Dockerfile + docker-compose for local Citadel development.

## Major parity gaps

Everything important, basically:

- Auth/RBAC/e-signatures.
- Results entry, QC, worksheets, instruments, calculations, specs/limits.
- COA/report rendering, COC package, labels.
- Client/contact/project hierarchy.
- Attachments and custody chain.
- Imports/exports and migration tooling.
- Multi-tenant hosting model.
- Real tamper-evident audit hardening and backup policy.

That gap is the point of the side lane: prove the architecture before pretending this can replace anything live.
