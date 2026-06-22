# Project Scientist — SENAITE Parity Field Mapping and Migration Spec

Status: lab-test/internal architecture artifact. This is not a customer migration plan, customer-facing claim, or approval to use Project Scientist with production/customer data.

Repository: https://github.com/warpapaya/ProjectScientist
Related artifacts:
- `docs/senaite-parity-roadmap.md`
- `docs/security-audit-model.md`
- PSC-001 durable architecture artifact: `Project Scientist/PSC-001 - SENAITE Parity Map and Domain Model.md`

Safety boundaries:
- Lab-test lane only.
- No customer/prod systems were accessed or mutated to produce this spec.
- Customer-sensitive values are intentionally replaced with neutral examples or conceptual field names.
- SENAITE/Bika/Plone vocabulary appears here only as internal implementation and migration context.

## 1. Purpose

This document turns the accepted SENAITE parity/domain model into a field-level migration map for Project Scientist. It answers three questions:

1. Which current Clearline/SENAITE concepts must Project Scientist preserve?
2. Where do those concepts land in the Project Scientist domain model?
3. Which fields are already present in the bootstrap prototype vs. missing/gapped before migration tooling is credible?

The mapping is intentionally conservative. When SENAITE data can be reconstructed into native relational Project Scientist objects, import it that way. When historical meaning is ambiguous or overly expensive to reconstruct, preserve the source object as an auditable `HistoricalSnapshot` plus searchable legacy identifiers and immutable artifacts.

## 2. Current Project Scientist bootstrap field baseline

Observed from the local Go implementation at the time this spec was written.

### `Client`

| Bootstrap field | Current meaning | Parity note |
| --- | --- | --- |
| `id` | Generated local client id like `C-00001` | Not enough for migrated customer identity. Needs stable internal id plus legacy ids. |
| `name` | Client display name | Keep, but split legal/display/report names later if needed. |
| `email` | Single contact-ish email | Too flat. Replace/augment with `Contact` and `OrganizationContactRole`. |
| `created_at` | Local creation timestamp | For migration, distinguish Project Scientist import timestamp from source-created timestamp. |

### `Sample`

| Bootstrap field | Current meaning | Parity note |
| --- | --- | --- |
| `id` | Generated local sample id like `S-000001` | Needs accession sequence strategy, client sample id, barcode, and legacy AR/sample ids. |
| `client_id` | Link to bootstrap `Client` | Eventually link to organization/site/project/work order with tenant/lab scope. |
| `project` | Free-text project/work grouping | Needs first-class `Project` / `WorkOrder` entity. |
| `matrix` | Free-text matrix/sample type | Needs controlled/versioned matrix/sample-type catalog with legacy display value preserved. |
| `status` | Simple sample workflow: `received -> in_prep -> in_analysis -> in_review -> released` | Needs entity-specific workflow states and guard conditions. Current direct release path is blocked but release prerequisites are not lab-grade. |
| `analyses` | Inline list of requested tests/results | Must split into catalog services, analysis request lines, results, and result versions. |
| `created_at`, `updated_at` | Local timestamps | For migration, preserve source timestamps separately and mark them source-asserted. |

### `Analysis`

| Bootstrap field | Current meaning | Parity note |
| --- | --- | --- |
| `id` | Generated under sample id, e.g. `S-000001-A01` | Needs stable `AnalysisRequestLine` id plus source analysis UID/path/id. |
| `name` | Free-text requested test name | Needs catalog reference and legacy display text. |
| `method` | Optional method text | Needs versioned `Method` entity and method snapshot at result/report time. |
| `result` | Optional result string | Needs typed result, raw/display values, qualifiers, limits, dilution, uncertainty, version history. |
| `units` | Optional unit text | Needs controlled `Unit` catalog plus legacy text escape hatch. |

### `AuditEvent`

