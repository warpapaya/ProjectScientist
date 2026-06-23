# PSC-RM-084 internal synthetic pilot-readiness drill evidence

Status: PASS — regenerated and committed under repo-local durable evidence path.

## Durable location

- Repository worktree: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness`
- Git branch at generation: `psc-rm-084-pilot-readiness`
- Source HEAD before evidence commit: `fd1198e7642ebf24b5588f0f8a4eeafdf63f0d45`
- Evidence directory: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z`
- Evidence packet: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/psc-rm-084-internal-synthetic-drill-evidence.md`
- Command transcript: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/command-transcript.log`
- Hash manifest: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/packet-and-artifact-hashes.sha256`
- Audit tail: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/audit-tail.json`
- Backup proof summary: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/proof-summary.md`
- Backup proof JSON: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/proof-result.json`
- MVP verify-suite JSON: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/mvp/artifacts/mvp-verification-suite.json`

Stable index copy: `docs/review-evidence/psc-rm-084/psc-rm-084-internal-synthetic-drill-evidence.md`.

## Safety proof

This was an internal synthetic local drill only.

- Synthetic data only: PASS. Fixture/client observed: `Okefenokee Synthetic Water Authority`; sample `S-000001`.
- No public/external URL: PASS. HTTP smoke used loopback port 18184 only.
- No SENAITE production integration: PASS. Commands ran local Go/Docker/SQLite paths only.
- No customer/customer-facing mutation or claims: PASS. Transcript safety line and proof scope explicitly reject customer/prod mutation.

## Verification gates

| Gate | Result | Evidence |
| --- | --- | --- |
| `go test ./...` | PASS | `command-transcript.log` exits 0 |
| `make fmt-check` | PASS | `command-transcript.log` exits 0 |
| `go vet ./...` | PASS | `command-transcript.log` exits 0 |
| Docker/HTTP smoke | PASS | `make docker-smoke` on loopback port 18184, local health/seed/api/coc/mvp checks, exits 0 |
| Backup/restore proof | PASS | `proof-result.json`, `proof-summary.md`, manifest DB SHA `27399b0295498af6a660d600b49136adcaccda5a85c7d3ada5d44fd8baef0324` |
| Performance/concurrency smoke | PASS | 12 samples, 12 results entered/accepted, 3 reports, 88 audit events, exits 0 |
| Audit tamper/denied controls | PASS | MVP negative controls and audit tail include denied operations; released result mutation denied |
| MVP vertical slice | PASS | sample `S-000001`, worksheet `WS-000001`, report artifact `RA-000001`, denied controls 5 |
| MVP verify-suite | PASS | `mvp-verification-suite.json` status `pass`, negative controls 5 |
| Cleanup transcript | PASS | `make dev-down` exit 0 |

## Failed / blocked controls exercised

- `illegal_workflow_jump:transition received -> in_review is not allowed`
- `unauthorized_mutation:authorization denied`
- `release_before_preconditions:sample "S-000001" has QC batch(es) not accepted: QCB-000001`
- `cross_tenant_attempt:unknown sample "S-000001"`
- `mutate_released_artifact:result "R-000001" is locked after review; reopen before amending`

## Audit evidence

- MVP verify-suite audit events: 51
- MVP denied audit events: 4
- MVP checkpoint: `ef5d9c3feba498b7d977f17434429c8a09100cc345710698ce5410d7f7326f88`
- Audit tail file: `audit-tail.json`
- MVP audit actions include: `client.created, site.created, contact.created, contact.role.assigned, sample_reference.created, catalog.configure, catalog.snapshot.created, project.created, client.defaults.upserted, sample.created, sample.transition.requested, sample.label_artifact.generated` ...

## Backup/restore proof

- Proof scope: `local/dev/staging-shaped lab-test drill; synthetic data only; no customer/prod mutation`
- Backup database: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/project-scientist-pilot-readiness/docs/review-evidence/psc-rm-084/20260623T052207Z/artifacts/backup-restore-proof/backup/project-scientist.db`
- Database SHA256: `27399b0295498af6a660d600b49136adcaccda5a85c7d3ada5d44fd8baef0324`
- Database bytes: `503808`
- Audit tail sequence/hash: `3` / `56688410e42a4458b62ce244aed63c2c70f0cd74525d6e6bce7c4a5c42d048bd`
- RPO: SQLite snapshot via VACUUM INTO after writes are closed: expected data loss is bounded by time since the last completed proof/backup snapshot.
- RTO: Restore target is a copied SQLite DB plus config/artifact files; lab-test target is operator-verified startup/audit-chain validation before serving writes.

## PSC-RM-084 go/no-go deltas

Go:
- Local synthetic app gates pass from branch `psc-rm-084-pilot-readiness`.
- Local Docker/HTTP smoke passes on loopback only.
- MVP vertical slice and verify-suite pass with denied controls.
- Backup/restore proof records DB/config/artifact checksums and verifies restored audit chain.

No-go / still not approved:
- No customer data.
- No production/customer-system mutation.
- No external/public exposure.
- No SENAITE production integration.
- No customer-facing readiness claims.

## Read-back verification

`read-back-verification.json` records existence, bytes, and SHA256 for reviewer-critical files. `packet-and-artifact-hashes.sha256` covers the timestamped evidence packet files.

Latest evidence timestamp: `20260623T052207Z`
