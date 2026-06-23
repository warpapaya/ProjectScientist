# Project Scientist — Roadmap to SENAITE Parity

Status: lab-test roadmap. This is not a production commitment, customer migration approval, or claim that Project Scientist can replace any live Clearline tenant today.

Repo: https://github.com/warpapaya/ProjectScientist
Local dev path: /Users/citadel/Projects/ProjectScientist
Local Docker URL: http://127.0.0.1:8097

## North Star

Build a focused Clearline LIMS that can eventually absorb the workflows we currently cover with SENAITE while improving:

1. UI/UX: daily work in fewer clicks, keyboard-first where useful, dense but not noisy.
2. Auditability: defensible, append-only, actor-attributed, tamper-evident, testable audit trail.
3. Operability: simple Docker development/deployment, backup/restore discipline, predictable migrations.
4. Dependency discipline: keep external dependencies low; add them only when the operational value beats the maintenance cost.
5. Migration confidence: prove data parity before pretending customer cutover is viable.

## Readiness Ladder

- RED: prototype only. No real customer data, no external exposure.
- ORANGE: feature slices work with synthetic data and tests.
- YELLOW: staging pilot with migrated non-sensitive/customer-approved test data.
- GREEN: production candidate after security, audit, reporting, migration, backup/restore, and customer workflow validation gates.

Current state after bootstrap: RED/ORANGE. The app runs locally and has the first audit/workflow spine, but almost every serious LIMS capability is still missing.

## Major SENAITE-Parity Domains

Detailed field-level mapping and migration rules are maintained in `docs/senaite-field-mapping-migration-spec.md`.

### 1. Platform foundation

Needed:
- Durable storage with transactional domain state + audit writes.
- Tenant/lab boundary from day one.
- Authenticated actor context, not spoofable request text.
- RBAC/ABAC permission checks for all protected operations.
- CLI/dev/test workflow and CI.
- Seed fixtures and deterministic smoke tests.

Target implementation bias:
- SQLite first unless/until concurrency/hosting requires Postgres. SQLite is dependency-light, inspectable, Docker-simple, and enough for a lab-test prototype. Abstract storage boundaries so Postgres is possible later.

### 2. Audit/security/compliance spine

Needed:
- Audit event schema with tenant, actor, resource, action, outcome, correlation id, previous hash, hash.
- Append-only writer and startup verifier.
- Denied/failed protected-operation audit events.
- Hash-chain verification and checkpointing.
- Permission model for admin, lab manager, analyst, reviewer, report releaser, client/contact, migration service.
- Amendment/supersession model for released data/artifacts.

### 3. Master data and catalog

Needed:
- Lab/tenant settings.
- Clients, sites/divisions, contacts, contact roles, projects/work orders.
- Sample types/matrices, containers, preservatives, storage locations.
- Analysis services, methods, analytes, units, profiles/panels, departments/categories.
- Specs/limits, qualifiers, report settings, EDD/export settings.
- Versioned catalog objects so reports/results can cite the configuration active at release time.
  - Lab-test v1: catalog create/update operations persist immutable `CatalogSnapshot` rows with version/hash metadata; sample analyses store the active snapshot id/version for downstream result/report artifact references.

### 4. Sample accessioning and custody

Needed:
- Sample intake with client sample id, lab id, project, sampled/received dates, matrix, priority, comments.
- Containers, preservation, received condition, cooler/batch support.
- Chain-of-custody events: received, transferred, split, stored, disposed, returned.
  - Lab-test v1: custody events are append-only rows with actor/time/location/reason, tenant/lab scope, audit entries for allowed and denied record attempts, API exposure through sample state, and workflow-board history/forms.
- Barcode/label generation and print workflows.
- Bulk intake and repeatable templates.

### 5. Analysis requests, worksheets, and result entry

Needed:
- Ordered analysis lines from services/profiles.
- Worksheets by method/department/batch with analyst assignment.
- Result entry with units, qualifiers, detection/reporting limits, uncertainty, dilution, comments.
- Calculations and derived results.
- Instrument/manual import hooks.
- Review queue and analyst/reviewer separation rules.
  - Lab-test v1: result rows support entered -> accepted/rejected review states, reviewer-separation enforcement when configured, lockout for post-review edits, explicit reopen/amend path, and audit events for create/update/review/reopen/denied attempts.

