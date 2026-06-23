# Project Scientist — Customer Workflow Gap Report

Status: internal lab-test gap report. This is not production readiness evidence, not customer migration approval, not a customer-facing claim, and not authorization to use Tindall, CENLA, RJ Lee, or other customer data.

Source evidence:

- Roadmap/readiness ladder: `docs/senaite-parity-roadmap.md`
- Security/audit gates: `docs/security-audit-model.md`
- Migration reconciliation scope: `docs/migration-reconciliation-reports.md`
- Synthetic golden migration fixture: `fixtures/golden_migration_dataset.json`
- Golden dataset contract: `docs/golden-migration-datasets.md`
- Executable fixture coverage: `internal/lab/golden_migration_dataset_test.go`

## Boundary

This report maps current synthetic migration evidence to likely customer workflow gaps. It does not use real customer records, reports, limits, contacts, sample identifiers, credentials, or production tenant access. Customer names below are internal planning labels only.

Readiness terms follow the roadmap:

- GREEN: current lab-test evidence supports this slice with synthetic data.
- YELLOW: partially modeled; useful for planning or controlled synthetic demos, but not enough for migration/pilot reliance.
- RED: blocker before a customer pilot, migration, or production-readiness claim.

## Executive take

Project Scientist has enough synthetic evidence to pressure-test the shape of three Clearline workflow families, but the customer workflow story is still mostly RED/YELLOW. The strongest current evidence is client import/reconciliation and report-artifact immutability foundations. The primary blockers are sample/custody migration, analysis/result migration, QC batch enforcement, report package generation from migrated data, and customer-specific workflow validation.

No customer lane should be described as production-ready. At most, RJ Lee-style materials workflows are demo/pilot-validation candidates after scripted/static evidence is kept clearly labeled and after the RED gaps below are remediated or explicitly excluded from the demo scope.

## Current evidence matrix

| Evidence area | Current evidence | Readiness | Customer impact |
| --- | --- | --- | --- |
| Synthetic workflow coverage | Golden fixture covers `precast-industrial`, `municipal-water`, and `materials-forensics` families with clients, samples, analyses, QC batches, report expectations, and SENAITE concept mappings. | GREEN for internal planning | Gives enough shape to compare Tindall, CENLA, and RJ Lee needs without touching real data. |
| Client import/reconciliation | Fixture clients import through `ImportForScope`; reconciliation report records clean client counts and hashes. | GREEN for client-only migration evidence | Useful but narrow; does not prove sample, contact, analysis, QC, or report migration. |
| Contact/report recipient roles | `gap-contact-role-import` remains medium severity. | YELLOW | Customer communication/report routing cannot be validated from migration evidence yet. |
| Sample/custody import | `gap-sample-custody-import` remains high severity. | RED | Receiving, containers, preservation, custody, and COC history are not migration-ready. |
| Analysis/result import | `gap-analysis-result-import` remains high severity. | RED | Historical/current analytical data, methods, units, qualifiers, and snapshots are not migration-ready. |
| QC batch evaluation | `gap-qc-batch-model` remains high severity. | RED | Batch acceptance/rejection and QC-driven release blocking are not fully defensible for customer workflows. |
| Report package artifacts from migrated data | `gap-report-package-artifacts` remains high severity. | RED | COA/EDD/narrative/custody packages cannot yet be claimed as generated from migrated data. |
| Holding-time/subcontract controls | `gap-subcontract-and-holding-time-controls` remains medium severity. | YELLOW/RED by workflow | Municipal microbiology and materials subcontract workflows need explicit release-precondition enforcement. |
| Security/pilot gates | Security gates still require production-candidate review, external checkpointing, complete auth/RBAC, backup/restore proof, and workflow smoke. | RED for customer pilot | No customer data, external exposure, or production claim without separate approval and gate completion. |

## Tindall workflow assessment

Planning analog: `precast-industrial`.

Overall status: RED for customer migration/pilot readiness; YELLOW for synthetic workflow pressure-testing.

| Workflow need | Current status | Evidence | Gap / next move |
| --- | --- | --- | --- |
| Industrial/precast sample intake | YELLOW | Fixture models multi-container industrial wastewater/solids sample receipt, profile expansion, custody expectations. | Implement sample/custody import and intake validation for matrix/container/preservation/custody fields. |
| Wet chemistry/metals analysis lines | RED | Fixture analysis rows exist for pH, TSS, alkalinity, lead; roadmap says result import and method snapshots are still required. | Build analysis/result migration path with method, unit, qualifier, limit/snapshot preservation. |
| Batch QC before release | RED | Fixture includes wet chemistry batch expectations; QC batch model gap remains high severity. | Implement batch QC model, acceptance decision, release-blocking, and audit proof. |
| COA with custody appendix | RED | Lab-test COA/COC package foundation exists for synthetic data, but report package artifacts from migrated data remain a high gap. | Generate COA/COC package from migrated sample/result/QC/custody data and reconcile hashes. |
| Customer pilot posture | RED | Roadmap requires migration reconciliation, security review, backup/restore, customer workflow smoke, and Petie approval. | Do not position as production-ready; first target is synthetic round-trip plus internal workflow smoke. |

Tindall-specific conclusion: the workflow shape is understood, but Tindall cannot be a migration candidate until migrated samples, results, QC, and COA/COC packages are proven end-to-end. Current evidence supports using Tindall-like synthetic cases as a baseline test harness only.

## CENLA workflow assessment

