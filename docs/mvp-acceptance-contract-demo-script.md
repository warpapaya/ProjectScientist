# Project Scientist — MVP Acceptance Contract and Operator Demo Script

Status: lab-test acceptance contract. This document does not approve production use, customer data, customer migration, public exposure, or customer-facing readiness claims.

Repo: https://github.com/warpapaya/ProjectScientist
Local path: `/Users/citadel/Projects/ProjectScientist`
Local URL: `http://127.0.0.1:8097`
Related docs:

- `docs/senaite-parity-roadmap.md`
- `docs/mvp-test-scope.md`
- `docs/security-audit-model.md`
- `docs/dev.md`

## 1. Purpose

The Project Scientist MVP is accepted only when one synthetic lab tenant can complete one realistic sample lifecycle end-to-end, from clean local start through released COA artifact, with auditable state changes and denied-operation evidence.

This contract exists to prevent a common failure mode: shipping modules that look useful in isolation but cannot prove a coherent LIMS workflow.

## 2. Non-negotiable boundaries

- Lab-test only.
- Synthetic data only.
- Loopback/local Docker only unless a later approved task changes scope.
- No Tindall, CENLA, RJ Lee, AmSpec, or other customer data.
- No external exposure, reverse proxy, DNS, Authentik, shared hosting, or production deployment.
- No production-ready, customer-ready, migration-ready, or SENAITE-replacement claim.
- Any failed acceptance item keeps readiness at RED/ORANGE and must produce a backlog item, not a softened pass.

## 3. Canonical MVP commands

The accepted MVP must expose these commands from the repo root. If a command does not exist, exits non-zero, is non-deterministic, requires manual hidden state, or mutates anything outside the local lab-test environment, the MVP fails.

```bash
# Clean local state and remove the MVP demo volume/data.
make mvp-reset

# Build/start the local Docker app on loopback and verify /healthz.
make mvp-up

# Seed the deterministic synthetic tenant, catalog, client, project, sample, worksheet, and report settings.
make mvp-seed

# Run the end-to-end happy-path workflow non-interactively.
make mvp-demo

# Verify audit chain, required events, and released-artifact hashes.
make mvp-audit-verify

# Run negative controls for authorization, tenant isolation, illegal workflow jumps, premature release, and released-artifact immutability.
make mvp-denied-controls

# One-command acceptance suite. Must run all of the above after a clean reset.
make mvp-acceptance

# Stop local containers after the demo. This may preserve data unless paired with mvp-reset.
make mvp-down
```

Current bootstrap compatibility commands are documented in `docs/dev.md` (`make dev-reset`, `make dev-up`, `make dev-seed`, `make docker-smoke`). They are not by themselves MVP acceptance because the bootstrap does not yet implement labels, worksheets, result entry, QC/review, COA generation/release, tenant isolation, authenticated actor controls, or full denied-operation audit events.

## 4. Acceptance checklist

Each row is a hard gate. Use `PASS`, `FAIL`, or `N/A-not-allowed`. For MVP acceptance, no row may be `FAIL` and no core lifecycle row may be `N/A`.

