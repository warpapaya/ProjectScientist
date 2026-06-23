# Synthetic Customer Workflow Smoke Matrix

Status: internal synthetic lab-test artifact. This uses only `fixtures/golden_migration_dataset.json`; it does not use customer records, production dumps, contacts, credentials, or production report artifacts.

Boundary: not production evidence, not customer migration approval, not customer-facing evidence, and not authorization for customer data or external exposure.

Source inputs:

- Fixture: `fixtures/golden_migration_dataset.json` (`psc-rm-073-golden-migration-v1`, version `2026-06-22`, synthetic_only=true)
- Gap report: `docs/customer-workflow-gap-report.md`
- Generator: `project-scientist customer-workflow smoke-matrix`
- Generated JSON matrix: `artifacts/customer-workflow-smoke/customer-workflow-smoke-matrix.json` (ignored runtime artifact; regenerate with command below)

Verified command output:

```text
go test ./...
ok  	github.com/warpapaya/ProjectScientist/cmd/project-scientist	(cached)
ok  	github.com/warpapaya/ProjectScientist/internal/lab	1.816s

go run ./cmd/project-scientist customer-workflow smoke-matrix --db /tmp/psc-customer-workflow-smoke.db --fixture fixtures/golden_migration_dataset.json --gap-report docs/customer-workflow-gap-report.md --out artifacts/customer-workflow-smoke --command-output "go test ./... PASS; project-scientist customer-workflow smoke-matrix synthetic artifact generation PASS"
customer workflow smoke-matrix ok lanes=3 green=6 yellow=3 red=3 matrix=artifacts/customer-workflow-smoke/customer-workflow-smoke-matrix.json
```

Status meanings:

- GREEN: current synthetic lab-test evidence supports this slice.
- YELLOW: partially modeled; useful for planning or controlled synthetic demos only.
- RED: blocker before migration, customer pilot, production, or customer-facing readiness statement.

## Matrix summary

| Analog | Overall | Green | Yellow | Red | Report proof binding |
| --- | --- | ---: | ---: | ---: | --- |
| Tindall/precast-industrial | RED | 2 | 1 | 1 | static-scripted |
| CENLA/municipal-water | RED | 2 | 1 | 1 | static-scripted |
| RJ Lee/materials-forensics | YELLOW | 2 | 1 | 1 | static-scripted |

## Tindall/precast-industrial

Overall status: RED

Expected workflow steps:

1. receive multi-container industrial sample set under one synthetic chain-of-custody
2. expand profile into pH, TSS, alkalinity, and metals analysis request lines
3. assign wet chemistry and metals worksheet batches with method blank and LCS controls
4. review batch QC before report release
5. produce COA package with custody summary and analyte table

Required synthetic fixture data:

- Sample `SYN-SAMPLE-PI-0001` / client sample `PI-WW-2026-0001` / matrix `industrial wastewater` / containers: 500 mL HDPE preserved, 250 mL HDPE unpreserved, wide-mouth solids jar / analyses: pH, Total Suspended Solids, Alkalinity, Lead / custody events: received, split-for-metals, worksheet-assigned, reviewed, reported
  - Analysis `pH` / method `Synthetic SM4500-H+B` / result `7.42 pH units` / qualifier `none` / QC role `field-check`
  - Analysis `Total Suspended Solids` / method `Synthetic SM2540-D` / result `18 mg/L` / qualifier `none` / QC role `batch-sample`
  - Analysis `Lead` / method `Synthetic EPA 200.8` / result `2.1 ug/L` / qualifier `J` / QC role `batch-sample`
- QC batch `SYN-QC-WETCHEM-001` / method `Synthetic wet chemistry batch` / checks: method blank is below reporting limit, LCS recovery within 80-120 percent, duplicate RPD reviewed when present
- Report `SYN-REPORT-PI-001` / outputs: COA PDF, custody appendix / includes: client/project, sample receipt, result table, QC summary, authorized release actor

Checks, artifacts, and status:

| Check | Status | Evidence | Artifact |
| --- | --- | --- | --- |
| synthetic client migration reconciliation | GREEN | imported 3/3 synthetic clients; matched=3 missing=0 mismatched=0 | `artifacts/customer-workflow-smoke/client-import-reconciliation.json` |
| fixture workflow shape | GREEN | family precast-industrial has 5 workflow steps, 1 samples, 3 analyses | `fixtures/golden_migration_dataset.json` |
| report package proof | YELLOW | package artifact is explicitly STATIC/SCRIPTED because sample/result/QC/custody imports are still documented gaps | `artifacts/customer-workflow-smoke/precast-industrial-report-package-proof.txt` |
| remaining customer workflow blockers | RED | gap-analysis-result-import (high): analysis services, result values, qualifiers, and method snapshots are documented in the fixture but not migrated by ImportForScope yet; gap-contact-role-import (medium): client import currently migrates client name/email only; contacts and report-recipient roles remain fixture-only expectations; gap-qc-batch-model (high): QC batch expectations exist as dataset assertions; production-grade batch QC evaluation is not implemented; gap-report-package-artifacts (high): COA, EDD, narrative, custody appendix, and attachment-manifest artifacts are not yet generated from migrated data; gap-sample-custody-import (high): sample, container, preservation, and chain-of-custody rows are not yet importable through the generic migration path | `docs/customer-workflow-gap-report.md` |

Runtime artifacts:

- Matrix JSON: `artifacts/customer-workflow-smoke/customer-workflow-smoke-matrix.json`
- Static/scripted package proof: `artifacts/customer-workflow-smoke/precast-industrial-report-package-proof.txt`
- Package artifact SHA-256: `sha256:7f1486a69734fbab45f38b787a1eed24a3eeeb410d5549944374d66e129bc134`

Blockers:

- gap-analysis-result-import (high): analysis services, result values, qualifiers, and method snapshots are documented in the fixture but not migrated by ImportForScope yet
- gap-contact-role-import (medium): client import currently migrates client name/email only; contacts and report-recipient roles remain fixture-only expectations
- gap-qc-batch-model (high): QC batch expectations exist as dataset assertions; production-grade batch QC evaluation is not implemented
- gap-report-package-artifacts (high): COA, EDD, narrative, custody appendix, and attachment-manifest artifacts are not yet generated from migrated data
- gap-sample-custody-import (high): sample, container, preservation, and chain-of-custody rows are not yet importable through the generic migration path

## CENLA/municipal-water

Overall status: RED

Expected workflow steps:

1. receive chilled drinking-water and wastewater grab samples
2. preserve container metadata and sampled/received timestamps
3. expand nutrient, field, metals, and microbiology placeholder services
4. review holding-time and qualifier checks
5. produce client portal result summary plus machine-readable EDD rows

Required synthetic fixture data:

- Sample `SYN-SAMPLE-MW-0001` / client sample `MW-DW-2026-0001` / matrix `drinking water` / containers: sterile 120 mL bottle, 250 mL amber, 500 mL HDPE preserved / analyses: pH, Nitrate as N, Total Coliform, Copper / custody events: received-chilled, microbiology-hold-started, worksheet-assigned, reviewed, edd-exported
  - Analysis `Nitrate as N` / method `Synthetic EPA 300.0` / result `0.64 mg/L` / qualifier `none` / QC role `batch-sample`
  - Analysis `Total Coliform` / method `Synthetic SM9223` / result `Absent P/A` / qualifier `none` / QC role `microbiology-placeholder`
- QC batch `SYN-QC-WATER-001` / method `Synthetic water compliance batch` / checks: holding time warning captured, microbiology placeholder is not released as production claim, EDD rows reconcile to imported samples
- Report `SYN-REPORT-MW-001` / outputs: COA PDF, CSV EDD / includes: holding-time notes, field measurements, EDD row hash, review status

Checks, artifacts, and status:

| Check | Status | Evidence | Artifact |
| --- | --- | --- | --- |
| synthetic client migration reconciliation | GREEN | imported 3/3 synthetic clients; matched=3 missing=0 mismatched=0 | `artifacts/customer-workflow-smoke/client-import-reconciliation.json` |
| fixture workflow shape | GREEN | family municipal-water has 5 workflow steps, 1 samples, 2 analyses | `fixtures/golden_migration_dataset.json` |
| report package proof | YELLOW | package artifact is explicitly STATIC/SCRIPTED because sample/result/QC/custody imports are still documented gaps | `artifacts/customer-workflow-smoke/municipal-water-report-package-proof.txt` |
| remaining customer workflow blockers | RED | gap-analysis-result-import (high): analysis services, result values, qualifiers, and method snapshots are documented in the fixture but not migrated by ImportForScope yet; gap-contact-role-import (medium): client import currently migrates client name/email only; contacts and report-recipient roles remain fixture-only expectations; gap-qc-batch-model (high): QC batch expectations exist as dataset assertions; production-grade batch QC evaluation is not implemented; gap-report-package-artifacts (high): COA, EDD, narrative, custody appendix, and attachment-manifest artifacts are not yet generated from migrated data; gap-sample-custody-import (high): sample, container, preservation, and chain-of-custody rows are not yet importable through the generic migration path; gap-subcontract-and-holding-time-controls (medium): subcontract-blocking, microbiology holding-time, and release-precondition controls are expressed as checks but are not enforced by migration/report code | `docs/customer-workflow-gap-report.md` |

Runtime artifacts:

- Matrix JSON: `artifacts/customer-workflow-smoke/customer-workflow-smoke-matrix.json`
- Static/scripted package proof: `artifacts/customer-workflow-smoke/municipal-water-report-package-proof.txt`
- Package artifact SHA-256: `sha256:d15abb7b8701ccf8e9b25c12005353d3df5b8145fcbce2e1f64c697c00f5abd1`

Blockers:

- gap-analysis-result-import (high): analysis services, result values, qualifiers, and method snapshots are documented in the fixture but not migrated by ImportForScope yet
- gap-contact-role-import (medium): client import currently migrates client name/email only; contacts and report-recipient roles remain fixture-only expectations
- gap-qc-batch-model (high): QC batch expectations exist as dataset assertions; production-grade batch QC evaluation is not implemented
- gap-report-package-artifacts (high): COA, EDD, narrative, custody appendix, and attachment-manifest artifacts are not yet generated from migrated data
- gap-sample-custody-import (high): sample, container, preservation, and chain-of-custody rows are not yet importable through the generic migration path
- gap-subcontract-and-holding-time-controls (medium): subcontract-blocking, microbiology holding-time, and release-precondition controls are expressed as checks but are not enforced by migration/report code

## RJ Lee/materials-forensics

Overall status: YELLOW

RJ Lee readiness language: demo/pilot-validation only. Do not claim EQuIS, CLP, Stage, production, or customer-pilot readiness without authoritative validation and explicit approval.

Expected workflow steps:

1. receive batch of filter and bulk material samples with project-level custody
2. create per-sample analysis lines for gravimetric, microscopy, and metals services
3. track subcontract-required line without treating it as complete
4. review analyst narrative and QC duplicate agreement
5. produce narrative report package with table, comments, and attachment manifest

Required synthetic fixture data:

- Sample `SYN-SAMPLE-MF-0001` / client sample `MF-BULK-2026-0001` / matrix `bulk material` / containers: sealed bulk bag, filter cassette / analyses: Gravimetric Dust, Microscopy Screen, Lead, Narrative Review / custody events: received, subsampled, microscopy-reviewed, narrative-approved, reported
  - Analysis `Microscopy Screen` / method `Synthetic Microscopy SOP-01` / result `No regulated fibers observed qualitative` / qualifier `none` / QC role `reviewed-finding`
  - Analysis `Narrative Review` / method `Synthetic Technical Review` / result `See synthetic narrative package text` / qualifier `N` / QC role `narrative`
- QC batch `SYN-QC-MATERIALS-001` / method `Synthetic materials review batch` / checks: analyst narrative has reviewer acknowledgement, subcontract flag blocks final release until resolved, attachment manifest is present
- Report `SYN-REPORT-MF-001` / outputs: narrative PDF, attachment manifest / includes: analyst narrative, qualitative findings, subcontract status, review signature placeholder

