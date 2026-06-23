# Project Scientist MVP Verification Suite

Status: lab-test only. This command uses synthetic data and must not be pointed at customer or production databases.

## One-command verification

Run:

```sh
make mvp-verify-suite
```

Equivalent CLI:

```sh
go run ./cmd/project-scientist mvp verify-suite \
  --db data/project-scientist-mvp.db \
  --artifacts artifacts/mvp-verification
```

The suite resets the selected local SQLite database, seeds and runs the MVP vertical slice, then writes a predictable JSON artifact:

```text
artifacts/mvp-verification/mvp-verification-suite.json
```

## Covered happy path

The suite proves this synthetic LIMS lifecycle:

1. Clean reset of the selected local database.
2. Seed tenant/client/contact/project/catalog/sample-reference data.
3. Receive/accession one sample with container and preservation metadata.
4. Generate a sample label artifact.
5. Expand an analysis profile into request lines.
6. Assign the lines to a worksheet.
7. Enter and review results with reviewer separation.
8. Accept the QC batch.
9. Release the sample.
10. Generate and release a COA/report artifact.
11. Verify audit actions exist for protected mutations.

## Negative controls

The suite must return exactly five denied controls:

- `illegal_workflow_jump`: direct sample workflow jump is denied.
- `release_before_preconditions`: sample release before QC/release preconditions is denied.
- `cross_tenant_attempt`: cross-tenant report generation is denied.
- `unauthorized_mutation`: client/contact actor result mutation is denied.
- `mutate_released_artifact`: post-review/released result mutation is denied.

## Pass/fail signal

A passing command prints `mvp verify-suite ok` with sample, worksheet, report artifact, negative-control count, and artifact path. Any missing expected control or write failure returns non-zero.