| Bootstrap field | Current meaning | Parity note |
| --- | --- | --- |
| `sequence` | Local monotonic sequence | Needs tenant/lab scoped sequence allocation and verification. |
| `timestamp` | Local event time | Migration must include both import time and source historical/source-asserted time. |
| `actor` | Caller-supplied actor string | Must be replaced by authenticated actor context before protected/customer data. |
| `action` | Event name like `sample.created` | Keep event vocabulary but expand to command/outcome model. |
| `entity_type`, `entity_id` | Target resource | Needs version/resource scope and tenant/lab id. |
| `details` | Minimal event metadata | Keep minimized; no full sensitive payloads by default. |
| `previous_hash`, `hash` | Local hash chain | Needs canonical schema, startup verifier, checkpoints, and backup/restore proof. |

## 3. Canonical migration envelope

Every imported object should carry a consistent migration envelope, either directly or through linked `LegacyReference` / `MigrationBatch` records.

| Field | Target model | Required | Notes |
| --- | --- | --- | --- |
| Source system | `MigrationBatch.source_system` / `LegacyReference.system` | Yes | Use `senaite` for SENAITE-origin records. |
| Source site/base URL alias | `MigrationBatch.source_site_alias` | Yes | Store sanitized site alias, not credentialed URL. |
| Source portal path | `LegacyReference.path` | Strongly recommended | Useful for traceability and old links. Sanitize host-specific secrets. |
| Source UID | `LegacyReference.uid` | Strongly recommended | Primary stable SENAITE object identity when available. |
| Source object id | `LegacyReference.source_id` | Yes | Human-searchable legacy id/object id. |
| Source title | entity display field + optional legacy display | Yes where available | Preserve exactly enough for search/reconciliation. |
| Source modified/created timestamps | entity `source_created_at` / `source_modified_at` or snapshot metadata | Yes where available | Mark source-asserted; do not treat as Project Scientist action time. |
| Import timestamp | `MigrationBatch.imported_at`, `LegacyReference.migrated_at` | Yes | Project Scientist system time. |
| Source checksum | `MigrationBatch.source_checksum` / per-record checksum | Yes | Hash canonical exported payload to prove reconciliation. |
| Mapping version | `MigrationBatch.mapping_version` | Yes | This document version or future schema version. |
| Import actor | `AuditEvent.actor_context` | Yes | Migration service account, not a human analyst unless manual import. |
| Import outcome | `ImportJob` / `MigrationBatch` | Yes | Accepted/rejected/warn counts, validation errors. |
| Historical payload snapshot | `HistoricalSnapshot.payload_ref` | Conditional | Use when full relational mapping is unsafe/incomplete. |

## 4. Clients, contacts, sites, projects

### Field map

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| Client organization/account | `Organization` | Create one organization per source client. Preserve legacy client id/path/UID. | Partially present as flat `Client`. | Need tenant/lab id, legal/display names, status, billing/reporting distinctions. |
| Client title/name | `Organization.display_name`; optional `legal_name` | Trim only for validation; preserve legacy display text. | `Client.name`. | Need alternate/reporting names. |
| Client email | `Contact` + `OrganizationContactRole` | If only one email exists, create contact role `general_recipient` or keep as org email until contact model exists. | `Client.email`. | Need role semantics; avoid pretending one email is all contacts. |
| Client contacts | `Contact` | One record per source contact/person. | Missing. | Need source field inventory for contact names, email, phone, active state. |
| Contact-to-client role | `OrganizationContactRole` | Map roles: submitter, report recipient, COC signer, billing, project manager, approver. | Missing. | Need authoritative SENAITE role fields and Clearline extension fields. |
| Client site/division/facility | `OrganizationSite` / `ClientDivision` or scoped `Project` metadata | Preserve name/address/code; link to organization. | Missing. | Clearline-specific division semantics need fixture coverage. |
| Project/work order/PO/program | `Project` / `WorkOrder` | Create durable grouping. Keep PO/contract/program as non-secret metadata. | `Sample.project` free text only. | Need separate project ids and client-specific defaults. |
| Client report defaults | `ClientConfiguration` | Store report template, paper/orientation, EDD/export, label defaults by version. | Missing. | Existing customer-specific values must be sanitized in test fixtures. |
| Client service/profile defaults | `ClientConfiguration` + catalog relationships | Store as versioned defaults. | Missing. | Need mapping from SENAITE profiles/services and Clearline overlays. |