| ID | Gate | Required proof | Pass criteria | Fail criteria |
|---|---|---|---|---|
| MVP-00 | Clean Docker reset/start | `make mvp-reset && make mvp-up`; `curl -fsS http://127.0.0.1:8097/healthz` | Fresh local app starts on loopback; health returns `ok`; no customer/prod services touched | Needs manual cleanup; binds publicly; health fails; uses non-local data |
| MVP-01 | Deterministic seed data | `make mvp-seed`; `curl -fsS http://127.0.0.1:8097/api/state` or equivalent state export | Seeds exactly one synthetic lab tenant, client, contact, project/work order, matrix, container/preservative, method/profile, analytes, units, limits/qualifiers, and report settings | Seed duplicates uncontrolled data; requires hand edits; lacks required catalog objects |
| MVP-02 | Sample intake/accession | `make mvp-demo` log plus state export | One sample is received with client sample id, lab id, project, matrix, received date, container/preservative, and initial custody event | Sample cannot be created from seeded config; required metadata missing; custody not audited |
| MVP-03 | Label artifact | `make mvp-demo` creates artifact path such as `var/mvp/artifacts/labels/<lab-id>.pdf` or `.txt` | Label artifact includes lab id/barcode or QR payload, client/sample metadata, and immutable generated timestamp/hash | Label missing, manually fabricated, not linked to sample, or not hashable |
| MVP-04 | Analysis request lines | State export after intake | 2-4 ordered analysis lines are created from seeded profile/services and carry method, analyte, unit, limit/qualifier metadata snapshots | Lines are hand-entered without catalog link; no version snapshot; missing analyte/unit/limit metadata |
| MVP-05 | Worksheet/batch assignment | `make mvp-demo` log/state export | Analysis lines are assigned to one worksheet/batch with analyst, method/department, and status | No worksheet concept; assignment only visual; sample skips worksheet stage |
| MVP-06 | Result entry | `make mvp-demo` log/state export and UI smoke where available | Results are entered for all required analytes with value, unit, qualifier/limit support, analyst attribution, and audit events | Results cannot be entered; missing attribution; results bypass worksheet/sample state |
| MVP-07 | Review and lock | `make mvp-demo` log/state export | Reviewer accepts results; reviewed results become locked against ordinary edit; analyst/reviewer separation is enforced where configured | Review is a status label only; same actor can violate configured separation; reviewed data remains silently editable |
| MVP-08 | COA generation | `make mvp-demo` creates artifact path such as `var/mvp/artifacts/coa/<sample-id>-v1.pdf` or `.html` | COA includes lab/client/sample identity, result table, units, qualifiers/limits, reviewer/releaser attribution, template/version, generation timestamp, and content hash | COA missing; artifact not reproducible; lacks result/provenance fields; uses customer branding/data |
| MVP-09 | COA release | `make mvp-demo` log/state export | Release requires reviewed/locked results, accepted QC/preconditions, releaser actor, release reason/attestation, immutable artifact hash, and audit event | Release can happen before review/results/artifact; released artifact can be overwritten; no release audit |
| MVP-10 | Audit verify | `make mvp-audit-verify` | Verifier passes clean chain and confirms required happy-path events in order: seed/config, sample intake, label generated, worksheet assigned, results entered, review, COA generated, COA released | Verifier missing; chain mismatch; required events absent; event order impossible to defend |
| MVP-11 | Denied-operation controls | `make mvp-denied-controls` | System denies and audits: cross-tenant access, unauthorized mutation, illegal workflow jump, release before review/QC/report preconditions, and mutation of released result/report artifact | Any control succeeds; denial not audited; error leaks sensitive data; check relies only on UI hiding |
| MVP-12 | One-command acceptance | `make mvp-acceptance` from clean repo/local reset | Runs reset, up, seed, happy path, audit verify, denied controls, and down/cleanup policy with a written pass/fail summary | Requires undocumented manual steps; passes despite skipped gates; leaves ambiguous state |
| MVP-13 | Standard engineering gates | `go test ./...`; `go vet ./...`; Docker build/smoke per `docs/dev.md` | Tests/vet pass; Docker starts and health checks; no unapproved dependency changes | Test/vet failure; Docker failure; hidden dependency drift |

## 5. Synthetic fixture contract

The MVP fixture must be stable enough for screenshots, demos, and regression checks.

Required fixture values:

```text
Tenant/lab: Clearline Demo Lab
Client: Okefenokee Synthetic Water Authority
Contact: Jordan Demo <jordan.demo@example.test>
Project/work order: MVP Drinking Water Compliance Demo
Sample matrix: Drinking Water
Container/preservation: 250 mL HDPE, nitric acid preserved
Analysis profile: MVP Metals + Field Checks
Analytes: pH, Turbidity, Lead, Copper
Units: pH units, NTU, ug/L, ug/L
Report template: MVP Synthetic COA v1
Actors:
  lab-manager: seeds/configures catalog
  receiving-tech: receives sample and prints label
  analyst: worksheet/result entry
  reviewer: reviews/locks results
  report-releaser: releases COA
  unauthorized-actor: negative controls
Second tenant for controls: Clearline Demo Lab B
```