### 6. QC and batch acceptance

Detailed QC taxonomy and relationship semantics are maintained in `docs/qc-sample-taxonomy.md`.

Needed:
- QC sample types: blank, duplicate, spike, LCS, MS/MSD, control sample, calibration verification.
- QC relationships to client samples and batches.
- QC limits/rules by method/matrix/analyte.
- Batch acceptance/rejection workflow.
- QC flags surfaced in result review and COA generation.

### 7. Reporting, COA, COC, labels, and artifacts

Needed:
- Report template model and versioning.
- COA generation with customer/lab branding, sample/result tables, qualifiers, signatures, pages, and attachments.
- COC generation/attachment/package support.
- Label generation with barcodes/QR, sample/container metadata.
- Lab-test v1: deterministic text COA renderer/template path for synthetic Tindall/CENLA-style data, with fixture/hash coverage and immutable ReportArtifact release/audit integration. PDF remains intentionally deferred to avoid adding renderer dependencies before layout requirements harden.
- Immutable ReportSnapshot/ReportArtifact with content hash, template version, data snapshot, reviewer/releaser attribution.
- Amendment/supersession reports.

### 8. Imports, exports, and migration

Needed:
- SENAITE migration mapping for clients, contacts, samples/ARs, services, analyses/results, worksheets/QC where extractable, reports/artifacts where available.
- CSV/XLSX import/export.
- EDD/export framework: fixed-width/CSV/XLSX initially; EQuIS-style only after authoritative spec/validation.
- Legacy ID/path/UID preservation.
- Migration audit provenance and reconciliation reports (see `docs/migration-reconciliation-reports.md` for the lab-test report artifact contract).
- Synthetic golden migration datasets and parity-gap fixtures (see `fixtures/golden_migration_dataset.json` and `docs/golden-migration-datasets.md`).

### 9. UX parity-plus

Needed:
- Dashboard: attention queue, intake, worksheet queue, review/release queue, audit exceptions.
- Low-click flows for intake, worksheet result entry, review/release, COA/package generation.
- Critical-path click budget for the lab-test MVP in `docs/mvp-critical-path-ux-click-budget.md`.
- Keyboard shortcuts and command palette.
- Customer-safe views and client portal concepts later.
- Accessibility and responsive layout, but desktop lab workstation first.

### 10. Operations and release gates

Needed:
- Docker dev/prod profile separation.
- Backup/restore test harness (`./scripts/backup-restore-proof.sh`; details in `docs/backup-restore-proof.md`).
- Audit verification on startup and as an operator command/proof step.
- Seed/demo data and smoke suite.
  - Lab-test v1: `./scripts/performance-concurrency-smoke.sh --json` / `make performance-concurrency-smoke` exercises synthetic concurrent sample mutations, result entry/review, audit writes, and COA report artifact generation with explicit local limits/observations.
- CI: tests, lint, build, container health.
- Deployment runbook, rollback plan, observability/logging.
- Security review gate before any externally reachable staging.

## Phased Plan

### Phase 0 — Guardrails and architecture artifacts

Goal: make the roadmap, parity model, UX spec, audit/security gates, and Docker/dev workflow durable inside the repo.

Exit criteria:
- Roadmap committed.
- Architecture/domain model committed.
- Security/audit gates committed.
- UX workflow spec committed.
- Kanban graph created with dependencies.

### Phase 1 — Production-shaped foundation, still local only

Goal: replace the toy file store shape with production-shaped primitives without overbuilding.

Tasks:
- Storage decision and migration plan: SQLite-first unless disproven.
- Tenant/lab model.
- Authenticated actor context.
- RBAC policy layer.
- Audit schema v1, denied events, verifier.
- CI/test/build hardening.

Exit criteria:
- Mutations are tenant-scoped, permission-checked, and audited.
- Audit verifier catches tampering cases.
- Tests prove negative authorization and illegal workflow paths.
- Docker smoke passes.

### Phase 2 — Master data and catalog

Goal: define enough lab configuration to drive sample intake and analysis ordering.

Tasks:
- Clients/sites/contacts/projects.
- Sample matrices/types/container/preservation/storage reference data.
- Analysis services, analytes, methods, units, profiles/panels.
- Versioned catalog snapshots.
- Admin/config UI.

