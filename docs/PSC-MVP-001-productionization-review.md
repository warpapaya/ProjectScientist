---
title: PSC-MVP-001 Productionization Review Packet
project: Project Scientist
status: lab-test docs productionization review
source_task: t_a795c506
productionization_task: t_1b900f7e
review_date: 2026-06-22
source_commit: 908e51a582c0435aeeba4b919e3b1770715b4da9
validated_checkout_commit: d69c6bd67099c4e9d478cade51325749644e9078
---

# PSC-MVP-001 Productionization Review Packet

This packet surfaces the MVP acceptance contract and operator demo script for Petie-facing internal review. It is a lab-test documentation artifact only. It does not approve production use, customer data, customer migration, public exposure, customer-facing readiness claims, or mutation of any client system.

## Source artifact

- Contract: `docs/mvp-acceptance-contract-demo-script.md`
- Source review commit: `908e51a582c0435aeeba4b919e3b1770715b4da9` (`docs: add MVP acceptance contract`)
- Current validated checkout: `d69c6bd67099c4e9d478cade51325749644e9078`
- Existing repo links:
  - `README.md` links to `docs/mvp-acceptance-contract-demo-script.md`
  - `docs/mvp-test-scope.md` links to `docs/mvp-acceptance-contract-demo-script.md`

## Contract validation

Verified the contract exists and contains every required canonical MVP command:

- `make mvp-reset`
- `make mvp-up`
- `make mvp-seed`
- `make mvp-demo`
- `make mvp-audit-verify`
- `make mvp-denied-controls`
- `make mvp-acceptance`
- `make mvp-down`

Verified the contract covers every required demo stage:

- clean Docker reset/start
- deterministic seed data
- sample intake/accession
- label artifact
- analysis request lines
- worksheet/batch assignment
- result entry
- review and lock
- COA generation
- COA release
- audit verification
- denied-operation controls
- one-command acceptance
- standard engineering gates

Verified the guardrails are explicit:

- lab-test only
- synthetic data only
- loopback/local Docker only unless a later approved task changes scope
- no Tindall, CENLA, RJ Lee, AmSpec, or other customer data
- no external exposure, reverse proxy, DNS, Authentik, shared hosting, or production deployment
- no production-ready, customer-ready, migration-ready, SENAITE-replacement, or customer-readiness language

## Validation run

Validation artifacts were written under `.hermes-validation/t_1b900f7e/`.

| Gate | Command | Result | Notes |
| --- | --- | --- | --- |
| Contract scan | Python scan of `docs/mvp-acceptance-contract-demo-script.md` | PASS | Required commands, stages, guardrails present. |
| Link resolution | README + `docs/mvp-test-scope.md` link scan | PASS | Both links resolve to the contract file. |
| Go tests | `go test ./...` | PASS | Final Friday review at `d69c6bd67099c4e9d478cade51325749644e9078`; all packages passed. |
| Go vet | `go vet ./...` | PASS | Final Friday review at `d69c6bd67099c4e9d478cade51325749644e9078`; no vet findings. |
| Docker compose config | `docker compose config --quiet` | PASS | Compose file validates. |
| Docker/HTTP smoke | `make docker-smoke` | PASS | Built local Docker app, `/healthz` returned `ok`, `dev-seed` seeded synthetic data, smoke found `Clearline Synthetic Lab`. |
| Cleanup | `make dev-down`; forced removal of lingering `project-scientist-*` containers | PASS | Final `docker ps -a --filter name=project-scientist` returned empty. |

Validation logs:

- `.hermes-validation/t_1b900f7e/final-review/final-review-validation.log`
- `.hermes-validation/t_1b900f7e/final-review/final-cleanup.log`

## Remaining gates

1. The canonical `make mvp-*` commands are specified by the contract; implementation is intentionally deferred to PSC-MVP-004 / `t_28171cbe`. The bootstrap commands remain `make dev-reset`, `make dev-up`, `make dev-seed`, and `make docker-smoke`.
2. This packet is internal review evidence only. No customer/prod deployment, external exposure, client data, DNS/auth/security/billing changes, or production-readiness claim is authorized.

## Accepted language

Project Scientist has a lab-test MVP acceptance contract and operator demo script for a synthetic local Docker-only vertical slice. The contract defines the proof required for sample lifecycle, audit evidence, negative controls, and artifact packet acceptance. It is not approved for customer data, production deployment, migration, or customer-facing readiness claims.
