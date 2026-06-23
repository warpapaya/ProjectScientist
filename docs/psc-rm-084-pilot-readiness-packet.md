# PSC-RM-084 — Project Scientist Pilot Readiness Packet

Status: internal Friday synthesis. Lab-test lane only. This packet does not approve customer data, customer migration, production deployment, externally reachable staging, customer-facing claims, DNS/auth/security/billing changes, or customer/prospect communication.

Repo: https://github.com/warpapaya/ProjectScientist
Reviewed ref: `origin/main` / `cb25a0ea571672cb03b34e50de0452c47faaea17` (`fix: protect report preview tenant scope (#72)`)
Packet branch: `psc-rm-084-pilot-readiness`
Generated for Kanban task: `t_9ea38999`

## Executive recommendation

Recommendation: **NO-GO for any customer pilot, external staging pilot, production claim, or customer-data migration.**

Recommendation: **GO for a controlled internal synthetic pilot-readiness exercise only** — local/private, synthetic fixture data, no customer data, no public URL, no SENAITE production integration, no customer-facing language. Treat this as an engineering validation lane, not a sales/demo lane.

Short version: Project Scientist has crossed from “toy prototype” into a credible synthetic lab-test vertical slice, with security remediations and smoke proof strong enough to justify another internal hardening cycle. It has not crossed the roadmap’s YELLOW bar because YELLOW requires staged pilot evidence with non-sensitive/customer-approved test data, migration reconciliation, and explicit Petie approval. We are not there. Calling it pilot-ready for a customer would be premature marketing cosplay, and those usually age like milk.

## Evidence reviewed

| Evidence | Result | What it proves | What it does not prove |
| --- | --- | --- | --- |
| Parent security re-review `t_a152955a` | PASS for internal synthetic lab-test only at `cb25a0e` | Aegis/Friday accepted remediation of report preview/download/COC/API tenant-boundary controls, backup actor authorization, audit minimization, Docker/image and smoke gates. | No approval for production, customer data, external staging, pilot, or customer claims. |
| `go test ./...` | PASS | Unit/integration packages pass at reviewed ref. | Does not prove production auth, migration, or customer workflow parity. |
| `make fmt-check` | PASS | Go formatting is clean. | Formatting only. |
| `make vet` | PASS | Basic Go static analysis passes. | Not a security audit by itself. |
| `SMOKE_PORT=18299 make docker-smoke` | PASS | Docker build, container health, synthetic seed, `/api/state`, COC package hash generation, MVP vertical-slice, and MVP verify-suite all pass through HTTP/container path. | Does not prove external deployment, real auth proxy, TLS, customer data, or scale. |
| `make backup-restore-proof` | PASS | Local synthetic SQLite backup/restore drill captures DB/config/artifact, checksums, audit tail, and reopens restored store through audit verification. | Does not prove encrypted/offsite backup, customer-data restore, external checkpoint anchoring, or production RTO/RPO. |
| `make performance-concurrency-smoke` | PASS | Synthetic concurrent sample mutations, result entry/acceptance, report generation, and audit writes complete with 12 samples, 12 results, 3 reports, 88 audit events. | Explicitly not production-grade throughput; in-process mutex and local SQLite shape only. |
| Roadmap `docs/senaite-parity-roadmap.md` | Reviewed | Current ladder and non-negotiable gates are defined. | Roadmap is not proof of gate completion. |
| Security gates `docs/security-audit-model.md` | Reviewed | Production-candidate security/audit requirements are documented. | Several production-candidate gates remain future work. |
| Workflow gap report `docs/customer-workflow-gap-report.md` | Reviewed | Customer workflow gaps are already mapped red/yellow/green for Tindall/CENLA/RJ Lee planning analogs. | It explicitly says no customer lane is migration/pilot ready. |