Exit criteria:
- A lab manager can configure client defaults and test catalog from UI/API.
- Sample intake can use configured profiles/services.
- Version snapshots are retained for downstream reports.

### Phase 3 — Sample accessioning and custody

Goal: support real lab receiving workflow.

Tasks:
- Sample intake v2.
- Containers/aliquots/partitions.
- COC events and custody chain.
- Labels/barcodes.
- Bulk intake/template workflows.

Exit criteria:
- Intake can model common Tindall/CENLA-style sample receipt.
- Custody history is auditable.
- Labels can be generated and verified.

### Phase 4 — Worksheets and result entry

Goal: analysts can do work without spreadsheet gymnastics.

Tasks:
- Analysis request lines.
- Worksheets by method/department/batch.
- Result entry grid (`docs/result-entry-grid-workflow.md`).
- Result qualifiers/limits/units.
- Calculations/derived results.
- Review queue.

Exit criteria:
- Sample results can be entered, reviewed, and locked.
- UI supports high-throughput daily workflow.
- Audit trail captures result changes and review decisions.

### Phase 5 — QC and batch acceptance

Goal: model enough QC to defend reported results.

Tasks:
- QC sample taxonomy.
- QC relationships and batch model.
- QC limits/rules engine.
- QC review and batch acceptance.
- QC flags into result review/report readiness.

Exit criteria:
- QC failures can block release.
- QC decisions are auditable.
- Batch summary is inspectable.

### Phase 6 — Reporting, COA, COC package, labels

Goal: produce defensible artifacts, not just screen data.

Tasks:
- ReportSnapshot/ReportArtifact model.
- COA renderer/template system.
- COC package generation and attachment handling. Implemented for lab-test synthetic data as immutable content-hashed COC packages with attachment manifests and audit events.
- Report release and amendment workflow. Lab-test v1 requires accepted/reviewed results, accepted QC batch readiness, immutable report artifact content, and a release signature equivalent before writing a released ReportSnapshot/ReportArtifact.
- Label print artifacts.

Exit criteria:
- Released report artifacts are immutable and hash-addressed, with releaser signature-equivalent provenance captured in the snapshot hash.
- Amendments supersede via immutable supersession edges; released snapshots/artifacts/supersession links are never silently edited.
- COA/COC package works with synthetic Tindall/CENLA-style data.

### Phase 7 — Migration and parity validation

Goal: prove whether current customer migration is plausible.

Tasks:
- SENAITE parity matrix with field mappings.
- Extract/import tooling for SENAITE exports or API dumps.
- Reconciliation reports.
- Golden dataset migration tests.
- Gap report per customer workflow (see `docs/customer-workflow-gap-report.md`).

Exit criteria:
- Synthetic and non-sensitive exported data can round-trip with reconciliation.
- Migration gaps are explicit and quantified.
- No live customer migration without separate approval.

### Phase 8 — Ops, staging, and pilot readiness

Goal: decide if this deserves a staging pilot.

Tasks:
- Backup/restore proof.
- Observability and operator commands.
- Security hardening/review.
- Performance/concurrency smoke.
- Release/rollback runbook.
- Pilot readiness packet.

Exit criteria:
- Friday/Aegis/Forge signoff artifacts exist.
- Petie has a clear go/no-go call for a controlled pilot.

## Non-negotiable Gates Before Customer Deployment

1. Auth/RBAC complete and tested.
2. Audit schema/verifier/checkpointing complete and tested.
3. Tenant isolation complete and tested.
4. Backup/restore proof complete.
5. Report artifacts immutable and amendment-safe.
6. Migration reconciliation complete for candidate customer.
7. Security review passed.
8. Customer workflow smoke tests passed.
9. Rollback plan exists.
10. Petie explicitly approves the pilot/customer move.

## Current Strategic Take

Project Scientist is worth pursuing as a side lane because SENAITE parity is less about cloning SENAITE and more about preserving lab workflow semantics while deleting historical complexity. The danger is building a generic CRUD app with lab words sprinkled on top. The roadmap deliberately fronts audit, tenant boundaries, catalog versioning, QC, report snapshots, and migration provenance because those are the places naive LIMS rewrites go to die.
