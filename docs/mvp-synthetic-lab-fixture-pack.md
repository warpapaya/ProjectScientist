# Project Scientist — MVP Synthetic Lab Fixture Pack

Status: lab-test fixture contract. Synthetic data only. This is not customer data, production configuration, a migration plan, or a production/customer readiness claim.

Primary machine-readable fixture:

- `fixtures/mvp_synthetic_lab.json`

## Purpose

PSC-MVP-002 defines the canonical sanitized scenario pack for the first Project Scientist MVP vertical slice. Downstream seed/reset and e2e tasks should load this fixture rather than inventing ad hoc demo values.

The pack is intentionally small: one realistic synthetic lab tenant, one control tenant for denied-operation checks, one client/contact/project, one sample setup, one profile with four analytes, minimal limits/qualifiers, minimal QC/review expectations, and report metadata.

## Safety boundary

- Lab-test only.
- Synthetic data only.
- No Tindall, CENLA, RJ Lee, AmSpec, or other customer identifiers.
- No real addresses, phone numbers, sample IDs, reports, limits, contacts, or credentials.
- SENAITE terminology is used only as concept mapping for internal development and tests.

## Canonical fixture values

| Concept | Fixture value |
| --- | --- |
| Tenant/lab | Clearline Demo Lab |
| Control tenant | Clearline Demo Lab B |
| Client | Okefenokee Synthetic Water Authority |
| Contact | Jordan Demo `<jordan.demo@example.test>` |
| Project/work order | MVP Drinking Water Compliance Demo |
| Client sample id | OK-SYN-2026-0001 |
| Matrix | Drinking Water |
| Container | 250 mL HDPE |
| Preservation | Nitric acid preserved |
| Analysis profile | MVP Metals + Field Checks |
| Analytes | pH, Turbidity, Lead, Copper |
| Report template | MVP Synthetic COA v1 |

## Scenario pack

### seed-reset

Expected downstream seed/reset behavior:

1. Reset local lab-test data store.
2. Load one synthetic tenant and one control tenant.
3. Load client/contact/project/catalog/profile/limits/qualifiers/report settings exactly once.
4. Export state and verify stable fixture ids.

### happy-path

Expected downstream e2e lifecycle:

1. `receiving-tech` receives `OK-SYN-2026-0001` with configured matrix/container/preservation.
2. System generates lab accession id and label artifact.
3. Profile expands to pH, Turbidity, Lead, and Copper analysis request lines.
4. `analyst` assigns worksheet and enters all results with unit/qualifier/limit snapshots.
5. `reviewer` confirms QC expectations and locks result set.
6. `report-releaser` generates and releases `MVP Synthetic COA v1` with immutable content hash.
7. Audit verifier confirms required event order and hash chain.

### denied-controls

Expected negative controls:

1. Deny and audit cross-tenant read/write from Clearline Demo Lab B.
2. Deny and audit `unauthorized-actor` protected mutation.
3. Deny and audit illegal sample/result/report workflow jump.
4. Deny and audit report release before review/QC/report preconditions.
5. Deny and audit mutation of released result or report artifact.

### audit-verification

Expected audit/provenance checks:

1. Verify hash-chain continuity.
2. Verify seed/config, sample intake, label generation, worksheet assignment, result entry, review, COA generation, and release events.
3. Verify denied-operation audit events carry actor, tenant, entity, and outcome without storing full sensitive payloads.
4. Verify released artifact hash matches report metadata.

## SENAITE concept mapping

This fixture maps to SENAITE concepts at the concept boundary, not to customer-specific implementation paths:

| Fixture key | SENAITE concept |
| --- | --- |
| `tenant` | Lab/portal boundary for clients, catalog, samples, and audit events |
| `client` | Client organization/account |
| `contact` | Client contact/report-recipient role |
| `project` | Project/work order grouping |
| `sample.matrix` | SampleType/sample matrix vocabulary |
| `sample.container` | Container catalog item |
| `sample.preservation` | Preservative/preservation requirement |
| `analysis_profile` | AnalysisProfile/order template expanded at request time |
| `analysis_profile.analytes[]` | AnalysisService/analyte line with method/unit/reporting snapshot |
| `limits[]` | Client/matrix/analyte reporting or acceptance limits |
| `qualifiers[]` | Controlled result qualifier vocabulary |
| `qc` | Minimal QC batch/review expectations; not a full QC migration model |
| `report` | Report template/default metadata for COA renderer/artifact store |

## Test coverage

`internal/lab/synthetic_fixture_test.go` validates that the JSON fixture:

- is explicitly synthetic and lab-test bounded;
- contains the canonical MVP fixture values;
- defines 2–4 analytes with method/unit snapshots;
- supplies reporting limits and qualifiers;
- carries minimal QC/review and report metadata;
- includes scenario steps for seed/reset, happy path, denied controls, and audit verification;
- maps required fixture concepts to SENAITE concepts.
