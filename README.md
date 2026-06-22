# Project Scientist

Greenfield Clearline LIMS lab-test lane.

Goal: explore whether we can build a focused, low-click, defensible-audit LIMS with eventual SENAITE feature parity for Clearline customers, without inheriting SENAITE/Bika/Plone complexity.

Current status: foundation prototype only. Useful for local architecture pressure-testing; not production-ready; not customer-ready.

## Principles

- Few external dependencies: current app is Go standard library only.
- Local-first development in Docker.
- Dense, low-click UI for daily lab work.
- Defensible audit logging: every mutation writes an append-only JSONL event with actor, entity, action, timestamp, previous hash, and hash.
- Workflow transitions are enforced by the domain layer, not just UI buttons.

## Run locally

```bash
docker compose up --build
open http://localhost:8097
```

Health check:

```bash
curl -fsS http://localhost:8097/healthz
```

Run tests:

```bash
go test ./...
```

## Implemented in bootstrap

- Client creation.
- Sample receiving/intake.
- Analysis list attached to sample.
- Legal workflow path: received -> in_prep -> in_analysis -> in_review -> released.
- Illegal transition rejection.
- Append-only hash-chained audit JSONL.
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
