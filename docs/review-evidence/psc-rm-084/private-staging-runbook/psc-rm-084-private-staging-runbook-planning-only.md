# PSC-RM-084 — Private Staging Runbook (Planning Only)

Status: DRAFT / PLANNING-ONLY. This is not deployment approval and not an executable deployment recipe.

Source packet: `docs/psc-rm-084-pilot-readiness-packet.md`
Prior Aegis blocker artifact: `docs/review-evidence/psc-rm-084/security-gate/aegis-pre-staging-security-gate-psc-rm-084.md`
Prior blocked marker: `docs/review-evidence/psc-rm-084/security-gate/psc-rm-084-private-staging-runbook-blocked.md`
Kanban task: `t_74d16018`

## Decision boundary

Friday maintenance accepted Aegis YELLOW only for planning-gate work. Actual reachable private staging remains blocked until a concrete request manifest/evidence bundle exists and named Aegis plus Petie/Friday final go/no-go approvals are recorded.

This runbook deliberately defines the required shape and gates without authorizing or performing deployment, DNS/TLS/auth/security mutation, external exposure, customer/prod mutation, customer data use, or customer-facing claims.

## Explicit non-actions confirmed

The following did not occur during this runbook task:

- No external staging deployment.
- No private reachable staging deployment.
- No public URL or external exposure.
- No DNS mutation.
- No TLS certificate issuance or TLS routing mutation.
- No auth/security account mutation.
- No billing mutation.
- No customer, prospect, production, or SENAITE data access/import/export/migration/copy/processing.
- No production or customer-system mutation.
- No customer/prospect access.
- No customer-facing claims.
- No production-readiness claims.

## Allowed scope of this document

Allowed:

1. Define the private-staging topology requirements.
2. Define minimum host/container/image/network/DNS/TLS/backup/observability/rollback/cleanup controls.
3. Define approval gates and stop-lines.
4. Preserve that actual staging is blocked until the approval bundle exists.

Not allowed:

1. Provide copy/paste deployment commands.
2. Name a live host as approved for mutation.
3. Create or modify DNS/TLS/auth/security accounts.
4. Touch customer/prod data or customer systems.
5. Represent Project Scientist as customer-pilot, externally staged, production-ready, or replacement-ready.

## Precondition gate — mandatory before execution

No one may convert this planning artifact into execution until all of these are attached to a future staging request:

1. A concrete environment request manifest:
   - target host identifier;
   - network zone;
   - exposure mode;
   - expected operators;
   - data classification;
   - retention window;
   - rollback owner;
   - cleanup deadline.
2. Aegis-approved auth/session and trusted tenant/lab boundary evidence.
3. Protected-operation RBAC/ABAC inventory with allow/deny tests across every HTTP/API path exposed in staging.
4. Signed or externally anchored audit checkpoint proof.
5. Audit privacy/retention/redaction/encryption-at-rest and backup-encryption implementation evidence.
6. Reachable-staging threat model covering abuse cases, operator access, ingress, TLS, secrets, logs, backups, rollback, and cleanup.
7. Named approvals:
   - Aegis: security boundary accepted for the exact requested environment.
   - Forge: ops/runbook boundary accepted for the exact requested environment.
   - Friday: scope and messaging accepted.
   - Petie: final go/no-go for any reachable environment.

Until all seven exist, status remains: BLOCKED FOR EXECUTION.

## Host boundary

Minimum host requirements for a future private staging request:

- Dedicated non-production host or VM. Do not co-locate with customer/prod SENAITE, Clearline, billing, identity, or customer-data systems.
- Patch level and base OS recorded in the request manifest before approval.
- SSH/operator access limited to named operators; no shared ad-hoc accounts.
- Host firewall denies inbound traffic by default except the approved private ingress path.
- Disk capacity and inode headroom recorded before start and monitored during the drill.
- Host resource ceilings documented for CPU, memory, disk, and log volume.
- Secrets are delivered through the approved secret store or one-time manual injection path named in the request; no secrets committed to repo, shell history, docs, transcripts, or artifacts.

Host selection is not approved by this document. The exact host must be named and approved in the future manifest.