### Migration rules

- Do not flatten contacts into the organization long-term. The bootstrap `Client.email` is a temporary display/communication shortcut only.
- Preserve inactive/archived clients if historical samples/reports reference them.
- If a contact cannot be confidently de-duplicated, import separate contacts and flag duplicates for reconciliation rather than merging silently.

## 5. Samples, Analysis Requests, containers, custody intake

### Field map

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| AnalysisRequest / AR | `SampleRequest` plus `AnalysisRequestLine`; legacy AR compatibility reference | Preserve AR id/path/UID. Link to physical `Sample`. | Collapsed into `Sample` with inline analyses. | Need explicit `SampleRequest` model before migration importer. |
| Physical sample/specimen | `Sample` | One physical specimen per source sample/AR unless evidence shows AR-only semantics. | Present minimally. | Need split rules for sample vs AR when SENAITE object stores both. |
| Lab sample id / accession id | `SampleIdentifier(type=lab_accession)` | Preserve original; generate Project Scientist id separately. | `Sample.id` generated local id. | Need accession sequence/scoping policy. |
| Client sample id | `SampleIdentifier(type=client_sample_id)` | Preserve exact submitted value; allow duplicates only under configured scope if lab allows. | Missing. | Need uniqueness rules by client/project/date. |
| External/legacy IDs | `LegacyReference` + `SampleIdentifier(type=legacy)` | Store source UID/path/id and old AR/sample labels. | Missing. | Required for search/reconciliation. |
| Matrix/sample type | `SampleCollection.matrix_id` + legacy display | Map to controlled matrix catalog; preserve unmapped text. | `Sample.matrix` free text. | Need matrix normalization table. |
| Sampled date/time | `SampleCollection.sampled_at` | Import source timestamp if present; source-asserted. | Missing. | Need timezone/partial-date policy. |
| Received date/time | `CustodyEvent(received)` + `Sample.received_at` | Import as custody receipt event where possible. | Implicit `created_at`. | Need distinguish intake creation vs receipt. |
| Sampler | `SampleCollection.sampler_contact_id` or text | Link contact if known; else source text. | Missing. | Need external sampler handling. |
| Sample point/location | `SampleCollection.sample_point` / `OrganizationSite` | Use structured site if known; else legacy text. | Missing. | Need reusable location model. |
| Priority/TAT | `SampleRequest.priority` / due date | Map to controlled priority plus due date. | Missing. | Need TAT rules and client defaults. |
| Remarks/comments | `SampleNote` / sanitized migration note | Preserve source comments only if approved for test data; avoid copying sensitive free text into audit. | Missing. | Need privacy policy. |
| Containers | `Container` | Container type, preservative, volume, condition, barcode. | Missing. | Need container/preservative catalog. |
| Partitions/aliquots | `SamplePartition` / `Aliquot` | Create per department/method/container where source supports it. | Missing. | Need SENAITE partition extraction strategy. |
| Receipt/cooler/batch | `ReceiptBatch` / `SampleBatch` | Group samples by login/cooler/batch identifiers. | Missing. | Need batch semantics. |
| COC uploaded PDF/document | `CustodyDocument` / `Artifact` | Store artifact checksum/page count/source; link to samples. | Missing. | Need attachment extraction and storage path policy. |
| Custody transfer events | `CustodyEvent` | Import structured events; snapshot unstructured PDFs. | Missing. | SENAITE may not have complete structured custody history. |

### Migration rules

- Project Scientist must not treat SENAITE AR id as the only physical sample id. Keep native id, lab accession id, client sample id, and legacy reference separate.
- If SENAITE source conflates sample and AR, import a `Sample` plus one `SampleRequest` and preserve the AR compatibility identity.
- Unknown/partial dates should remain partial/source-asserted rather than being rounded to midnight without a flag.
- Custody PDFs without structured event data should be imported as `CustodyDocument`/`HistoricalSnapshot`, not converted into fake event timelines.

## 6. Services, profiles, methods, analyses, results