Planning analog: `municipal-water`.

Overall status: RED for customer migration/pilot readiness; YELLOW for synthetic workflow pressure-testing.

| Workflow need | Current status | Evidence | Gap / next move |
| --- | --- | --- | --- |
| Municipal water intake with timestamps/containers | YELLOW | Fixture models chilled drinking-water/wastewater samples, sampled/received timestamps, preservation/container metadata. | Implement sample/custody migration and holding-time calculation inputs. |
| Routine chemistry/metals/microbiology placeholder | RED | Fixture includes nutrient, field, metals, and microbiology-placeholder services; result import remains high severity. | Build result import plus microbiology placeholder/release boundary so placeholders cannot become production claims. |
| Holding-time and qualifier checks | RED | Fixture expects holding-time warnings; `gap-subcontract-and-holding-time-controls` remains medium severity. | Implement holding-time rule evaluation and release-blocking/override audit path. |
| EDD/client summary export | RED | Fixture expects CSV EDD and portal summary outputs; report package artifact gap remains high severity. | Build deterministic EDD/export framework with row hashes and reconciliation evidence before customer use. |
| Customer pilot posture | RED | Security/pilot gates and migration round-trip remain incomplete. | Keep CENLA discussions internal until synthetic EDD/export + holding-time proof exists. |

CENLA-specific conclusion: CENLA needs stronger export/EDD and holding-time discipline than the current artifact set proves. The municipal-water fixture is the right pressure shape, but no pilot claim is defensible until sample/result migration, holding-time controls, and export reconciliation are green.

## RJ Lee demo / pilot-validation assessment

Planning analog: `materials-forensics`.

Overall status: YELLOW for a tightly scoped internal/static/scripted demo; RED for production or real pilot readiness.

| Workflow need | Current status | Evidence | Gap / next move |
| --- | --- | --- | --- |
| Materials/filter/bulk sample batch | YELLOW | Fixture models filter/bulk material samples, project-level custody, microscopy-style qualitative findings. | Implement sample/analysis migration if demo scope includes imported or historical data. |
| Qualitative narrative findings | YELLOW | Fixture includes narrative review analysis and narrative report package expectation; text renderer/report artifact foundations exist for synthetic data. | Bind narrative report output deterministically to sample/result/QC/custody fields and fixture hashes. |
| Subcontract flags | RED | Fixture expects subcontract-required line to block final release; control gap remains. | Implement subcontract release-precondition and audit path before claiming workflow readiness. |
| CLP/Stage/EDD-style package credibility | RED | Roadmap explicitly limits EQuIS-style claims until authoritative spec/validation; fixture has narrative/manifest expectations only. | Keep any package demo static/visual/scripted unless deterministic data-bound generation and external format validation exist. |
| Demo posture | YELLOW/RED | Materials-forensics fixture supports internal demo shape; production gates remain red. | Accept only demo/pilot-validation language with green/yellow/red caveats; no blanket production-ready claim. |

RJ Lee-specific conclusion: the strongest near-term lane is a clearly labeled demo/pilot-validation narrative, not a migration claim. Keep CLP/Stage/EDD artifacts static or scripted until data binding and authoritative validation are proven.

## Cross-customer remediation backlog

1. Sample/custody migration import
   - Target: import sample metadata, client sample ids, lab ids, matrices, containers, preservation, received condition, and custody event expectations from the golden fixture.
   - Exit proof: synthetic sample/custody rows import, reconcile, and audit without customer data.

2. Analysis/result migration import
   - Target: import analysis services, method snapshots, units, results, qualifiers, detection/reporting limits, and comments.
   - Exit proof: fixture analysis rows round-trip into result/review models with immutable snapshots.

3. QC batch model and release blocking
   - Target: represent QC batches, linked samples, blanks/LCS/duplicates/holding-time/subcontract checks, acceptance decisions, and audit events.
   - Exit proof: release is denied when QC prerequisites fail and allowed only after accepted QC state.

4. Report package generation from migrated data
   - Target: generate COA, COC appendix, EDD/narrative outputs, attachment manifests, content hashes, and reconciliation evidence from migrated fixture data.
   - Exit proof: report artifacts are data-bound, immutable, and hash-reconciled against migration inputs.

5. Customer workflow smoke matrix
   - Target: explicit synthetic smoke scripts for Tindall/precast-industrial, CENLA/municipal-water, and RJ Lee/materials-forensics.
   - Exit proof: each smoke produces a green/yellow/red result with command output, artifact paths, and remaining gaps.

6. Pilot/security gate review
   - Target: Aegis/Friday/Forge review after migration/report/QC proof exists.
   - Exit proof: documented go/no-go for any controlled staging pilot, with Petie approval required before customer data or external exposure.

## Go/no-go summary

| Lane | Current go/no-go | Why |
| --- | --- | --- |
| Tindall | NO-GO for migration/pilot; OK for synthetic baseline tests | Critical sample/result/QC/report migration gaps remain. |
| CENLA | NO-GO for migration/pilot; OK for synthetic municipal workflow tests | Holding-time, EDD/export, sample/result migration, and security gates remain incomplete. |
| RJ Lee | NO-GO for production; conditional OK for internal/static/scripted demo prep | Materials narrative shape exists, but data-bound package generation, subcontract controls, and authoritative export validation are not proven. |

Bottom line: the golden dataset is doing its job. It shows the path and exposes the blockers. The next useful work is not more readiness language; it is executable remediation against the RED rows above.
