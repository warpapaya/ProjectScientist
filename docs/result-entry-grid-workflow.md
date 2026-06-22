# Project Scientist — Result Entry Grid Workflow

Status: lab-test implementation-ready UX/backend contract. This document is a local/synthetic MVP contract only; it does not approve production use, customer data, customer migration, public exposure, or customer-facing readiness claims.

Related docs:

- `docs/senaite-parity-roadmap.md`
- `docs/mvp-critical-path-ux-click-budget.md`
- `docs/mvp-test-scope.md`
- `docs/security-audit-model.md`

## 1. Scope

Build a keyboard-first result entry surface for worksheet lines. The goal is fast analyst entry with explicit save states, validation feedback, dirty-state preservation, and comments without turning the UI into a general spreadsheet clone.

No spreadsheet clone: no formulas, merged cells, arbitrary copy/paste blocks, hidden column scripting, ad-hoc row creation, pivoting, or multi-tab workbook behavior. Rows come from server-owned worksheet lines; fields are domain inputs for lab results.

## 2. Backend contract

The grid is implementation-ready once the worksheet/result backend exposes these lab-scoped routes. Route handlers must use authenticated actor context, tenant/lab scope, authorization, and audit behavior from `docs/security-audit-model.md`.

### 2.1 Load grid

`GET /api/worksheets/{id}/result-entry`

Returns one worksheet with ordered editable lines:

```json
{
  "worksheet": {
    "id": "WS-00001",
    "status": "open",
    "batch_id": "BATCH-2026-001",
    "department_name": "Metals",
    "method_name": "EPA 200.8",
    "analyst_id": "analyst-jane"
  },
  "lines": [
    {
      "analysis_request_line_id": "ARL-00001",
      "sample_id": "SMP-00001",
      "lab_sample_id": "CL-26-0001",
      "client_sample_id": "MW-1",
      "analyte": "Lead",
      "method_name": "EPA 200.8",
      "catalog_snapshot_id": "CAT-SNAP-00001",
      "status": "requested",
      "result": {
        "raw_value": "",
        "value": null,
        "unit": "mg/L",
        "qualifier": "",
        "mdl": "0.0005",
        "rl": "0.0010",
        "loq": "0.0010",
        "dilution": "1",
        "uncertainty": "",
        "comments": ""
      },
      "validation": [],
      "audit_hint": {"last_actor": "", "last_event_id": ""}
    }
  ]
}
```

### 2.2 Save draft

`POST /api/worksheets/{id}/results/draft`

Request body:

```json
{
  "lines": [
    {
      "analysis_request_line_id": "ARL-00001",
      "raw_value": "0.0023",
      "unit": "mg/L",
      "qualifier": "J",
      "mdl": "0.0005",
      "rl": "0.0010",
      "loq": "0.0010",
      "dilution": "1",
      "uncertainty": "0.0002",
      "comments": "Low-level estimated result"
    }
  ],
  "client_revision": "optional-etag-or-row-version"
}
```

Response body returns per-line state, not just global success:

```json
{
  "worksheet_id": "WS-00001",
  "save_state": "saved",
  "lines": [
    {
      "analysis_request_line_id": "ARL-00001",
      "state": "saved",
      "validation": [],
      "audit_event_id": "AUD-00042",
      "server_revision": "rev-42"
    }
  ]
}
```

Draft saves may persist incomplete rows. They must audit allowed changes as `result.draft_saved` or equivalent safe result-update action without marking the worksheet complete.

### 2.3 Complete entry

`POST /api/worksheets/{id}/results/complete`

Completes analyst entry for valid rows and transitions eligible worksheet/line state toward review. It must reject missing required values, invalid numeric fields, unauthorized actors, wrong tenant/lab scope, locked/released lines, and stale revisions.

Hard-fail response shape:

```json
{
  "save_state": "invalid",
  "message": "3 rows need attention before result entry can be completed",
  "lines": [
    {
      "analysis_request_line_id": "ARL-00002",
      "state": "invalid",
      "validation": [{"field": "raw_value", "severity": "error", "message": "Result value is required"}]
    }
  ]
}
```

Denied protected operations must be audited with outcome `denied` and safe details only.

## 3. Grid columns

Required columns, left to right:

1. Sample: lab sample id with client sample id secondary.
2. Analyte.
3. Result value.
4. Unit.
5. Qualifier.
6. MDL/RL/LOQ limit hint.
7. Dilution.
8. Uncertainty.
9. Comments.
10. Status/save state.
11. Audit hint.