### Catalog field map

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| AnalysisService | `TestCatalogItem` + `TestDefinition` | Preserve service UID/id/title; map analyte/method/unit/category. | Inline `Analysis.name` only. | Need catalog schema and versioning. |
| AnalysisProfile | `TestCatalogItem(type=profile)` + profile members | Preserve profile id/title and member service versions. | Missing. | Need profile expansion rules at order time. |
| Method | `Method` | Versioned method/SOP/accreditation scope. | `Analysis.method` free text optional. | Need source method inventory and version policy. |
| Analyte/parameter | `Analyte` | Normalize analyte identity; preserve display name. | Not separate. | Need synonym/cas/regulatory fields if relevant. |
| Unit | `Unit` | Normalize canonical unit; preserve source display if unmapped. | `Analysis.units` free text. | Need controlled unit table. |
| Category/department | `Department` / catalog category | Drives queues/worksheets. | Missing. | Need SENAITE category mapping. |
| Calculation/dependency | `CalculationDefinition` / derived result rule | Import only explicit known rules; otherwise flag. | Missing. | High-risk area; do not infer silently. |
| Spec/limit | `SpecificationLimit` | Version by client/matrix/method/analyte/regulation. | Missing. | Need authoritative source for limits. |

### Transactional analysis/result field map

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| Analysis object on AR | `AnalysisRequestLine` | Create line from service/profile expansion; keep source analysis UID/path/id. | Inline `Sample.Analyses[]`. | Need separate line table. |
| Requested service name | `AnalysisRequestLine.ordered_display_name` + catalog ref | Catalog ref if mapped; else legacy display with mapping warning. | `Analysis.name`. | Need mapping profile. |
| Result value | `Result.value_raw`, `Result.value_display`, typed numeric where safe | Preserve display exactly; parse numeric only with validation. | `Analysis.result` string. | Need typed result model and flags. |
| Result unit | `Result.unit_id` + legacy unit text | Map normalized unit; preserve unmapped source. | `Analysis.units`. | Need unit mapping. |
| Qualifier | `Result.qualifier_id` / `ResultQualifier` | Map controlled qualifier such as ND/J/U/etc where known. | Missing. | Need qualifier inventory per lab/report style. |
| MDL/RL/LOQ/detection limits | `Result.detection_limit`, `reporting_limit`, `loq` | Import as versioned numeric fields with unit. | Missing. | Need source fields and units. |
| Dilution/prep factor | `Result.dilution_factor` / prep metadata | Import if source has it. | Missing. | Need workflow/instrument fixture. |
| Analyst | `Result.analyst_actor_id` or legacy analyst text | Link user if migrated; else source text. | Missing. | Actor identity mapping required. |
| Instrument/run | `InstrumentRun` + `Result.instrument_run_id` | Import if available; otherwise unknown. | Missing. | Instrument lane unspecified. |
| Analysis status/review | `AnalysisRequestLine.workflow_instance` / `Result.status` | Map source workflow state to analysis workflow. | Only sample status exists. | Need status crosswalk. |
| Result version/history | `ResultVersion` | Import historical versions if extractable; else current plus historical snapshot. | Missing. | SENAITE history extraction unknown. |
| Reportable flag | `AnalysisRequestLine.reportable` / `Result.reportable` | Preserve source hidden/cancelled/retracted states. | Missing. | Need canceled/retracted rules. |

### Migration rules

- Never treat free-text test names as the long-term catalog. Free text is a migration fallback with a warning.
- Parse numeric results only when unit, qualifier, and decimal semantics are safe; otherwise preserve display string and flag for review.
- Profile expansion must preserve the source profile membership/version used at the time of ordering, not the current profile definition.

