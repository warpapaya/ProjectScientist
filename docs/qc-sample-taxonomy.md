# QC Sample Taxonomy and Relationships

Status: lab-test domain specification for Project Scientist. This is not a customer-facing claim, production approval, or full SENAITE migration promise.

## Scope

PSC-RM-050 defines the first production-shaped QC vocabulary and relationship model needed before QC limits, batch acceptance, review blocking, and report/COA surfacing can be implemented.

The model intentionally separates:

1. QC sample taxonomy: what kind of QC artifact/sample this is.
2. Relationship type: why the QC sample is associated with a client sample, batch, method, or analysis line.
3. Future QC rules: acceptance windows, RPD/recovery calculations, and review decisions. Those remain downstream tasks.

## Taxonomy

| Kind | Label | Primary purpose | Relationship expectation |
| --- | --- | --- | --- |
| `method_blank` | Method blank | Reagent/process blank carried through a method batch to prove preparation/analysis contamination was not introduced. | Optional batch or method-control relationship. |
| `trip_blank` | Trip blank | Blank transported with field samples to assess shipment/field transport contamination. | Optional batch relationship. |
| `equipment_blank` | Equipment blank | Blank passed over/through sampling equipment to assess field/equipment contamination. | Optional batch relationship. |
| `field_duplicate` | Field duplicate | Independently collected duplicate of a client sample used to assess field and analytical precision. | Required `duplicate_of` relationship to a client sample/line. |
| `lab_duplicate` | Laboratory duplicate | Duplicate aliquot/preparation of a client sample used to assess lab precision. | Required `duplicate_of` relationship to a client sample/line. |
| `matrix_spike` | Matrix spike | Client matrix fortified with target analytes to assess matrix-specific recovery. | Required `spike_of` relationship to the source client sample/line. |
| `matrix_spike_duplicate` | Matrix spike duplicate | Second fortified aliquot used with the MS for recovery and precision. | Required `spike_of` and/or `duplicate_of` relationship. |
| `laboratory_control_sample` | Laboratory control sample (LCS) | Known clean/reference matrix fortified independently of client matrix to assess method performance. | Optional method-control or batch relationship. |
| `control_sample` | Control sample | Generic positive/negative control or certified/reference sample associated with a method or batch. | Optional method-control or batch relationship. |
| `initial_calibration_verification` | ICV | Independent standard confirming the initial calibration before sample analysis. | Required method-calibration relationship. |
| `continuing_calibration_verification` | CCV | Continuing standard confirming instrument calibration remains acceptable during a run. | Required method-calibration relationship. |

## Relationship types

| Type | Meaning | Typical QC kinds |
| --- | --- | --- |
| `batch_control` | QC sample applies to a preparation/analytical batch. | Blanks, LCS, control samples. |
| `duplicate_of` | QC sample duplicates a client sample or another QC aliquot. | Field/lab duplicates, MSD. |
| `spike_of` | QC sample is fortified from a source sample/matrix. | MS/MSD. |
| `control_for_method` | QC sample verifies method performance independent of a specific client sample. | LCS, control samples, method blanks. |
| `calibration_for_method` | QC sample/standard verifies initial or continuing calibration for a method/run. | ICV, CCV. |

## Persisted relationship contract

`qc_sample_relationships` stores tenant/lab scoped links with:

- `qc_sample_id`: the sample record representing the QC artifact.
- `qc_sample_kind`: controlled taxonomy value above.
- `relationship_type`: controlled relationship value above.
- `related_sample_id`: optional client/source sample.
- `method_id`: optional catalog method link.
- `analysis_request_line_id`: optional analysis line link for analyte/method-specific QC traceability.
- `batch_id`: optional future batch/worksheet/run identifier.
- `notes`: minimal non-secret lab note.

Validation rules implemented in lab-test v1:

1. QC kind must be in the controlled taxonomy.
2. Relationship type must be allowed for that QC kind.
3. Required-relationship kinds must have at least one sample, method, line, or batch link.
4. QC sample, related sample, method, and analysis request line must be in the requested tenant/lab scope.
5. If `analysis_request_line_id` and `related_sample_id` are both supplied, the line must belong to the related sample.
6. Creating a relationship is protected by `qc.relate`, allowed to admin, lab manager, analyst, and reviewer roles.
7. Creation emits an allowed `qc_sample.relationship.created` audit event with identifiers only, not full sample/result payloads.

## SENAITE parity mapping

SENAITE commonly models QC through sample types, analysis/request relationships, worksheets/batches, methods, instruments, and result interpretation rules. Project Scientist's lab-test v1 does not clone SENAITE object internals; it preserves the semantics required for downstream parity:

- QC samples are real sample records with explicit type metadata in the relationship table.
- Client-sample linkage is explicit instead of implied by naming conventions.
- Method linkage is explicit and points to versioned catalog methods.
- Analysis-line linkage allows future analyte-specific recovery/RPD/blank checks.
- Batch linkage is a string placeholder until worksheet/batch entities are promoted into the domain model.

## QC batch acceptance model

PSC-RM-051 promotes QC batch composition into explicit scoped objects:

- `qc_batches`: method/matrix batch header with status `open`, `in_review`, `accepted`, or `rejected`.
- `qc_items`: ordered batch membership for production/client samples and QC samples, with QC items carrying a controlled taxonomy kind.
- `qc_relationships`: links a QC item to an affected production sample and/or analysis request line using the controlled relationship types above.

Lab-test v1 workflow rules:

1. New batches start `open`.
2. Items and relationships can be added only while the batch is `open`.
3. `open` batches may move to `in_review` only after at least one QC item has a relationship.
4. `in_review` batches may move to `accepted` or `rejected`; both terminal decisions require a reason.
5. Rejected batches may be reopened for correction; accepted batches are terminal in v1.
6. Creation, composition, relationship, and status changes emit append-only audit events with identifiers only.

## Downstream implementation tasks

1. Replace any remaining free-form `batch_id` compatibility fields with the scoped QC batch identifier where callers can provide it safely.
2. Add QC limit/rule definitions by method, matrix, analyte, and QC kind.
3. Compute blank contamination, duplicate RPD, spike/LCS recovery, and calibration pass/fail flags.
4. Block report release when required QC relationships/rules fail or are unreviewed.
5. Surface QC summaries and exceptions in result review and COA/report artifact generation.