The dense default hides no required data but keeps metadata visually quieter than editable cells. Repeated sample metadata can collapse into a sticky group label if multiple analytes belong to one sample.

## 4. Keyboard behavior

Opening a worksheet focuses the first empty editable result value cell.

- Enter commits the current cell and advances to the next required value cell.
- Shift+Enter moves to the previous editable result cell.
- Tab moves to the next editable field in the same row, then the next row.
- Arrow keys move cell focus without opening selects or dialogs.
- `Ctrl+Enter` / `Cmd+Enter` submits complete-entry when all required rows are valid.
- `Esc` exits an active cell editor without discarding already saved server state.
- Qualifier accepts direct tokens such as `ND`, `J`, `U`, `H`; dropdown-only qualifier entry is not allowed.
- Comments opens as an inline field or compact popover reachable by keyboard, with visible text preview after save.

## 5. Save states and dirty handling

Each editable cell and row has a visible state:

- `clean`: server value and local value match.
- `dirty`: user changed a value locally and it is not persisted.
- `saving`: save request is in flight.
- `saved`: server acknowledged the latest value.
- `invalid`: server or client validation blocks save/complete.
- `conflict`: server revision changed; user must reload or merge manually.
- `blocked`: authorization, workflow, lock, or tenant/lab scope prevents mutation.

Dirty values must remain visible after a network/API failure. The UI must not clear a row or navigate away silently. The primary error region uses `aria-live="polite"` for normal validation and `aria-live="assertive"` for blocked/denied completion.

Draft-save policy:

- Blur or row navigation may trigger a debounced draft save.
- Complete-entry uses an explicit button or `Ctrl+Enter` / `Cmd+Enter`; no automatic completion.
- The page shows unsaved-row count near the toolbar.
- Browser unload/navigation while dirty shows a native confirmation.

## 6. Validation feedback

Validation is in-cell first, then summarized at the toolbar.

Client-side prechecks:

- Required result value before completion.
- Numeric fields parse for `raw_value` when no non-detect qualifier is present.
- MDL/RL/LOQ/dilution/uncertainty parse as positive decimal values when populated.
- Qualifier token is known for the lab-test catalog.
- Unit is either defaulted from the catalog snapshot or explicitly selected.

Server-side authority:

- Tenant/lab scope, actor role, worksheet status, line lock/release state, and catalog snapshot validity.
- Server returns per-field errors; UI focuses the first invalid cell after failed completion.
- Warnings such as out-of-limit, estimated result, or qualifier-required remain warnings until a later QC/review rule promotes them to blockers.

## 7. Comments

Comments are a first-class result field, not an audit substitute.

- Empty comments show a quiet `Add comment` affordance.
- Existing comments show the first line inline and full text on focus/click.
- Comments save with the same draft/complete result payload as the row.
- Audit details may include `comment_changed: true` but must not rely on comments to explain protected-operation authorization decisions.

## 8. Toolbar and workflow actions

Toolbar contents:

- Worksheet id, batch, method, department, analyst.
- Unsaved count and invalid count.
- `Save draft` secondary action.
- `Complete entry` primary action.
- `Back to worksheet queue` secondary navigation.

Empty state: `No editable result lines on this worksheet. Return to worksheet queue.`

Blocked state examples:

- `Worksheet is completed; reopen/amend path required before edits.`
- `You can view this worksheet but analyst role is required for result entry.`
- `This worksheet belongs to another lab scope.`

## 9. Smoke checklist

A local HTTP smoke for this feature should prove:

1. Seed/create a worksheet with 2–4 analysis lines.
2. Load `GET /api/worksheets/{id}/result-entry` and verify ordered rows and defaults.
3. Save one draft row and observe row state `saved` plus audit event.
4. Attempt complete with one missing result and receive per-cell `invalid` feedback.
5. Complete all valid rows and verify worksheet/line status moves to review-ready state.
6. Attempt unauthorized or cross-scope edit and verify HTTP denial plus denied audit event.

## 10. Acceptance mapping

- Keyboard-first entry: defined in section 4 and constrained to real worksheet/result fields.
- Save states: defined in section 5 with per-row server response contract.
- Validation feedback: defined in section 6 with client prechecks and server authority.
- Dirty-state handling: defined in section 5, including network failure and unload behavior.
- Comments: defined in section 7 as a result field.
- Tied to backend: sections 2 and 9 define route contracts and smoke proof against worksheet/result APIs.
- Not overbuilt: section 1 explicitly excludes spreadsheet-clone features.