## 7. Worksheets and bench workflow

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| Worksheet template | `WorksheetTemplate` | Preserve template id/name/method/category/capacity/QC slot rules. | Missing. | Need source template extraction. |
| Worksheet | `Worksheet` | Concrete bench run with assigned analyst/department/instrument/due date. | Missing. | Need workflow state map. |
| Worksheet slots | `WorksheetSlot` | Slot position, production sample analysis, QC item, blank/spike/duplicate/control. | Missing. | Need slot semantics and ordering. |
| Work assignment | `WorkAssignment` | Link analysis request line/result to worksheet slot. | Missing. | Need line-level identity first. |
| Printable bench sheet | `BenchSheetArtifact` | Generated/exported artifact with checksum/version. | Missing. | Report/export system required. |
| Worksheet result entry state | `Worksheet.workflow_instance` and line/result states | Map planned/assigned/in progress/submitted/reviewed/closed. | Missing. | SENAITE workflow/history extraction unknown. |

Migration rules:

- If worksheet structure is incomplete, preserve worksheet identifiers and source snapshots rather than fabricating slot-level assignments.
- Worksheet migration depends on `AnalysisRequestLine` identities. Do not build worksheet importer against inline bootstrap `Analysis` arrays.

## 8. QC and QA

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| QC sample type | `QCType` / `QCItem` | Map blank, duplicate, spike, LCS, MS/MSD, surrogate, control, calibration check. | Missing. | Need lab/method-specific taxonomy. |
| QC batch | `QCBatch` | Scope by method/prep/run/matrix/date as source allows. | Missing. | Need batch identity rules. |
| QC relationship | `QCRelationship` | Link production results/samples to QC item/result. | Missing. | High-risk; do not infer without source evidence. |
| QC limits | `QCLimitSet` | Versioned acceptance criteria by method/matrix/analyte/client. | Missing. | Need authoritative limits source. |
| QC calculations | `QCOutcome` | Recovery/RPD/blank contamination/control flags only when formula and inputs are explicit. | Missing. | Calculation rules unknown. |
| QC narrative/flags | `NarrativeSection` / result flags | Preserve source narrative/qualifiers. | Missing. | Need report package model. |

Migration rules:

- QC is not only sample metadata. It must connect batches, QC items, analytes/results, and affected production samples.
- For migration MVP, acceptable fallback is: import QC items as samples/results plus `HistoricalSnapshot` and explicit `qc_relationship_unknown=true` warnings.
- Automated QC acceptance/rejection must wait for authoritative formulas, limits, and source relationship data.

## 9. Reports, COA, COC packages, labels, artifacts

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| IMPRESS/report template | `ReportTemplate` | Version template id/name/type/paper/orientation/default scope. | Missing. | Need renderer boundary decision. |
| Client default template/paper/orientation | `ClientConfiguration.report_defaults` | Store versioned defaults by organization/site/project when needed. | Missing. | Need customer-specific values sanitized in fixtures. |
| COA/report selection | `ReportPackage` | Selected samples/results/QC/narrative/attachments. | Missing. | Need package builder. |
| Render data | `ReportSnapshot` | Immutable JSON/data snapshot for issued artifact. | Missing. | Non-negotiable before real reporting. |
| PDF/HTML artifact | `ReportArtifact` | Store content hash, media type, source snapshot, generator, template version. | Missing. | Artifact store path/security policy needed. |
| COC attachment/merged package | `COCArtifact` + `ReportArtifact` | Store inbound COC and merged/package artifact separately. | Missing. | Need COC merge rules and attachment discovery. |
| Level 4 narrative/QC package | `ReportPackage` + `NarrativeSection` + EDD export | Preserve narrative sections and QC summaries. | Missing. | Requires QC and export framework. |
| EDD output | `ExportJob` + `ReportArtifact` | Store mapping profile/version/checksum. | Missing. | Need exact specs and validators. |
| Label template | `LabelTemplate` | Version sample/container/work-order label config. | Missing. | Need format/printer routing. |
| Barcode | `Barcode` | Link value/type to sample/container/work order. | Missing. | Need barcode standard/format. |
| Print/reprint event | `PrintJob` + audit | Actor, printer, template, version, reason for reprint. | Missing. | Need client printer workflow. |

Migration rules:

- Released historical reports should be imported as immutable artifacts even when underlying relational reconstruction is incomplete.
- Report regeneration from migrated mutable current data is not equivalent to preserving the issued historical report.
- COC PDFs and report PDFs must carry checksums and source links. Do not store anonymous blobs.

