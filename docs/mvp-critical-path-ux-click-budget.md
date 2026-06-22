# Project Scientist — MVP Critical-Path UX Click Budget

Status: lab-test MVP UX specification. This document defines the target daily-driver operator experience for synthetic/local MVP testing only. It does not approve production use, customer data, customer migration, public exposure, or customer-facing readiness claims.

Repo: https://github.com/warpapaya/ProjectScientist
Local path: `/Users/citadel/Projects/ProjectScientist`
Local URL: `http://127.0.0.1:8097`
Related docs:

- `docs/senaite-parity-roadmap.md`
- `docs/mvp-test-scope.md`
- `docs/mvp-acceptance-contract-demo-script.md`
- `docs/security-audit-model.md`

## 1. Purpose

The MVP must feel like a lab daily-driver, not a CRUD demo. This spec sets the target click budget, keyboard behavior, default focus rules, error-state requirements, and dense/no-noise UI guidance for the MVP critical path:

`dashboard -> intake -> label -> worksheet/result entry -> review/release -> audit/report package`

The budget is intentionally aggressive because the north star is SENAITE parity-plus: fewer clicks, keyboard-first where useful, dense but not noisy, and defensible audit trail visibility without modal clutter.

## 2. Counting rules

Use these rules consistently when evaluating a prototype or implementation:

- A click is any pointer activation required to advance the task: button, tab, dropdown open, dropdown selection, checkbox, row action, menu action, modal confirmation.
- Typing is not a click. `Tab`, `Enter`, arrow keys, scanner input, barcode input, and shortcut chords are not clicks.
- Auto-navigation after save does not count as a click.
- Toasts that do not require dismissal do not count.
- A confirmation modal counts at least one extra click and is allowed only for irreversible or release-grade actions.
- Hidden hover-only controls are not allowed on critical path actions because they increase visual hunting and break scanner/keyboard workflows.
- Error recovery counts the clicks required after the validation error is shown.

## 3. MVP click budget summary

Target happy-path budget from dashboard to released package: 18 clicks or fewer after seed data exists.

Stretch budget: 12 clicks or fewer if the operator uses keyboard shortcuts and defaults.

Maximum allowed budget for MVP acceptance: 24 clicks. If the MVP demo exceeds 24 clicks for the synthetic fixture, the UI fails the daily-driver test even if the API workflow works.

| Step | Target clicks | Maximum clicks | Required low-click behavior |
|---|---:|---:|---|
| Dashboard to intake | 1 | 2 | Primary `Receive sample` action is visible above the fold and shortcut-accessible. |
| Intake complete | 3 | 5 | Seeded client/project/profile defaults prefill; first invalid field gets focus; submit advances to label. |
| Label generation/print | 1 | 2 | Label is generated from intake completion; print/download action is primary and sample-linked. |
| Worksheet/result entry open | 2 | 3 | Newly accessioned lines appear in worksheet queue; opening the active worksheet focuses first empty result. |
| Enter 4-analyte result set | 4 | 6 | Spreadsheet-like entry with Enter-to-next-row, unit/limit defaults, qualifier shortcuts. |
| Review/lock results | 2 | 3 | Reviewer queue opens directly to changed/flagged rows with approve all valid rows action. |
| Generate/release COA | 3 | 4 | Generate package, preview exceptions, release with attestation; release requires explicit confirmation. |
| Audit/report package verification | 2 | 3 | Package page shows audit chain/hash status and artifact download without separate navigation hunt. |

## 4. Critical path details

### 4.1 Dashboard

Target: the operator can answer “what needs my attention next?” without scanning unrelated admin chrome.

Required regions, in order:

1. Attention queue: blocked/failed validations, audit exceptions, release blockers.
2. Intake queue: `Receive sample` primary action and recent received samples.
3. Worksheet queue: active batches, assigned analyst, unentered/flagged counts.
4. Review/release queue: ready-for-review, ready-for-release, failed preconditions.
5. Audit/package tail: latest protected mutation, verifier status, latest released package.

Click budget:

- Open dashboard from app load: 0 clicks.
- Start intake from dashboard: 1 click or `g i` / `n s` shortcut.
- Open active worksheet: 1 click from worksheet queue.
- Open review queue: 1 click from review/release queue.

Default focus:

- On page load, focus the command/search field if present; otherwise focus the first actionable queue item without stealing focus from a scanner input during active scan.
- The primary action must be reachable as the first `Tab` stop after skip-link/navigation.

Error/empty states:

- Empty state must say what to do next: `No samples received yet. Press N S or click Receive sample.`
- Queue errors must include the blocking reason and the next valid action, not just status names.
- Audit verifier failure is a top-priority banner with a link to the exact event/resource.

### 4.2 Intake

Target: receive the synthetic sample using seeded defaults with minimal hunting.

Target click path:

1. Dashboard `Receive sample`.
2. Optional choose seeded client/project only if not already defaulted.
3. `Save and print label`.

Required defaulting:

- Tenant/lab defaults to `Clearline Demo Lab`.
- Client defaults to `Okefenokee Synthetic Water Authority` when running the MVP fixture.
- Project/work order defaults to `MVP Drinking Water Compliance Demo` after client selection.
- Matrix defaults to `Drinking Water`.
- Container/preservation defaults to `250 mL HDPE, nitric acid preserved`.
- Analysis profile defaults to `MVP Metals + Field Checks`.
- Received date defaults to current local date with explicit timezone display.
- Client sample id is the first required blank field and receives focus.

Keyboard shortcuts:

- `n s`: new sample/intake.
- `Ctrl+Enter` or `Cmd+Enter`: save intake when required fields are valid.
- `/`: focus command/search.
- `Esc`: leave an open picker without losing field contents.

Validation and error states:

- Inline validation appears beside the field, not in a generic modal.
- Failed submit moves focus to the first invalid field and announces the error via accessible live region.
- Duplicate client sample id warning must show the existing sample link and allow explicit override only if the domain permits it.
- Tenant mismatch or unauthorized catalog access must block save and create a denied audit event; the UI must not imply “try again later.”

### 4.3 Label

Target: label generation is a continuation of intake, not a separate search task.

Click budget:

- Generate label after intake save: 0 additional clicks if `Save and print label` was used.
- Print/download label from label view: 1 click.
- Return to dashboard or worksheet queue: 1 click or auto-advance if the operator chooses `Print and continue`.

Required UI behavior:

- Label view shows lab id, barcode/QR payload, client sample id, matrix, container, received timestamp, and artifact hash.
- The primary action is `Print label`; secondary is `Download label artifact`.
- If printing is unavailable in the lab-test environment, a deterministic PDF/HTML/TXT artifact path is shown and hashable.

Error states:

- Label generation failure must preserve the accessioned sample and show `Retry label generation` without duplicate sample creation.
- Barcode/QR payload mismatch blocks print and writes an audit/error event.

### 4.4 Worksheet and result entry

Target: analysts enter the 4-analyte MVP result set without opening each analyte in a detail page.

Click budget:

- Dashboard to active worksheet: 1 click.
- Select all ready lines for seeded sample: 1 click only if grouping is not already active.
- Enter four analyte values: 0 clicks after focus enters grid.
- Mark entry complete: 1 click or `Ctrl+Enter` / `Cmd+Enter`.

Default focus:

- Opening a worksheet focuses the first empty result value cell.
- Pressing `Enter` commits the current cell and advances to the next required value cell.
- Pressing `Shift+Enter` moves to the previous editable result cell.
- Arrow keys move cell focus without opening menus.
- Qualifier cell supports short tokens (`ND`, `J`, `U`, `H`) without dropdown-only interaction.

Required columns for the dense MVP grid:

1. Sample/lab id.
2. Analyte.
3. Result value.
4. Unit.
5. Qualifier.
6. Limit/spec hint.
7. Method/profile snapshot.
8. Status/error.
9. Analyst/audit indicator.

Dense/no-noise rules:

- Units and limits default from catalog snapshots and are visible but not visually louder than the value field.
- Repeated sample/client metadata collapses into a sticky group header.
- Audit badges are compact icons/text chips with hover/focus details, not full event dumps in the grid.
- No marketing copy, empty illustration, or card chrome inside the worksheet grid.

Error states:

- Invalid numeric format stays in-cell and prevents completion.
- Out-of-limit or qualifier-required conditions are warnings until domain rules say they are blockers.
- Unauthorized edit, released-result edit, or analyst/reviewer separation violation must be blocked by the domain layer and shown as a hard error with audit evidence.
- Network/API save failure keeps dirty values locally visible and marks rows unsaved; no silent clear.

### 4.5 Review and release

Target: reviewer can see what changed, what is blocked, and what can be released without rereading the entire sample file.

Click budget:

- Open review queue from dashboard: 1 click.
- Open ready sample/package candidate: 1 click.
- Approve/lock all valid rows: 1 click plus optional confirmation only if e-signature/attestation is configured.
- Generate COA package: 1 click.
- Release COA: 1 click to open release, 1 explicit attestation/confirm click.

Keyboard shortcuts:

- `g r`: review queue.
- `a`: approve focused valid row.
- `A`: approve all currently valid rows after focus is in the review panel.
- `g p`: package/report panel.
- `Ctrl+Enter` or `Cmd+Enter`: submit the current review/release action when valid.

Required review UI:

- Show only rows needing reviewer attention by default, with a toggle for all rows.
- Surface QC/precondition blockers before the release button.
- Display analyst and reviewer actors distinctly.
- After lock, ordinary edit controls disappear or disable with an explicit reason.

Release hard stops:

- No release before all required results are entered.
- No release before review/lock.
- No release while QC/preconditions are blocking.
- No release if package hash cannot be computed or stored.
- No release by an unauthorized actor.

Every hard stop must show the reason and write/retain denied-operation audit evidence per `docs/security-audit-model.md`.

### 4.6 Audit/report package

