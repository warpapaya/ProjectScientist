# Project Scientist — Golden Migration Datasets

Status: lab-test fixture contract. Synthetic data only. Not production configuration, not customer data, not customer migration approval, and not a customer-facing readiness claim.

Primary machine-readable fixture:

- `fixtures/golden_migration_dataset.json`
- Dataset id: `psc-rm-073-golden-migration-v1`

## Purpose

PSC-RM-073 adds a synthetic golden dataset pack for migration and SENAITE parity tests. The pack gives the migration lane realistic workflow pressure without using customer records, customer identifiers, customer reports, or customer limits.

The fixture has three workflow families that emulate common Clearline/SENAITE patterns at the shape level only:

1. `precast-industrial` — industrial/precast-style sample intake, multi-container chemistry, custody appendix, batch QC, COA.
2. `municipal-water` — municipal water-style samples, holding-time checks, microbiology placeholder, EDD-style export.
3. `materials-forensics` — materials review-style filter/bulk samples, qualitative findings, subcontract flags, narrative report package.

The fixture intentionally does not use real customer names, domains, addresses, sample IDs, contacts, limits, reports, or credentials. All legacy ids are `SYN-*`; all emails use `.example.test`.

## What is committed

- Three synthetic clients, one per workflow family.
- Three synthetic samples with matrices, containers, preservation, received-condition, analysis lists, and custody event expectations.
- Seven result-like analysis rows across field/wet chemistry/metals/microbiology-placeholder/narrative domains.
- One QC batch expectation per workflow family.
- One report/package expectation per workflow family.
- SENAITE concept mapping for client/contact/sample/AR/service/worksheet/QC/report/COC.
- Expected parity gaps that should stay visible until migration/report code supports those domains.
- Executable migration checks tested by `internal/lab/golden_migration_dataset_test.go`.

## Current executable coverage

`internal/lab/golden_migration_dataset_test.go` verifies:

- the fixture is explicitly lab-test and synthetic-only;
- all three workflow families are present with executable workflow steps;
- the fixture contains no forbidden customer-sensitive identifiers;
- client rows can be transformed into current `ImportForScope` JSON client import rows;
- client import creates three records and `ClientImportReconciliationReportForScope` reports a clean reconciliation;
- sample/custody rows derived from the fixture import through `ImportForScope` with `ImportEntitySamples`, preserving client sample ids, lab ids, matrices, containers, preservation, received-condition, analysis expectations, and custody event reasons/sequences;
- `SampleImportReconciliationReportForScope` emits clean hash-backed reconciliation and audit evidence for the imported synthetic sample/custody rows;
- required SENAITE mapping keys are present;
- parity gaps and migration checks are documented;
- this document describes the dataset and parity gaps.

## Expected parity gaps

These are deliberate testable gaps, not failures of the fixture:

| Gap | Severity | Meaning |
| --- | --- | --- |
| `gap-contact-role-import` | medium | Current generic import handles client name/email only; contact/report-recipient roles are not migrated yet. |
| `gap-sample-custody-import` | remediated for lab-test fixture import | Synthetic sample matrix/container/preservation/received-condition/custody expectations now import and reconcile locally; this is not production/customer migration approval. |
| `gap-analysis-result-import` | high | Analysis services, result values, qualifiers, and method snapshots are not migrated by `ImportForScope` yet. |
| `gap-qc-batch-model` | high | QC batch expectations exist, but production-grade QC evaluation is not implemented. |
| `gap-report-package-artifacts` | high | COA/EDD/narrative/custody/manifest artifacts are not generated from migrated data yet. |
| `gap-subcontract-and-holding-time-controls` | medium | Subcontract-blocking, microbiology holding-time, and release-precondition controls are not enforced yet. |

## SENAITE parity boundary

The fixture maps SENAITE concepts at the data-contract level:

- `Client` → organization/account boundary.
- `Contact` → report recipient/contact role.
- `Sample` → specimen metadata plus matrix/container/preservation.
- `AnalysisRequest` → requested services per sample.
- `AnalysisService` → method/analyte/unit/qualifier/result snapshot.
- `Worksheet` → method/batch/analyst work queue.
- `QC` → method blanks, LCS, duplicates, holding-time, subcontract, and reviewer acknowledgement controls.
- `Report` → COA, EDD, narrative, custody appendix, and attachment manifest outputs.
- `ChainOfCustody` → custody event timeline with actor/timestamp expectations.

## Safety use

Use this dataset for local tests, migration contract tests, and roadmap gap tests only. It does not authorize production mutation, customer data import, customer demonstrations, or claims that Project Scientist has reached SENAITE parity.