## 10. Audit, workflow, permissions, legacy history

| SENAITE/Clearline concept | Project Scientist target | Transform/default | Current bootstrap status | Gap/unknown |
| --- | --- | --- | --- | --- |
| Object workflow state | `WorkflowInstance.current_state` | Map source states through an explicit crosswalk. | Only `Sample.status`. | Need crosswalk per entity type. |
| Workflow transition history | `WorkflowEvent` + `AuditEvent` | Import if available with source timestamps/actors. | Missing except new local transitions. | Source history completeness unknown. |
| Actor/user | `User` / `ActorContext` / legacy actor text | Link migrated users where possible; preserve source display if not. | Caller-supplied actor string. | Auth/RBAC not implemented. |
| Roles/permissions | `Role`, `Permission`, `ScopeGrant` | Map internal lab roles first; external client visibility default-deny. | Missing. | Need access model before portal/customer data. |
| Audit event | `AuditEvent` | Import as `migration.imported` plus optional historical source event snapshot. | Bootstrap audit only for local actions. | Need canonical schema and privacy rules. |
| Electronic signature/release | `ElectronicSignature` / release workflow event | Preserve source signature/release markers if available. | Missing. | Required before defensible release. |
| Deletion/cancellation/retraction | void/amendment workflow + audit | Preserve source state and reason if available. | Missing. | Need correction policy. |
| Legacy object path/UID/id | `LegacyReference` | Required on every migrated entity and artifact when available. | Missing. | Must be first-class. |

Migration rules:

- Source historical actions are not Project Scientist actions. Store them as source-asserted history or snapshots, and audit the import as a Project Scientist action.
- Permissions default closed. Do not infer client portal visibility from client/contact existence.
- If workflow history is incomplete, keep current state plus `HistoricalSnapshot` and an explicit gap flag.

## 11. Import/export job model

Minimum fields for `MigrationBatch` / `ImportJob`:

| Field | Required | Notes |
| --- | --- | --- |
| `id` | Yes | Stable Project Scientist import batch id. |
| `mapping_version` | Yes | Version of this mapping/crosswalk. |
| `source_system` | Yes | `senaite`. |
| `source_site_alias` | Yes | Sanitized source alias. |
| `source_export_started_at`, `source_export_completed_at` | Yes where available | Source-system timing. |
| `import_started_at`, `import_completed_at` | Yes | Project Scientist timing. |
| `actor_context` | Yes | Migration service account. |
| `dry_run` | Yes | Dry-run imports must not mutate domain state. |
| `input_manifest_checksum` | Yes | Hash of source manifest/export. |
| `object_counts_by_type` | Yes | Expected/accepted/rejected/warn counts. |
| `validation_errors` | Yes | Structured errors; no sensitive payload dumps. |
| `reconciliation_report_artifact_id` | Yes | Human-reviewable proof. |
| `idempotency_key` | Yes | Re-running same batch should not duplicate objects. |
| `rollback_plan_ref` | Conditional | Required before any cutover-like operation. |

## 12. Crosswalks to define before importer code

These crosswalks are required before production code should be written. Use strict TDD for each importer/crosswalk implementation.

| Crosswalk | Needed for | Current status |
| --- | --- | --- |
| SENAITE client/contact fields -> Organization/Contact/Role | Client master data import | Unknown. |
| SENAITE AR/sample state -> Project Scientist Sample/SampleRequest/Analysis states | Sample/AR migration | Unknown. |
| SENAITE sample type/matrix -> controlled Matrix catalog | Sample migration/reporting | Unknown. |
| SENAITE AnalysisService/Profile -> TestCatalogItem/TestDefinition/Profile | Catalog/order migration | Unknown. |
| SENAITE Method -> Method/version/SOP/accreditation fields | Catalog/result/report reproducibility | Unknown. |
| SENAITE units/qualifiers -> Unit/Qualifier | Results/EDD/reporting | Unknown. |
| SENAITE worksheet/template/slot -> WorksheetTemplate/Worksheet/Slot | Bench workflow migration | Unknown. |
| SENAITE QC sample/category/relationship -> QCBatch/QCItem/QCRelationship | QC migration/Level 4 | Unknown/high-risk. |
| SENAITE report template/defaults/artifacts -> ReportTemplate/ReportArtifact | COA/report package migration | Unknown. |
| SENAITE attachments/COC -> Artifact/CustodyDocument/CustodyLink | Custody/report package | Unknown. |
| SENAITE users/roles/workflow actors -> User/Role/ActorContext | Audit/RBAC/history | Unknown. |
| Source object path/UID/id rules -> LegacyReference | All migrations | Required; not implemented. |

