# Project Scientist — Functional MVP Test Scope

Status: lab-test MVP scope. This is not production readiness and not customer migration approval.

## Short answer

The broad SENAITE-parity roadmap is necessary, but by itself it does not guarantee a fully functional MVP at the point we need one. It decomposes parity domains, but it needed an explicit vertical-slice acceptance track: one coherent demo workflow that proves Project Scientist works as a LIMS, not just as a pile of completed modules.

This document defines the MVP gap and the supplemental Kanban cards added to close it.

## MVP definition

A fully functional lab-test MVP must support one synthetic lab tenant completing one realistic sample lifecycle end-to-end:

1. Start local Docker environment from a clean clone/reset.
2. Log in or operate with a dev-auth actor context that cannot be spoofed by arbitrary form/header text.
3. Use one tenant/lab boundary.
4. Configure or seed one client, contact, project/work order, sample matrix, container/preservative, analysis profile, method, analytes, units, limits/qualifiers, and report settings.
5. Receive/accession a sample with one or more containers.
6. Generate a label artifact.
7. Create analysis request lines from configured services/profile.
8. Assign lines to a worksheet/batch.
9. Enter results quickly from a usable result-entry UI.
10. Apply at least minimal QC/review rules.
11. Review and lock results.
12. Generate a COA/report artifact with stable hash/provenance.
13. Release the report through a guarded workflow.
14. Show custody/history/audit trail for every protected mutation.
15. Verify the audit chain and show denied-operation audit evidence.
16. Reset/reseed the demo deterministically.
17. Run one command or script proving the happy path and key negative controls.

If we cannot do those steps, we do not have a functional MVP. We have modules.

## MVP non-goals

For the first MVP, do not require:

- Full SENAITE parity.
- Customer data migration.
- Real production auth/SSO.
- Full EDD/EQuIS exports.
- Every QC type/rule.
- Multi-customer branding matrix.
- Production hosting.

Those remain roadmap items, but they are not required to answer: “Can this become our own LIMS?”

## Minimum demo scenarios

### Scenario A — Happy path

- Seed `Clearline Demo Lab` tenant.
- Seed one client, one project/work order, one contact.
- Seed one water/soil-style sample matrix and container/preservation setup.
- Seed one small analysis profile with 2–4 analytes.
- Receive a sample.
- Generate label.
- Create analysis lines.
- Assign worksheet.
- Enter results.
- Review results.
- Generate and release COA.
- Verify audit chain.

### Scenario B — Denied controls

Prove the system denies and audits:

- Cross-tenant read/write attempt.
- Unauthorized actor attempting protected mutation.
- Illegal sample/result/report workflow jump.
- Report release before review/QC/report preconditions.
- Attempted mutation of released result/report artifact.

### Scenario C — Operational proof

- Clean clone/reset command works.
- Docker health endpoint passes.
- Seed command works.
- E2E smoke command works.
- Backup/restore or at least export/import/reset proof exists for MVP data.

## Supplemental Kanban track

The broad roadmap remains the source of truth for parity. The MVP track adds explicit vertical-slice closure:

- MVP-001: MVP acceptance contract and demo script.
- MVP-002: Synthetic lab fixture/scenario pack.
- MVP-003: Critical-path UX click-budget spec.
- MVP-004: Local demo seed/reset command.
- MVP-005: End-to-end vertical-slice integration.
- MVP-006: Result-entry/report-release demo UX polish.
- MVP-007: E2E smoke and negative-control test suite.
- MVP-008: Audit/security challenge pack for MVP.
- MVP-009: MVP readiness review packet.

## Readiness call

The existing 44-card parity roadmap could eventually produce a SENAITE-parity system, but it was too broad to guarantee a testable MVP quickly. The supplemental MVP track turns the work into an executable milestone with proof, demo data, and negative controls.
