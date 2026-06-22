---
title: PSC-MVP-003 Productionization Review Packet
project: Project Scientist
status: lab-test docs productionization review
source_task: t_5589edce
productionization_task: t_8e61ad56
review_date: 2026-06-22
source_commit: 472336b9a0a4fd5e66de78b32e1d40445c103251
source_spec_sha256: 470e1c4d80d92dc2bd016ded6e20c6488b82551c4b8f6e61548d4913c2451b11
---

# PSC-MVP-003 Productionization Review Packet

This packet surfaces the Project Scientist MVP critical-path UX click-budget specification for Petie-facing internal review. It is a lab-test documentation artifact only. It does not approve production use, customer data, customer migration, public exposure, customer-facing readiness claims, or mutation of any client system.

## Source artifact

- Spec: `docs/mvp-critical-path-ux-click-budget.md`
- Source review commit: `472336b9a0a4fd5e66de78b32e1d40445c103251` (`docs: define MVP UX click budget`)
- Source spec SHA256: `470e1c4d80d92dc2bd016ded6e20c6488b82551c4b8f6e61548d4913c2451b11`
- Repo path: `/Users/citadel/Projects/ProjectScientist`
- Existing repo links:
  - `README.md` references `docs/mvp-critical-path-ux-click-budget.md`
  - `docs/mvp-test-scope.md` lists MVP-003 as the critical-path UX click-budget spec
  - `docs/senaite-parity-roadmap.md` references the critical-path click budget

## Critical-path gate

The spec defines the daily-driver path that the MVP UI must support:

`dashboard -> intake -> label -> worksheet/result entry -> review/release -> audit/report package`

Click-budget gates:

| Gate | Budget |
| --- | ---: |
| Target happy path | 18 clicks or fewer |
| Stretch, with keyboard/defaults | 12 clicks or fewer |
| Maximum MVP acceptance budget | 24 clicks |

If the synthetic MVP fixture exceeds 24 clicks from dashboard to released package, the UI fails the daily-driver test even if the API workflow works.

## Stage-level click budgets

| Stage | Target clicks | Maximum clicks | Required behavior |
| --- | ---: | ---: | --- |
| Dashboard to intake | 1 | 2 | Primary `Receive sample` action visible above the fold and shortcut-accessible. |
| Intake complete | 3 | 5 | Seeded defaults prefill; first invalid field receives focus; submit advances to label. |
| Label generation/print | 1 | 2 | Label generated from intake completion; print/download is primary and sample-linked. |
| Worksheet/result entry open | 2 | 3 | Newly accessioned lines appear in worksheet queue; active worksheet focuses first empty result. |
| Enter 4-analyte result set | 4 | 6 | Spreadsheet-like entry; Enter advances rows; unit/limit defaults; qualifier shortcuts. |
| Review/lock results | 2 | 3 | Reviewer queue opens to changed/flagged rows with approve-all-valid action. |
| Generate/release COA | 3 | 4 | Generate package, preview exceptions, release with explicit attestation. |
| Audit/report package verification | 2 | 3 | Package page shows audit chain/hash status and artifact download without navigation hunt. |

## UX coverage verified

The source review verified that the spec covers the required MVP-003 UX contract:

- Keyboard shortcuts for dashboard, intake, worksheet, review, release/package, command/search, help, save/submit, and grid navigation.
- Default focus behavior for app load, intake, pickers, validation failures, worksheet grid, review, release, and package success.
- Validation and error states for intake, label generation, result-entry save, unauthorized/protected operations, release blockers, audit/hash failures, and artifact download failure.
- Dense/no-noise UI guidance that rejects marketing chrome, oversized cards, modal wizard sprawl, kebab-menu critical actions, toast-only errors, and four-detail-page result entry.
- Stop-lines preserving local/synthetic lab-test framing only.

## Validation evidence

Validation was run in an isolated clean worktree at commit `472336b9a0a4fd5e66de78b32e1d40445c103251` because the shared repo checkout currently contains unrelated uncommitted Project Scientist work from other cards.

| Gate | Result | Notes |
| --- | --- | --- |
| Source spec checksum | PASS | `470e1c4d80d92dc2bd016ded6e20c6488b82551c4b8f6e61548d4913c2451b11` |
| Phrase/content scan | PASS | Click target/max, shortcuts, focus contract, error states, dense/no-noise rules, lab-only stop-line, and security doc link present. |
| Link/reference scan | PASS | README, MVP test scope, and SENAITE parity roadmap reference the spec. |
| `go test ./...` | PASS | Passed in isolated clean worktree at `472336b`. |
| `go vet ./...` | PASS | Passed in isolated clean worktree at `472336b`. |
| `make fmt-check` | PASS | Passed in isolated clean worktree at `472336b`. |
| `make docker-smoke` | PASS | Passed in isolated clean worktree after transient existing-container conflict was cleared. |
| `make dev-down` cleanup | PASS | Cleanup passed after smoke validation. |

## Remaining implementation caveat

This is a specification and review packet, not proof that the UI already meets the click budget. Future implementation cards must instrument the synthetic MVP fixture with stable selectors/focus checks and count the critical path against the 18-click target and 24-click maximum.

## Accepted internal language

Project Scientist has a lab-test MVP critical-path UX click-budget specification for a synthetic local operator workflow. The spec defines the click gates, keyboard shortcuts, default focus contract, validation/error behavior, and dense/no-noise UI constraints needed before daily-driver testing. It is not approved for customer data, production deployment, migration, external exposure, or customer-facing readiness claims.
