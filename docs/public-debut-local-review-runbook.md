# Project Scientist public-debut local review runbook

Project Scientist is a public-debut candidate only after local/private review. This runbook does not deploy, expose, publish, or message anything publicly. It is for synthetic local demo data on a developer workstation.

## Stop-lines

Do not use this runbook to:

- Deploy to production, staging, public internet, customer systems, or shared hosting.
- Change DNS, TLS, auth, billing, secrets, reverse proxy, Tailscale, Traefik, Authentik, or cloud infrastructure.
- Load, import, expose, or screenshot real customer/client data.
- Send customer-facing/public messages.
- Claim the app is production-ready or customer-ready.

Allowed scope here: local Docker build, local loopback browser review, synthetic fixture reset, smoke verification, screenshots/artifacts, and cleanup.

## Prerequisites

From the repository root:

```bash
go version
docker version
docker compose version
make --version
```

Use a unique Compose project name and loopback port for review so concurrent Kanban/dev workers do not collide. The preferred prospect-review port is `8108`; if another local worker already owns it, choose an unused loopback port and keep the same `PSC_REVIEW_PORT` value for every command in this runbook.

```bash
export PSC_REVIEW_PROJECT=project-scientist-public-debut-review
export PSC_REVIEW_PORT=8108
export PSC_REVIEW_URL=http://127.0.0.1:${PSC_REVIEW_PORT}
```

Local browser credentials are synthetic dev credentials only:

```text
username: lab-dev
password: project-scientist-dev
```

## Verification baseline before browser review

Run host tests without changing module files:

```bash
go test -mod=readonly ./...
```

Validate the Compose file renders cleanly with the review project/port:

```bash
COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} \
PSC_DEV_PORT=${PSC_REVIEW_PORT} \
docker compose config --quiet
```

## Start or reset the local private demo

Start/rebuild the loopback-only local container and wait for health:

```bash
make dev-up \
  COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} \
  DEV_PORT=${PSC_REVIEW_PORT}
```

Reset to deterministic synthetic demo data through the local API:

```bash
make demo-reset \
  COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} \
  DEV_PORT=${PSC_REVIEW_PORT}
```

Confirm health:

```bash
curl -fsS ${PSC_REVIEW_URL}/healthz
```

Expected output:

```text
ok
```

## Smoke path

Run the prospect browser-route smoke against the rebuilt local container:

```bash
make prospect-trial-smoke \
  COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} \
  DEV_PORT=${PSC_REVIEW_PORT}
```

The smoke path signs in with local synthetic credentials, verifies the deterministic workspace, and checks the prospect route sequence:

```text
login -> dashboard -> samples/S-000001 -> results -> reports
```

This is local lab-test evidence only. It is not a customer-readiness or production-readiness claim.

## Manual local review path

Open:

```text
${PSC_REVIEW_URL}/login
```

With the default review port, that is:

```text
http://127.0.0.1:8108/login
```

Sign in with the synthetic local credentials above, then review:

1. Dashboard/guided workflow loads the synthetic Okefenokee workspace.
2. Sample `S-000001` is visible and tied to only synthetic fixture data.
3. Results and reports pages load without 5xx errors.
4. UI copy stays prospect-safe: no enum-ish/internal labels in primary calls to action, no dense developer tables as the main story, no production/customer-ready claims.
5. Known rough edges are either fixed in dependent implementation or listed as blockers before public/staging approval.

## Artifact capture

Create a local artifact directory:

```bash
mkdir -p artifacts/public-debut-review
```

Suggested screenshot set, using browser or macOS screenshot tooling:

```text
artifacts/public-debut-review/login.png
artifacts/public-debut-review/dashboard.png
artifacts/public-debut-review/sample-S-000001.png
artifacts/public-debut-review/results.png
artifacts/public-debut-review/reports.png
```

Suggested command-output capture:

```bash
{
  date -u
  git rev-parse HEAD
  go test -mod=readonly ./...
  make prospect-trial-smoke COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} DEV_PORT=${PSC_REVIEW_PORT}
} 2>&1 | tee artifacts/public-debut-review/local-review-verification.log
```

Do not commit screenshots/logs unless explicitly requested; attach or reference them in the Kanban/review packet as local evidence.

## Cleanup

Stop the review Compose project while preserving its named local data volume:

```bash
make dev-down \
  COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} \
  DEV_PORT=${PSC_REVIEW_PORT}
```

If a clean local prototype state is intentionally required, remove the review volume only for this local project:

```bash
make dev-reset \
  COMPOSE_PROJECT_NAME=${PSC_REVIEW_PROJECT} \
  DEV_PORT=${PSC_REVIEW_PORT}
```

Do not use broad Docker cleanup patterns. If stale resources must be inspected, keep name filters narrow and scoped to `${PSC_REVIEW_PROJECT}`.

## Later approved public/private staging deployment checklist

These items define what Petie would be approving if he later says to stage or public-demo it. They are not performed by this runbook.

- Target environment: exact host/account/region, private vs public reachability, and rollback owner.
- Exposure boundary: loopback/private Tailscale/private staging/public internet explicitly selected.
- DNS/TLS: hostname, certificate source, renewal path, and no wildcard/customer-domain changes without approval.
- Auth/session: production-grade identity/session plan or explicitly private-lab session wrapper; no reuse of local dev credentials/token.
- Data: synthetic/demo data only unless a separate customer-data handling plan is approved.
- Secrets: generated per-environment and stored in approved secret storage; no checked-in tokens.
- Demo reset: disabled outside disposable local/private lab review unless separately approved.
- Persistence: named volume/database location, backup path, restore test, and audit verification gate.
- Observability: health/readiness, logs, metrics, and alert owner.
- Security review: Aegis/Friday gate for ingress/auth/CSRF/cookie posture before exposure.
- Rollback: image tag/commit, database backup, and exact revert command.
- Communications: no customer-facing/public announcement until Petie approves copy and timing.

## Approval statement template

If Petie later approves staging/public exposure, capture the approval with this shape before acting:

```text
I approve Project Scientist <private staging|public demo> for <hostname/environment>, using commit <sha>, synthetic demo data only, no customer data, with rollback to <prior image/commit> and no production-ready/customer-ready claims.
```

Without that explicit approval, the stop-line remains: local/private review only; no deploy or public exposure.
