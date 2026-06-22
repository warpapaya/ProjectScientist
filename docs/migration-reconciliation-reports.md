# Migration Reconciliation Reports

Status: lab-test lane only. These reports are validation artifacts for synthetic/non-customer migration exercises; they are not customer cutover approval or production migration evidence by themselves.

Project Scientist reconciliation reports compare a completed import result against the objects currently present in the Project Scientist store.

## Scope in PSC-RM-072

The first supported entity is `clients`, matching the import/export framework slice. A report records:

- Source provenance: source filename/label, entity, format, generator actor, generated timestamp.
- Counts: source rows, imported rows, matched rows, missing rows, extra rows, mismatched rows.
- Hashes: deterministic SHA-256 digests for canonicalized source rows and imported objects.
- Row findings:
  - missing: an import result references an object ID that is no longer present.
  - extra: an object exists in Project Scientist but was not produced by the import result.
  - mismatched: source row fields differ from the imported object.
- Audit provenance: an `import.reconciliation_reported` audit event with report counts and source/imported hashes.

## Readable artifact

`ImportReconciliationReport.Markdown()` renders a human-readable report with summary counts, hashes, audit event id, and detailed missing/extra/mismatched sections. The markdown output is the report artifact for lab-test reconciliation review.

## Dependency note

PSC-RM-072 uses only Go standard library hashing/encoding. No new runtime dependency is required.