Checks, artifacts, and status:

| Check | Status | Evidence | Artifact |
| --- | --- | --- | --- |
| synthetic client migration reconciliation | GREEN | imported 3/3 synthetic clients; matched=3 missing=0 mismatched=0 | `artifacts/customer-workflow-smoke/client-import-reconciliation.json` |
| fixture workflow shape | GREEN | family materials-forensics has 5 workflow steps, 1 samples, 2 analyses | `fixtures/golden_migration_dataset.json` |
| report package proof | YELLOW | package artifact is explicitly STATIC/SCRIPTED because sample/result/QC/custody imports are still documented gaps | `artifacts/customer-workflow-smoke/materials-forensics-report-package-proof.txt` |
| remaining customer workflow blockers | RED | gap-analysis-result-import (high): analysis services, result values, qualifiers, and method snapshots are documented in the fixture but not migrated by ImportForScope yet; gap-contact-role-import (medium): client import currently migrates client name/email only; contacts and report-recipient roles remain fixture-only expectations; gap-qc-batch-model (high): QC batch expectations exist as dataset assertions; production-grade batch QC evaluation is not implemented; gap-report-package-artifacts (high): COA, EDD, narrative, custody appendix, and attachment-manifest artifacts are not yet generated from migrated data; gap-sample-custody-import (high): sample, container, preservation, and chain-of-custody rows are not yet importable through the generic migration path; gap-subcontract-and-holding-time-controls (medium): subcontract-blocking, microbiology holding-time, and release-precondition controls are expressed as checks but are not enforced by migration/report code | `docs/customer-workflow-gap-report.md` |

Runtime artifacts:

- Matrix JSON: `artifacts/customer-workflow-smoke/customer-workflow-smoke-matrix.json`
- Static/scripted package proof: `artifacts/customer-workflow-smoke/materials-forensics-report-package-proof.txt`
- Package artifact SHA-256: `sha256:dac749635e9e69e2d1bc6bd4d6b4ea29dad9e0ecf628db1e18d0e0b788c3a271`

Blockers:

- gap-analysis-result-import (high): analysis services, result values, qualifiers, and method snapshots are documented in the fixture but not migrated by ImportForScope yet
- gap-contact-role-import (medium): client import currently migrates client name/email only; contacts and report-recipient roles remain fixture-only expectations
- gap-qc-batch-model (high): QC batch expectations exist as dataset assertions; production-grade batch QC evaluation is not implemented
- gap-report-package-artifacts (high): COA, EDD, narrative, custody appendix, and attachment-manifest artifacts are not yet generated from migrated data
- gap-sample-custody-import (high): sample, container, preservation, and chain-of-custody rows are not yet importable through the generic migration path
- gap-subcontract-and-holding-time-controls (medium): subcontract-blocking, microbiology holding-time, and release-precondition controls are expressed as checks but are not enforced by migration/report code

## Implementation child-card recommendations

Recommended follow-up cards should remain synthetic-only until Petie/Friday explicitly approves customer data, external staging, or customer-facing claims.

- Data-bound smoke package proof: Generate COA/COC, CSV EDD, and narrative/manifest outputs from migrated synthetic sample/result/QC/custody models instead of static/scripted fixture text.
- Post-remediation matrix refresh: Re-run the smoke matrix after merged sample/custody, result, QC, and report-package remediation work and downgrade only blockers with executable evidence.
- RJ Lee export validation gate: Add an authoritative validation gate before any EQuIS/CLP/Stage readiness language; until then keep RJ Lee language to demo/pilot-validation only.

## Reproduction

```bash
go test ./...
go run ./cmd/project-scientist customer-workflow smoke-matrix \
  --db /tmp/psc-customer-workflow-smoke.db \
  --fixture fixtures/golden_migration_dataset.json \
  --gap-report docs/customer-workflow-gap-report.md \
  --out artifacts/customer-workflow-smoke \
  --command-output "go test ./... PASS; project-scientist customer-workflow smoke-matrix synthetic artifact generation PASS"
```