Target: released package proof is one screen: artifact, hash, audit chain status, release actor, and download path.

Click budget:

- Open released package from release success screen: 0 clicks.
- Download/view package artifact: 1 click.
- Open audit evidence for the package: 1 click.

Required package summary:

- Sample/lab id and client sample id.
- Report artifact path and content hash.
- Template/version snapshot.
- Reviewer and releaser actor attribution.
- Release timestamp and attestation/reason.
- Audit verifier status.
- Required event checklist: intake, label, worksheet assignment, result entry, review, COA generation, release.

Error states:

- Hash mismatch or missing audit event is a blocking red state, not a warning toast.
- Package download failure must not mark release failed if release already committed; it must show artifact retrieval failure separately.
- Released artifact mutation attempts must be denied and visible in audit evidence.

## 5. Global keyboard map

These shortcuts are MVP targets. They should be documented in-app via `?` or command palette help before daily-driver testing.

| Shortcut | Target action |
|---|---|
| `/` | Focus command/search. |
| `?` | Open keyboard shortcut help. |
| `g d` | Go to dashboard. |
| `n s` | New sample intake. |
| `g w` | Go to worksheet queue/current worksheet. |
| `g r` | Go to review queue. |
| `g p` | Go to report/package panel. |
| `Ctrl+Enter` / `Cmd+Enter` | Save/submit current valid form or workflow action. |
| `Esc` | Close picker/dialog without discarding entered field values. |
| `Enter` in result grid | Commit cell and move to next required result value. |
| `Shift+Enter` in result grid | Move to previous editable result value. |

Shortcut requirements:

- Never override native browser text-editing shortcuts inside text inputs.
- Every shortcut action must have a visible button or menu equivalent.
- Shortcut-triggered protected mutations must hit the same domain/API path as click-triggered mutations.
- If a shortcut cannot run because preconditions fail, focus the blocking message.

## 6. Default focus contract

The MVP UI must be testable with keyboard-only operation for the critical path.

Required focus behavior:

- App load: focus command/search or first critical queue action.
- Intake open: focus first required blank field, normally client sample id.
- Picker selection: return focus to the field that opened the picker.
- Intake validation failure: focus first invalid field.
- Intake success: focus print label action or auto-generate label result.
- Worksheet open: focus first empty result value cell.
- Row validation failure: keep focus on failed cell.
- Review open: focus first blocker or first approvable row/action.
- Release open: focus first missing precondition, otherwise release attestation control.
- Package success: focus package summary heading, with download as next tab stop.

Accessibility notes:

- Use visible focus rings.
- Preserve logical tab order matching visual workflow order.
- Use semantic buttons/links/forms, not div-click handlers for primary actions.
- Announce save failures, validation failures, and release blockers via accessible live regions.

## 7. Dense/no-noise UI guidance

The MVP is for desktop lab workstation use first. It can be responsive, but the critical-path design optimizes for fast high-density operator work.

Do:

- Use compact tables/grids for sample queues and result entry.
- Keep primary action placement consistent: top-right or first queue action, not buried in card footers.
- Show status chips with useful labels: `Received`, `Worksheet ready`, `Needs review`, `Release blocked`, `Released`.
- Show audit/provenance as compact evidence attached to the resource.
- Use sticky headers for worksheet and review grids.
- Use deterministic IDs and labels that make screenshots/demo verification stable.

Do not:

- Add marketing hero sections, decorative empty states, or oversized cards to operator screens.
- Require modal wizards for every step.
- Hide primary actions behind kebab menus.
- Use toast-only errors for validation or protected-operation denial.
- Split the four-analyte MVP result entry across four detail pages.
- Display raw audit JSON by default on operator screens; link to detailed evidence when needed.

## 8. Acceptance checks for MVP-003

MVP-003 is accepted when this spec is linked from the MVP scope and roadmap/README context, and later implementation work can be checked against these hard gates:

- Happy-path critical path is documented at 18 target clicks or fewer and 24 maximum clicks.
- Each stage has a target/maximum click count.
- Keyboard shortcuts are defined for dashboard, intake, worksheet, review, release/package, and submit/save.
- Default focus behavior is defined for page load, forms, validation errors, worksheet grid, review, release, and package success.
- Error states are defined for validation, label failure, result-entry save failure, unauthorized/illegal workflow attempts, release blockers, hash/audit failures, and artifact download failure.
- Dense/no-noise UI guidance is explicit enough to reject noisy dashboard/card/wizard implementations.

## 9. Implementation notes for future UI tasks

- Prefer a single-page flow with queue panels and deep-linkable resources over multi-page wizard sprawl.
- Add `data-testid` or stable semantic labels for critical path controls so the E2E smoke suite can count clicks and verify focus movement.
- Keep the click-budget test fixture tied to the synthetic MVP fixture from `docs/mvp-acceptance-contract-demo-script.md`.
- Do not add a UI dependency just for the grid until native table/input behavior is proven insufficient. If a grid library becomes necessary, justify it against bundle size, keyboard support, accessibility, and maintenance cost.
