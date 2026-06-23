# PSC-RM-084 Protected HTTP/API Operation Inventory

Scope: Project Scientist lab-development only. This inventory maps local HTTP/API routes and store operations to the authorization policy checks that guard actor role, tenant/lab scope, reviewer separation, and audited denial evidence. It is not a production-readiness claim.

## Policy primitives

- Actor context: `lab.ActorContext` created from the trusted local dev/session boundary in `cmd/project-scientist/main.go` (`actor`, `demoResetActor`) and normalized by `internal/lab/actor_context.go`.
- Tenant/lab scope: `lab.Scope` from `scopeFromRequest`; every mutating store method normalizes scope and checks tenant membership through `Authorize` / `authorizeOperationTx`.
- Role policy: `internal/lab/authorization.go` `operationAllowedRoles`.
- Audited denial: protected store operations use `authorizeOperationTx` or `AuthorizeOperationForScope`; denied attempts emit `audit_events.outcome=denied`, `reason=authorization_denied`, normalized resource metadata, and sanitized details before returning `ErrAuthorizationDenied`.
- Reviewer separation: result review has a workflow-specific ABAC check in `ReviewResultForScope` when `EnforceReviewerSeparation` is set; same actor who entered the result cannot review it.

## HTTP/API route inventory

| Route / operation | Handler | Store/API method | Policy operation | Required role(s) | Scope / ABAC checks | Denial evidence tests |
|---|---|---|---|---|---|---|
| `GET /`, `GET /api/state` | `index`, `apiState` | read aggregators + `AuditEventsForScopeAsActor` | `audit.view` for audit panel; read scope membership for page/state | `admin`, `lab-manager` for audit events | `httpActorCanReadScope` rejects arbitrary tenant/lab reads; audit panel uses scoped audit authorization | `main_test.go` tenant-boundary tests; `audit_read_authorization_test.go` |
| `POST /api/demo/reset` | `demoReset` | `ResetAndSeedSyntheticDemo` | `admin.configure` | `admin` | Endpoint disabled unless explicitly enabled; trusted `demoResetActor` only | `main_test.go` demo reset enabled/disabled; `demo_seed_test.go` denied admin reset |
| `POST /api/clients` | `createClient` | `CreateClientForScope` | `client.create` | `admin`, `lab-manager` | Tenant membership and lab scope required before insert | `authorization_test.go`; `main_test.go` arbitrary tenant denial |
| `POST /api/sites` | `createSite` | `CreateSiteForScope` | `client.update` | `admin`, `lab-manager` | Client must exist in requested tenant/lab | `master_data_test.go`; `authorization_test.go` |
| `POST /api/contacts` | `createContact` | `CreateContactForScope` | `contact.create` | `admin`, `lab-manager` | Client/site must exist in requested tenant/lab | `master_data_test.go`; `authorization_test.go` |
| `POST /api/contact-roles` | `assignContactRole` | `AssignContactRoleForScope` | `contact.update` | `admin`, `lab-manager` | Contact must exist in requested tenant/lab | `master_data_test.go`; `authorization_test.go` |
| `POST /api/projects` | `createProject` | `CreateProjectForScope` | `project.create` | `admin`, `lab-manager` | Client/site must exist in requested tenant/lab | `master_data_test.go`; `authorization_test.go` |
| `POST /api/client-defaults` | `upsertClientDefaults` | `UpsertClientDefaultsForScope` | `client.update` | `admin`, `lab-manager` | Client must exist in requested tenant/lab | `master_data_api_test.go`; `authorization_test.go` |
| `POST /api/sample-intake-templates` | `createSampleIntakeTemplate` | `CreateSampleIntakeTemplateForScope` | `sample.intake` | `admin`, `lab-manager` | Template references resolve within requested tenant/lab | `sample_intake_template_test.go`; `authorization_test.go` |
| `POST /api/sample-intake-templates/{id}/samples` | `createSamplesFromTemplate` | `CreateSamplesFromTemplateForScope` | `sample.intake` | `admin`, `lab-manager` | Template and generated samples scoped to tenant/lab | `sample_intake_template_test.go`; `authorization_test.go` |
| `POST /api/samples` | `createSample` | `CreateSampleForScope` | `sample.intake` | `admin`, `lab-manager` | Client/project/reference IDs must be in requested tenant/lab | `sample_scope_security_test.go`; `authorization_test.go`; `main_test.go` |
| `POST /api/samples/{id}/transition` | `transitionSample` | `TransitionSampleForScope` | `sample.transition` | `admin`, `lab-manager`, `analyst` | Sample must exist in requested tenant/lab; cross-tenant attempts avoid existence leaks | `sample_scope_security_test.go`; `authorization_test.go`; `main_test.go` |
| `POST /api/samples/{id}/custody-events` | `recordCustodyEvent` | `RecordCustodyEventForScope` | `sample.custody` | `admin`, `lab-manager`, `analyst` | Sample must exist in requested tenant/lab; custody history immutable | `custody_events_test.go`; `authorization_test.go` |
| `GET /api/samples/{id}/label-artifact` | `sampleLabelArtifact` | `GenerateSampleLabelArtifactForScope` | `report.generate` | `admin`, `lab-manager`, `reviewer`, `report-releaser` | Sample must exist in requested tenant/lab | `label_artifact_http_test.go`; `authorization_test.go` |
| `POST /api/samples/{id}/coc-package` | `generateCOCPackage` | `GenerateCOCPackageForScope` | `report.generate` | `admin`, `lab-manager`, `reviewer`, `report-releaser` | Sample/artifact attachments scoped to tenant/lab | `coc_package_test.go`; `authorization_test.go` |
| `GET /api/samples/{id}/report-preview` | `previewReportArtifact` | `PreviewCOAArtifactForScope` | read membership only | scope member | Preview is not persisted/released; sample lookup is scoped | `aegis_download_boundary_challenge_test.go`; `main_test.go` read-scope boundary |
| `POST /api/samples/{id}/report-release` | `releaseReportArtifact` | `GenerateCOAReportArtifactForScope` | `report.release` | `admin`, `report-releaser` | Sample readiness, QC release, and artifact scope enforced | `report_artifact_test.go`; `report_release_readiness_test.go`; `authorization_test.go` |
| `GET /api/report-artifacts/{id}` | `reportArtifactDownload` | `ReportArtifactForScope` | read membership only | scope member | Artifact lookup is scoped by tenant/lab; cross-tenant returns not found/forbidden without leak | `aegis_download_boundary_challenge_test.go` |
| `GET /api/coc-packages/{id}` | `cocPackageDownload` | `COCPackageForScope` | read membership only | scope member | Package lookup is scoped by tenant/lab; cross-tenant returns not found/forbidden without leak | `aegis_download_boundary_challenge_test.go` |
| `POST /api/results` | `createResult` | `CreateResultForScope` | `result.entry` | `admin`, `lab-manager`, `analyst` | Analysis request line/sample must be in requested tenant/lab | `result_test.go`; `result_http_test.go`; `authorization_test.go` |
| `POST /api/results/{id}` | `resultAction` update branch | `UpdateResultForScope` | `result.update` | `admin`, `lab-manager`, `analyst` | Result must be in requested tenant/lab and mutable state | `result_test.go`; `authorization_test.go` |
| `POST /api/results/{id}/review` | `resultAction` review branch | `ReviewResultForScope` | `result.review` | `admin`, `lab-manager`, `reviewer` | Reviewer separation ABAC if requested; result scoped to tenant/lab | `result_review_test.go`; `authorization_test.go` |
| `POST /api/results/{id}/reopen` | `resultAction` reopen branch | `ReopenResultForScope` | `result.update` | `admin`, `lab-manager`, `analyst` | Result scoped to tenant/lab; reopen reason required by workflow | `result_review_test.go`; `authorization_test.go` |
| `POST /api/worksheets` | `createWorksheet` | `CreateWorksheetForScope` | `result.update` | `admin`, `lab-manager`, `analyst` | Worksheet lines must belong to requested tenant/lab | `worksheet_test.go`; `worksheet_http_test.go`; `authorization_test.go` |
| `POST /api/worksheets/{id}/assign` | `routeWorksheetMutation` assign branch | `AssignWorksheetAnalystForScope` | `result.update` | `admin`, `lab-manager`, `analyst` | Worksheet scoped to tenant/lab | `worksheet_test.go`; `authorization_test.go` |
| `POST /api/worksheets/{id}/transition` | `routeWorksheetMutation` transition branch | `TransitionWorksheetForScope` | `result.update` | `admin`, `lab-manager`, `analyst` | Worksheet scoped to tenant/lab | `worksheet_test.go`; `authorization_test.go` |
| `POST /api/worksheets/{id}/lines/{lineID}/remove` | `routeWorksheetMutation` remove branch | `RemoveWorksheetLineForScope` | `result.update` | `admin`, `lab-manager`, `analyst` | Worksheet line scoped to same tenant/lab | `worksheet_test.go`; `authorization_test.go` |
| `POST /api/catalog/departments` | `createCatalogDepartment` | `CreateCatalogDepartmentForScope` | `catalog.configure` | `admin`, `lab-manager` | Catalog rows scoped to tenant/lab; denied attempts now audited | `catalog_test.go`; `authorization_test.go` |
| `POST /api/catalog/units` | `createCatalogUnit` | `CreateCatalogUnitForScope` | `catalog.configure` | `admin`, `lab-manager` | Catalog rows scoped to tenant/lab; denied attempts now audited | `catalog_test.go`; `authorization_test.go` |
| `POST /api/catalog/methods` | `createCatalogMethod` | `CreateCatalogMethodForScope` | `catalog.configure` | `admin`, `lab-manager` | Catalog rows scoped to tenant/lab; denied attempts now audited | `catalog_test.go`; `authorization_test.go` |
| `POST /api/catalog/analytes` | `createCatalogAnalyte` | `CreateCatalogAnalyteForScope` | `catalog.configure` | `admin`, `lab-manager` | Optional unit scoped to tenant/lab; denied attempts now audited | `catalog_test.go`; `authorization_test.go` |
| `POST /api/catalog/services` | `createAnalysisService` | `CreateAnalysisServiceForScope` | `catalog.configure` | `admin`, `lab-manager` | Department/method/analyte/unit references scoped to tenant/lab; denied attempts now audited | `catalog_test.go`; `authorization_test.go` |
| `POST /api/catalog/profiles` | `createAnalysisProfile` | `CreateAnalysisProfileForScope` | `catalog.configure` | `admin`, `lab-manager` | Service IDs scoped to tenant/lab; denied attempts now audited | `catalog_test.go`; `authorization_test.go` |
| `POST /api/sample-reference` | `createSampleReferenceItem` | `CreateSampleReferenceItemForScope` | `catalog.configure` | `admin`, `lab-manager` | Reference rows scoped to tenant/lab; denied attempts now audited | `sample_reference_test.go`; `authorization_test.go` |
| `POST /api/sample-reference/{id}` | `updateSampleReferenceItem` | `UpdateSampleReferenceItemForScope` | `catalog.configure` | `admin`, `lab-manager` | Reference ID scoped to tenant/lab; denied attempts now audited | `sample_reference_test.go`; `authorization_test.go` |
| `DELETE /api/sample-reference/{id}` | `deleteSampleReferenceItem` | `DeleteSampleReferenceItemForScope` | `catalog.configure` | `admin`, `lab-manager` | Reference ID scoped to tenant/lab; denied attempts now audited | `sample_reference_test.go`; `authorization_test.go` |
| CLI/import/export APIs | CLI/store methods | `Import*`, `Export*`, `AuditEventsForScope*` | `import.run`, `export.run`, `audit.view`, `audit.export` | per `operationAllowedRoles` | Scope normalized and actor tenant membership enforced | `import_export_test.go`; `audit_read_authorization_test.go`; `authorization_test.go` |

## Coverage assertions

- `TestAuthorizeOperationDeniesAndAuditsEveryProtectedOperation` enumerates all protected `Operation` constants, including `sample.custody` and `qc.relate`, and asserts audited denied evidence for each.
- `TestPolicyAllowsExpectedRolesForProtectedOperations` asserts positive allow cases for every operation in the inventory.
- `TestCatalogAndReferenceMutationsDenyAndAuditUnauthorizedActors` closes the catalog/sample-reference gap by proving unauthorized catalog configuration attempts emit durable denied audit events.
- Route-specific tests exercise tenant/lab boundary behavior, no cross-tenant existence leaks, reviewer separation, report/QC release gates, and download boundaries.