## 13. Validation and reconciliation requirements

A migration run is not acceptable without these artifacts:

1. Dry-run report with no domain mutation.
2. Object count reconciliation by source object type and Project Scientist target type.
3. Rejected/warning rows with field-level reasons.
4. Legacy identifier coverage report: percent of migrated entities with UID/path/source id.
5. Required-field coverage report by target model.
6. Duplicate identifier report for client sample ids, accession ids, contacts, and catalog items.
7. Result parsing report: raw/display values vs safely typed numeric values.
8. Unit/qualifier/matrix/method mapping report with unmapped values.
9. Artifact checksum manifest for reports, COCs, labels, EDDs, and snapshots.
10. Audit/provenance report showing `migration.imported` events, source checksums, mapping version, and importer actor.
11. Idempotency proof: re-running the same batch does not create duplicate domain objects.
12. Rollback/reversal plan for lab-test/staging data before any pilot-like run.

## 14. Recommended implementation slices

These are backlog-ready slices, ordered to avoid importer code against unstable models.

1. Add `LegacyReference` and `MigrationBatch` model spec/tests.
2. Add Organization/Site/Contact/ContactRole field model and migration fixture tests.
3. Split bootstrap `Sample` semantics into Sample + SampleRequest + AnalysisRequestLine in design/tests before importer code.
4. Add controlled Matrix/Unit/Qualifier/Method/TestCatalogItem mapping tables and unmapped-value behavior.
5. Define result value parse/preserve rules with fixtures for numeric, ND/qualifier, text, blank, and unit mismatch cases.
6. Define report/artifact snapshot model before importing historical reports/COCs.
7. Define workflow state crosswalks and unknown-history fallback behavior.
8. Build dry-run importer harness with reconciliation artifact generation.
9. Add synthetic SENAITE-like fixture dataset only; no customer data.
10. Add idempotency and checksum tests before enabling non-dry-run imports.

## 15. Explicit unknowns and blockers

- Exact SENAITE export/API shape to use for Project Scientist migration is not selected.
- Source field names for contacts, divisions/sites, worksheets, QC relationships, report artifacts, and attachments need fixture-driven discovery.
- SENAITE workflow history completeness is unknown; current-state-only fallback may be required for some historical records.
- QC relationship extraction is high risk. Do not infer production-sample/QC relationships from names alone.
- Report template/default fields and saved report artifact locations vary by SENAITE/IMPRESS configuration and Clearline overlays.
- Customer-specific EDD formats and Level 4 packages require authoritative specs before mapping code.
- Actor/user identity mapping cannot be finalized until Project Scientist auth/RBAC exists.
- Audit import privacy/minimization rules need implementation gates from `docs/security-audit-model.md`.
- Accession id sequence rules and duplicate client-sample-id scoping need a Petie/Friday architecture decision before importer implementation.
- All test fixtures must remain synthetic or explicitly approved non-sensitive data.

## 16. Stop-lines

Stop and request review before:

- Importing or copying customer/prod data.
- Connecting to live SENAITE, Tindall, CENLA, RJ Lee, or other customer systems.
- Exposing Project Scientist outside local/lab-test infrastructure.
- Presenting this spec as a customer migration commitment.
- Implementing importer code that silently drops legacy IDs, source timestamps, source checksums, or unmapped result/catalog values.
- Treating historical source timestamps as Project Scientist action timestamps.
- Generating reports from mutable migrated state without immutable `ReportSnapshot` / `ReportArtifact` provenance.