The fixture may be implemented as API calls, CLI commands, SQL migrations, or checked-in JSON/YAML fixtures, but `make mvp-seed` must be the only operator entrypoint.

## 6. Operator demo script

Run this from a clean local checkout on Citadel or another approved development machine with Go, Docker Compose v2, and make installed.

### Step 0 — Confirm lab-test posture

Say this before the demo:

```text
This is a local lab-test Project Scientist MVP demo using synthetic data only. It is not production-ready, not customer-ready, and not approved for customer data, migration, or external exposure.
```

Pass/fail:

- PASS: operator states the boundary and the app is local-only.
- FAIL: operator implies customer/production readiness or uses customer data.

### Step 1 — Clean reset and start

```bash
cd /Users/citadel/Projects/ProjectScientist
make mvp-reset
make mvp-up
curl -fsS http://127.0.0.1:8097/healthz
```

Expected:

```text
ok
```

Pass/fail:

- PASS: reset completes, container starts, health returns `ok` from loopback.
- FAIL: any command fails, app binds externally, or manual state cleanup is required.

Bootstrap-only fallback for current prototype validation:

```bash
make dev-reset
make dev-up
curl -fsS http://127.0.0.1:8097/healthz
```

### Step 2 — Seed deterministic lab data

```bash
make mvp-seed
curl -fsS http://127.0.0.1:8097/api/state
```

Expected proof:

- One tenant/lab: `Clearline Demo Lab`.
- One client/contact/project/work order.
- One configured matrix/container/preservation.
- One analysis profile with pH, Turbidity, Lead, Copper.
- One report template/settings object.
- Seed/config audit events attributed to `lab-manager` or explicit seed service actor.

Pass/fail:

- PASS: fixture exists exactly once and can be reset/reseeded deterministically.
- FAIL: fixture duplicates, requires manual UI edits, or omits required objects.

Bootstrap-only fallback:

```bash
make dev-seed
curl -fsS http://127.0.0.1:8097/api/state
```

This fallback only proves client/sample seed, not full MVP seed acceptance.

### Step 3 — Receive/accession sample

```bash
make mvp-demo STEP=intake
```

Expected proof:

- Sample has a generated lab id.
- Sample references the seeded client/project/matrix/container.
- Initial status is accessioned/received.
- Custody/history records receipt.
- Audit includes sample intake event.

Pass/fail:

- PASS: sample is intake-complete and audit-visible.
- FAIL: sample lacks required metadata, bypasses catalog, or has no custody/audit event.

### Step 4 — Generate label

```bash
make mvp-demo STEP=label
```

Expected proof:

- Label artifact path is printed.
- Label contains lab id/barcode or QR payload and sample/client metadata.
- Artifact hash is recorded in state/audit.

Pass/fail:

- PASS: label can be opened/inspected and ties back to the sample.
- FAIL: no artifact, artifact is manually fabricated, or no hash/provenance is stored.

### Step 5 — Create analysis lines

```bash
make mvp-demo STEP=analysis-lines
```

Expected proof:

- Analysis lines are created from `MVP Metals + Field Checks` profile.
- Each line has method/analyte/unit/limit/qualifier metadata snapshot.
- Audit records the request/order action.

Pass/fail:

- PASS: lines are catalog-driven and inspectable.
- FAIL: lines are only display text, are missing snapshots, or are not audited.

### Step 6 — Assign worksheet/batch

```bash
make mvp-demo STEP=worksheet
```

Expected proof:

- Worksheet/batch id is created.
- Lines are assigned to the worksheet.
- Analyst assignment and worksheet status are visible.
- Audit records assignment.

Pass/fail:

- PASS: worksheet owns the result-entry work queue.
- FAIL: worksheet is absent or not connected to analysis lines.

### Step 7 — Enter results

```bash
make mvp-demo STEP=results
```

Expected proof:

- Results are entered for pH, Turbidity, Lead, and Copper.
- Each result includes value, unit, qualifier/limit support, analyst actor, timestamp, and audit event.
- UI/API supports usable result-entry flow; no direct database editing.

Pass/fail:

- PASS: all required results are entered through supported app workflow.
- FAIL: direct DB edits, missing attribution, or partial result set.

### Step 8 — Review and lock results

```bash
make mvp-demo STEP=review
```

Expected proof:

- Reviewer accepts results.
- Reviewed result versions lock against ordinary edit.
- Review event is audited.
- Reviewer separation rule is enforced where configured.

Pass/fail:

- PASS: reviewed results are locked and provenance is clear.
- FAIL: review can be skipped, same actor violates configured policy, or reviewed result can be silently edited.

### Step 9 — Generate and release COA

```bash
make mvp-demo STEP=coa
make mvp-demo STEP=release
```

Expected proof:

- COA/report artifact path is printed.
- Artifact includes sample identity, result table, units, qualifiers/limits, template/version, reviewer, releaser, release timestamp, and content hash.
- Release requires reviewed results and report preconditions.
- Released artifact is immutable; changes require amendment/supersession.

Pass/fail:

- PASS: generated COA is hash-addressed and released through guarded workflow.
- FAIL: report can release before review/results; artifact lacks hash/provenance; released report can be overwritten.

### Step 10 — Verify audit chain and evidence

```bash
make mvp-audit-verify
```

Expected proof:

- Hash chain verifies.
- Required happy-path events exist in defensible order.
- Report/label artifact hashes match files on disk or object store.

Pass/fail:

- PASS: verifier returns success and lists checked event/artifact ids.
- FAIL: verifier missing, event gap, hash mismatch, or artifact mismatch.

### Step 11 — Run denied-operation controls

```bash
make mvp-denied-controls
```

Required controls:

1. Cross-tenant read/write attempt.
2. Unauthorized actor protected mutation.
3. Illegal sample/result/report workflow jump.
4. Report release before review/QC/report preconditions.
5. Mutation of released result/report artifact.

Expected proof:

- Each operation returns a denial/error.
- Each denial emits a safe audit event with outcome/reason.
- Denials do not leak protected cross-tenant resource details.

Pass/fail:

- PASS: all controls deny and audit.
- FAIL: any control succeeds, is not audited, or relies only on UI hiding.

### Step 12 — Run one-command acceptance

```bash
make mvp-acceptance
```

Expected proof:

- Prints a pass/fail summary with command outputs or artifact paths.
- Runs from clean reset without manual hidden state.
- Leaves local-only state according to documented cleanup policy.

Pass/fail:

- PASS: every acceptance gate is exercised and summarized.
- FAIL: skipped gates, manual steps, non-deterministic output, or ambiguous state.

## 7. Required acceptance artifact packet

A passing MVP run must leave a local artifact packet, for example under `var/mvp/acceptance/<timestamp>/`, containing:

- `summary.txt` or `summary.json` with all gate outcomes.
- Seed fixture manifest.
- State export after release.
- Audit verification output.
- Denied-control output.
- Label artifact(s) and hash manifest.
- COA artifact(s) and hash manifest.
- Test command output for `go test ./...`, `go vet ./...`, and Docker/HTTP smoke.

The packet must contain synthetic data only.

## 8. Readiness language

Allowed language after all gates pass:

```text
Project Scientist has a lab-test MVP vertical slice using synthetic data. It demonstrates a local Docker-only sample lifecycle with audit evidence and negative controls. It is not approved for customer data, production deployment, migration, or customer-facing readiness claims.
```

Disallowed language:

- Production-ready.
- Customer-ready.
- SENAITE replacement.
- Ready for Tindall/CENLA/RJ Lee data.
- Migration-approved.
- Secure/compliant without Aegis/security implementation review.

## 9. Backlog implications

Any missing command or failed gate should become a specific implementation card. Do not dilute the acceptance contract to match current code. The current bootstrap prototype can pass foundation checks, but MVP acceptance requires the full vertical slice above.
