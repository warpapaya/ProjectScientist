# PSC-RM-084 — Aegis Pre-Staging Security Gate

Generated: 2026-06-23T05:34:11Z
Kanban source: t_7f23eca4, preserving blocker evidence from t_63dad400, t_cc46863f, t_9388c692, and blocked t_74d16018.
Repo branch: psc-rm-084-pilot-readiness

## Verdict

CHANGES_REQUESTED / BLOCKED for any private/external staging.

This artifact is a durable repo-tracked reconstruction of the prior Aegis gate. It does not approve private staging, external staging, production deployment, customer data, customer-facing claims, DNS/auth/security/billing changes, or any reachable environment.

Allowed lane remains: local/private internal synthetic lab-test validation only.

## Security gate table

| Gate | Color | Decision | Required before private/external staging |
| --- | --- | --- | --- |
| Authenticated actor/session model | RED | BLOCKED | Replace fabricated/local privileged `lab-dev` request actor with a real authenticated identity/session boundary. Bind tenant/lab scope to trusted server-side claims, not caller-selected request fields. |
| RBAC/ABAC protected-operation coverage | YELLOW | CHANGES_REQUESTED | Domain authorization exists for reviewed slices, but staging-grade coverage must prove every protected HTTP/API operation enforces actor role, tenant/lab scope, reviewer separation, and audited denial behavior. |
| Signed/external audit checkpoint | RED | BLOCKED | Add signed or externally anchored audit checkpoints outside mutable primary SQLite storage. Local SQLite-only audit evidence is acceptable only for synthetic lab-test drills. |
| Audit privacy, retention, redaction, and encryption | YELLOW/RED | CHANGES_REQUESTED / BLOCKED | Audit detail minimization exists for some paths, but staging requires implemented retention/legal-hold, redaction, encryption-at-rest/backups, and operator access policy. |
| Reachable-staging threat model | RED | BLOCKED | No approved threat model/runbook exists for any reachable host, private network, TLS/DNS boundary, auth proxy/session boundary, observability, backup, rollback, abuse cases, or cleanup. |

## Green gates — allowed only for local/private synthetic lab-test work

- Local/private Docker or process execution with synthetic fixtures only.
- No public URL, no customer/prospect access, no SENAITE production integration, no customer data, and no customer-facing language.
- Existing synthetic vertical-slice tests, Docker smoke, backup/restore proof, and performance/concurrency smoke are acceptable engineering signals for the internal drill lane only.
- Prior tenant-boundary fixes were accepted only for internal synthetic lab-test use, not for production or staging exposure.

## Yellow gates — acceptable lab-test-only risk, not staging approval

1. RBAC/ABAC coverage is partial and slice-specific.
   - Risk: unreviewed protected endpoints may lack denial coverage or actor separation.
   - Required proof: endpoint inventory mapped to authorization checks and negative tests.
2. Tenant/lab scoping exists but is not a real authenticated tenant model.
   - Risk: caller-controlled tenant/lab inputs remain inside the trust boundary.
   - Required proof: tenant/lab scope derived from trusted identity/session claims.
3. Audit minimization exists in covered areas but privacy operations are incomplete.
   - Risk: retention, redaction, legal hold, encryption-at-rest, backup encryption, and operator access policy are not enforced as a complete system.
   - Required proof: implemented policy, tests, and restore/readback evidence.

## Red blockers — must remain blocked

1. Fabricated privileged actor/session model.
   - The HTTP/server path still depends on a local/dev privileged actor shape for reviewed flows.
   - Private/external staging needs real authentication, session lifecycle, trusted identity propagation, and denial behavior when missing/expired/invalid.
2. Caller-controlled tenant/lab boundary.
   - Any request-selected tenant/lab header/input is not a trusted isolation boundary.
   - Private/external staging needs server-side tenant/lab binding and cross-tenant negative tests.
3. No signed or external audit checkpoint.
   - Mutable SQLite audit rows are not enough once the service is reachable.
   - Required: signed checkpoint, append-only external sink, or equivalent tamper-evident anchor.
4. Incomplete audit privacy/retention/redaction/encryption controls.
   - Required: retention windows, redaction workflow, legal-hold behavior, encryption at rest/backups, restore proof, and access-control review.
5. No reachable-staging threat model/runbook.
   - Required: host/container/image pinning, network exposure boundary, DNS/TLS assumptions, auth proxy/session boundary, observability, backup/restore, rollback, cleanup, abuse cases, and named approval gates.

## Blockers versus acceptable lab-test-only risks

Blocking for private/external staging:
- No real authenticated actor/session boundary.
- Caller-controlled tenant/lab scope remains within the trust boundary.
- No signed/external audit checkpoint.
- No implemented audit retention/legal-hold/redaction/encryption-at-rest/backups policy.
- No approved reachable-staging threat model or runbook.

Acceptable only for local/private synthetic lab-test drills:
- Fixed local lab-dev actor.
- Local SQLite checkpoint/audit store only.
- Caller-controlled request IDs and synthetic tenant/lab labels.
- Synthetic-only workflow evidence and local Docker/process execution.

## Stop-lines

The following did not occur and remain prohibited by this gate:

- No deployment.
- No production or customer mutation.
- No customer data.
- No external exposure.
- No DNS/auth/security/billing changes.
- No customer-facing claims.
- No production-readiness claims.

## Required unblock evidence for a future Aegis re-review

A future card may request re-review only after all of the following are physically present and read back from repo-tracked or otherwise durable artifacts:

1. Auth/session design and implementation evidence for non-local actors.
2. Protected-operation inventory with RBAC/ABAC allow/deny tests across HTTP/API paths.
3. Tamper-evident external/signed audit checkpoint proof.
4. Audit privacy/retention/redaction/encryption policy implementation with tests and restore proof.
5. Forge reachable-staging runbook with Aegis threat model and explicit approval gates.

Until then: CHANGES_REQUESTED / BLOCKED for any private/external staging.
