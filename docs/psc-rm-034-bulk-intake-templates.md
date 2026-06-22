# PSC-RM-034 Bulk Intake and Repeatable Templates

Scope: lab-test lane only. This workflow is for synthetic/local Project Scientist validation and makes no customer-facing or production mutation claims.

## Fewer-click workflow target

Repeated monthly/weekly client intake should be driven by a saved intake template instead of re-entering client/project/matrix/container/analysis fields for every sample.

Target path for a 3-sample recurring project:

1. Select saved template: 1 click.
2. Paste/type row identifiers: client sample IDs and optional lab sample IDs/comments for each bottle: 1 field group per row.
3. Submit bulk intake: 1 click.
4. Review created accession rows/labels: existing sample list.

Click budget:

- Current v2 single-sample intake: choose client, project, references, profile/services, priority, identifiers, submit per sample. For three samples this repeats shared fields three times.
- Template-driven intake: shared client/project/sample-reference/analysis selections are captured once in `sample_intake_templates`; each repeated accession only needs row-specific IDs/comments/dates/overrides.
- Expected reduction for 3 samples: from roughly 30+ selection/input actions to template selection + 6 required row identifiers + submit.

## Implemented backend contract

### Create template

`POST /api/sample-intake-templates`

Form fields mirror `/api/samples` shared fields:

- `name` required
- `client_id` required
- `project_id` or `project`
- `matrix` or `matrix_reference_id`
- optional `container_id`, `preservative_id`, `storage_location_id`, `received_condition_id`
- optional `priority`
- `analysis_profile_ids`, `analysis_service_ids`, or fallback `tests`

The store validates the client scope, project/reference IDs, and analysis selection before saving the template.

### Bulk create from template

`POST /api/sample-intake-templates/{template_id}/samples`

Preferred JSON body:

```json
[
  {"client_sample_id":"FIELD-001","lab_sample_id":"PSL-001","comments":"north tap"},
  {"client_sample_id":"FIELD-002","lab_sample_id":"PSL-002","comments":"south tap"}
]
```

Each row inherits template client/project/matrix/reference/priority/analysis fields and may override row-specific or shared sample fields when needed. Existing sample uniqueness checks still apply to client sample ID per client and lab sample ID per lab.

Form fallback for simple low-click HTML posts:

- `client_sample_ids` comma/newline-separated
- `lab_sample_ids` comma/newline-separated, matched by index

## Security/audit notes

- Uses existing `OperationSampleIntake` authorization gate.
- Template create emits `sample_intake_template.created` audit event.
- Bulk-created samples use the existing `sample.created` audit event per sample.
- Scope checks are tenant/lab-aware and reject clients outside the requested scope.
- No customer/prod data path was touched.

## Verification

Tests added:

- `internal/lab/sample_intake_template_test.go` validates template creation and 3-row bulk intake through the store.
- `cmd/project-scientist/sample_intake_v2_http_test.go` validates HTTP template creation and JSON bulk creation.
