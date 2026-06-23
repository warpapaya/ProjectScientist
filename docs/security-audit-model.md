# Security and Audit Production Gates

Status: planning artifact for future engineering work. This document does not approve Project Scientist for production, customer data, customer migration, or external exposure.

Source reviews:

- PSC-004 Aegis audit/security model review: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_e71f2c74/psc-004-audit-security-review.md`
- Friday corrected review gate: `/Users/citadel/.hermes/kanban/boards/project-scientist/workspaces/t_e71f2c74/friday-psc-004-review-gate.md`
- Repo baseline observed by Friday: `f39b51b feat: bootstrap Project Scientist local LIMS prototype`

## Scope and release posture

Project Scientist currently has a bootstrap local prototype with a useful audit spine. It is suitable for architecture pressure-testing only.

Before the project can be called production-candidate, the gates in this document must be implemented, tested, and re-reviewed by Aegis/security and Friday. Petie approval is still required before customer data, customer migration, external exposure, or any customer-facing production claim.

## Current bootstrap behavior

The current implementation provides:

- Go standard-library local app and Docker development flow.
- Client creation, sample intake, analysis list attachment, and simple sample workflow.
- Server-side transition edge enforcement for `received -> in_prep -> in_analysis -> in_review -> released`.
- SQLite-backed persistence with domain state and audit writes committed in one transaction.
- Audit event fields: event id, tenant/lab id, timestamp, authenticated actor context snapshot, resource, action, outcome/reason, correlation id, sequence, details, previous hash, and hash.
- Startup audit verification that refuses writes when the local hash chain or checkpoint tail is damaged.
- Basic tests for sample creation audit, allowed workflow path, illegal direct release denial auditing, actor-context spoofing controls, tenant/lab scoping, and hash chaining.

Known bootstrap limitations:

- Local HTTP uses a fixed authenticated dev actor (`lab-dev`) and ignores spoofable `X-PSC-Actor`/form `actor`; real login/session middleware is not implemented yet.
- No RBAC/ABAC enforcement exists beyond tenant/lab boundary checks.
- Tenant/lab boundary exists for current client/sample/audit paths, but is not yet applied across future result/report/import domains.
- Audit denial events are emitted for protected sample transition denials; other future protected operations still need denial events as they are added.
- External checkpointing is not implemented, so a writer with filesystem access can still regenerate the local chain.
- Remaining advanced crash/recovery modes beyond SQLite transaction rollback still need explicit backup/restore and operator recovery tests.
- Release workflow lacks lab-grade preconditions, immutable report/result artifacts, e-signature, and amendment/supersession model.
- Audit privacy, retention, backup/restore, checkpointing, and migration provenance are not yet specified in code.

## Production-candidate gates

### 1. Audit schema gate

Required event schema:

- Event id: stable unique identifier.
- Tenant/lab id: required on every event.
- Resource: type, id, and optional version.
- Action: stable command/event name such as `sample.transition.requested` or `migration.imported`.
- Actor context: authenticated principal metadata, not caller text.
- Timestamp: trusted server time, with clock source/skew policy documented.
- Sequence: monotonic per tenant/lab or per audit stream; allocation rules documented.
- Previous hash and event hash: canonical hash rules documented and tested.
- Correlation/request id: trace one user/API request through emitted events.
- Outcome: `allowed`, `denied`, `failed`, or `system` as applicable.
- Reason/error code: required for denied/failed protected operations.
- Details: minimal non-secret metadata only; no full sample payloads/results unless explicitly justified.

Acceptance tests:

- All mutating commands emit schema-valid events.
- Missing tenant, actor context, sequence, hash, outcome, or correlation id fails validation.
- Denied protected commands emit safe denial events without leaking sensitive payloads.

### 2. Actor/auth model gate

Current bootstrap behavior: actor is a spoofable string from request header/form/default.

Required model:

- Store/API commands accept an authenticated actor context, not free text.
- Actor context includes stable user id, display name snapshot, auth provider/session id, role set, tenant/lab id, service-account flag, impersonation/delegation marker, and source IP/device when available.
- Privileged overrides require a reason/comment and emit audit events.
- Service accounts are explicit, narrowly scoped, and policy-bound.

Acceptance tests:

- Spoofed `X-PSC-Actor`/form actor input cannot change audit identity.
- Anonymous users cannot mutate or read protected resources unless explicitly allowed by policy.
- Service-account and impersonation events retain both effective actor and original principal.

### 3. Authorization and RBAC/ABAC gate

Required roles and permissions must be committed before API expansion:

- Admin.
- Lab manager.
- Analyst.
- Reviewer.
- Report releaser.
- Client/contact.
- Migration service account.

Protected operations:

- Client/contact/project create/update/archive.
- Sample intake/update/transition.
- Result entry/update/review/release.
- Report/COA generation, release, export, amendment.
- Audit view/export.
- Import/migration operations.
- Admin/security configuration.

Acceptance tests:

- Negative authorization tests exist for every protected operation.
- Denied events are audited with outcome and reason/error code.
- Authorization is enforced server-side and cannot be bypassed through UI-only restrictions.

Lab-test implementation note (PSC-RM-005): the committed server-side policy layer defines protected operations for client/contact/project, sample, result, report, audit, import/export, and admin/security configuration. Store mutations call the policy before writing state. Denied authorization attempts commit safe audit events with `outcome=denied` and `reason=authorization_denied` without full request payloads.

### 4. Tenant/lab boundary gate

Current bootstrap behavior: clients exist but there is no tenant/lab boundary.

Required model:

- Every domain object carries tenant/lab id.
- Every audit event carries tenant/lab id.
- Every query is tenant-scoped by construction.
- Import/export/report/audit views are tenant-scoped.
- Authorization decisions include tenant/lab membership.

Acceptance tests:

- Cross-tenant reads, writes, transitions, audit views, exports, and imports are denied.
- Cross-tenant object ids cannot be used to infer resource existence beyond authorized scope.
- Tenant id cannot be supplied by untrusted request data alone.

### 5. Append-only audit gate

Current bootstrap behavior: audit writes use `O_APPEND`, but no full append-only policy exists.

Required model:

- No public or internal update/delete audit API.
- Audit writer uses append-only semantics, restrictive file/database permissions, and documented fsync/commit policy.
- Startup verification checks chain and sequence before serving writes.
- Failure mode is closed: damaged audit stream blocks protected writes until operator recovery.

Acceptance tests:

- Attempts to rewrite, truncate, skip sequence, duplicate sequence, or append malformed events are detected.
- Startup verifier refuses writes after audit damage.
- Crash/concurrency behavior is tested.

### 6. Tamper-evidence and checkpoint gate

Current bootstrap behavior: local hash chaining detects some in-file edits but cannot defeat full-chain regeneration by a writer with filesystem access.

Required model:

- Canonical JSON/event serialization and hash input are specified.
- Event hash and previous hash verification catches modify/delete/reorder/truncate/duplicate/sequence-gap cases.
- Periodic signed or externally anchored checkpoints exist outside the mutable primary store.
- Checkpoint verification catches regenerated-chain attacks.

Acceptance tests:

- Verifier fails on modified event body, deleted event, reordered event, duplicate sequence, gap, truncated tail, malformed JSON, hash mismatch, and checkpoint mismatch.
- Checkpoint creation and verification are deterministic and documented.

Implemented v1 lab-test behavior:

- Audit events are written to SQLite with schema v1 fields: `event_id`, `tenant_id`, `lab_id`, actor context JSON, resource JSON, stable action, `outcome`, denied/failed `reason`, `correlation_id`, monotonic `sequence`, canonical `previous_hash`, and event `hash`.
- Canonical hash input is the JSON serialization of every schema field except `hash`, with timestamps normalized through RFC3339Nano UTC formatting. `details` is restricted by convention to minimal non-secret metadata.
- Every append updates the local `audit_checkpoints.latest` row with the tail sequence and hash in the same transaction as the domain mutation/audit insert. This is only a deterministic local checkpoint for lab-test tamper detection.
- PSC-RM-084 adds an internal JSON checkpoint artifact outside the mutable primary SQLite rows. `Store.GenerateAuditCheckpointArtifact` / `WriteAuditCheckpointArtifact` produce a deterministic `ed25519-sha256-json-v1` artifact containing event count, tail sequence, tail hash, tail timestamp, signer key id, public key, checksum, and Ed25519 signature. `ReadAuditCheckpointArtifact` and `VerifyAuditCheckpointArtifact` read/verify the artifact and compare it with the current SQLite tail, so a regenerated in-database chain fails against a prior artifact. See `docs/psc-rm-084-audit-checkpoint-proof.md`.
- This remains lab-development proof, not external staging approval. Production-candidate work still requires real external anchoring/key custody such as WORM/object storage, KMS/HSM-backed signing, append-only ops ledger, key rotation, independent timestamping, and a security review against the implementation.
- Startup opens fail closed: `OpenSQLiteStore` runs the verifier before serving writes and returns `audit verification failed` on damaged streams. The verifier checks schema validity, duplicate/gap sequence, previous-hash linkage, event hash recomputation, malformed JSON/timestamps, and checkpoint tail mismatch. `OpenSQLiteStoreWithoutVerification` exists for controlled tests/operator recovery only and must not be used by the serving app.
- Protected sample transition denials emit `sample.transition.requested` with `outcome=denied` and a safe reason code before returning the denial; no full request payload is recorded.

Checkpoint plan for the next gate:

1. Keep the local checkpoint as the fast startup guard.
2. Add periodic signed checkpoints containing tenant/lab id, sequence, tail hash, creation time, signer key id, and signature.
3. Anchor signed checkpoint records outside the writable primary store (object storage/WORM bucket, append-only log, or ops-controlled Git/secret-backed ledger).
4. Startup verification should compare the SQLite tail against the newest trusted external checkpoint and block writes on mismatch, missing checkpoint, or signer/key policy failure.

### 7. State/audit transaction gate

Current bootstrap behavior: state and audit can diverge because audit and state are separate file writes.

Required model:

- State mutation and audit event commit are atomic or recoverable.
- Preferred path: database transaction that writes domain state and audit event together.
- File-based fallback must use locks, atomic state writes, monotonic sequence allocation, and startup reconciliation.

Acceptance tests:

- Simulated crash before/after audit write and state write produces either a fully committed mutation or a recoverable blocked state.
- Concurrent writers cannot duplicate sequences, lose updates, or split state from audit.

### 8. Protected workflow gate

Current bootstrap behavior: transition edge order is enforced, but release preconditions are absent.

Required model:

- Workflow commands enforce guard conditions, not just allowed edges.
- Release requires completed analyses/results, QC acceptance, reviewer/releaser identity policy, e-signature or equivalent attestation, immutable report/result artifact, and explicit release reason.
- Corrections require amendment/supersession workflow; silent post-release edits are forbidden.
- Denied workflow attempts emit safe denial audit events.

Acceptance tests:

- Release without required result/QC/review/report/e-signature preconditions is denied and audited.
- Reviewer separation rules are enforced where configured.
- Released artifacts cannot be edited in place; amendments create new versions with audit links.

### 9. Report/result immutability gate

Required model:

- Result and report versions are immutable after release.
- Generated COA/report artifacts store content hash, generation inputs, template/version, reviewer/releaser attribution, and release timestamp.
- Supersession/amendment links old and new versions.

Acceptance tests:

- Released report/result versions cannot be overwritten.
- Amendment creates a new version and audit chain link.
- Artifact hash verification detects modification after release.

### 10. Migration provenance gate

Required model:

- SENAITE migration produces explicit `migration.imported` events.
- Events include source system, source object id, import batch id, importer service actor, checksum of source payload, mapping version, source historical timestamp marked as source-asserted, and Project Scientist import timestamp.
- Migration is dry-runable, idempotent, reconcilable, and reversible before cutover.
- Migrated historical facts are visually and programmatically distinct from native Project Scientist actions.

Acceptance tests:

- Dry run produces reconciliation report without state mutation.
- Re-running the same batch is idempotent.
- Source checksums and mapping versions are stored and verifiable.
- Historical source timestamps are never treated as current-system action timestamps.

### 11. Privacy/minimization gate

Required model:

- Audit details store identifiers and minimal metadata, not full sample/customer payloads by default.
- No credentials, secrets, tokens, or sensitive payload dumps in audit events.
- Audit read/export is permissioned and tenant-scoped.
- Retention, legal hold, redaction, encryption at rest, and backup handling are documented.

Acceptance tests:

- Secret/credential-like values are rejected or redacted before audit write.
- Unauthorized audit view/export is denied and audited.
- Tenant-scoped audit export cannot include another tenant's records.

### 12. Operational backup/restore gate

Required model:

- Backup includes state, audit stream, checkpoint anchors, and enough metadata to verify integrity after restore.
- Restore procedure verifies audit chain and checkpoints before serving writes.
- Clock skew monitoring and service-account policy are documented.
- Recovery runbook defines operator steps when audit verification fails.

Acceptance tests:

- Restore drill verifies chain/checkpoints and returns a pass/fail artifact.
- Restore from damaged/incomplete backup blocks protected writes.
- Checkpoints survive backup/restore and catch regenerated-chain attacks.

### 13. Human review and approval gate

Required before production-candidate or customer pilot:

- `go test ./...` passes.
- `go vet ./...` passes.
- Security/audit test suite passes.
- Aegis/security review re-runs against implementation, not just this plan.
- Friday gate explicitly approves the implementation for the requested stage.
- Petie explicitly approves any customer data, live migration, external exposure, auth/DNS/billing/security infrastructure change, or production/customer-facing claim.

## Backlog-ready engineering slices

Use these as future Kanban cards/issues. They are ordered to avoid building customer-facing LIMS surface area on a weak security spine.

1. Implement authenticated actor context
   - Replace free-text actor propagation with an `ActorContext` model.
   - Add tests proving request actor spoofing cannot set audit identity.

2. Commit RBAC/ABAC matrix and protected command boundary
   - Define roles, permissions, and command handlers.
   - Add negative authorization tests for create/update/transition/release/export/import/audit-view.

3. Add tenant/lab scoping to domain and audit records
   - Add tenant/lab id to domain objects, audit events, query APIs, and auth decisions.
   - Add cross-tenant denial tests.

4. Formalize audit schema and verifier
   - Add canonical event schema, validation, hash rules, sequence checks, and startup verifier.
   - Add tamper tests for modify/delete/reorder/truncate/duplicate/gap/malformed cases.

5. Add external/signed checkpoint strategy
   - Define checkpoint interval, signing/anchoring location, verification flow, and recovery behavior.
   - Add regenerated-chain-with-missing/mismatched-checkpoint tests.

6. Make state and audit atomic or recoverable
   - Prefer database-backed transaction for domain mutation plus audit insert.
   - If files remain, add locking, atomic protocol, and crash/concurrency tests.

7. Harden workflow release/amendment model
   - Add result/QC/review/e-signature/release preconditions and denial audit events.
   - Add immutable released result/report versions and amendment/supersession workflow.

8. Define migration provenance path
   - Add `migration.imported` event schema, dry-run/reconcile/idempotency behavior, source checksums, and mapping version handling.

9. Define audit privacy and retention policy
   - Add payload minimization rules, restricted audit views/export, retention/legal-hold/redaction, and backup encryption handling.

10. Write backup/restore verification runbook and tests
    - Add backup manifest, restore verifier, checkpoint validation, and damaged-restore blocked-write behavior.

## Non-goals for this artifact

- This artifact does not implement the gates.
- This artifact does not approve Project Scientist for customer data or production.
- This artifact does not mutate any customer/production systems.
- This artifact does not expose Project Scientist externally.
