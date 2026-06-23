# PSC-RM-084 — Private Staging Runbook Blocked Marker

Generated: 2026-06-23T05:34:11Z
Kanban source: t_7f23eca4, preserving Forge blocked-runbook evidence from t_74d16018.
Repo branch: psc-rm-084-pilot-readiness

## Status

BLOCKED. This is not an executable deployment runbook.

Aegis gate remains: CHANGES_REQUESTED / BLOCKED for any private/external staging.

This artifact intentionally avoids deploy commands, host mutation steps, DNS/TLS instructions, auth/security mutations, customer-data handling, or customer-facing claims. It preserves the blocker state so nobody mistakes a missing workspace file for approval.

## Non-actions explicitly confirmed

The following did not occur:

- No external staging deployment.
- No private staging deployment.
- No production deployment.
- No DNS mutation.
- No auth/security mutation.
- No billing mutation.
- No customer data access, import, export, migration, copy, or processing.
- No customer/prospect access.
- No customer-facing claims.
- No production-readiness claims.
- No external exposure.

## Why the runbook is blocked

The Aegis security gate is not satisfied. Private/external staging remains blocked until these gates are approved in a future Aegis review:

1. Real authenticated actor/session model.
   - Current blocker: local/dev fabricated privileged actor assumptions are not an exposure-safe identity boundary.
2. Trusted tenant/lab boundary.
   - Current blocker: request-selected tenant/lab scope cannot be trusted for reachable staging.
3. Complete RBAC/ABAC protected-operation coverage.
   - Current blocker: reviewed slices are not proof that every protected operation has allow/deny enforcement and audited denials.
4. Signed or externally anchored audit checkpoint.
   - Current blocker: mutable local SQLite audit state is not sufficient for a reachable environment.
5. Audit privacy/retention/redaction/encryption policy.
   - Current blocker: retention, legal hold, redaction, encryption-at-rest/backups, and operator access are not implemented as a complete staging-grade control set.
6. Reachable-staging threat model.
   - Current blocker: no approved threat model covers host, container, network, DNS/TLS, auth proxy/session, observability, backups, rollback, cleanup, abuse cases, and approval gates.

## Deferred runbook sections — intentionally non-executable

These sections must be authored only after the Aegis gate is satisfied. They are listed as placeholders, not steps:

- Host selection and hardening baseline.
- Container image pinning and provenance.
- Private network and exposure boundary.
- DNS/TLS assumptions, if any.
- Auth/session boundary and identity provider assumptions.
- Secrets handling and rotation.
- Backup/restore location, encryption, retention, and restore proof.
- Observability: logs, metrics, readiness, alerting, audit access.
- Rollback procedure and rollback proof.
- Cleanup procedure scoped to the staging environment only.
- Approval gates and named approvers.

No commands are provided here because commands would create an attractive nuisance: a blocked runbook must not become a deployment recipe.

## Minimum future unblock packet

A future private-staging runbook card should require, at minimum:

1. Aegis-approved auth/session and tenant/lab trust boundary evidence.
2. RBAC/ABAC protected-operation inventory with negative tests.
3. Signed/external audit checkpoint proof.
4. Audit retention/redaction/encryption implementation proof.
5. Forge deployment topology/runbook draft reviewed by Aegis.
6. Petie approval for any reachable environment scope.
7. Explicit statement that synthetic data only is used unless a separate customer-data approval exists.

Until those exist, the only permitted lane is local/private internal synthetic lab-test validation.

## Stop-lines

Stop immediately if a task attempts any of the following under this blocked marker:

- deployment or environment creation;
- external exposure;
- DNS/TLS mutation;
- auth/security/billing mutation;
- customer data usage;
- customer-facing claims;
- production-readiness claims.

This marker preserves the block. It does not unblock private/external staging.