## Container and Docker image pinning

Minimum container requirements:

- Every image is pinned by immutable digest, not floating tags.
- Build provenance records:
  - git commit SHA;
  - build timestamp;
  - Dockerfile path;
  - image digest;
  - SBOM or dependency inventory if available;
  - vulnerability scan result or explicit waiver.
- Compose/project name is unique to the staging request and must not collide with dev, CI, or another worker.
- Containers run with least practical privilege:
  - non-root where supported;
  - read-only filesystem where practical;
  - explicit writable mounts only for DB/artifacts/logs/tmp;
  - no Docker socket mount;
  - no host network mode unless explicitly justified by Aegis.
- Resource limits are specified for app/database/proxy/logging containers.
- Health and readiness endpoints are enabled and checked before any operator workflow begins.

No image tag/digest is approved by this document. The future request must provide exact digests.

## Private network and exposure boundary

Approved planning shape only:

- Default: no public internet exposure.
- Preferred ingress: private tailnet/VPN-only or single private bastion/proxy path approved by Aegis.
- No anonymous access.
- No customer/prospect access.
- No broad LAN exposure unless Aegis accepts the host/network segment and abuse cases.
- All inbound paths must terminate at an approved auth/session boundary before app access.
- Staging must deny direct app-port access from untrusted networks.
- Egress must be documented; no unreviewed outbound integrations to production SENAITE, customer mail, customer storage, billing, or external webhook targets.

The future threat model must include an ingress diagram, allowed source identities/networks, denied paths, and failure behavior if auth/session is missing, expired, or invalid.

## DNS/TLS assumptions

Planning assumptions:

- DNS is optional. If not required, use private hostnames or tailnet names only.
- If DNS is requested, it must be a non-customer, non-production, private/staging subdomain with explicit approval.
- No wildcard or production certificate reuse unless Aegis explicitly accepts it for the exact host and service.
- TLS must be enforced for any browser/API path crossing a network boundary.
- Certificate issuance, renewal, storage, and revocation ownership must be documented.
- HSTS, cookies, redirects, and callback/origin values must match the actual private staging domain and must not bleed into production/customer domains.

No DNS records or certificates are authorized by this document.

## Auth/session and identity boundary

Minimum future gate:

- Real authenticated non-local actors only; no fabricated privileged `lab-dev` actor for reachable staging.
- Tenant/lab scope derived from trusted server-side identity/session claims, not caller-selected request fields.
- Session expiry, invalid-session denial, missing-session denial, and cross-tenant denial are tested through the actual HTTP/API path.
- Reviewer separation and protected-operation authorization are enforced and audited.
- Operator/admin roles are named and limited to the minimum required for the drill.
- Break-glass access, if any, is documented with logging, expiration, and post-drill review.

This document does not create, approve, or mutate auth configuration.

## Data boundary

Default data classification: synthetic only.

Allowed only with future explicit approval:

- non-sensitive/customer-approved fixture data;
- redacted/import-test datasets with written scope;
- customer data only after a separate customer-data approval, migration plan, legal/privacy review, and Petie approval.

Prohibited by default:

- Tindall/CENLA/RJ Lee production data;
- SENAITE production dumps;
- customer contact records;
- production report artifacts;
- production credentials;
- live workflow exports;
- customer-facing demo artifacts.

## Backup and restore

Minimum future backup requirements:

- Pre-start snapshot of config, database, generated artifacts, image digests, and request manifest.
- Encrypted backup at rest.
- Off-host or host-independent backup copy unless explicitly waived for synthetic-only single-session drill.
- Backup manifest with file paths, sizes, checksums, creation time, retention deadline, and data classification.
- Restore proof before declaring the staging environment usable:
  - restore into a clean location;
  - verify database opens;
  - verify audit chain/checkpoints;
  - verify expected synthetic artifacts/hashes;
  - record proof output in the evidence bundle.
- RPO/RTO target stated in the request manifest.

Backup retention and cleanup must match the data classification. Synthetic artifacts still get retention deadlines; customer data requires a separate approval path.

## Observability

Minimum future observability requirements:

- `/healthz` and `/readyz` or equivalent health/readiness signals.
- Request/error logs with sensitive fields minimized/redacted.
- Audit logs/checkpoints for protected operations and denied attempts.
- Metrics or periodic snapshots for uptime, request rate, error rate, latency, disk usage, memory, CPU, and backup success/failure.
- Alerts routed to named internal operators for:
  - service down/unready;
  - disk threshold;
  - backup failure;
  - auth/session failure spikes;
  - cross-tenant denial spikes;
  - audit checkpoint failure.
- Operator commands/runbook notes for reading health, logs, audit status, backup status, and rollback status without exposing secrets.

Observability must not ship customer data or secrets to unapproved sinks.

## Rollback

Minimum future rollback requirements:

- Rollback trigger list:
  - auth/session bypass or unexpected allow;
  - cross-tenant access;
  - audit checkpoint failure;
  - backup/restore proof failure;
  - service instability;
  - accidental external exposure;
  - customer/prod data contamination;
  - unapproved DNS/TLS/auth/security mutation.
- Rollback owner and escalation path named before start.
- Restore point verified before operator workflows begin.
- Rollback returns to the previous known-safe state or clean shutdown with evidence preserved.
- Rollback proof captured in the evidence bundle.
- No broad Docker or host cleanup commands; rollback scope is limited to the approved staging project and named volumes/directories.

This document does not provide rollback commands because the exact environment is not approved.

## Cleanup

Minimum future cleanup requirements:

- Cleanup deadline recorded before start.
- Named owner confirms shutdown/removal and evidence preservation.
- Remove only approved staging containers, volumes, directories, test identities, DNS/TLS records, and secrets created for that exact request.
- Preserve required evidence bundle until retention deadline:
  - manifest;
  - approvals;
  - image digests;
  - backup/restore proof;
  - audit/checkpoint proof;
  - logs with secrets/customer data redacted;
  - rollback/cleanup proof.
- Confirm no reachable service remains after cleanup.
- Confirm no customer/prod data was present, or follow the separate customer-data destruction proof if that was ever approved.

## Approval gates

Gate 0 — Planning artifact:
- This document exists.
- It is not executable.
- It records stop-lines and non-actions.

Gate 1 — Security evidence readiness:
- Aegis accepts auth/session, tenant/lab trust boundary, RBAC/ABAC coverage, signed/external audit checkpoints, audit privacy/retention/redaction/encryption evidence.

Gate 2 — Environment request manifest:
- Exact host/network/DNS/TLS/auth/data/backup/rollback/cleanup shape is written and read back.

Gate 3 — Threat model:
- Aegis accepts abuse cases and exposure boundary for the exact environment.

Gate 4 — Ops runbook review:
- Forge accepts that the concrete runbook is safe, scoped, pinned, observable, backed up, reversible, and cleanable.

Gate 5 — Final go/no-go:
- Friday accepts scope/messaging.
- Petie approves final go/no-go.

Gate 6 — Execution evidence:
- Only after Gates 1-5, execution may be performed by the assigned operator under the approved runbook. Evidence must be captured, hashed, and attached.

Current state for this task: Gate 0 only. Gates 1-6 are not satisfied by this document.

## Stop-lines

Stop immediately and block if any request attempts to:

- deploy or mutate a reachable environment without the complete approval bundle;
- expose a service externally or broadly on LAN/tailnet without Aegis approval;
- change DNS/TLS/auth/security/billing without explicit approval;
- use customer/prod/SENAITE data without separate approval;
- create customer/prospect access;
- claim pilot/customer/production readiness;
- skip backup/restore proof;
- skip audit checkpoint proof;
- use floating Docker tags for staging execution;
- run broad cleanup that could affect other Project Scientist workers or systems.

## Current runbook disposition

This planning-only runbook satisfies the drafting lane requested after the Aegis planning gate moved to YELLOW. It does not satisfy the actual private-staging execution gate.

Exact remaining gate: actual private reachable staging is BLOCKED until a concrete private-staging request manifest/evidence bundle exists and named Aegis plus Petie/Friday final go/no-go approvals are recorded.