Verification transcript: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_9ea38999/pilot-readiness-verification.log`
Backup proof summary: `tmp/backup-restore-proof/20260623T045009Z/proof-summary.md`
Backup proof result: `tmp/backup-restore-proof/20260623T045009Z/proof-result.json`

## Current readiness ladder position

Using the roadmap ladder:

- RED: prototype only. No real customer data, no external exposure.
- ORANGE: feature slices work with synthetic data and tests.
- YELLOW: staging pilot with migrated non-sensitive/customer-approved test data.
- GREEN: production candidate after security, audit, reporting, migration, backup/restore, and customer workflow validation gates.

Current rating: **ORANGE+ for internal synthetic lab-test validation. RED for customer pilot / external staging / production.**

Why ORANGE+:

1. Synthetic vertical slice is executable, not just described.
2. Docker/HTTP smoke passes with seed data and package-generation checks.
3. Backup/restore proof exists for local synthetic data.
4. Security re-review accepted the recent tenant-boundary fixes for internal lab-test use.
5. Performance/concurrency smoke now gives a bounded correctness signal for local SQLite/in-process serialization.

Why not YELLOW:

1. No migrated non-sensitive/customer-approved dataset has been run through a staged pilot.
2. No external staging environment is approved or hardened.
3. Real auth/session middleware, deployment topology, observability, backup retention/encryption, and externally anchored audit checkpoints are not production-candidate complete.
4. Customer-specific workflow smoke is not green for Tindall, CENLA, or RJ Lee.
5. Petie has not approved customer data, external exposure, or customer-facing claims.

## Candidate pilot scope

Allowed next pilot: **Internal Synthetic Pilot Readiness Drill**

Scope:

1. Environment: local/private Docker only, or isolated non-public staging only after a separate Forge/Aegis approval card.
2. Data: synthetic fixtures only. No Tindall, CENLA, RJ Lee, SENAITE dumps, customer contacts, production report artifacts, credentials, or live workflow exports.
3. Actors: internal Friday/Hermes/Aegis/Forge validation only.
4. Workflows:
   - seed synthetic lab fixture;
   - create/accession sample;
   - attach profile/service lines;
   - worksheet/result entry;
   - reviewer separation and result acceptance;
   - QC acceptance;
   - report release and immutable artifact generation;
   - COC package generation/download;
   - backup/restore proof;
   - tenant-boundary negative controls;
   - audit verification and tamper/denial checks.
5. Success output: one reviewed drill packet with command transcript, generated artifact hashes, audit tail, backup proof path, failed/blocked controls, and next remediation cards.

Explicitly excluded:

- customer demos;
- external/public URL;
- customer or production data;
- SENAITE production migration;
- claiming replacement readiness for Clearline LIMS;
- EQuIS/EDD/CLP/Stage package claims beyond static/scripted/internal proof;
- billing/pricing/customer communications.

## Remaining gaps before any customer pilot

### Security / identity / audit

1. Real authenticated actor/session model beyond local/dev assumptions.
2. Complete RBAC/ABAC enforcement across every protected operation, not just covered slices.
3. Signed or externally anchored audit checkpoints outside mutable primary SQLite storage.
4. Audit privacy/retention/redaction/encryption-at-rest policy and implementation.
5. External staging threat model and Aegis review before any reachable environment.

### Operations

1. Production-shaped deployment topology and runbook.
2. Observability beyond `/healthz` and operator command tokens: request logs, `/readyz`, metrics, alerting.
3. Backup retention, encryption, off-host storage, restore fire drill, and cleanup policy.
4. Rollback procedure exercised against the chosen staging/deployment shape.
5. Docker/host resource limits and concurrency expectations documented for pilot environment.

### LIMS parity / customer workflow

1. Sample/custody migration import and reconciliation.
2. Analysis/result import with units, qualifiers, limits, methods, and catalog snapshots.
3. QC batch model with release-blocking and auditable decisions.
4. Report package generation from migrated data, not only synthetic native fixture state.
5. Deterministic EDD/export framework with authoritative validation where required.
6. Customer workflow smoke matrix for Tindall/precast-industrial, CENLA/municipal-water, and RJ Lee/materials-forensics analogs.

### Product / UX

1. Daily-driver receiving/result/review/release UX needs more operator polish.
2. Error states and denial reasons need lab-operator friendly presentation.
3. Admin/config flows need enough guardrails to avoid invalid catalog/report states.
4. Export/report download flows need browser-level review beyond command smoke.

## Rollback plan for the allowed internal drill

For local/private synthetic drill:

1. Before drill, record commit SHA, Docker image tag, DB path, artifact directory, and compose project name.
2. Use a fresh disposable data directory or volume.
3. Run `make backup-restore-proof` and retain its `proof-result.json`, manifest, DB snapshot, and proof summary.
4. If validation fails, stop and remove only the drill compose project/volumes, preserving the proof directory and command transcript.
5. Restore path: use the generated backup manifest/SQLite snapshot and `project-scientist restore --force` into a clean local data directory, then run audit verification before writes.
6. Cleanup path: `docker compose down --volumes --remove-orphans` for the drill project only; do not use broad Docker cleanup patterns that could affect concurrent Project Scientist workers.

For any future external staging/customer pilot, this rollback plan is insufficient. A separate Forge/Aegis runbook must define host, DNS/TLS, auth boundary, backup location, restore drill, observability, deployment artifact pinning, and approval IDs.

## Customer-risk language

Safe internal language:

- “Project Scientist is currently an internal synthetic lab-test LIMS prototype with an executable vertical slice.”
- “Recent security remediations passed internal lab-test review only.”
- “It is suitable for internal workflow pressure-testing with synthetic data.”
- “A customer pilot is not approved until migration, security, operations, and workflow-smoke gates pass and Petie approves the scope.”

Unsafe language to avoid:

- “Production-ready.”
- “Ready to replace SENAITE/Clearline LIMS.”
- “Ready for Tindall/CENLA/RJ Lee pilot.”
- “EQuIS-ready” or equivalent export-readiness claims without authoritative spec/validator proof.
- “Secure” without scope; use “passed internal lab-test security controls for the reviewed slice.”

## Go/no-go decision table

| Decision | Recommendation | Rationale |
| --- | --- | --- |
| Internal synthetic validation drill | GO | Evidence is strong enough to continue validating the vertical slice and hardening gaps privately. |
| External staging pilot | NO-GO | Requires separate Forge/Aegis runbook, auth/exposure threat model, observability, backup/rollback, and Petie approval. |
| Customer-data pilot | NO-GO | Migration, customer workflow smoke, security, and approval gates remain incomplete. |
| Customer-facing demo claims | NO-GO | Current proof is internal/synthetic; customer-safe claim language would require approved positioning and artifact review. |
| Production candidate | NO-GO | Roadmap non-negotiable gates are not all complete. |

## Next useful work

1. Create an internal synthetic pilot drill card that runs the full verification suite and captures artifacts/hashes in one packet.
2. Create Aegis card for actor/auth/RBAC and external-checkpoint gap review before staging.
3. Create Forge card for private staging runbook only after Aegis defines the minimum exposure/auth boundary.
4. Create Sylas/Friday card for customer workflow smoke matrix using synthetic analogs only.
5. Create implementation cards for sample/custody migration, result migration, QC release blocking, and report packages generated from migrated fixture data.

Bottom line: keep going, but keep the wall up. The product is starting to look like a real LIMS spine under synthetic load. It is not yet a customer pilot candidate.
